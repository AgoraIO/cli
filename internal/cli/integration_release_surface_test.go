package cli

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReleaseCatalogAndStatusCommands(t *testing.T) {
	configHome := t.TempDir()
	env := map[string]string{
		"AGORA_BROWSER_AUTO_OPEN": "0",
		"AGORA_LOG_LEVEL":         "error",
		"XDG_CONFIG_HOME":         configHome,
	}

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "quickstart list JSON", args: []string{"quickstart", "list", "--json"}, want: []string{`"command":"quickstart list"`, `"items"`, `"python"`}},
		{name: "quickstart list pretty", args: []string{"--pretty", "--no-color", "quickstart", "list"}, want: []string{"Quickstarts", "python"}},
		{name: "version pretty", args: []string{"--pretty", "--no-color", "version"}, want: []string{"Version", "dev"}},
		{name: "telemetry status JSON", args: []string{"telemetry", "status", "--json"}, want: []string{`"command":"telemetry"`, `"action":"status"`, `"enabled"`}},
		{name: "telemetry status pretty", args: []string{"--pretty", "--no-color", "telemetry", "status"}, want: []string{"Telemetry"}},
		{name: "telemetry disable", args: []string{"--pretty", "--no-color", "telemetry", "disable"}, want: []string{"Telemetry", "Enabled", "no"}},
		{name: "telemetry enable", args: []string{"telemetry", "enable", "--json"}, want: []string{`"command":"telemetry"`, `"action":"enable"`, `"enabled":true`}},
		{name: "open docs", args: []string{"--pretty", "--no-color", "open", "--target", "docs", "--no-browser"}, want: []string{"https://", "docs"}},
		{name: "full help", args: []string{"--help", "--all"}, want: []string{"agora init", "agora quickstart create", "agora project doctor"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runCLI(t, tt.args, cliRunOptions{env: env})
			if result.exitCode != 0 {
				t.Fatalf("exit = %d, stderr = %s", result.exitCode, result.stderr)
			}
			for _, want := range tt.want {
				if !strings.Contains(strings.ToLower(result.stdout), strings.ToLower(want)) {
					t.Errorf("output does not contain %q: %s", want, result.stdout)
				}
			}
		})
	}
}

func TestLogoutPublicOutputModes(t *testing.T) {
	configHome := t.TempDir()
	env := map[string]string{
		"AGORA_LOG_LEVEL": "error",
		"XDG_CONFIG_HOME": configHome,
	}
	persistSessionForIntegration(t, configHome)
	projectID := "project-id"
	projectName := "demo"
	if err := saveContext(env, projectContext{CurrentProjectID: &projectID, CurrentProjectName: &projectName}); err != nil {
		t.Fatalf("saveContext() error = %v", err)
	}

	prettyResult := runCLI(t, []string{"--pretty", "--no-color", "logout"}, cliRunOptions{env: env})
	if prettyResult.exitCode != 0 || !strings.Contains(strings.ToLower(prettyResult.stdout), "logged-out") {
		t.Fatalf("pretty logout = exit %d, stdout %q, stderr %q", prettyResult.exitCode, prettyResult.stdout, prettyResult.stderr)
	}

	jsonResult := runCLI(t, []string{"auth", "logout", "--json"}, cliRunOptions{env: env})
	if jsonResult.exitCode != 0 || !strings.Contains(jsonResult.stdout, `"command":"logout"`) {
		t.Fatalf("JSON logout = exit %d, stdout %q, stderr %q", jsonResult.exitCode, jsonResult.stdout, jsonResult.stderr)
	}
}

