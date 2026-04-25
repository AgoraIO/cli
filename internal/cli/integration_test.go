package cli

import (
	"bufio"
	"bytes"
	"context"
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

func TestCLIHelperProcess(t *testing.T) {
	if os.Getenv("GO_WANT_CLI_HELPER_PROCESS") != "1" {
		return
	}
	args := os.Args
	index := 0
	for i, arg := range args {
		if arg == "--" {
			index = i + 1
			break
		}
	}
	cliArgs := args[index:]
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

func runCLI(t *testing.T, args []string, options cliRunOptions) cliResult {
	t.Helper()
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=TestCLIHelperProcess", "--"}, args...)...)
	if options.workdir != "" {
		cmd.Dir = options.workdir
	}
	cmd.Env = append(os.Environ(), "GO_WANT_CLI_HELPER_PROCESS=1")
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
		if exitErr, ok := err.(*exec.ExitError); ok {
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

type fakeCLIBFF struct {
	server   *http.Server
	baseURL  string
	mu       sync.Mutex
	projects map[string]*fakeProject
	requests []struct {
		Method        string
		Pathname      string
		Authorization string
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
		}{
			Method:        r.Method,
			Pathname:      r.URL.Path,
			Authorization: r.Header.Get("Authorization"),
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

func parseAuthURL(stderr string) string {
	match := regexp.MustCompile(`Open this URL to continue login:\n(https?://\S+)`).FindStringSubmatch(stderr)
	if len(match) == 2 {
		return match[1]
	}
	return ""
}

func TestCLIHelpSurfaceAndRemovedCommands(t *testing.T) {
	result := runCLI(t, []string{"--help"}, cliRunOptions{})
	if result.exitCode != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s", result.exitCode, result.stderr)
	}
	for _, token := range []string{"auth", "project", "quickstart", "init", "login", "logout", "whoami"} {
		if !strings.Contains(result.stdout, token) {
			t.Fatalf("expected help to contain %q, got %s", token, result.stdout)
		}
	}
	if strings.Contains(result.stdout, "add") {
		t.Fatalf("did not expect experimental add command in root help: %s", result.stdout)
	}
	if strings.Contains(result.stdout, "completion") {
		t.Fatalf("did not expect completion command in help: %s", result.stdout)
	}
	for _, args := range [][]string{{"uap"}, {"rtm2"}, {"project", "onboard"}, {"add"}} {
		result := runCLI(t, args, cliRunOptions{})
		if result.exitCode != 1 || !strings.Contains(result.stderr, "unknown command") {
			t.Fatalf("expected unknown command for %v, got exit=%d stderr=%s", args, result.exitCode, result.stderr)
		}
	}
}

func TestCLIHelpContentIsTaskOriented(t *testing.T) {
	root := runCLI(t, []string{"--help"}, cliRunOptions{})
	if root.exitCode != 0 || !strings.Contains(root.stdout, "project     Manage remote Agora project resources") || !strings.Contains(root.stdout, "quickstart  Clone official standalone quickstart repositories") || !strings.Contains(root.stdout, "init        Create a project and quickstart in one onboarding flow") || !strings.Contains(root.stdout, "agora --help --all") || strings.Contains(root.stdout, "add         ") {
		t.Fatalf("unexpected root help output: %+v", root)
	}
	rootAll := runCLI(t, []string{"--help", "--all"}, cliRunOptions{})
	if rootAll.exitCode != 0 || !strings.Contains(rootAll.stdout, "Full Command Tree") || !strings.Contains(rootAll.stdout, "agora project env write") || !strings.Contains(rootAll.stdout, "agora quickstart env write") || strings.Contains(rootAll.stdout, "agora add") {
		t.Fatalf("unexpected root help --all output: %+v", rootAll)
	}

	quickstart := runCLI(t, []string{"quickstart", "create", "--help"}, cliRunOptions{})
	if quickstart.exitCode != 0 || !strings.Contains(quickstart.stdout, "If a current project context exists") || !strings.Contains(quickstart.stdout, "agora quickstart create my-nextjs-demo --template nextjs") {
		t.Fatalf("unexpected quickstart create help output: %+v", quickstart)
	}
	quickstartEnv := runCLI(t, []string{"quickstart", "env", "write", "--help"}, cliRunOptions{})
	if quickstartEnv.exitCode != 0 || !strings.Contains(quickstartEnv.stdout, "Next.js quickstarts receive NEXT_PUBLIC_* client env vars") {
		t.Fatalf("unexpected quickstart env write help output: %+v", quickstartEnv)
	}
	initHelp := runCLI(t, []string{"init", "--help"}, cliRunOptions{})
	if initHelp.exitCode != 0 || (!strings.Contains(initHelp.stdout, "creates a new Agora project") && !strings.Contains(strings.ToLower(initHelp.stdout), "create a new agora project")) || !strings.Contains(initHelp.stdout, "--project") {
		t.Fatalf("unexpected init help output: %+v", initHelp)
	}

	project := runCLI(t, []string{"project", "--help"}, cliRunOptions{})
	if project.exitCode != 0 || !strings.Contains(project.stdout, "These commands do not clone local application code") || !strings.Contains(project.stdout, "agora project env write .env.local") {
		t.Fatalf("unexpected project help output: %+v", project)
	}

	add := runCLI(t, []string{"add", "--help"}, cliRunOptions{})
	if add.exitCode != 1 || !strings.Contains(add.stderr, "unknown command") {
		t.Fatalf("expected add to remain unavailable, got %+v", add)
	}
}

func TestCLIJSONErrorsUseEnvelope(t *testing.T) {
	result := runCLI(t, []string{"project", "env", "write", ".env.custom", "--append", "--overwrite", "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME": t.TempDir(),
			"AGORA_LOG_LEVEL": "error",
		},
	})
	if result.exitCode != 1 || !strings.Contains(result.stdout, `"ok":false`) || !strings.Contains(result.stdout, `"command":"project env write"`) || !strings.Contains(result.stdout, `"exitCode":1`) || result.stderr != "" {
		t.Fatalf("unexpected json error envelope: %+v", result)
	}

	unknown := runCLI(t, []string{"project", "onboard", "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME": t.TempDir(),
			"AGORA_LOG_LEVEL": "error",
		},
	})
	if unknown.exitCode != 1 || !strings.Contains(unknown.stdout, `"ok":false`) || !strings.Contains(unknown.stdout, `"command":"project onboard"`) || unknown.stderr != "" {
		t.Fatalf("unexpected unknown-command json envelope: %+v", unknown)
	}
}

func TestCLIQuickstartListAndCreate(t *testing.T) {
	configHome := t.TempDir()
	api := newFakeCLIBFF()
	defer api.server.Close()
	nextjsRepo := createLocalGitRepo(t, map[string]string{
		"README.md":         "# Next.js Quickstart\n",
		"env.local.example": "NEXT_PUBLIC_AGORA_APP_ID=\nNEXT_AGORA_APP_CERTIFICATE=\n",
		"package.json":      `{"name":"nextjs-quickstart"}`,
		"app/page.tsx":      "export default function Page() { return null }\n",
	})
	pythonRepo := createLocalGitRepo(t, map[string]string{
		"README.md":                  "# Python Quickstart\n",
		"server-python/.env.example": "APP_ID=\nAPP_CERTIFICATE=\nPORT=8000\n",
		"server-python/main.py":      "print('hello')\n",
		"web-client/package.json":    `{"name":"python-quickstart-web"}`,
	})
	goRepo := createLocalGitRepo(t, map[string]string{
		"README.md":               "# Go Quickstart\n",
		"server-go/.env.example":  "APP_ID=\nAPP_CERTIFICATE=\nPORT=8080\n",
		"server-go/main.go":       "package main\nfunc main() {}\n",
		"web-client/package.json": `{"name":"go-quickstart-web"}`,
	})

	project := buildFakeProject("Project Alpha", "prj_123456", "app_123456", "global")
	api.projects[project.ProjectID] = &project
	persistSessionForIntegration(t, configHome)
	if err := saveContext(map[string]string{"XDG_CONFIG_HOME": configHome}, projectContext{
		CurrentProjectID:   &project.ProjectID,
		CurrentProjectName: &project.Name,
		CurrentRegion:      "global",
		PreferredRegion:    "global",
	}); err != nil {
		t.Fatal(err)
	}

	list := runCLI(t, []string{"quickstart", "list", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME": configHome,
		"AGORA_LOG_LEVEL": "error",
	}})
	if list.exitCode != 0 || !strings.Contains(list.stdout, `"id":"nextjs"`) || !strings.Contains(list.stdout, `"id":"python"`) || !strings.Contains(list.stdout, `"id":"go"`) {
		t.Fatalf("unexpected quickstart list result: %+v", list)
	}
	listAll := runCLI(t, []string{"quickstart", "list", "--all", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME": configHome,
		"AGORA_LOG_LEVEL": "error",
	}})
	if listAll.exitCode != 0 || !strings.Contains(listAll.stdout, `"id":"go"`) {
		t.Fatalf("unexpected quickstart list --all result: %+v", listAll)
	}

	rootDir := t.TempDir()
	unboundTarget := filepath.Join(rootDir, "video-demo")
	createUnbound := runCLI(t, []string{"quickstart", "create", "video-demo", "--template", "nextjs", "--dir", unboundTarget, "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":                  t.TempDir(),
			"AGORA_LOG_LEVEL":                  "error",
			"AGORA_QUICKSTART_NEXTJS_REPO_URL": nextjsRepo,
		},
		workdir: rootDir,
	})
	if createUnbound.exitCode != 0 || !strings.Contains(createUnbound.stdout, `"envStatus":"template-only"`) {
		t.Fatalf("unexpected unbound quickstart create result: %+v", createUnbound)
	}
	if _, err := os.Stat(filepath.Join(unboundTarget, ".git")); err != nil {
		t.Fatalf("expected cloned git repo in unbound scaffold: %v", err)
	}
	if _, err := os.Stat(filepath.Join(unboundTarget, ".env.local")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("did not expect .env.local in unbound scaffold, got %v", err)
	}

	boundTarget := filepath.Join(rootDir, "agent-demo")
	createBound := runCLI(t, []string{"quickstart", "create", "agent-demo", "--template", "python", "--dir", boundTarget, "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":                  configHome,
			"AGORA_API_BASE_URL":               api.baseURL,
			"AGORA_LOG_LEVEL":                  "error",
			"AGORA_QUICKSTART_PYTHON_REPO_URL": pythonRepo,
		},
		workdir: rootDir,
	})
	if createBound.exitCode != 0 || !strings.Contains(createBound.stdout, `"envStatus":"configured"`) || !strings.Contains(createBound.stdout, `"projectId":"prj_123456"`) {
		t.Fatalf("unexpected bound quickstart create result: %+v", createBound)
	}
	localEnv, err := os.ReadFile(filepath.Join(boundTarget, "server-python", ".env.local"))
	if err != nil {
		t.Fatalf("expected .env.local in bound scaffold: %v", err)
	}
	if !strings.Contains(string(localEnv), "# Project ID: prj_123456") || !strings.Contains(string(localEnv), "# Project Name: Project Alpha") || !strings.Contains(string(localEnv), "APP_ID=app_123456") || !strings.Contains(string(localEnv), "APP_CERTIFICATE=") {
		t.Fatalf("unexpected .env.local contents: %s", string(localEnv))
	}
	metadataRaw, err := os.ReadFile(filepath.Join(boundTarget, ".agora", "project.json"))
	if err != nil {
		t.Fatalf("expected .agora/project.json in bound scaffold: %v", err)
	}
	if !strings.Contains(string(metadataRaw), `"projectId": "prj_123456"`) || !strings.Contains(string(metadataRaw), `"template": "python"`) {
		t.Fatalf("unexpected .agora/project.json contents: %s", string(metadataRaw))
	}
	if strings.Contains(string(localEnv), "AGORA_PROJECT_ID=") || strings.Contains(string(localEnv), "AGORA_PROJECT_NAME=") {
		t.Fatalf("did not expect project metadata env vars in python env file: %s", string(localEnv))
	}
	readme, err := os.ReadFile(filepath.Join(boundTarget, "README.md"))
	if err != nil {
		t.Fatalf("expected README in bound scaffold: %v", err)
	}
	if !strings.Contains(string(readme), "Python Quickstart") {
		t.Fatalf("unexpected README contents: %s", string(readme))
	}

	nextjsBoundTarget := filepath.Join(rootDir, "nextjs-demo")
	createNextjsBound := runCLI(t, []string{"quickstart", "create", "nextjs-demo", "--template", "nextjs", "--dir", nextjsBoundTarget, "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":                  configHome,
			"AGORA_API_BASE_URL":               api.baseURL,
			"AGORA_LOG_LEVEL":                  "error",
			"AGORA_QUICKSTART_NEXTJS_REPO_URL": nextjsRepo,
		},
		workdir: rootDir,
	})
	if createNextjsBound.exitCode != 0 || !strings.Contains(createNextjsBound.stdout, `"envStatus":"configured"`) {
		t.Fatalf("unexpected bound nextjs quickstart create result: %+v", createNextjsBound)
	}
	nextjsEnv, err := os.ReadFile(filepath.Join(nextjsBoundTarget, ".env.local"))
	if err != nil {
		t.Fatalf("expected .env.local in bound nextjs scaffold: %v", err)
	}
	if !strings.Contains(string(nextjsEnv), "# Project ID: prj_123456") || !strings.Contains(string(nextjsEnv), "# Project Name: Project Alpha") || !strings.Contains(string(nextjsEnv), "NEXT_PUBLIC_AGORA_APP_ID=app_123456") || !strings.Contains(string(nextjsEnv), "NEXT_AGORA_APP_CERTIFICATE=") {
		t.Fatalf("unexpected nextjs .env.local contents: %s", string(nextjsEnv))
	}
	if strings.Contains(string(nextjsEnv), "AGORA_PROJECT_ID=") || strings.Contains(string(nextjsEnv), "AGORA_PROJECT_NAME=") {
		t.Fatalf("did not expect project metadata env vars in nextjs env file: %s", string(nextjsEnv))
	}

	writeNextjsEnv := runCLI(t, []string{"quickstart", "env", "write", nextjsBoundTarget, "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":    configHome,
			"AGORA_API_BASE_URL": api.baseURL,
			"AGORA_LOG_LEVEL":    "error",
		},
		workdir: rootDir,
	})
	if writeNextjsEnv.exitCode != 0 || !strings.Contains(writeNextjsEnv.stdout, `"template":"nextjs"`) {
		t.Fatalf("unexpected quickstart env write result: %+v", writeNextjsEnv)
	}

	writePythonEnv := runCLI(t, []string{"quickstart", "env", "write", boundTarget, "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":    configHome,
			"AGORA_API_BASE_URL": api.baseURL,
			"AGORA_LOG_LEVEL":    "error",
		},
		workdir: rootDir,
	})
	if writePythonEnv.exitCode != 0 || !strings.Contains(writePythonEnv.stdout, `"template":"python"`) {
		t.Fatalf("unexpected python quickstart env write result: %+v", writePythonEnv)
	}

	repoScopedConfig := t.TempDir()
	persistSessionForIntegration(t, repoScopedConfig)
	repoShow := runCLI(t, []string{"project", "show", "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":    repoScopedConfig,
			"AGORA_API_BASE_URL": api.baseURL,
			"AGORA_LOG_LEVEL":    "error",
		},
		workdir: filepath.Join(boundTarget, "server-python"),
	})
	if repoShow.exitCode != 0 || !strings.Contains(repoShow.stdout, `"projectId":"prj_123456"`) {
		t.Fatalf("expected repo-local .agora binding to resolve project context, got %+v", repoShow)
	}

	goBoundTarget := filepath.Join(rootDir, "go-demo")
	createGoBound := runCLI(t, []string{"quickstart", "create", "go-demo", "--template", "go", "--dir", goBoundTarget, "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":              configHome,
			"AGORA_API_BASE_URL":           api.baseURL,
			"AGORA_LOG_LEVEL":              "error",
			"AGORA_QUICKSTART_GO_REPO_URL": goRepo,
		},
		workdir: rootDir,
	})
	if createGoBound.exitCode != 0 || !strings.Contains(createGoBound.stdout, `"envStatus":"configured"`) {
		t.Fatalf("unexpected bound go quickstart create result: %+v", createGoBound)
	}
	goEnv, err := os.ReadFile(filepath.Join(goBoundTarget, "server-go", ".env.local"))
	if err != nil {
		t.Fatalf("expected .env.local in bound go scaffold: %v", err)
	}
	if !strings.Contains(string(goEnv), "APP_ID=app_123456") || !strings.Contains(string(goEnv), "APP_CERTIFICATE=") {
		t.Fatalf("unexpected go .env.local contents: %s", string(goEnv))
	}

	noCertProject := buildFakeProject("No Cert", "prj_nocert", "app_nocert", "global")
	noCertProject.SignKey = nil
	noCertProject.CertificateEnabled = false
	api.projects[noCertProject.ProjectID] = &noCertProject
	rollbackTarget := filepath.Join(rootDir, "rollback-demo")
	createRollback := runCLI(t, []string{"quickstart", "create", "rollback-demo", "--template", "python", "--dir", rollbackTarget, "--project", "prj_nocert", "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":                  configHome,
			"AGORA_API_BASE_URL":               api.baseURL,
			"AGORA_LOG_LEVEL":                  "error",
			"AGORA_QUICKSTART_PYTHON_REPO_URL": pythonRepo,
		},
		workdir: rootDir,
	})
	if createRollback.exitCode != 1 || !strings.Contains(createRollback.stdout, `"ok":false`) || !strings.Contains(createRollback.stdout, `failed to configure quickstart env after clone`) {
		t.Fatalf("unexpected rollback quickstart result: %+v", createRollback)
	}
	if _, err := os.Stat(rollbackTarget); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected rollback target to be removed, got %v", err)
	}
}

