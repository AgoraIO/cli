package cli

// Shared infrastructure for the integration tests in this package.
//
// The CLI is exercised end-to-end by re-invoking the test binary with the
// special TestCLIHelperProcess test selected; that helper boots a fresh
// *App, runs Execute(), and translates the result into a process exit
// code. runCLI spawns this helper and captures stdout / stderr / exit.
//
// The fakeOAuthServer and fakeCLIBFF stand in for the public OAuth flow and
// the Agora CLI BFF, so we can assert request shapes (User-Agent, headers,
// auth) and inject failure modes without leaving the test binary.
//
// Per-command tests live in sibling files:
//
//   integration_help_test.go         help / discovery / agentic surfaces
//   integration_quickstart_test.go   `agora quickstart`
//   integration_init_test.go         `agora init`
//   integration_auth_test.go         `agora login` / whoami / auth status
//   integration_project_test.go      `agora project` (env, doctor, use, ...)
//   golden_test.go                   golden-file snapshots for stable agent envelopes

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"
)

type cliResult struct {
	exitCode int
	stdout   string
	stderr   string
}

type cliRunOptions struct {
	env      map[string]string
	workdir  string
	onStderr func(string) bool
}

// TestCLIHelperProcess is the in-process re-entry point used by runCLI.
// When invoked with GO_WANT_CLI_HELPER_PROCESS=1, it builds a fresh *App
// and runs Execute() with the args passed through GO_CLI_HELPER_ARGS_JSON;
// otherwise it returns immediately so it does not show up as a regular test.
func TestCLIHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_CLI_HELPER_PROCESS") != "1" {
		return
	}
	cliArgs := helperCLIArgs(t)
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()
	os.Args = append([]string{"agora"}, cliArgs...)

	app, err := NewApp()
	if err != nil {
		if JSONRequested(cliArgs) {
			_ = EmitJSONError("agora", err, 1, "")
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	app.root.SetArgs(cliArgs)
	if err := app.Execute(); err != nil {
		if code, ok := ExitCode(err); ok {
			os.Exit(code)
		}
		if ErrorRendered(err) {
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, err.Error())
		os.Exit(1)
	}
	os.Exit(0)
}

func helperCLIArgs(t *testing.T) []string {
	t.Helper()
	if raw := os.Getenv("GO_CLI_HELPER_ARGS_JSON"); raw != "" {
		var args []string
		if err := json.Unmarshal([]byte(raw), &args); err != nil {
			t.Fatalf("invalid GO_CLI_HELPER_ARGS_JSON: %v", err)
		}
		return args
	}
	// Fallback for manually invoking the helper while debugging.
	for i, arg := range os.Args {
		if arg == "--" {
			return os.Args[i+1:]
		}
	}
	return nil
}

// runCLI spawns the test binary as a subprocess that reroutes through
// TestCLIHelperProcess, captures stdout/stderr line-by-line, and returns
// the exit code. The optional onStderr callback is invoked on every stderr
// line so tests can react to interactive prompts (e.g. follow the OAuth
// URL the moment we see it).
func runCLI(t *testing.T, args []string, options cliRunOptions) cliResult {
	t.Helper()
	cmd := exec.Command(os.Args[0], "-test.run=TestCLIHelperProcess")
	if options.workdir != "" {
		cmd.Dir = options.workdir
	}
	encodedArgs, err := json.Marshal(args)
	if err != nil {
		t.Fatal(err)
	}
	cmd.Env = append(os.Environ(),
		"GO_WANT_CLI_HELPER_PROCESS=1",
		"GO_CLI_HELPER_ARGS_JSON="+string(encodedArgs),
		// Keep integration tests deterministic when the suite itself runs in CI.
		// Unit tests cover CI auto-detection explicitly; command-surface tests
		// should not silently switch from pretty to JSON because CI=true leaked
		// in from the parent process.
		"AGORA_DISABLE_CI_DETECT=1",
	)
	for key, value := range options.env {
		cmd.Env = append(cmd.Env, key+"="+value)
	}

	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		t.Fatal(err)
	}

	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _ = io.Copy(&stdoutBuf, stdoutPipe)
	}()

	go func() {
		defer wg.Done()
		reader := bufio.NewReader(stderrPipe)
		for {
			chunk, err := reader.ReadString('\n')
			if chunk != "" {
				stderrBuf.WriteString(chunk)
				if options.onStderr != nil {
					_ = options.onStderr(stderrBuf.String())
				}
			}
			if err != nil {
				if err == io.EOF {
					return
				}
				return
			}
		}
	}()

	err = cmd.Wait()
	wg.Wait()

	code := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			code = exitErr.ExitCode()
		} else {
			t.Fatal(err)
		}
	}

	return cliResult{
		exitCode: code,
		stdout:   stdoutBuf.String(),
		stderr:   stderrBuf.String(),
	}
}

