package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTelemetryConsentLegacyAndInvalidConfig(t *testing.T) {
	for _, raw := range []string{
		`{"version":4,"telemetryEnabled":true}`,
		`{"version":4,"telemetryEnabled":false}`,
		`{"version":1,"telemetryEnabled":true}`,
		`{"version":1,"telemetryEnabled":false}`,
		`{"version":4,"telemetryEnabled":true,"telemetryConsent":"invalid"}`,
		`{"version":4,"telemetryEnabled":true,"telemetryConsent":true}`,
	} {
		t.Run(raw, func(t *testing.T) {
			server, read := analyticsRecorder(t)
			a := &App{env: map[string]string{"AGORA_HOME": t.TempDir(), "AGORA_TELEMETRY_URL": server.URL}, osEnv: map[string]string{}}
			path, err := resolveConfigFilePath(a.env)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
				t.Fatal(err)
			}
			state, err := ensureAppConfigState(a.env)
			if err != nil {
				t.Fatal(err)
			}
			a.cfg, a.cfgState = state.Config, state
			if a.cfg.TelemetryConsent != telemetryConsentUnknown {
				t.Fatal("legacy config inferred consent")
			}
			if strings.Contains(raw, `"telemetryEnabled":false`) && a.cfg.TelemetryEnabled {
				t.Fatal("legacy opt-out lost")
			}
			for _, channel := range []string{"shell", "mcp"} {
				if op := a.beginAnalytics("init", channel, "browser", "session"); op != nil {
					op.finish(nil)
					t.Fatal("capture started without consent")
				}
			}
			if len(read()) != 0 {
				t.Fatal("sent without consent")
			}
			if _, err := os.Stat(filepath.Join(a.env["AGORA_HOME"], "analytics-id")); !os.IsNotExist(err) {
				t.Fatal("created identity without consent")
			}
		})
	}
}

func TestTelemetryConsentPromptPersistence(t *testing.T) {
	for _, tc := range []struct {
		answer, want string
		wantError    bool
	}{
		{"yes\n", telemetryConsentGranted, false},
		{"Y\n", telemetryConsentGranted, false},
		{"\n", telemetryConsentDenied, false},
		{"no\n", telemetryConsentDenied, false},
		{"invalid\ny\n", telemetryConsentGranted, false},
		{"", telemetryConsentUnknown, true},
		{"yes", telemetryConsentUnknown, true},
	} {
		t.Run(tc.answer, func(t *testing.T) {
			server, read := analyticsRecorder(t)
			a := &App{env: map[string]string{"AGORA_HOME": t.TempDir(), "AGORA_TELEMETRY_URL": server.URL}, osEnv: map[string]string{}, cfg: defaultConfig()}
			var out bytes.Buffer
			input := strings.NewReader(tc.answer)
			if err := a.promptTelemetryConsent(input, &out); (err != nil) != tc.wantError {
				t.Fatalf("prompt error: %v", err)
			}
			if !strings.Contains(out.String(), "[y/N]") || !strings.Contains(out.String(), "optional webpage source IDs") {
				t.Fatal("missing disclosure/default-no")
			}
			if len(read()) != 0 {
				t.Fatal("prompt itself sent telemetry")
			}
			if a.currentTelemetryConsent() != tc.want {
				t.Fatalf("consent: %s", a.currentTelemetryConsent())
			}
			if !tc.wantError {
				state, err := ensureAppConfigState(a.env)
				if err != nil {
					t.Fatal(err)
				}
				a.cfg, a.cfgState = state.Config, state
				if a.cfg.TelemetryConsent != tc.want {
					t.Fatal("choice did not survive reload")
				}
			}
			op := a.beginAnalytics("recipes list", "shell", "", "")
			if tc.want == telemetryConsentGranted {
				if op == nil {
					t.Fatal("consent did not enable capture")
				}
				op.finish(nil)
				if len(waitAnalyticsEvents(t, read, 1)) != 1 {
					t.Fatal("missing consented capture")
				}
			} else if op != nil {
				op.finish(nil)
				t.Fatal("captured after rejection/EOF")
			}
		})
	}
}

func TestTelemetryConsentPromptEligibility(t *testing.T) {
	for _, reason := range []string{"interactive", "json", "ci", "yes", "no-input", "quiet", "no-stdin-tty", "no-stderr-tty", "disabled", "denied", "granted", "dnt", "env-disabled", "sentry-disabled", "no-endpoint"} {
		t.Run(reason, func(t *testing.T) {
			a := &App{env: map[string]string{"AGORA_HOME": t.TempDir(), "AGORA_TELEMETRY_URL": "https://collector.example/events"}, osEnv: map[string]string{}, cfg: defaultConfig()}
			stdinTTY, stderrTTY := true, true
			switch reason {
			case "json":
				a.cfg.Output = outputJSON
			case "ci":
				a.osEnv["CI"] = "1"
			case "yes":
				a.rootYes = true
			case "no-input":
				a.env["AGORA_NO_INPUT"] = "1"
			case "quiet":
				a.rootQuiet = true
			case "no-stdin-tty":
				stdinTTY = false
			case "no-stderr-tty":
				stderrTTY = false
			case "disabled":
				a.cfg.TelemetryEnabled = false
			case "denied":
				a.cfg.TelemetryConsent = telemetryConsentDenied
			case "granted":
				a.cfg.TelemetryConsent = telemetryConsentGranted
			case "dnt":
				a.env["DO_NOT_TRACK"] = "1"
			case "env-disabled":
				a.env["AGORA_TELEMETRY_ENABLED"] = "0"
			case "sentry-disabled":
				a.env["AGORA_SENTRY_ENABLED"] = "0"
			case "no-endpoint":
				delete(a.env, "AGORA_TELEMETRY_URL")
			}
			a.root = a.buildRoot()
			// Cobra binds these fields to defaults while building the root.
			a.rootYes = reason == "yes"
			a.rootQuiet = reason == "quiet"
			if got := a.shouldPromptTelemetryConsent(a.root, stdinTTY, stderrTTY); got != (reason == "interactive") {
				t.Fatalf("unexpected prompt: %v", got)
			}
		})
	}
}