func TestCLIQuickstartEnvWriteUsesTargetRepoBindingPrecedence(t *testing.T) {
	configHome := t.TempDir()
	rootDir := t.TempDir()
	api := newFakeCLIBFF()
	defer api.server.Close()

	alpha := buildFakeProject("Project Alpha", "prj_alpha", "app_alpha", "global")
	alpha.FeatureState.RTMEnabled = true
	alpha.FeatureState.ConvoAIEnabled = true
	beta := buildFakeProject("Project Beta", "prj_beta", "app_beta", "global")
	beta.FeatureState.RTMEnabled = true
	beta.FeatureState.ConvoAIEnabled = true
	api.projects[alpha.ProjectID] = &alpha
	api.projects[beta.ProjectID] = &beta
	persistSessionForIntegration(t, configHome)
	if err := saveContext(map[string]string{"XDG_CONFIG_HOME": configHome}, projectContext{
		CurrentProjectID:   &beta.ProjectID,
		CurrentProjectName: &beta.Name,
		CurrentRegion:      "global",
		PreferredRegion:    "global",
	}); err != nil {
		t.Fatal(err)
	}

	targetDir := filepath.Join(rootDir, "demo-go")
	if err := os.MkdirAll(filepath.Join(targetDir, "server-go"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "server-go", ".env.example"), []byte("APP_ID=\nAPP_CERTIFICATE=\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeLocalProjectBinding(targetDir, localProjectBinding{
		ProjectID:   alpha.ProjectID,
		ProjectName: alpha.Name,
		Region:      "global",
		Template:    "go",
		EnvPath:     "server-go/.env.local",
	}); err != nil {
		t.Fatal(err)
	}

	result := runCLI(t, []string{"quickstart", "env", "write", targetDir, "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":    configHome,
			"AGORA_API_BASE_URL": api.baseURL,
			"AGORA_LOG_LEVEL":    "error",
		},
		workdir: t.TempDir(),
	})
	if result.exitCode != 0 || !strings.Contains(result.stdout, `"projectId":"prj_alpha"`) {
		t.Fatalf("expected repo-local project binding precedence, got %+v", result)
	}
	envRaw, err := os.ReadFile(filepath.Join(targetDir, "server-go", ".env.local"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(envRaw), "APP_ID=app_alpha") || strings.Contains(string(envRaw), "APP_ID=app_beta") {
		t.Fatalf("expected target repo binding project app id in env, got %s", string(envRaw))
	}
}

