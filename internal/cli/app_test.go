package cli

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
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

func TestDetectInstallProvenanceUsesReceiptThenExecutablePath(t *testing.T) {
	installerDir := t.TempDir()
	installerPath := filepath.Join(installerDir, "agora")
	receipt := installReceipt{
		SchemaVersion: 1,
		Tool:          "agora",
		InstallMethod: "installer",
		InstallPath:   installerPath,
		Version:       "0.1.10",
		InstalledAt:   "2026-04-30T11:00:00Z",
		Source:        "install.sh",
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installReceiptPath(installerPath), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	provenance := detectInstallProvenanceForPath(map[string]string{"HOMEBREW_PREFIX": "/usr/local"}, installerPath)
	if provenance.Method != "installer" || provenance.Source != "install.sh" || provenance.ReceiptPath == "" {
		t.Fatalf("expected installer receipt provenance, got %+v", provenance)
	}
}

func TestDetectInstallProvenanceFallsBackToExecutablePath(t *testing.T) {
	tests := []struct {
		name       string
		env        map[string]string
		exePath    string
		wantMethod string
	}{
		{
			name:       "installer wins when Homebrew is only in environment",
			env:        map[string]string{"HOMEBREW_PREFIX": "/usr/local"},
			exePath:    "/usr/local/bin/agora",
			wantMethod: "installer",
		},
		{
			name:       "homebrew detected from resolved Cellar path",
			env:        map[string]string{"HOMEBREW_PREFIX": "/usr/local"},
			exePath:    "/usr/local/Cellar/agora-cli/0.1.10/bin/agora",
			wantMethod: "homebrew",
		},
		{
			name:       "installer wins when npm is only in environment",
			env:        map[string]string{"npm_config_prefix": "/usr/local"},
			exePath:    "/usr/local/bin/agora",
			wantMethod: "installer",
		},
		{
			name:       "npm detected from node_modules path",
			env:        map[string]string{"npm_config_prefix": "/usr/local"},
			exePath:    "/usr/local/lib/node_modules/agoraio-cli-darwin-arm64/bin/agora",
			wantMethod: "npm",
		},
		{
			name:       "unknown detected for test binary",
			env:        map[string]string{},
			exePath:    "/tmp/go-build/cli.test",
			wantMethod: "unknown",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			provenance := detectInstallProvenanceForPath(tt.env, tt.exePath)
			if provenance.Method != tt.wantMethod {
				t.Fatalf("expected %s, got %s", tt.wantMethod, provenance.Method)
			}
		})
	}
}