// createLocalGitRepo materializes a minimal git repository in a temp dir
// and seeds it with the given files. Used as a stand-in for the upstream
// quickstart repos so quickstart-clone tests do not hit the network.
func createLocalGitRepo(t *testing.T, files map[string]string) string {
	t.Helper()
	repoDir := t.TempDir()
	for path, content := range files {
		filePath := filepath.Join(repoDir, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	init := exec.Command("git", "init")
	init.Dir = repoDir
	if output, err := init.CombinedOutput(); err != nil {
		t.Fatalf("git init failed: %v output=%s", err, string(output))
	}
	add := exec.Command("git", "add", ".")
	add.Dir = repoDir
	if output, err := add.CombinedOutput(); err != nil {
		t.Fatalf("git add failed: %v output=%s", err, string(output))
	}
	commit := exec.Command("git", "-c", "user.name=Test", "-c", "user.email=test@example.com", "-c", "commit.gpgsign=false", "commit", "-m", "init")
	commit.Dir = repoDir
	if output, err := commit.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v output=%s", err, string(output))
	}
	return repoDir
}

// fakeOAuthServer impersonates the Agora SSO authorize / token endpoints
// for end-to-end login tests. It records every redirect_uri we hand out
// and every token request body, so tests can assert PKCE is in use and
// the redirect URI loops back to localhost.
type fakeOAuthServer struct {
	server                *http.Server
	baseURL               string
	authorizeRedirectURIs []string
	tokenRequests         []string
}

func newFakeOAuthServer() *fakeOAuthServer {
	oauth := &fakeOAuthServer{}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v0/oauth/authorize":
			redirectURI := r.URL.Query().Get("redirect_uri")
			state := r.URL.Query().Get("state")
			if redirectURI == "" || state == "" {
				http.Error(w, "missing redirect", http.StatusBadRequest)
				return
			}
			oauth.authorizeRedirectURIs = append(oauth.authorizeRedirectURIs, redirectURI)
			http.Redirect(w, r, redirectURI+"?code=test-auth-code&state="+state, http.StatusFound)
		case r.Method == http.MethodPost && r.URL.Path == "/api/v0/oauth/token":
			body, _ := io.ReadAll(r.Body)
			_ = r.Body.Close()
			oauth.tokenRequests = append(oauth.tokenRequests, string(body))
			w.Header().Set("content-type", "application/json")
			values := string(body)
			if strings.Contains(values, "grant_type=authorization_code") {
				_, _ = io.WriteString(w, `{"access_token":"access-token-value","token_type":"Bearer","expires_in":7199,"refresh_token":"refresh-token-value","scope":"basic_info,console"}`)
				return
			}
			if strings.Contains(values, "grant_type=refresh_token") {
				_, _ = io.WriteString(w, `{"access_token":"refreshed-access-token","token_type":"Bearer","expires_in":7199,"refresh_token":"refresh-token-value-2","scope":"basic_info,console"}`)
				return
			}
			http.Error(w, `{"error":"unsupported_grant_type"}`, http.StatusBadRequest)
		default:
			http.NotFound(w, r)
		}
	})
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	oauth.server = &http.Server{Handler: handler}
	oauth.baseURL = "http://" + listener.Addr().String()
	go func() { _ = oauth.server.Serve(listener) }()
	return oauth
}

