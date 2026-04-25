package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestResolveAgoraDirectoryUsesAgoraHomeDirectly(t *testing.T) {
	dir, err := resolveAgoraDirectory(map[string]string{"AGORA_HOME": "/tmp/agora-home"})
	if err != nil {
		t.Fatal(err)
	}
	if dir != "/tmp/agora-home" {
		t.Fatalf("unexpected dir: %s", dir)
	}
}

func TestRenderProjectEnvDotenvAndShell(t *testing.T) {
	values := map[string]any{
		"AGORA_PROJECT_ID":       "prj_123",
		"AGORA_PROJECT_NAME":     "Project Alpha",
		"AGORA_REGION":           "global",
		"AGORA_APP_ID":           "app_123",
		"AGORA_ENABLED_FEATURES": "rtc,convoai",
		"AGORA_FEATURE_RTC":      true,
		"AGORA_FEATURE_RTM":      false,
		"AGORA_FEATURE_CONVOAI":  true,
	}
	dotenv := renderProjectEnv(values, envDotenv)
	if !strings.Contains(dotenv, `AGORA_PROJECT_NAME="Project Alpha"`) {
		t.Fatalf("expected quoted dotenv output, got %s", dotenv)
	}
	shell := renderProjectEnv(values, envShell)
	if !strings.Contains(shell, "export AGORA_PROJECT_NAME='Project Alpha'") {
		t.Fatalf("expected quoted shell output, got %s", shell)
	}
}