func TestDetectInstallProvenanceIgnoresStaleReceipt(t *testing.T) {
	dir := t.TempDir()
	exePath := filepath.Join(dir, "agora")
	receipt := installReceipt{
		SchemaVersion: 1,
		Tool:          "agora",
		InstallMethod: "npm",
		InstallPath:   filepath.Join(dir, "old-agora"),
		Version:       "0.1.10",
		InstalledAt:   "2026-04-30T11:00:00Z",
		Source:        "test",
	}
	raw, err := json.Marshal(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(installReceiptPath(exePath), raw, 0o644); err != nil {
		t.Fatal(err)
	}

	provenance := detectInstallProvenanceForPath(map[string]string{}, exePath)
	if provenance.Method != "installer" || provenance.Source != "fallback" {
		t.Fatalf("expected stale receipt fallback, got %+v", provenance)
	}
}

func TestWriteInstallReceiptRoundTrips(t *testing.T) {
	exePath := filepath.Join(t.TempDir(), "agora")
	receiptPath, err := writeInstallReceipt(exePath, "v0.1.10", "agora upgrade")
	if err != nil {
		t.Fatal(err)
	}
	receipt, err := readInstallReceipt(receiptPath)
	if err != nil {
		t.Fatal(err)
	}
	if !receipt.validForPath(exePath) {
		t.Fatalf("expected valid receipt for %s: %+v", exePath, receipt)
	}
	if receipt.Version != "0.1.10" || receipt.Source != "agora upgrade" {
		t.Fatalf("unexpected receipt contents: %+v", receipt)
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

func TestWriteProjectEnvFileWritesOnlyCredentials(t *testing.T) {
	dir := t.TempDir()
	oldwd := t.TempDir()
	_ = oldwd
	path := filepath.Join(dir, ".env.local")
	first, err := writeProjectEnvFile(path, map[string]any{
		"AGORA_APP_ID":          "app_1",
		"AGORA_APP_CERTIFICATE": "cert_1",
	}, false, false, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if first.Status != "created" {
		t.Fatalf("expected created, got %s", first.Status)
	}
	second, err := writeProjectEnvFile(path, map[string]any{
		"AGORA_APP_ID":          "app_2",
		"AGORA_APP_CERTIFICATE": "cert_2",
	}, false, false, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if second.Status != "updated" {
		t.Fatalf("expected updated, got %s", second.Status)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(saved), "AGORA_APP_ID=app_2") || !strings.Contains(string(saved), "AGORA_APP_CERTIFICATE=cert_2") || strings.Contains(string(saved), "AGORA_PROJECT_ID") || strings.Contains(string(saved), "# BEGIN AGORA CLI") {
		t.Fatalf("unexpected env contents: %s", string(saved))
	}
}

func TestWriteProjectEnvFileReplacesOldManagedBlockWithCredentials(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env.local")
	oldContent := strings.Join([]string{
		"FOO=bar",
		"",
		"# BEGIN AGORA CLI",
		"# Generated by `agora project env`",
		"AGORA_PROJECT_ID=prj_old",
		"AGORA_PROJECT_NAME=Old",
		"AGORA_APP_ID=old_app",
		"# END AGORA CLI",
		"",
	}, "\n")
	if err := os.WriteFile(path, []byte(oldContent), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := writeProjectEnvFile(path, map[string]any{
		"AGORA_APP_ID":          "app_1",
		"AGORA_APP_CERTIFICATE": "cert_1",
	}, false, false, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "updated" {
		t.Fatalf("expected updated, got %s", result.Status)
	}
	saved, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(saved), "FOO=bar") || !strings.Contains(string(saved), "AGORA_APP_ID=app_1") || !strings.Contains(string(saved), "AGORA_APP_CERTIFICATE=cert_1") || strings.Contains(string(saved), "AGORA_PROJECT_ID") || strings.Contains(string(saved), "# BEGIN AGORA CLI") {
		t.Fatalf("unexpected migrated env contents: %s", string(saved))
	}
}

func TestMergeEnvAssignmentsUpdatesExpectedAndCommentsConflicts(t *testing.T) {
	existing := strings.Join([]string{
		"USER_VALUE=keep",
		"export NEXT_PUBLIC_AGORA_APP_ID=old_public",
		"NEXT_PUBLIC_AGORA_APP_ID=duplicate_public",
		"AGORA_APP_ID=old_generic",
		"APP_ID=old_app",
		"NEXT_AGORA_APP_CERTIFICATE=old_cert",
		"APP_CERTIFICATE=old_generic_cert",
		"",
	}, "\n")
	merged, status := mergeEnvAssignments(existing, map[string]any{
		"NEXT_PUBLIC_AGORA_APP_ID":   "app_new",
		"NEXT_AGORA_APP_CERTIFICATE": "cert_new",
	}, nil, []string{"AGORA_APP_ID", "APP_ID", "APP_CERTIFICATE"})
	if status != "updated" {
		t.Fatalf("expected updated, got %s", status)
	}
	for _, expected := range []string{
		"USER_VALUE=keep",
		"NEXT_PUBLIC_AGORA_APP_ID=app_new",
		"NEXT_AGORA_APP_CERTIFICATE=cert_new",
		"# Replaced by Agora CLI: NEXT_PUBLIC_AGORA_APP_ID=duplicate_public",
		"# Replaced by Agora CLI: AGORA_APP_ID=old_generic",
		"# Replaced by Agora CLI: APP_ID=old_app",
		"# Replaced by Agora CLI: APP_CERTIFICATE=old_generic_cert",
	} {
		if !strings.Contains(merged, expected) {
			t.Fatalf("expected merged env to contain %q, got %s", expected, merged)
		}
	}
}

func TestBuildProjectDoctorResultWarning(t *testing.T) {
	project := projectDetail{ProjectID: "prj_1", Name: "Alpha", AppID: "app_1", TokenEnabled: false}
	result := buildProjectDoctorResult(project, "global", []featureItem{
		{Feature: "rtc", Message: "rtc included with the project", Status: "included"},
		{Feature: "rtm", Message: "rtm enabled", Status: "enabled"},
		{Feature: "convoai", Message: "convoai enabled", Status: "enabled"},
	}, "convoai", false)
	if result.Status != "warning" {
		t.Fatalf("expected warning, got %s", result.Status)
	}
}

func TestEnsureAppConfigStateMigratesPreviousDefaults(t *testing.T) {
	dir := t.TempDir()
	configPath, err := resolveConfigFilePath(map[string]string{"XDG_CONFIG_HOME": dir})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(configPath), 0o755); err != nil {
		t.Fatal(err)
	}
	raw := map[string]any{
		"apiBaseUrl":       previousAPIBaseURL,
		"browserAutoOpen":  true,
		"logLevel":         "info",
		"oauthBaseUrl":     previousOAuthBaseURL,
		"oauthClientId":    previousOAuthClientID,
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
		"AGORA_APP_ID":          "app_1",
		"AGORA_APP_CERTIFICATE": "cert_1",
	}, false, false, nil, false)
	if err == nil || !strings.Contains(err.Error(), "--append") || !strings.Contains(err.Error(), "--overwrite") {
		t.Fatalf("expected explicit file error, got %v", err)
	}
}

func TestWriteProjectEnvFileCreatesNestedDirectories(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "apps", "web", ".env.local")
	result, err := writeProjectEnvFile(path, map[string]any{
		"AGORA_APP_ID":          "app_1",
		"AGORA_APP_CERTIFICATE": "cert_1",
	}, false, false, nil, false)
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
	if !strings.Contains(string(saved), "AGORA_APP_ID=app_1") || !strings.Contains(string(saved), "AGORA_APP_CERTIFICATE=cert_1") || strings.Contains(string(saved), "AGORA_PROJECT_ID") {
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

func TestEnsureAppConfigStateMigratesPartialAndCustomPreviousConfigs(t *testing.T) {
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

	cases := []struct {
		name     string
		mode     outputMode
		isTTY    bool
		status   string
		env      map[string]string
		expected bool
	}{
		{name: "tty pretty created no-ci", mode: outputPretty, isTTY: true, status: "created", env: map[string]string{}, expected: true},
		{name: "tty pretty created with CI=true", mode: outputPretty, isTTY: true, status: "created", env: map[string]string{"CI": "true"}, expected: false},
		{name: "tty pretty created with GITHUB_ACTIONS", mode: outputPretty, isTTY: true, status: "created", env: map[string]string{"GITHUB_ACTIONS": "true"}, expected: false},
		{name: "tty pretty created with GITLAB_CI", mode: outputPretty, isTTY: true, status: "created", env: map[string]string{"GITLAB_CI": "true"}, expected: false},
		{name: "tty pretty created with BUILDKITE", mode: outputPretty, isTTY: true, status: "created", env: map[string]string{"BUILDKITE": "true"}, expected: false},
		{name: "tty pretty created with CIRCLECI", mode: outputPretty, isTTY: true, status: "created", env: map[string]string{"CIRCLECI": "true"}, expected: false},
		{name: "tty pretty created with CI=false", mode: outputPretty, isTTY: true, status: "created", env: map[string]string{"CI": "false"}, expected: true},
		{name: "tty pretty created with CI=0", mode: outputPretty, isTTY: true, status: "created", env: map[string]string{"CI": "0"}, expected: true},
		{name: "ci-detect-disabled escape hatch", mode: outputPretty, isTTY: true, status: "created", env: map[string]string{"CI": "true", "AGORA_DISABLE_CI_DETECT": "1"}, expected: true},
	}
	for _, tc := range cases {
		got := shouldPrintConfigBannerWithEnv(tc.mode, tc.isTTY, tc.status, tc.env)
		if got != tc.expected {
			t.Errorf("%s: shouldPrintConfigBannerWithEnv = %v; want %v", tc.name, got, tc.expected)
		}
	}
}

func TestIsCIEnvironment(t *testing.T) {
	cases := []struct {
		name     string
		env      map[string]string
		expected bool
	}{
		{name: "nil env", env: nil, expected: false},
		{name: "empty env", env: map[string]string{}, expected: false},
		{name: "CI=true", env: map[string]string{"CI": "true"}, expected: true},
		{name: "CI=1", env: map[string]string{"CI": "1"}, expected: true},
		{name: "CI=false", env: map[string]string{"CI": "false"}, expected: false},
		{name: "CI=0", env: map[string]string{"CI": "0"}, expected: false},
		{name: "CI empty string", env: map[string]string{"CI": ""}, expected: false},
		{name: "GITHUB_ACTIONS only", env: map[string]string{"GITHUB_ACTIONS": "true"}, expected: true},
		{name: "GITLAB_CI only", env: map[string]string{"GITLAB_CI": "true"}, expected: true},
		{name: "BUILDKITE only", env: map[string]string{"BUILDKITE": "true"}, expected: true},
		{name: "CIRCLECI only", env: map[string]string{"CIRCLECI": "true"}, expected: true},
		{name: "JENKINS_URL only", env: map[string]string{"JENKINS_URL": "https://jenkins.example.com"}, expected: true},
		{name: "TF_BUILD only", env: map[string]string{"TF_BUILD": "True"}, expected: true},
		{name: "AGORA_DISABLE_CI_DETECT overrides CI=true", env: map[string]string{"CI": "true", "AGORA_DISABLE_CI_DETECT": "1"}, expected: false},
		{name: "AGORA_DISABLE_CI_DETECT overrides GITHUB_ACTIONS", env: map[string]string{"GITHUB_ACTIONS": "true", "AGORA_DISABLE_CI_DETECT": "1"}, expected: false},
	}
	for _, tc := range cases {
		got := isCIEnvironment(tc.env)
		if got != tc.expected {
			t.Errorf("%s: isCIEnvironment = %v; want %v", tc.name, got, tc.expected)
		}
	}
}

func TestResolveOutputModeFromEnvCIPrecedence(t *testing.T) {
	cases := []struct {
		name      string
		raw       string
		osEnv     map[string]string
		cfgOutput outputMode
		expected  outputMode
	}{
		{name: "no env, default config: pretty", raw: "", osEnv: map[string]string{}, cfgOutput: outputPretty, expected: outputPretty},
		{name: "explicit --output pretty wins over CI", raw: "pretty", osEnv: map[string]string{"CI": "true"}, cfgOutput: outputPretty, expected: outputPretty},
		{name: "explicit --output json wins over no-CI", raw: "json", osEnv: map[string]string{}, cfgOutput: outputPretty, expected: outputJSON},
		{name: "user-set AGORA_OUTPUT=pretty wins over CI", raw: "", osEnv: map[string]string{"CI": "true", "AGORA_OUTPUT": "pretty"}, cfgOutput: outputPretty, expected: outputPretty},
		{name: "user-set AGORA_OUTPUT=json wins regardless", raw: "", osEnv: map[string]string{"AGORA_OUTPUT": "json"}, cfgOutput: outputPretty, expected: outputJSON},
		{name: "config json without env stays json", raw: "", osEnv: map[string]string{}, cfgOutput: outputJSON, expected: outputJSON},
		{name: "CI=true alone yields json", raw: "", osEnv: map[string]string{"CI": "true"}, cfgOutput: outputPretty, expected: outputJSON},
		{name: "GITHUB_ACTIONS yields json", raw: "", osEnv: map[string]string{"GITHUB_ACTIONS": "true"}, cfgOutput: outputPretty, expected: outputJSON},
		{name: "AGORA_DISABLE_CI_DETECT escape hatch", raw: "", osEnv: map[string]string{"CI": "true", "AGORA_DISABLE_CI_DETECT": "1"}, cfgOutput: outputPretty, expected: outputPretty},
	}
	for _, tc := range cases {
		app := &App{osEnv: tc.osEnv, cfg: defaultConfig()}
		app.cfg.Output = tc.cfgOutput
		got := app.resolveOutputModeFromEnv(tc.raw)
		if got != tc.expected {
			t.Errorf("%s: resolveOutputModeFromEnv(%q) = %v; want %v", tc.name, tc.raw, got, tc.expected)
		}
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
		resolveConfiguredOutputMode("json", map[string]string{"AGORA_OUTPUT": "pretty"}) != outputJSON ||
		resolveConfiguredOutputMode("pretty", map[string]string{"AGORA_OUTPUT": "json"}) != outputPretty {
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
	if _, err := timeoutServer.Wait(); err == nil || !strings.HasPrefix(err.Error(), "Timed out waiting for the OAuth callback.") {
		t.Fatalf("expected timeout error, got %v", err)
	}
}

func TestWaitForOAuthCallbackAcceptsIPv4WhenRedirectUsesLocalhost(t *testing.T) {
	server, err := waitForOAuthCallback("expected-state", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer server.Close()
	if !strings.HasPrefix(server.RedirectURI, "http://localhost:") {
		t.Fatalf("expected localhost redirect URI for OAuth compatibility, got %s", server.RedirectURI)
	}
	if len(server.listeners) == 0 {
		t.Fatal("expected at least one loopback listener")
	}
	parsed, err := url.Parse(server.RedirectURI)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.Get("http://127.0.0.1:" + parsed.Port() + "/oauth/callback?code=test-code&state=expected-state")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	payload, err := server.Wait()
	if err != nil {
		t.Fatal(err)
	}
	if payload.Code != "test-code" || payload.State != "expected-state" {
		t.Fatalf("unexpected callback payload: %+v", payload)
	}
}

func TestExchangeAuthorizationCodeFailureAndScopeArray(t *testing.T) {
	failServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"error":"invalid_grant","error_description":"redirect_uri mismatch"}`)
	}))
	defer failServer.Close()

	app := &App{httpClient: failServer.Client(), env: map[string]string{}}
	if _, err := app.exchangeAuthorizationCode(failServer.URL, "cli_demo", "bad-code", "code-verifier", "http://localhost/callback"); err == nil || !strings.Contains(err.Error(), "OAuth token exchange failed (HTTP 400)") {
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

func TestAPIRequestReturnsStructuredErrors(t *testing.T) {
	dir := t.TempDir()
	apiServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("content-type", "application/json")
		w.Header().Set("x-request-id", "req_123")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = io.WriteString(w, `{"code":"PROJECT_INVALID","message":"Project name is invalid"}`)
	}))
	defer apiServer.Close()

	app := &App{
		env: map[string]string{
			"XDG_CONFIG_HOME":    dir,
			"AGORA_API_BASE_URL": apiServer.URL,
		},
		httpClient: apiServer.Client(),
	}
	if err := saveSession(app.env, session{
		AccessToken:  "valid-access-token",
		RefreshToken: "refresh-token-value",
		TokenType:    "Bearer",
		Scope:        "basic_info,console",
		ObtainedAt:   time.Now().UTC().Format(time.RFC3339),
		ExpiresAt:    time.Now().UTC().Add(time.Hour).Format(time.RFC3339),
	}); err != nil {
		t.Fatal(err)
	}
	var out projectDetail
	err := app.apiRequest(http.MethodGet, "/api/cli/v1/projects/prj_1", nil, nil, &out)
	var structured *cliError
	if !errors.As(err, &structured) {
		t.Fatalf("expected structured API error, got %T %v", err, err)
	}
	if structured.Code != "PROJECT_INVALID" || structured.HTTPStatus != http.StatusBadRequest || structured.RequestID != "req_123" || !strings.Contains(structured.Message, "Project name is invalid") {
		t.Fatalf("unexpected structured error: %+v", structured)
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
	for _, file := range []string{"README.md", "RELEASING.md", filepath.Join("docs", "automation.md"), filepath.Join(".github", "workflows", "ci.yml"), filepath.Join(".github", "workflows", "release.yml")} {
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
		if file == filepath.Join(".github", "workflows", "ci.yml") && (!strings.Contains(string(raw), "pull_request:") || !strings.Contains(string(raw), "ubuntu-latest") || !strings.Contains(string(raw), "macos-latest") || !strings.Contains(string(raw), "windows-latest")) {
			t.Fatalf("unexpected ci workflow contents: %s", string(raw))
		}
		if file == filepath.Join(".github", "workflows", "release.yml") && (!strings.Contains(string(raw), "refs/tags/v") && !strings.Contains(string(raw), "tags:")) {
			t.Fatalf("unexpected release workflow contents: %s", string(raw))
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

func TestDoNotTrackSuppressesAppLog(t *testing.T) {
	dir := t.TempDir()
	env := map[string]string{
		"XDG_CONFIG_HOME": dir,
		"DO_NOT_TRACK":    "1",
	}
	if err := appendAppLog("error", "test.do_not_track", env, map[string]any{"detail": "hidden"}); err != nil {
		t.Fatal(err)
	}
	logPath, err := resolveLogFilePath(env)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(logPath); err == nil || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected no log file when DO_NOT_TRACK is set, got err=%v", err)
	}
}