func TestCLIQuickstartEnvWriteMissingBindingEvenWhenEnvExists(t *testing.T) {
	configHome := t.TempDir()
	targetDir := filepath.Join(t.TempDir(), "demo-go")
	if err := os.MkdirAll(filepath.Join(targetDir, "server-go"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "server-go", ".env.example"), []byte("APP_ID=\nAPP_CERTIFICATE=\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "server-go", ".env.local"), []byte("APP_ID=stale\nAPP_CERTIFICATE=stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := runCLI(t, []string{"quickstart", "env", "write", targetDir, "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME": configHome,
			"AGORA_LOG_LEVEL": "error",
		},
		workdir: t.TempDir(),
	})
	if result.exitCode != 1 || !strings.Contains(result.stdout, `"ok":false`) || !strings.Contains(result.stdout, ".agora/project.json") || !strings.Contains(result.stdout, "--project") || !strings.Contains(result.stdout, "agora project use") {
		t.Fatalf("unexpected missing binding result: %+v", result)
	}
}

func TestCLIInitCreatesProjectAndQuickstart(t *testing.T) {
	configHome := t.TempDir()
	rootDir := t.TempDir()
	api := newFakeCLIBFF()
	defer api.server.Close()
	persistSessionForIntegration(t, configHome)

	nextjsRepo := createLocalGitRepo(t, map[string]string{
		"README.md":         "# Next.js Quickstart\n",
		"env.local.example": "NEXT_PUBLIC_AGORA_APP_ID=\nNEXT_AGORA_APP_CERTIFICATE=\n",
		"package.json":      `{"name":"nextjs-quickstart"}`,
		"app/page.tsx":      "export default function Page() { return null }\n",
	})

	initResult := runCLI(t, []string{"init", "starter-demo", "--template", "nextjs", "--dir", filepath.Join(rootDir, "starter-demo"), "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":                  configHome,
			"AGORA_API_BASE_URL":               api.baseURL,
			"AGORA_LOG_LEVEL":                  "error",
			"AGORA_QUICKSTART_NEXTJS_REPO_URL": nextjsRepo,
		},
		workdir: rootDir,
	})
	if initResult.exitCode != 0 || !strings.Contains(initResult.stdout, `"action":"init"`) || !strings.Contains(initResult.stdout, `"projectAction":"created"`) || !strings.Contains(initResult.stdout, `"template":"nextjs"`) {
		t.Fatalf("unexpected init result: %+v", initResult)
	}
	if !strings.Contains(initResult.stdout, `"enabledFeatures":["rtc","convoai"]`) && !strings.Contains(initResult.stdout, `"enabledFeatures":["convoai","rtc"]`) {
		t.Fatalf("expected default features in init result: %+v", initResult)
	}
	localEnv, err := os.ReadFile(filepath.Join(rootDir, "starter-demo", ".env.local"))
	if err != nil {
		t.Fatalf("expected init env file: %v", err)
	}
	if !strings.Contains(string(localEnv), "NEXT_PUBLIC_AGORA_APP_ID=app_0001") {
		t.Fatalf("unexpected init env contents: %s", string(localEnv))
	}
	if _, err := os.Stat(filepath.Join(rootDir, "starter-demo", ".agora", "project.json")); err != nil {
		t.Fatalf("expected init to create .agora/project.json: %v", err)
	}
	ctx, err := loadContext(map[string]string{"XDG_CONFIG_HOME": configHome})
	if err != nil {
		t.Fatal(err)
	}
	if ctx.CurrentProjectName == nil || *ctx.CurrentProjectName != "starter-demo" {
		t.Fatalf("expected init to persist current project context, got %+v", ctx)
	}
}