func TestWriteProjectEnvFileUpdatesManagedBlock(t *testing.T) {
	dir := t.TempDir()
	oldwd := t.TempDir()
	_ = oldwd
	path := filepath.Join(dir, ".env.local")
	first, err := writeProjectEnvFile(path, map[string]any{
		"AGORA_PROJECT_ID":       "prj_1",
		"AGORA_PROJECT_NAME":     "Alpha",
		"AGORA_REGION":           "global",
		"AGORA_APP_ID":           "app_1",
		"AGORA_ENABLED_FEATURES": "rtc",
		"AGORA_FEATURE_RTC":      true,
		"AGORA_FEATURE_RTM":      false,
		"AGORA_FEATURE_CONVOAI":  false,
	}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != "created" {
		t.Fatalf("expected created, got %s", first.Status)
	}
	second, err := writeProjectEnvFile(path, map[string]any{
		"AGORA_PROJECT_ID":       "prj_2",
		"AGORA_PROJECT_NAME":     "Beta",
		"AGORA_REGION":           "global",
		"AGORA_APP_ID":           "app_2",
		"AGORA_ENABLED_FEATURES": "rtc,convoai",
		"AGORA_FEATURE_RTC":      true,
		"AGORA_FEATURE_RTM":      false,
		"AGORA_FEATURE_CONVOAI":  true,
	}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != "updated" {
		t.Fatalf("expected updated, got %s", second.Status)
	}
}

func TestBuildProjectDoctorResultWarning(t *testing.T) {
	project := projectDetail{ProjectID: "prj_1", Name: "Alpha", AppID: "app_1", TokenEnabled: false}
	result := buildProjectDoctorResult(project, "global", []featureItem{
		{Feature: "rtc", Message: "rtc included with the project", Status: "included"},
		{Feature: "rtm", Message: "rtm enabled", Status: "enabled"},
		{Feature: "convoai", Message: "convoai enabled", Status: "enabled"},
	}, false)
	if result.Status != "warning" {
		t.Fatalf("expected warning, got %s", result.Status)
	}
}

func TestEnsureAppConfigStateMigratesLegacyDefaults(t *testing.T) {
	dir := t.TempDir()
	configPath, err := resolveConfigFilePath(map[string]string{"XDG_CONFIG_HOME": dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := map[string]any{
		"apiBaseUrl":       legacyAPIBaseURL,
		"browserAutoOpen":  true,
		"logLevel":         "info",
		"oauthBaseUrl":     legacyOAuthBaseURL,
		"oauthClientId":    legacyOAuthClientID,
		"oauthScope":       "basic_info,console",
		"output":           "pretty",
		"telemetryEnabled": true,
		"verbose":          false,
		"version":          1,
	}
	data, _ := json.Marshal(raw)
	if err := os.WriteFile(configPath, data, 0o600); err != nil {
		t.Fatal(err)
	}

	state, err := ensureAppConfigState(map[string]string{"XDG_CONFIG_HOME": dir})
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != "migrated" {
		t.Fatalf("expected migrated, got %s", state.Status)
	}
	if state.Config.APIBaseURL != defaultConfig().APIBaseURL {
		t.Fatalf("expected prod API base URL, got %s", state.Config.APIBaseURL)
	}
	if state.Config.OAuthBaseURL != defaultConfig().OAuthBaseURL {
		t.Fatalf("expected prod OAuth base URL, got %s", state.Config.OAuthBaseURL)
	}
}

func TestWaitForOAuthCallbackAdvertisesLocalhost(t *testing.T) {
	server, err := waitForOAuthCallback("expected-state", 2_000_000_000)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if !strings.Contains(server.RedirectURI, "http://localhost:") {
		t.Fatalf("expected localhost redirect URI, got %s", server.RedirectURI)
	}
	resp, err := http.Get(server.RedirectURI + "?code=test-code&state=expected-state")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	payload, err := server.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if payload.Code != "test-code" {
		t.Fatalf("unexpected code: %s", payload.Code)
	}
}

func TestExchangeTokenLogsSanitizedInvalidShape(t *testing.T) {
	dir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = w.Write([]byte(`{"access_token":"secret-access-token","expires_in":7199,"token_type":"Bearer"}`))
	}))
	defer server.Close()

	app := &App{
		env: map[string]string{
			"XDG_CONFIG_HOME": dir,
			"AGORA_LOG_LEVEL": "debug",
		},
		httpClient: server.Client(),
	}

	if _, err := app.exchangeAuthorizationCode(server.URL, "cli_demo", "code", "verifier", "http://localhost/callback"); err == nil || !strings.Contains(err.Error(), "missing required fields") {
		t.Fatalf("expected invalid shape error, got %v", err)
	}
	logPath, err := resolveLogFilePath(app.env)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(saved), `"access_token":"[REDACTED]"`) {
		t.Fatalf("expected redacted access token in logs, got %s", string(saved))
	}
	if strings.Contains(string(saved), "secret-access-token") {
		t.Fatalf("expected secret token to be absent from logs, got %s", string(saved))
	}
}

func TestWriteProjectEnvFileRequiresAppendOrOverwriteForExplicitFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env.custom")
	if err := os.WriteFile(path, []byte("FOO=bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := writeProjectEnvFile(path, map[string]any{
		"AGORA_PROJECT_ID":       "prj_1",
		"AGORA_PROJECT_NAME":     "Alpha",
		"AGORA_REGION":           "global",
		"AGORA_APP_ID":           "app_1",
		"AGORA_ENABLED_FEATURES": "rtc",
		"AGORA_FEATURE_RTC":      true,
		"AGORA_FEATURE_RTM":      false,
		"AGORA_FEATURE_CONVOAI":  false,
	}, false, false)
	if err == nil || !strings.Contains(err.Error(), "--append") || !strings.Contains(err.Error(), "--overwrite") {
		t.Fatalf("expected explicit file error, got %v", err)
	}
}