func TestTelemetryConsentLiveMCPAndSaveFailure(t *testing.T) {
	server, read := analyticsRecorder(t)
	a := &App{env: map[string]string{"AGORA_HOME": t.TempDir(), "AGORA_TELEMETRY_URL": server.URL}, osEnv: map[string]string{}, cfg: defaultConfig()}
	state, err := ensureAppConfigState(a.env)
	if err != nil {
		t.Fatal(err)
	}
	a.cfg, a.cfgState = state.Config, state
	a.applyConfigToEnv()
	_, _ = a.callMCPTool("agora.init", map[string]any{}, nil)
	if len(read()) != 0 {
		t.Fatal("MCP inferred consent")
	}
	other := &App{env: a.env, cfg: a.cfg}
	if err := other.persistTelemetryChoice(true); err != nil {
		t.Fatal(err)
	}
	_, _ = a.callMCPTool("agora.init", map[string]any{}, nil)
	if len(waitAnalyticsEvents(t, read, 3)) == 0 {
		t.Fatal("live MCP did not observe consent")
	}
	if err := other.persistTelemetryChoice(false); err != nil {
		t.Fatal(err)
	}
	before := len(read())
	_, _ = a.callMCPTool("agora.init", map[string]any{}, nil)
	if len(read()) != before {
		t.Fatal("live MCP ignored withdrawal")
	}
	// An invalid persisted store must not be replaced by an in-memory grant.
	if err := os.WriteFile(state.ConfigFilePath, []byte("{invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := a.promptTelemetryConsent(strings.NewReader("yes\n"), &bytes.Buffer{}); err == nil {
		t.Fatal("save failure was hidden")
	}
	if a.analyticsEnabled() {
		t.Fatal("failed save granted consent")
	}
}

func TestCLITelemetryConsentJSON(t *testing.T) {
	server, read := analyticsRecorder(t)
	env := map[string]string{"AGORA_HOME": t.TempDir(), "AGORA_TELEMETRY_URL": server.URL, "DO_NOT_TRACK": "", "AGORA_SENTRY_ENABLED": "1", "AGORA_TELEMETRY_ENABLED": "1"}
	for _, args := range [][]string{
		{"recipes", "list", "--type", "invalid", "--json", "--yes"},
		{"recipes", "list", "--type", "invalid", "--source-distinct-id", "browser", "--source-session-id", "session", "--json"},
	} {
		result := runCLI(t, args, cliRunOptions{env: env})
		if strings.Contains(result.stderr, "Allow usage collection?") {
			t.Fatal("noninteractive prompt")
		}
	}
	if len(read()) != 0 {
		t.Fatal("CLI/MCP captured without consent")
	}
	for _, args := range [][]string{{"telemetry", "enable", "--json"}, {"config", "update", "--telemetry-enabled=true", "--json"}} {
		result := runCLI(t, args, cliRunOptions{env: env})
		var envelope map[string]any
		if result.exitCode != 0 || json.Unmarshal([]byte(result.stdout), &envelope) != nil {
			t.Fatalf("invalid JSON: %+v", result)
		}
		if !strings.Contains(result.stderr, "No code, conversation content") {
			t.Fatal("explicit enable missing disclosure")
		}
		status := runCLI(t, []string{"telemetry", "status", "--json"}, cliRunOptions{env: env})
		if !strings.Contains(status.stdout, `"consent":"granted"`) || !strings.Contains(status.stdout, `"captureActive":true`) {
			t.Fatalf("wrong status: %+v", status)
		}
		receivedBefore := len(read())
		result = runCLI(t, []string{"recipes", "list", "--type", "invalid", "--json"}, cliRunOptions{env: env})
		if result.exitCode == 0 || len(waitAnalyticsEvents(t, read, receivedBefore+1)) == 0 {
			t.Fatal("explicit consent did not activate capture")
		}
		disabled := runCLI(t, []string{"telemetry", "disable", "--json"}, cliRunOptions{env: env})
		if disabled.exitCode != 0 || !strings.Contains(disabled.stdout, `"consent":"denied"`) {
			t.Fatalf("disable failed: %+v", disabled)
		}
		before := len(read())
		_ = runCLI(t, []string{"recipes", "list", "--type", "invalid", "--json"}, cliRunOptions{env: env})
		if len(read()) != before {
			t.Fatal("disabled CLI still sent")
		}
	}
}
