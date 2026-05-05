package cli

// Shared infrastructure for the integration tests in this package.
//
// The CLI is exercised end-to-end by runCLI, which runs a fresh *App with
// isolated env, cwd, stdout, and stderr for each invocation. TestMain still
// supports GO_WANT_CLI_HELPER_PROCESS=1 as a manual debugging hook, but the
// suite no longer depends on subprocess re-entry.
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

var cliRunMu sync.Mutex

// TestMain keeps a manual subprocess entry point for debugging the harness.
// Normal tests use runCLI in-process below; avoiding subprocess re-entry keeps
// the integration suite deterministic across GitHub's Linux/macOS/Windows
// runners while still exercising the same App.Execute path.
func TestMain(m *testing.M) {
	if os.Getenv("GO_WANT_CLI_HELPER_PROCESS") == "1" {
		cliArgs := helperCLIArgs()
		if len(cliArgs) == 0 {
			fmt.Fprintln(os.Stderr, "agora-cli helper: missing CLI args (GO_CLI_HELPER_ARGS_JSON was empty and no -- fallback args were present)")
			os.Exit(64)
		}
		os.Exit(executeCLI(cliArgs))
		return
	}
	os.Exit(m.Run())
}

func executeCLI(cliArgs []string) int {
	originalArgs := os.Args
	defer func() { os.Args = originalArgs }()
	os.Args = append([]string{"agora"}, cliArgs...)

	app, err := NewApp()
	if err != nil {
		if JSONRequested(cliArgs) {
			_ = EmitJSONError("agora", err, 1, "")
			return 1
		}
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	app.root.SetArgs(cliArgs)
	if err := app.Execute(); err != nil {
		if code, ok := ExitCode(err); ok {
			return code
		}
		if ErrorRendered(err) {
			return 1
		}
		fmt.Fprintln(os.Stderr, err.Error())
		return 1
	}
	return 0
}

func helperCLIArgs() []string {
	if raw := os.Getenv("GO_CLI_HELPER_ARGS_JSON"); raw != "" {
		var args []string
		if err := json.Unmarshal([]byte(raw), &args); err != nil {
			fmt.Fprintf(os.Stderr, "agora-cli helper: invalid GO_CLI_HELPER_ARGS_JSON: %v\n", err)
			os.Exit(64)
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

// runCLI executes the CLI in-process with isolated process globals, captures
// stdout and stderr line-by-line, and returns the exit code. The optional
// onStderr callback is invoked on every stderr line so tests can react to
// interactive prompts (e.g. follow the OAuth URL the moment we see it).
func runCLI(t *testing.T, args []string, options cliRunOptions) cliResult {
	t.Helper()

	cliRunMu.Lock()
	defer cliRunMu.Unlock()

	runEnv := helperEnv(os.Environ(), map[string]string{
		// Keep integration tests deterministic when the suite itself runs in CI.
		// Unit tests cover CI auto-detection explicitly; command-surface tests
		// should not silently switch from pretty to JSON because CI=true leaked
		// in from the parent process.
		"AGORA_DISABLE_CI_DETECT": "1",
	})
	for key, value := range options.env {
		runEnv = helperEnv(runEnv, map[string]string{key: value})
	}

	originalEnv := os.Environ()
	originalDir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	originalStdout := os.Stdout
	originalStderr := os.Stderr
	defer func() {
		restoreProcessEnv(originalEnv)
		if options.workdir != "" {
			_ = os.Chdir(originalDir)
		}
		os.Stdout = originalStdout
		os.Stderr = originalStderr
	}()

	restoreProcessEnv(runEnv)
	if options.workdir != "" {
		if err := os.Chdir(options.workdir); err != nil {
			t.Fatal(err)
		}
	}

	stdoutReader, stdoutWriter, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	stderrReader, stderrWriter, err := os.Pipe()
	if err != nil {
		_ = stdoutReader.Close()
		_ = stdoutWriter.Close()
		t.Fatal(err)
	}
	os.Stdout = stdoutWriter
	os.Stderr = stderrWriter

	defer func() {
		_ = stdoutReader.Close()
		_ = stderrReader.Close()
	}()

	var stdoutBuf bytes.Buffer
	var stderrBuf bytes.Buffer
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_, _ = io.Copy(&stdoutBuf, stdoutReader)
	}()

	go func() {
		defer wg.Done()
		reader := bufio.NewReader(stderrReader)
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

	code := executeCLI(args)
	_ = stdoutWriter.Close()
	_ = stderrWriter.Close()
	wg.Wait()

	return cliResult{
		exitCode: code,
		stdout:   stdoutBuf.String(),
		stderr:   stderrBuf.String(),
	}
}

func restoreProcessEnv(env []string) {
	os.Clearenv()
	for _, item := range env {
		key, value, ok := strings.Cut(item, "=")
		if ok {
			_ = os.Setenv(key, value)
		}
	}
}

func helperEnv(base []string, overrides map[string]string) []string {
	result := make([]string, 0, len(base)+len(overrides))
	for _, item := range base {
		key, _, ok := strings.Cut(item, "=")
		if ok {
			if _, replaced := overrides[key]; replaced {
				continue
			}
		}
		result = append(result, item)
	}
	for key, value := range overrides {
		result = append(result, key+"="+value)
	}
	return result
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
	authorizeRawQueries   []string
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
			oauth.authorizeRawQueries = append(oauth.authorizeRawQueries, r.URL.RawQuery)
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
		RTMRegion      string
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
				var body map[string]any
				_ = json.NewDecoder(r.Body).Decode(&body)
				project.FeatureState.RTMEnabled = true
				if region, _ := body["region"].(string); region != "" {
					project.FeatureState.RTMRegion = region
				}
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