func TestCLILoginAndWhoAmIParity(t *testing.T) {
	configHome := t.TempDir()
	oauth := newFakeOAuthServer()
	defer oauth.server.Close()

	result := runCLI(t, []string{"login"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":         configHome,
		"AGORA_OAUTH_BASE_URL":    oauth.baseURL,
		"AGORA_OAUTH_CLIENT_ID":   "test-public-client",
		"AGORA_OAUTH_SCOPE":       "basic_info,console",
		"AGORA_BROWSER_AUTO_OPEN": "0",
		"AGORA_LOGIN_TIMEOUT_MS":  "2000",
		"AGORA_LOG_LEVEL":         "error",
		"AGORA_VERBOSE":           "0",
	}, onStderr: func(stderr string) bool {
		u := parseAuthURL(stderr)
		if u == "" {
			return false
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
		}
		return err == nil
	}})
	if result.exitCode != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s", result.exitCode, result.stderr)
	}

	status := runCLI(t, []string{"whoami", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME": configHome,
		"AGORA_LOG_LEVEL": "error",
		"AGORA_VERBOSE":   "0",
	}})
	if status.exitCode != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s", status.exitCode, status.stderr)
	}
	if len(oauth.authorizeRedirectURIs) != 1 || !strings.Contains(oauth.authorizeRedirectURIs[0], "http://localhost:") {
		t.Fatalf("expected localhost redirect URI, got %+v", oauth.authorizeRedirectURIs)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(status.stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	data := envelope["data"].(map[string]any)
	if data["authenticated"] != true {
		t.Fatalf("expected authenticated response, got %v", status.stdout)
	}
}

func TestCLIProjectAndEnvAndDoctorParity(t *testing.T) {
	configHome := t.TempDir()
	projectDir := t.TempDir()
	api := newFakeCLIBFF()
	defer api.server.Close()

	alpha := buildFakeProject("Project Alpha", "prj_123456", "app_123456", "global")
	api.projects[alpha.ProjectID] = &alpha
	persistSessionForIntegration(t, configHome)
	if err := saveContext(map[string]string{"XDG_CONFIG_HOME": configHome}, projectContext{
		CurrentProjectID:   &alpha.ProjectID,
		CurrentProjectName: &alpha.Name,
		CurrentRegion:      "global",
		PreferredRegion:    "global",
	}); err != nil {
		t.Fatal(err)
	}

	envResult := runCLI(t, []string{"project", "env"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":    configHome,
		"AGORA_API_BASE_URL": api.baseURL,
		"AGORA_LOG_LEVEL":    "error",
		"AGORA_VERBOSE":      "0",
	}})
	if envResult.exitCode != 0 || !strings.Contains(envResult.stdout, "AGORA_PROJECT_ID=prj_123456") {
		t.Fatalf("unexpected project env result: exit=%d stdout=%s stderr=%s", envResult.exitCode, envResult.stdout, envResult.stderr)
	}

	oldwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldwd) }()
	if err := os.Chdir(projectDir); err != nil {
		t.Fatal(err)
	}
	writeResult := runCLI(t, []string{"project", "env", "write", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":    configHome,
		"AGORA_API_BASE_URL": api.baseURL,
		"AGORA_LOG_LEVEL":    "error",
		"AGORA_VERBOSE":      "0",
	}, workdir: projectDir})
	if writeResult.exitCode != 0 {
		t.Fatalf("unexpected env write result: exit=%d stderr=%s", writeResult.exitCode, writeResult.stderr)
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".env.local")); err != nil {
		t.Fatalf("expected .env.local to be created: %v", err)
	}

	doctorResult := runCLI(t, []string{"project", "doctor", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":    configHome,
		"AGORA_API_BASE_URL": api.baseURL,
		"AGORA_LOG_LEVEL":    "error",
		"AGORA_VERBOSE":      "0",
	}})
	if doctorResult.exitCode != 1 {
		t.Fatalf("expected doctor exit 1, got %d stdout=%s stderr=%s", doctorResult.exitCode, doctorResult.stdout, doctorResult.stderr)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(doctorResult.stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	data := envelope["data"].(map[string]any)
	if data["status"] != "not_ready" {
		t.Fatalf("expected not_ready doctor result, got %s", doctorResult.stdout)
	}
}

func TestCLIAuthStatusExitCodeParity(t *testing.T) {
	result := runCLI(t, []string{"auth", "status", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME": t.TempDir(),
		"AGORA_LOG_LEVEL": "error",
		"AGORA_VERBOSE":   "0",
	}})
	if result.exitCode != 3 {
		t.Fatalf("expected exit 3, got %d stdout=%s stderr=%s", result.exitCode, result.stdout, result.stderr)
	}
}