func TestProjectAndWebhookPublicPrettyOutput(t *testing.T) {
	configHome := t.TempDir()
	api := newFakeCLIBFF()
	t.Cleanup(func() { _ = api.server.Close() })
	project := buildFakeProject("demo", "prj_0001", "app_0001", "global")
	api.projects[project.ProjectID] = &project
	persistSessionForIntegration(t, configHome)
	env := webhookTestEnv(configHome, api.baseURL)

	create := runCLI(t, []string{
		"project", "webhook", "create",
		"--project", "demo",
		"--feature", "rtc",
		"--url", "https://example.com/webhook",
		"--events", "channel-created,1002",
		"--json",
	}, cliRunOptions{env: env})
	if create.exitCode != 0 || !strings.Contains(create.stdout, `"configId":42`) {
		t.Fatalf("webhook create = exit %d, stdout %s, stderr %s", create.exitCode, create.stdout, create.stderr)
	}

	tests := []struct {
		name string
		args []string
		want []string
	}{
		{name: "project list", args: []string{"--pretty", "--no-color", "project", "list"}, want: []string{"demo", "prj_0001"}},
		{name: "project use", args: []string{"--pretty", "--no-color", "project", "use", "demo"}, want: []string{"Current Project", "demo"}},
		{name: "project show", args: []string{"--pretty", "--no-color", "project", "show", "demo"}, want: []string{"demo", "app_0001"}},
		{name: "project feature list", args: []string{"--pretty", "--no-color", "project", "feature", "list", "demo"}, want: []string{"rtc", "rtm", "convoai"}},
		{name: "webhook list", args: []string{"--pretty", "--no-color", "project", "webhook", "list", "--project", "demo", "--feature", "rtc"}, want: []string{"42", "example.com/webhook", "enabled"}},
		{name: "webhook show", args: []string{"--pretty", "--no-color", "project", "webhook", "show", "42", "--project", "demo", "--feature", "rtc"}, want: []string{"42", "example.com/webhook", "channel-created"}},
		{name: "project env", args: []string{"--pretty", "--no-color", "project", "env", "--project", "demo"}, want: []string{"AGORA_APP_ID", "app_0001"}},
		{name: "project feature status", args: []string{"--pretty", "--no-color", "project", "feature", "status", "rtc", "demo"}, want: []string{"rtc", "demo"}},
		{name: "project feature enable", args: []string{"--pretty", "--no-color", "project", "feature", "enable", "rtm", "demo"}, want: []string{"rtm", "demo"}},
		{name: "project doctor", args: []string{"--pretty", "--no-color", "project", "doctor", "demo", "--feature", "rtc"}, want: []string{"rtc", "demo", "Summary"}},
		{name: "webhook update", args: []string{"--pretty", "--no-color", "project", "webhook", "update", "42", "--project", "demo", "--feature", "rtc", "--url", "https://example.com/updated"}, want: []string{"42", "example.com/updated"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runCLI(t, tt.args, cliRunOptions{env: env})
			if result.exitCode != 0 {
				t.Fatalf("exit = %d, stdout = %s, stderr = %s", result.exitCode, result.stdout, result.stderr)
			}
			for _, want := range tt.want {
				if !strings.Contains(strings.ToLower(result.stdout), strings.ToLower(want)) {
					t.Errorf("output does not contain %q: %s", want, result.stdout)
				}
			}
		})
	}

	deleted := runCLI(t, []string{"--pretty", "--no-color", "project", "webhook", "delete", "42", "--project", "demo", "--feature", "rtc", "--yes"}, cliRunOptions{env: env})
	if deleted.exitCode != 0 || !strings.Contains(strings.ToLower(deleted.stdout), "deleted") {
		t.Fatalf("webhook delete = exit %d, stdout %s, stderr %s", deleted.exitCode, deleted.stdout, deleted.stderr)
	}

	created := runCLI(t, []string{"--pretty", "--no-color", "project", "create", "release-coverage"}, cliRunOptions{env: env})
	if created.exitCode != 0 || !strings.Contains(created.stdout, "release-coverage") {
		t.Fatalf("project create = exit %d, stdout %s, stderr %s", created.exitCode, created.stdout, created.stderr)
	}
}