// fakeProject mirrors the BFF project payload (camelCase keys, optional
// pointers) so we can hand back the same shape the real API would return.
type fakeProject struct {
	AllowStaticWithDynamic bool   `json:"allowStaticWithDynamic"`
	AppID                  string `json:"appId"`
	CertificateEnabled     bool   `json:"certificateEnabled"`
	CreatedAt              string `json:"createdAt"`
	FeatureState           struct {
		ConvoAIEnabled bool `json:"convoaiEnabled"`
		RTMEnabled     bool `json:"rtmEnabled"`
	} `json:"-"`
	Name         string  `json:"name"`
	ProjectID    string  `json:"projectId"`
	ProjectType  string  `json:"projectType"`
	Region       string  `json:"region"`
	SignKey      *string `json:"signKey"`
	Stage        int     `json:"stage"`
	Status       string  `json:"status"`
	TokenEnabled bool    `json:"tokenEnabled"`
	UpdatedAt    string  `json:"updatedAt"`
	Usage7d      int     `json:"usage7d"`
	UseCaseID    *string `json:"useCaseId"`
	Vid          int     `json:"vid"`
}

func buildFakeProject(name, projectID, appID, region string) fakeProject {
	signKey := "4854d28b48a9439c9f2546e2216fc07a"
	useCase := "education"
	return fakeProject{
		AllowStaticWithDynamic: true,
		AppID:                  appID,
		CertificateEnabled:     true,
		CreatedAt:              "2026-04-07T12:34:56.000Z",
		Name:                   name,
		ProjectID:              projectID,
		ProjectType:            "paas",
		Region:                 region,
		SignKey:                &signKey,
		Stage:                  3,
		Status:                 "active",
		TokenEnabled:           true,
		UpdatedAt:              "2026-04-07T13:34:56.000Z",
		Usage7d:                0,
		UseCaseID:              &useCase,
		Vid:                    100001788,
	}
}

// fakeCLIBFF impersonates the Agora CLI Backend-For-Frontend. It supports
// the project list/create/get endpoints plus uap-configs (ConvoAI) and
// rtm2-config (RTM) feature flag toggles. Every request is captured under
// `requests` so tests can assert headers (e.g. AGORA_AGENT propagation).
type fakeCLIBFF struct {
	server   *http.Server
	baseURL  string
	mu       sync.Mutex
	projects map[string]*fakeProject
	requests []struct {
		Method        string
		Pathname      string
		Authorization string
		UserAgent     string
	}
}