func TestCLIProjectUseShowFeatureAndDoctorHappyPath(t *testing.T) {
	configHome := t.TempDir()
	api := newFakeCLIBFF()
	defer api.server.Close()

	alpha := buildFakeProject("Project Alpha", "prj_123456", "app_123456", "global")
	beta := buildFakeProject("Project Beta", "prj_9999", "app_9999", "cn")
	beta.FeatureState.ConvoAIEnabled = true
	beta.FeatureState.RTMEnabled = true
	api.projects[alpha.ProjectID] = &alpha
	api.projects[beta.ProjectID] = &beta
	persistSessionForIntegration(t, configHome)

	useResult := runCLI(t, []string{"project", "use", "Project Beta", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":    configHome,
		"AGORA_API_BASE_URL": api.baseURL,
		"AGORA_LOG_LEVEL":    "error",
	}})
	if useResult.exitCode != 0 || !strings.Contains(useResult.stdout, `"projectId":"prj_9999"`) {
		t.Fatalf("unexpected use result: %+v", useResult)
	}

	showPretty := runCLI(t, []string{"project", "show", "--output", "pretty"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":    configHome,
		"AGORA_API_BASE_URL": api.baseURL,
		"AGORA_LOG_LEVEL":    "error",
		"AGORA_OUTPUT":       "pretty",
	}})
	if showPretty.exitCode != 0 || !strings.Contains(showPretty.stdout, "App Certificate") || !strings.Contains(showPretty.stdout, "Region") {
		t.Fatalf("unexpected pretty show output: %+v", showPretty)
	}

	featureStatus := runCLI(t, []string{"project", "feature", "status", "convoai", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":    configHome,
		"AGORA_API_BASE_URL": api.baseURL,
		"AGORA_LOG_LEVEL":    "error",
	}})
	if featureStatus.exitCode != 0 || !strings.Contains(featureStatus.stdout, `"status":"enabled"`) {
		t.Fatalf("unexpected feature status: %+v", featureStatus)
	}

	doctor := runCLI(t, []string{"project", "doctor", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":    configHome,
		"AGORA_API_BASE_URL": api.baseURL,
		"AGORA_LOG_LEVEL":    "error",
	}})
	if doctor.exitCode != 0 || !strings.Contains(doctor.stdout, `"status":"healthy"`) {
		t.Fatalf("unexpected doctor result: %+v", doctor)
	}

	rtmDoctor := runCLI(t, []string{"project", "doctor", "--feature", "rtm", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":    configHome,
		"AGORA_API_BASE_URL": api.baseURL,
		"AGORA_LOG_LEVEL":    "error",
	}})
	if rtmDoctor.exitCode != 0 || !strings.Contains(rtmDoctor.stdout, `"feature":"rtm"`) || !strings.Contains(rtmDoctor.stdout, `"status":"healthy"`) {
		t.Fatalf("unexpected rtm doctor result: %+v", rtmDoctor)
	}
}