func TestConfigPublicCommands(t *testing.T) {
	env := map[string]string{
		"AGORA_LOG_LEVEL": "error",
		"XDG_CONFIG_HOME": t.TempDir(),
	}

	pathResult := runCLI(t, []string{"config", "path", "--json"}, cliRunOptions{env: env})
	if pathResult.exitCode != 0 || !strings.Contains(pathResult.stdout, `"command":"config path"`) || !strings.Contains(pathResult.stdout, "config.json") {
		t.Fatalf("config path = exit %d, stdout %s, stderr %s", pathResult.exitCode, pathResult.stdout, pathResult.stderr)
	}

	prettyGet := runCLI(t, []string{"--pretty", "--no-color", "config", "get"}, cliRunOptions{env: env})
	if prettyGet.exitCode != 0 || !strings.Contains(prettyGet.stdout, "config get") {
		t.Fatalf("config get = exit %d, stdout %s, stderr %s", prettyGet.exitCode, prettyGet.stdout, prettyGet.stderr)
	}

	update := runCLI(t, []string{"config", "update", "--log-level", "debug", "--browser-auto-open=false", "--telemetry-enabled=false", "--json"}, cliRunOptions{env: env})
	if update.exitCode != 0 || !strings.Contains(update.stdout, `"command":"config update"`) {
		t.Fatalf("config update = exit %d, stdout %s, stderr %s", update.exitCode, update.stdout, update.stderr)
	}

	jsonGet := runCLI(t, []string{"config", "get", "--json"}, cliRunOptions{env: env})
	for _, want := range []string{`"command":"config get"`, `"logLevel":"debug"`, `"browserAutoOpen":false`, `"telemetryEnabled":false`} {
		if !strings.Contains(jsonGet.stdout, want) {
			t.Errorf("config get output does not contain %q: %s", want, jsonGet.stdout)
		}
	}
}

func TestInstallDoctorPublicFailureOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	emptyHome := t.TempDir()
	result := runCLI(t, []string{"doctor", "--json"}, cliRunOptions{env: map[string]string{
		"AGORA_API_BASE_URL":   server.URL,
		"AGORA_LOG_LEVEL":      "error",
		"AGORA_OAUTH_BASE_URL": server.URL,
		"HOME":                 emptyHome,
		"PATH":                 filepath.Join(emptyHome, "bin"),
		"XDG_CONFIG_HOME":      filepath.Join(emptyHome, "config"),
	}})
	if result.exitCode == 0 {
		t.Fatalf("doctor failure fixture unexpectedly succeeded: %s", result.stdout)
	}
	for _, want := range []string{`"command":"doctor"`, `"status":"fail"`, `"category":"network"`, `"category":"auth"`} {
		if !strings.Contains(result.stdout, want) {
			t.Errorf("doctor failure output does not contain %q: %s", want, result.stdout)
		}
	}
}

func TestUpgradeCheckUsesReleaseFixture(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/releases/latest") {
			_, _ = io.WriteString(w, `{"tag_name":"v99.0.0"}`)
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(server.Close)
	executable, err := os.Executable()
	if err != nil {
		t.Fatalf("os.Executable() error = %v", err)
	}
	if resolved, resolveErr := filepath.EvalSymlinks(executable); resolveErr == nil {
		executable = resolved
	}
	receiptPath, err := writeInstallReceipt(executable, "0.0.0", "test fixture")
	if err != nil {
		t.Fatalf("writeInstallReceipt() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Remove(receiptPath) })

	result := runCLI(t, []string{"upgrade", "--check", "--json"}, cliRunOptions{env: map[string]string{
		"AGORA_LOG_LEVEL": "error",
		"GITHUB_API_URL":  server.URL,
		"XDG_CONFIG_HOME": t.TempDir(),
	}})
	if result.exitCode != 0 {
		t.Fatalf("upgrade --check exit = %d, stdout = %s, stderr = %s", result.exitCode, result.stdout, result.stderr)
	}
	for _, want := range []string{`"command":"upgrade"`, `"latestVersion":"99.0.0"`} {
		if !strings.Contains(result.stdout, want) {
			t.Errorf("upgrade check output does not contain %q: %s", want, result.stdout)
		}
	}
}