func TestWriteProjectEnvFileCreatesNestedDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "apps", "web", ".env.local")
	result, err := writeProjectEnvFile(path, map[string]any{
		"AGORA_PROJECT_ID":       "prj_1",
		"AGORA_PROJECT_NAME":     "Alpha",
		"AGORA_REGION":           "global",
		"AGORA_APP_ID":           "app_1",
		"AGORA_ENABLED_FEATURES": "rtc",
		"AGORA_FEATURE_RTC":      true,
		"AGORA_FEATURE_RTM":      false,
		"AGORA_FEATURE_CONVOAI":  false,
	}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "created" {
		t.Fatalf("expected created, got %s", result.Status)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(saved), "AGORA_PROJECT_ID=prj_1") {
		t.Fatalf("expected nested env file contents, got %s", string(saved))
	}
}

func TestEnsureAppConfigStateCreatedAndLoaded(t *testing.T) {
	dir := t.TempDir()
	state, err := ensureAppConfigState(map[string]string{"XDG_CONFIG_HOME": dir})
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != "created" || state.Config.Version != currentAppConfigVersion {
		t.Fatalf("unexpected state: %+v", state)
	}

	loaded, err := ensureAppConfigState(map[string]string{"XDG_CONFIG_HOME": dir})
	if err != nil {
		t.Fatal(err)
	}
	if loaded.Status != "loaded" {
		t.Fatalf("expected loaded state, got %+v", loaded)
	}
}