func TestCLIProjectDoctorDeepDetectsWorkspaceDrift(t *testing.T) {
	configHome := t.TempDir()
	api := newFakeCLIBFF()
	defer api.server.Close()

	project := buildFakeProject("Project Alpha", "prj_123456", "app_123456", "global")
	project.FeatureState.RTMEnabled = true
	project.FeatureState.ConvoAIEnabled = true
	api.projects[project.ProjectID] = &project
	persistSessionForIntegration(t, configHome)
	if err := saveContext(map[string]string{"XDG_CONFIG_HOME": configHome}, projectContext{
		CurrentProjectID:   &project.ProjectID,
		CurrentProjectName: &project.Name,
		CurrentRegion:      "global",
		PreferredRegion:    "global",
	}); err != nil {
		t.Fatal(err)
	}

	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "server-go"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeLocalProjectBinding(repoRoot, localProjectBinding{
		ProjectID:   project.ProjectID,
		ProjectName: project.Name,
		Region:      "global",
		Template:    "go",
		EnvPath:     "server-go/.env.local",
	}); err != nil {
		t.Fatal(err)
	}
	mismatched := strings.Join([]string{
		"# BEGIN AGORA CLI QUICKSTART",
		"# Project ID: prj_other",
		"# Project Name: Project Other",
		"APP_ID=app_other",
		"APP_CERTIFICATE=other",
		"# END AGORA CLI QUICKSTART",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(repoRoot, "server-go", ".env.local"), []byte(mismatched), 0o644); err != nil {
		t.Fatal(err)
	}

	doctor := runCLI(t, []string{"project", "doctor", "--deep", "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":    configHome,
			"AGORA_API_BASE_URL": api.baseURL,
			"AGORA_LOG_LEVEL":    "error",
		},
		workdir: repoRoot,
	})
	if doctor.exitCode != 1 || !strings.Contains(doctor.stdout, `"mode":"deep"`) || !strings.Contains(doctor.stdout, `"status":"not_ready"`) || !strings.Contains(doctor.stdout, `"category":"workspace"`) || !strings.Contains(doctor.stdout, `"code":"WORKSPACE_ENV_APP_ID_MISMATCH"`) {
		t.Fatalf("unexpected deep doctor mismatch result: %+v", doctor)
	}
	if !strings.Contains(doctor.stdout, `"workspace":`) || !strings.Contains(doctor.stdout, `"envAppID":"app_other"`) {
		t.Fatalf("expected deep doctor workspace details, got %+v", doctor)
	}
}

