package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type failingNoticeWriter struct{}

func (failingNoticeWriter) Write([]byte) (int, error) { return 0, errors.New("closed") }

func TestTelemetryNoticeAndPreference(t *testing.T) {
	for _, quiet := range []bool{false, true} {
		a := analyticsTestApp(t, "https://collector.example/events")
		a.rootQuiet = quiet
		path, _ := resolveConfigFilePath(a.env)
		before, _ := os.ReadFile(path)
		var out bytes.Buffer
		a.showTelemetryNotice(&out, a.analyticsEnabled())
		marker := filepath.Join(a.env["AGORA_HOME"], "telemetry-notice-shown")
		_, err := os.Stat(marker)
		if quiet {
			if out.Len() != 0 || !os.IsNotExist(err) {
				t.Fatal("quiet notice was recorded")
			}
			a.rootQuiet = false
			a.showTelemetryNotice(&out, a.analyticsEnabled())
		}
		if !strings.Contains(out.String(), "App IDs") || strings.Contains(out.String(), "[y/N]") {
			t.Fatal("missing disclosure or interactive prompt")
		}
		length := out.Len()
		a.showTelemetryNotice(&out, a.analyticsEnabled())
		if out.Len() != length {
			t.Fatal("notice repeated")
		}
		after, _ := os.ReadFile(path)
		if !bytes.Equal(before, after) {
			t.Fatal("notice overwrote config")
		}
		if err := a.persistTelemetryChoice(false); err != nil {
			t.Fatal(err)
		}
		if a.analyticsEnabled() {
			t.Fatal("disable lost")
		}
	}
}

func TestTelemetryNoticeFailureDoesNotGrantMarker(t *testing.T) {
	a := analyticsTestApp(t, "https://collector.example/events")
	a.showTelemetryNotice(failingNoticeWriter{}, true)
	if _, err := os.Stat(filepath.Join(a.env["AGORA_HOME"], "telemetry-notice-shown")); !os.IsNotExist(err) {
		t.Fatal("failed write marked shown")
	}
	// Printing a notice after another command disables collection never rewrites its preference.
	if err := a.persistTelemetryChoice(false); err != nil {
		t.Fatal(err)
	}
	a.showTelemetryNotice(&bytes.Buffer{}, true)
	cfg, valid := a.currentTelemetryConfig()
	if !valid || cfg.TelemetryEnabled {
		t.Fatal("notice changed saved disable")
	}
}

func TestCLITelemetryDefaultJSONAndExcludedCommands(t *testing.T) {
	server, read := analyticsRecorder(t)
	env := map[string]string{"AGORA_HOME": t.TempDir(), "AGORA_TELEMETRY_URL": server.URL, "DO_NOT_TRACK": "", "AGORA_SENTRY_ENABLED": "1", "AGORA_TELEMETRY_ENABLED": "1"}
	for _, args := range [][]string{{"recipes", "--help"}, {"version", "--json"}, {"project", "create", "demo", "--dry-run", "--json"}, {"completion", "bash"}} {
		result := runCLI(t, args, cliRunOptions{env: env})
		if strings.Contains(result.stderr, "Agora CLI collects") {
			t.Fatalf("excluded notice: %v", args)
		}
	}
	if len(read()) != 0 {
		t.Fatal("excluded commands captured")
	}
	result := runCLI(t, []string{"recipes", "list", "--type", "invalid", "--json"}, cliRunOptions{env: env})
	if !strings.Contains(result.stderr, "Agora CLI collects") || strings.Contains(result.stdout, "Agora CLI collects") {
		t.Fatal("wrong notice stream")
	}
	waitAnalyticsEvents(t, read, 1)
	env["DO_NOT_TRACK"] = "1"
	for _, action := range []string{"status", "enable"} {
		result := runCLI(t, []string{"telemetry", action, "--json"}, cliRunOptions{env: env})
		if result.exitCode != 0 || !strings.Contains(result.stdout, `"enabled":false`) ||
			!strings.Contains(result.stdout, `"savedPreference":true`) ||
			!strings.Contains(result.stdout, `"captureActive":false`) {
			t.Fatalf("unexpected telemetry %s result: %+v", action, result)
		}
	}
	disabled := runCLI(t, []string{"telemetry", "disable", "--json"}, cliRunOptions{env: env})
	if strings.Contains(disabled.stderr, "Agora CLI collects") {
		t.Fatal("disable notice")
	}
}