func TestEnsureAppConfigStateMigratesPartialAndCustomLegacyConfigs(t *testing.T) {
	dir := t.TempDir()
	configPath, err := resolveConfigFilePath(map[string]string{"XDG_CONFIG_HOME": dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := `{"output":"json","verbose":true}`
	if err := os.WriteFile(configPath, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err := ensureAppConfigState(map[string]string{"XDG_CONFIG_HOME": dir})
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != "migrated" || state.Config.Output != outputJSON || !state.Config.Verbose {
		t.Fatalf("unexpected migrated partial config: %+v", state)
	}

	raw = `{"apiBaseUrl":"https://staging.internal.example.com","browserAutoOpen":false,"logLevel":"debug","oauthBaseUrl":"https://auth.internal.example.com","oauthClientId":"custom-dev-client","oauthScope":"basic_info,console","output":"json","telemetryEnabled":false,"verbose":true,"version":1}`
	if err := os.WriteFile(configPath, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
	state, err = ensureAppConfigState(map[string]string{"XDG_CONFIG_HOME": dir})
	if err != nil {
		t.Fatal(err)
	}
	if state.Status != "migrated" || state.Config.APIBaseURL != "https://staging.internal.example.com" || state.Config.OAuthClientID != "custom-dev-client" || state.Config.TelemetryEnabled {
		t.Fatalf("unexpected migrated custom config: %+v", state)
	}
}

func TestConfigBannerFormattingAndPrintingRules(t *testing.T) {
	created := formatConfigBanner(configState{
		Config:         defaultConfig(),
		ConfigFilePath: "/tmp/agora-cli/config.json",
		Status:         "created",
	})
	if !strings.Contains(created, "Config initialized") {
		t.Fatalf("expected created banner, got %s", created)
	}

	previous := 0
	migrated := formatConfigBanner(configState{
		Config:          defaultConfig(),
		ConfigFilePath:  "/tmp/agora-cli/config.json",
		PreviousVersion: &previous,
		Status:          "migrated",
	})
	if !strings.Contains(migrated, "Config upgraded") {
		t.Fatalf("expected migrated banner, got %s", migrated)
	}

	if shouldPrintConfigBanner(outputPretty, true, "created") != true ||
		shouldPrintConfigBanner(outputPretty, true, "loaded") != false ||
		shouldPrintConfigBanner(outputJSON, true, "created") != false ||
		shouldPrintConfigBanner(outputPretty, false, "created") != false {
		t.Fatal("unexpected banner print rules")
	}
}

func TestResolveConfiguredOutputModeAndConfigApplication(t *testing.T) {
	env := map[string]string{
		"AGORA_API_BASE_URL": "https://env.example.com",
		"AGORA_OUTPUT":       "json",
	}
	app := &App{env: env, cfg: defaultConfig()}
	app.cfg.APIBaseURL = "https://config.example.com"
	app.cfg.LogLevel = "warn"
	app.cfg.BrowserAutoOpen = false
	app.cfg.Verbose = true
	app.applyConfigToEnv()

	if env["AGORA_API_BASE_URL"] != "https://env.example.com" || env["AGORA_OUTPUT"] != "json" {
		t.Fatalf("expected env values to win, got %+v", env)
	}
	if env["AGORA_BROWSER_AUTO_OPEN"] != "0" || env["AGORA_LOG_LEVEL"] != "warn" || env["AGORA_VERBOSE"] != "1" {
		t.Fatalf("expected missing env values to be filled, got %+v", env)
	}

	if resolveConfiguredOutputMode("", map[string]string{}) != outputPretty ||
		resolveConfiguredOutputMode("json", map[string]string{"AGORA_OUTPUT": "pretty"}) != outputJSON {
		t.Fatal("unexpected resolved output mode")
	}
}

func TestGeneratePKCEShape(t *testing.T) {
	pair, err := generatePKCE()
	if err != nil {
		t.Fatal(err)
	}
	if matched := regexp.MustCompile(`^[A-Za-z0-9._~-]{43,128}$`).MatchString(pair.CodeVerifier); !matched {
		t.Fatalf("unexpected code verifier: %s", pair.CodeVerifier)
	}
	if matched := regexp.MustCompile(`^[A-Za-z0-9_-]+$`).MatchString(pair.CodeChallenge); !matched {
		t.Fatalf("unexpected code challenge: %s", pair.CodeChallenge)
	}
}

func TestEnsureValidAccessTokenRefreshesExpiredSession(t *testing.T) {
	dir := t.TempDir()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"refreshed-access-token","token_type":"Bearer","expires_in":7200,"refresh_token":"refreshed-refresh-token","scope":"basic_info,console"}`)
	}))
	defer server.Close()

	app := &App{
		env: map[string]string{
			"XDG_CONFIG_HOME":       dir,
			"AGORA_OAUTH_BASE_URL":  server.URL,
			"AGORA_OAUTH_CLIENT_ID": "refresh-client",
		},
		httpClient: server.Client(),
	}
	if err := saveSession(app.env, session{
		AccessToken:  "expired-access-token",
		RefreshToken: "refresh-token-value",
		TokenType:    "Bearer",
		Scope:        "basic_info,console",
		ObtainedAt:   time.Now().UTC().Add(-10 * time.Minute).Format(time.RFC3339),
		ExpiresAt:    time.Now().UTC().Add(-5 * time.Minute).Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}

	s, err := app.ensureValidAccessToken()
	if err != nil {
		t.Fatal(err)
	}
	if s.AccessToken != "refreshed-access-token" {
		t.Fatalf("expected refreshed token, got %+v", s)
	}
}

func TestWaitForOAuthCallbackMismatchAndTimeout(t *testing.T) {
	server, err := waitForOAuthCallback("expected-state", 25*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	resp, err := http.Get(server.RedirectURI + "?code=test-code&state=wrong-state")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if _, err := server.Wait(); err == nil || err.Error() != "OAuth state mismatch." {
		t.Fatalf("expected mismatch error, got %v", err)
	}

	timeoutServer, err := waitForOAuthCallback("expected-state", 25*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	defer timeoutServer.Close()
	if _, err := timeoutServer.Wait(); err == nil || err.Error() != "Timed out waiting for the OAuth callback." {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestExchangeAuthorizationCodeFailureAndScopeArray(t *testing.T) {
	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"invalid_grant","error_description":"redirect_uri mismatch"}`)
	}))
	defer failServer.Close()

	app := &App{httpClient: failServer.Client(), env: map[string]string{}}
	if _, err := app.exchangeAuthorizationCode(failServer.URL, "cli_demo", "bad-code", "code-verifier", "http://localhost/callback"); err == nil || !strings.Contains(err.Error(), "OAuth token exchange failed with status 400") {
		t.Fatalf("unexpected auth-code failure error: %v", err)
	}

	scopeServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"secret-access-token","expires_in":7199,"refresh_token":"secret-refresh-token","scope":["basic_info","console"],"token_type":"Bearer"}`)
	}))
	defer scopeServer.Close()
	app.httpClient = scopeServer.Client()
	s, err := app.exchangeAuthorizationCode(scopeServer.URL, "cli_demo", "good-code", "code-verifier", "http://localhost/callback")
	if err != nil {
		t.Fatal(err)
	}
	if s.Scope != "basic_info,console" {
		t.Fatalf("unexpected scope normalization: %+v", s)
	}
}

func TestAPIRequestRetriesAfter401(t *testing.T) {
	dir := t.TempDir()
	authHeaders := []string{}
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeaders = append(authHeaders, r.Header.Get("Authorization"))
		if len(authHeaders) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			_, _ = io.WriteString(w, `{"code":"UNAUTHORIZED","message":"expired token","requestId":"req-401"}`)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"allowStaticWithDynamic": true,
			"appId":                  "app_1",
			"certificateEnabled":     true,
			"createdAt":              "2026-04-07T12:34:56.000Z",
			"name":                   "Project Alpha",
			"projectId":              "prj_1",
			"projectType":            "paas",
			"signKey":                "sig",
			"stage":                  3,
			"status":                 "active",
			"tokenEnabled":           true,
			"updatedAt":              "2026-04-07T13:34:56.000Z",
			"usage7d":                0,
			"useCaseId":              "education",
			"vid":                    100001788,
		})
	}))
	defer apiServer.Close()
	oauthServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		_, _ = io.WriteString(w, `{"access_token":"refreshed-access-token","expires_in":7200,"refresh_token":"refreshed-refresh-token","scope":"basic_info,console","token_type":"Bearer"}`)
	}))
	defer oauthServer.Close()

	app := &App{
		env: map[string]string{
			"XDG_CONFIG_HOME":       dir,
			"AGORA_API_BASE_URL":    apiServer.URL,
			"AGORA_OAUTH_BASE_URL":  oauthServer.URL,
			"AGORA_OAUTH_CLIENT_ID": "refresh-client",
		},
		httpClient: apiServer.Client(),
	}
	if err := saveSession(app.env, session{
		AccessToken:  "expired-on-server",
		RefreshToken: "refresh-token-value",
		TokenType:    "Bearer",
		Scope:        "basic_info,console",
		ObtainedAt:   time.Now().UTC().Format(time.RFC3339),
		ExpiresAt:    time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
	app.httpClient = &http.Client{}
	var out projectDetail
	if err := app.apiRequest(http.MethodGet, "/api/cli/v1/projects/prj_1", nil, nil, &out); err != nil {
		t.Fatal(err)
	}
	if len(authHeaders) != 2 || authHeaders[0] != "Bearer expired-on-server" || authHeaders[1] != "Bearer refreshed-access-token" {
		t.Fatalf("unexpected auth headers: %+v", authHeaders)
	}
}

func TestPathsLogsAndArtifactsParity(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{"XDG_CONFIG_HOME": dir}
	agoraDir, err := resolveAgoraDirectory(env)
	if err != nil {
		t.Fatal(err)
	}
	sessionPath, err := resolveSessionFilePath(env)
	if err != nil {
		t.Fatal(err)
	}
	logPath, err := resolveLogFilePath(env)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(agoraDir, "agora-cli") || !strings.HasSuffix(sessionPath, filepath.Join("agora-cli", "session.json")) || !strings.HasSuffix(logPath, filepath.Join("agora-cli", "logs", "agora-cli.log")) {
		t.Fatalf("unexpected path layout: %s %s %s", agoraDir, sessionPath, logPath)
	}

	configExample, err := os.ReadFile(filepath.Join("..", "..", "config.example.json"))
	if err != nil {
		t.Fatal(err)
	}
	var parsed map[string]any
	if err := json.Unmarshal(configExample, &parsed); err != nil {
		t.Fatal(err)
	}
	if int(parsed["version"].(float64)) != currentAppConfigVersion {
		t.Fatalf("unexpected config.example version: %v", parsed)
	}
	for _, file := range []string{"README.md", "RELEASING.md", filepath.Join("docs", "automation.md")} {
		raw, err := os.ReadFile(filepath.Join("..", "..", file))
		if err != nil {
			t.Fatalf("expected %s: %v", file, err)
		}
		if file == "README.md" && (!strings.Contains(string(raw), "go build -o agora .") || !strings.Contains(string(raw), "agora init") || !strings.Contains(string(raw), "quickstart env write") || !strings.Contains(string(raw), "--help --all")) {
			t.Fatalf("unexpected README contents: %s", string(raw))
		}
		if file == filepath.Join("docs", "automation.md") && (!strings.Contains(string(raw), "--json") || !strings.Contains(string(raw), "\"command\": \"init\"") || !strings.Contains(string(raw), "project doctor")) {
			t.Fatalf("unexpected automation doc contents: %s", string(raw))
		}
	}
}

func TestLogRotationFilteringAndVerboseMirror(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{
		"XDG_CONFIG_HOME":     dir,
		"AGORA_LOG_MAX_BYTES": "180",
		"AGORA_LOG_MAX_FILES": "3",
	}
	for i := 0; i < 12; i++ {
		if err := appendAppLog("info", "test.log.entry", env, map[string]any{
			"iteration": i,
			"message":   "This is a log line long enough to force file rotation for the Agora CLI logger.",
		}); err != nil {
			t.Fatal(err)
		}
	}
	logDir, err := resolveLogDirectoryPath(env)
	if err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(logDir)
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, entry := range entries {
		found[entry.Name()] = true
	}
	for _, name := range []string{"agora-cli.log", "agora-cli.log.1", "agora-cli.log.2"} {
		if !found[name] {
			t.Fatalf("missing rotated log file %s", name)
		}
	}

	filterEnv := map[string]string{
		"XDG_CONFIG_HOME": dir,
		"AGORA_LOG_LEVEL": "error",
	}
	if err := appendAppLog("info", "test.info.hidden", filterEnv, nil); err != nil {
		t.Fatal(err)
	}
	if err := appendAppLog("error", "test.error.kept", filterEnv, nil); err != nil {
		t.Fatal(err)
	}
	logPath, err := resolveLogFilePath(filterEnv)
	if err != nil {
		t.Fatal(err)
	}
	saved, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(saved), "test.info.hidden") || !strings.Contains(string(saved), "test.error.kept") {
		t.Fatalf("unexpected filtered log contents: %s", string(saved))
	}

	verboseDir := t.TempDir()
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	originalStderr := os.Stderr
	os.Stderr = writePipe
	defer func() { os.Stderr = originalStderr }()
	verboseEnv := map[string]string{
		"XDG_CONFIG_HOME": verboseDir,
		"AGORA_LOG_LEVEL": "debug",
		"AGORA_VERBOSE":   "1",
	}
	if err := appendAppLog("debug", "test.verbose.output", verboseEnv, map[string]any{
		"accessToken": "secret-value",
		"detail":      "visible",
	}); err != nil {
		t.Fatal(err)
	}
	_ = writePipe.Close()
	mirrored, err := io.ReadAll(readPipe)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(mirrored), "test.verbose.output") || !strings.Contains(string(mirrored), `"detail":"visible"`) || !strings.Contains(string(mirrored), `"accessToken":"[REDACTED]"`) || strings.Contains(string(mirrored), "secret-value") {
		t.Fatalf("unexpected verbose mirror output: %s", string(mirrored))
	}
}