func newFakeCLIBFF() *fakeCLIBFF {
	api := &fakeCLIBFF{projects: map[string]*fakeProject{}}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		api.mu.Lock()
		api.requests = append(api.requests, struct {
			Method        string
			Pathname      string
			Authorization string
			UserAgent     string
		}{
			Method:        r.Method,
			Pathname:      r.URL.Path,
			Authorization: r.Header.Get("Authorization"),
			UserAgent:     r.Header.Get("User-Agent"),
		})
		api.mu.Unlock()

		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/cli/v1/projects":
			keyword := strings.ToLower(r.URL.Query().Get("keyword"))
			items := []map[string]any{}
			for _, project := range api.projects {
				if keyword != "" && !strings.Contains(strings.ToLower(project.Name), keyword) && !strings.Contains(strings.ToLower(project.ProjectID), keyword) {
					continue
				}
				items = append(items, map[string]any{
					"allowStaticWithDynamic": project.AllowStaticWithDynamic,
					"appId":                  project.AppID,
					"createdAt":              project.CreatedAt,
					"name":                   project.Name,
					"projectId":              project.ProjectID,
					"projectType":            project.ProjectType,
					"region":                 project.Region,
					"signKey":                project.SignKey,
					"stage":                  project.Stage,
					"status":                 project.Status,
					"updatedAt":              project.UpdatedAt,
					"vid":                    project.Vid,
				})
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"items":    items,
				"page":     1,
				"pageSize": 20,
				"total":    len(items),
			})
		case r.Method == http.MethodPost && r.URL.Path == "/api/cli/v1/projects":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			name := body["name"].(string)
			projectID := fmt.Sprintf("prj_%04d", len(api.projects)+1)
			appID := fmt.Sprintf("app_%04d", len(api.projects)+1)
			project := buildFakeProject(name, projectID, appID, "global")
			api.projects[projectID] = &project
			_ = json.NewEncoder(w).Encode(project)
		case strings.HasPrefix(r.URL.Path, "/api/cli/v1/projects/") && !strings.Contains(r.URL.Path, "/uap-configs/") && !strings.HasSuffix(r.URL.Path, "/rtm2-config"):
			projectID := strings.TrimPrefix(r.URL.Path, "/api/cli/v1/projects/")
			project, ok := api.projects[projectID]
			if !ok {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `{"code":"NOT_FOUND","message":"resource not found","requestId":"req-not-found"}`)
				return
			}
			_ = json.NewEncoder(w).Encode(project)
		case strings.Contains(r.URL.Path, "/uap-configs/"):
			parts := strings.Split(r.URL.Path, "/")
			projectID := parts[5]
			project := api.projects[projectID]
			switch r.Method {
			case http.MethodGet:
				_ = json.NewEncoder(w).Encode(map[string]any{
					"enabled":          project.FeatureState.ConvoAIEnabled,
					"maxSubscribeLoad": 20,
					"productKey":       parts[len(parts)-1],
					"projectId":        projectID,
					"region":           map[bool]string{true: "cn", false: "global"}[project.Region == "cn"],
				})
			case http.MethodPut:
				project.FeatureState.ConvoAIEnabled = true
				_ = json.NewEncoder(w).Encode(map[string]any{
					"enabled":          true,
					"maxSubscribeLoad": 20,
					"productKey":       parts[len(parts)-1],
					"projectId":        projectID,
					"region":           map[bool]string{true: "cn", false: "global"}[project.Region == "cn"],
				})
			}
		case strings.HasSuffix(r.URL.Path, "/rtm2-config"):
			parts := strings.Split(r.URL.Path, "/")
			projectID := parts[5]
			project := api.projects[projectID]
			switch r.Method {
			case http.MethodGet:
				_ = json.NewEncoder(w).Encode(map[string]any{
					"enabled":   project.FeatureState.RTMEnabled,
					"projectId": projectID,
				})
			case http.MethodPut:
				project.FeatureState.RTMEnabled = true
				_ = json.NewEncoder(w).Encode(map[string]any{
					"enabled":   true,
					"projectId": projectID,
				})
			}
		default:
			http.NotFound(w, r)
		}
	})
	listener, err := net.Listen("tcp4", "127.0.0.1:0")
	if err != nil {
		panic(err)
	}
	api.server = &http.Server{Handler: handler}
	api.baseURL = "http://" + listener.Addr().String()
	go func() { _ = api.server.Serve(listener) }()
	return api
}

// persistSessionForIntegration writes a fresh, valid-for-an-hour session
// into the test's config home so tests do not need to walk through the
// OAuth flow each time.
func persistSessionForIntegration(t *testing.T, configHome string) {
	t.Helper()
	err := saveSession(map[string]string{"XDG_CONFIG_HOME": configHome}, session{
		AccessToken:  "access-token-value",
		RefreshToken: "refresh-token-value",
		TokenType:    "Bearer",
		Scope:        "basic_info,console",
		ObtainedAt:   time.Now().UTC().Format(time.RFC3339),
		ExpiresAt:    time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
	})
	if err != nil {
		t.Fatal(err)
	}
}

// parseAuthURL extracts the OAuth login URL the CLI prints to stderr in
// non-browser mode. Used by login tests to follow the redirect with a raw
// HTTP client.
func parseAuthURL(stderr string) string {
	match := regexp.MustCompile(`Open this URL to continue login:\n(https?://\S+)`).FindStringSubmatch(stderr)
	if len(match) == 2 {
		return match[1]
	}
	return ""
}
