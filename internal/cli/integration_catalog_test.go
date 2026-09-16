package cli

import (
	"strings"
	"testing"
)

func TestEnvHelpPublicOutputModes(t *testing.T) {
	jsonResult := runCLI(t, []string{"env-help", "--json"}, cliRunOptions{})
	if jsonResult.exitCode != 0 {
		t.Fatalf("env-help --json exit = %d, stderr = %s", jsonResult.exitCode, jsonResult.stderr)
	}
	for _, want := range []string{`"command":"env-help"`, `"name":"AGORA_OUTPUT"`, `"category":"output"`} {
		if !strings.Contains(jsonResult.stdout, want) {
			t.Errorf("env-help JSON output does not contain %q: %s", want, jsonResult.stdout)
		}
	}

	prettyResult := runCLI(t, []string{"--pretty", "--no-color", "env-help"}, cliRunOptions{})
	if prettyResult.exitCode != 0 {
		t.Fatalf("pretty env-help exit = %d, stderr = %s", prettyResult.exitCode, prettyResult.stderr)
	}
	for _, want := range []string{"Agora CLI environment variables", "[OUTPUT]", "AGORA_OUTPUT"} {
		if !strings.Contains(prettyResult.stdout, want) {
			t.Errorf("pretty env-help output does not contain %q: %s", want, prettyResult.stdout)
		}
	}
}

func TestSkillsPublicCommands(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want []string
	}{
		{
			name: "filtered JSON list",
			args: []string{"skills", "list", "--category", "scaffold", "--json"},
			want: []string{`"command":"skills list"`, `"category":"scaffold"`, `"items"`},
		},
		{
			name: "pretty list",
			args: []string{"--pretty", "--no-color", "skills", "list", "--tag", "convoai"},
			want: []string{"Skills (", "convoai"},
		},
		{
			name: "pretty show",
			args: []string{"--pretty", "--no-color", "skills", "show", "create-python-voice-agent"},
			want: []string{"create-python-voice-agent", "Steps", "Next Steps"},
		},
		{
			name: "pretty search",
			args: []string{"--pretty", "--no-color", "skills", "search", "voice"},
			want: []string{"Skills matching", "voice"},
		},
		{
			name: "empty search",
			args: []string{"--pretty", "--no-color", "skills", "search", "no-such-skill-query"},
			want: []string{"No skills matched"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runCLI(t, tt.args, cliRunOptions{})
			if result.exitCode != 0 {
				t.Fatalf("exit = %d, stderr = %s", result.exitCode, result.stderr)
			}
			for _, want := range tt.want {
				if !strings.Contains(result.stdout, want) {
					t.Errorf("output does not contain %q: %s", want, result.stdout)
				}
			}
		})
	}
}

func TestSkillsPublicErrors(t *testing.T) {
	tests := []struct {
		name string
		args []string
		code string
	}{
		{name: "missing show ID", args: []string{"skills", "show", "--json"}, code: "skill id is required"},
		{name: "unknown skill", args: []string{"skills", "show", "no-such-skill", "--json"}, code: "SKILL_NOT_FOUND"},
		{name: "missing search query", args: []string{"skills", "search", "--json"}, code: "search query is required"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runCLI(t, tt.args, cliRunOptions{})
			if result.exitCode == 0 {
				t.Fatalf("expected failure, stdout = %s", result.stdout)
			}
			if !strings.Contains(result.stdout, tt.code) {
				t.Fatalf("output does not contain error marker %q: %s", tt.code, result.stdout)
			}
		})
	}
}

func TestPublicShellCompletions(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want string
	}{
		{name: "skill ID", args: []string{"__complete", "skills", "show", "create-n"}, want: "create-nextjs"},
		{name: "skill category", args: []string{"__complete", "skills", "list", "--category", "s"}, want: "scaffold"},
		{name: "skill tag", args: []string{"__complete", "skills", "list", "--tag", "conv"}, want: "convoai"},
		{name: "quickstart template", args: []string{"__complete", "quickstart", "create", "demo", "--template", "py"}, want: "python"},
		{name: "feature", args: []string{"__complete", "project", "feature", "status", "r"}, want: "rtc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := runCLI(t, tt.args, cliRunOptions{})
			if result.exitCode != 0 {
				t.Fatalf("completion exit = %d, stderr = %s", result.exitCode, result.stderr)
			}
			if !strings.Contains(result.stdout, tt.want) {
				t.Fatalf("completion output does not contain %q: %s", tt.want, result.stdout)
			}
		})
	}
}