func TestCLIProjectEnvFormatsAndWriteRules(t *testing.T) {
	configHome := t.TempDir()
	projectDir := t.TempDir()
	api := newFakeCLIBFF()
	defer api.server.Close()

	project := buildFakeProject("Project Beta", "prj_9999", "app_9999", "global")
	project.FeatureState.ConvoAIEnabled = true
	project.FeatureState.RTMEnabled = true
	api.projects[project.ProjectID] = &project
	persistSessionForIntegration(t, configHome)
	if err := saveContext(map[string]string{"XDG_CONFIG_HOME": configHome}, projectContext{
		CurrentProjectID:   &project.ProjectID,
		CurrentProjectName: &project.Name,
		CurrentRegion:      "global",
		PreferredRegion:    "global",
	}); err != nil {
		t.Fatal(err)
	}

	shellResult := runCLI(t, []string{"project", "env", "--shell", "--with-secrets"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":    configHome,
			"AGORA_API_BASE_URL": api.baseURL,
			"AGORA_LOG_LEVEL":    "error",
		},
		workdir: projectDir,
	})
	if shellResult.exitCode != 0 || !strings.Contains(shellResult.stdout, "export AGORA_APP_CERTIFICATE=") {
		t.Fatalf("unexpected shell env result: %+v", shellResult)
	}

	jsonResult := runCLI(t, []string{"project", "env", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":    configHome,
		"AGORA_API_BASE_URL": api.baseURL,
		"AGORA_LOG_LEVEL":    "error",
	}})
	if jsonResult.exitCode != 0 || !strings.Contains(jsonResult.stdout, `"command":"project env"`) || !strings.Contains(jsonResult.stdout, `"AGORA_FEATURE_CONVOAI":true`) {
		t.Fatalf("unexpected json env result: %+v", jsonResult)
	}

	if err := os.WriteFile(filepath.Join(projectDir, ".env.custom"), []byte("FOO=bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	explicitConflict := runCLI(t, []string{"project", "env", "write", ".env.custom"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":    configHome,
			"AGORA_API_BASE_URL": api.baseURL,
			"AGORA_LOG_LEVEL":    "error",
		},
		workdir: projectDir,
	})
	if explicitConflict.exitCode != 1 || !strings.Contains(explicitConflict.stderr, "--append") {
		t.Fatalf("unexpected explicit write conflict: %+v", explicitConflict)
	}

	explicitConflictJSON := runCLI(t, []string{"project", "env", "write", ".env.custom", "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":    configHome,
			"AGORA_API_BASE_URL": api.baseURL,
			"AGORA_LOG_LEVEL":    "error",
		},
		workdir: projectDir,
	})
	if explicitConflictJSON.exitCode != 1 || !strings.Contains(explicitConflictJSON.stdout, `"ok":false`) || !strings.Contains(explicitConflictJSON.stdout, `"command":"project env write"`) || !strings.Contains(explicitConflictJSON.stdout, `--append`) || explicitConflictJSON.stderr != "" {
		t.Fatalf("unexpected explicit write conflict json result: %+v", explicitConflictJSON)
	}

	appendResult := runCLI(t, []string{"project", "env", "write", "--append", "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":    configHome,
			"AGORA_API_BASE_URL": api.baseURL,
			"AGORA_LOG_LEVEL":    "error",
		},
		workdir: projectDir,
	})
	if appendResult.exitCode != 0 {
		t.Fatalf("unexpected append result: %+v", appendResult)
	}

	nestedResult := runCLI(t, []string{"project", "env", "write", "apps/web/.env.local", "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":    configHome,
			"AGORA_API_BASE_URL": api.baseURL,
			"AGORA_LOG_LEVEL":    "error",
		},
		workdir: projectDir,
	})
	if nestedResult.exitCode != 0 {
		t.Fatalf("unexpected nested write result: %+v", nestedResult)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "apps", "web", ".env.local")); err != nil {
		t.Fatalf("expected nested env file, got %v", err)
	}
}

