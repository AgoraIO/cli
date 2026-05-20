package cli

// Integration tests for help / discovery / agentic surfaces.
//
// Shared helpers (TestCLIHelperProcess, runCLI, fakeOAuthServer, fakeCLIBFF, ...)
// live in integration_test.go.

import (
	"strings"
	"testing"
)

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
	upgradeCheck := runCLI(t, []string{"--upgrade-check", "--json"}, cliRunOptions{env: map[string]string{"AGORA_HOME": t.TempDir()}})
	if upgradeCheck.exitCode != 0 || !strings.Contains(upgradeCheck.stdout, `"command":"upgrade check"`) || !strings.Contains(upgradeCheck.stdout, `"action":"upgrade-check"`) || strings.Contains(upgradeCheck.stdout, "AgoraIO-Community") {
		t.Fatalf("unexpected upgrade check output: %+v", upgradeCheck)
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

	unauthProject := runCLI(t, []string{"project", "list", "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME": t.TempDir(),
			"AGORA_LOG_LEVEL": "error",
		},
	})
	if unauthProject.exitCode != 3 || !strings.Contains(unauthProject.stdout, `"code":"AUTH_UNAUTHENTICATED"`) || !strings.Contains(unauthProject.stdout, `"exitCode":3`) {
		t.Fatalf("expected structured unauthenticated project error, got %+v", unauthProject)
	}
}

func TestCLIAgenticSurfaces(t *testing.T) {
	configHome := t.TempDir()
	versionResult := runCLI(t, []string{"version", "--json", "--pretty"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME": configHome,
		"AGORA_LOG_LEVEL": "error",
	}})
	if versionResult.exitCode != 0 || !strings.Contains(versionResult.stdout, "\n  \"ok\": true") || !strings.Contains(versionResult.stdout, `"command": "version"`) {
		t.Fatalf("unexpected version result: %+v", versionResult)
	}

	introspect := runCLI(t, []string{"introspect", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME": configHome,
		"AGORA_LOG_LEVEL": "error",
	}})
	if introspect.exitCode != 0 || !strings.Contains(introspect.stdout, `"command":"introspect"`) || !strings.Contains(introspect.stdout, `"features":["rtc","rtm","convoai"]`) {
		t.Fatalf("unexpected introspect result: %+v", introspect)
	}

	telemetry := runCLI(t, []string{"telemetry", "disable", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME": configHome,
		"AGORA_LOG_LEVEL": "error",
	}})
	if telemetry.exitCode != 0 || !strings.Contains(telemetry.stdout, `"command":"telemetry"`) || !strings.Contains(telemetry.stdout, `"enabled":false`) {
		t.Fatalf("unexpected telemetry result: %+v", telemetry)
	}

	upgrade := runCLI(t, []string{"upgrade", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME": configHome,
		"AGORA_LOG_LEVEL": "error",
	}})
	if upgrade.exitCode != 0 || !strings.Contains(upgrade.stdout, `"command":"upgrade"`) || !strings.Contains(upgrade.stdout, `"installMethod"`) {
		t.Fatalf("unexpected upgrade result: %+v", upgrade)
	}
}

func TestCLIOpenRejectsConflictingBrowserFlags(t *testing.T) {
	result := runCLI(t, []string{"open", "--target", "docs", "--browser", "--no-browser", "--json"}, cliRunOptions{
		env: map[string]string{
			"AGORA_HOME":      t.TempDir(),
			"AGORA_LOG_LEVEL": "error",
		},
	})
	if result.exitCode != 1 || !strings.Contains(result.stdout, `"ok":false`) || !strings.Contains(result.stdout, "choose only one of --browser or --no-browser") {
		t.Fatalf("unexpected open conflicting flag result: %+v", result)
	}
}