func TestMCPTelemetryDisclosureDoesNotMarkShellNoticeShown(t *testing.T) {
	a := analyticsTestApp(t, "https://collector.example/events")
	cmd := a.buildMCPCommand()
	cmd.SetArgs([]string{"serve"})
	cmd.SetIn(strings.NewReader("{\"jsonrpc\":\"2.0\",\"id\":1,\"method\":\"initialize\"}\n"))
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}

	var response struct {
		Result struct {
			Instructions string `json:"instructions"`
		} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	if response.Result.Instructions != telemetryDisclosure || stderr.Len() != 0 {
		t.Fatalf("unexpected MCP disclosure: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
	if _, err := os.Stat(filepath.Join(a.env["AGORA_HOME"], "telemetry-notice-shown")); !os.IsNotExist(err) {
		t.Fatalf("MCP wrote the shell notice marker: %v", err)
	}
}

func TestMCPTelemetryStatusReportsEffectiveAndSavedPreference(t *testing.T) {
	for _, optOut := range []bool{false, true} {
		a := analyticsTestApp(t, "https://collector.example/events")
		if optOut {
			a.env["DO_NOT_TRACK"] = "1"
		}
		result, err := a.callMCPTool("agora.telemetry.status", nil, nil)
		if err != nil {
			t.Fatal(err)
		}
		status := result.(map[string]any)
		if status["savedPreference"] != true || status["enabled"] != !optOut || status["captureActive"] != !optOut || status["transportConfigured"] != true || status["doNotTrack"] != optOut {
			t.Fatalf("optOut=%v: unexpected MCP telemetry status %v", optOut, status)
		}
	}
}

func TestCNTelemetryMakesNoShellOrMCPRequests(t *testing.T) {
	collector, read := analyticsRecorder(t)
	env := map[string]string{
		"AGORA_HOME": t.TempDir(), "AGORA_TELEMETRY_URL": collector.URL,
		"DO_NOT_TRACK": "", "AGORA_SENTRY_ENABLED": "1", "AGORA_TELEMETRY_ENABLED": "1",
	}
	if err := saveContext(env, projectContext{CurrentRegion: regionCN}); err != nil {
		t.Fatal(err)
	}
	shell := runCLI(t, []string{"recipes", "list", "--type", "invalid", "--json"}, cliRunOptions{env: env})
	if !strings.Contains(shell.stdout, "RECIPE_TYPE_INVALID") || strings.Contains(shell.stderr, telemetryDisclosure) {
		t.Fatalf("unexpected CN shell result: %+v", shell)
	}
	a := analyticsTestApp(t, collector.URL)
	a.env["AGORA_HOME"] = env["AGORA_HOME"]
	initialize, err := a.handleMCPRequest(mcpRequest{Method: "initialize"}, nil)
	if err != nil || initialize.(map[string]any)["instructions"] != nil {
		t.Fatalf("unexpected CN MCP initialization: %v, %v", initialize, err)
	}
	status, err := a.callMCPTool("agora.telemetry.status", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	fields := status.(map[string]any)
	if fields["savedPreference"] != true || fields["enabled"] != false || fields["captureActive"] != false || fields["transportConfigured"] != true {
		t.Fatalf("unexpected CN MCP status: %v", fields)
	}
	if _, err := a.callMCPTool("agora.recipes.list", map[string]any{"type": "invalid"}, nil); err == nil {
		t.Fatal("expected the MCP command to validate its input")
	}
	if _, err := os.Stat(filepath.Join(env["AGORA_HOME"], "analytics-id")); !os.IsNotExist(err) {
		t.Fatalf("CN created a telemetry identity: %v", err)
	}
	// A long-lived MCP server must also honor a change in the active region.
	for _, channel := range []string{"shell", "mcp"} {
		if err := saveContext(a.env, projectContext{CurrentRegion: regionGlobal}); err != nil {
			t.Fatal(err)
		}
		op := a.beginAnalytics("recipes list", channel)
		if op == nil {
			t.Fatal("expected capture in Global")
		}
		if err := saveContext(a.env, projectContext{CurrentRegion: regionCN}); err != nil {
			t.Fatal(err)
		}
		op.finish(a, nil)
	}
	if len(read()) != 0 {
		t.Fatal("CN sent telemetry")
	}
}