func TestCLIFeatureEnableAndDoctorAuthError(t *testing.T) {
	configHome := t.TempDir()
	api := newFakeCLIBFF()
	defer api.server.Close()

	project := buildFakeProject("Project Alpha", "prj_123456", "app_123456", "global")
	api.projects[project.ProjectID] = &project
	persistSessionForIntegration(t, configHome)
	if err := saveContext(map[string]string{"XDG_CONFIG_HOME": configHome}, projectContext{
		CurrentProjectID:   &project.ProjectID,
		CurrentProjectName: &project.Name,
		CurrentRegion:      "global",
		PreferredRegion:    "global",
	}); err != nil {
		t.Fatal(err)
	}

	enable := runCLI(t, []string{"project", "feature", "enable", "convoai", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":    configHome,
		"AGORA_API_BASE_URL": api.baseURL,
		"AGORA_LOG_LEVEL":    "error",
	}})
	if enable.exitCode != 0 || !strings.Contains(enable.stdout, `"status":"enabled"`) {
		t.Fatalf("unexpected feature enable result: %+v", enable)
	}

	unauthDoctor := runCLI(t, []string{"project", "doctor", "--deep", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME": t.TempDir(),
		"AGORA_LOG_LEVEL": "error",
	}})
	if unauthDoctor.exitCode != 3 || !strings.Contains(unauthDoctor.stdout, `"status":"auth_error"`) || !strings.Contains(unauthDoctor.stdout, `"mode":"deep"`) {
		t.Fatalf("unexpected unauth doctor result: %+v", unauthDoctor)
	}
}
