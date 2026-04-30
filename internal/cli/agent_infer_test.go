package cli

import "testing"

func TestAgentLabelFromOSEnv(t *testing.T) {
	tests := []struct {
		name string
		env  map[string]string
		want string
	}{
		{name: "explicit wins", env: map[string]string{"AGORA_AGENT": "custom", "CURSOR_AGENT": "1"}, want: "custom"},
		{name: "disabled", env: map[string]string{"AGORA_AGENT_DISABLE_INFER": "1", "CURSOR_AGENT": "1"}, want: ""},
		{name: "cursor", env: map[string]string{"CURSOR_AGENT": "1"}, want: "cursor"},
		{name: "claude code", env: map[string]string{"CLAUDE_CODE": "1"}, want: "claude-code"},
		{name: "cline", env: map[string]string{"CLINE_SESSION": "abc"}, want: "cline"},
		{name: "windsurf", env: map[string]string{"WINDSURF_SESSION": "abc"}, want: "windsurf"},
		{name: "codex", env: map[string]string{"OPENAI_CODEX_SESSION": "abc"}, want: "codex"},
		{name: "aider", env: map[string]string{"AIDER_AUTO_COMMITS": "false"}, want: "aider"},
		{name: "empty", env: map[string]string{}, want: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := agentLabelFromOSEnv(tt.env); got != tt.want {
				t.Fatalf("agentLabelFromOSEnv() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAgoraUserAgentInfersCursorAgent(t *testing.T) {
	got := agoraUserAgent(map[string]string{"CURSOR_AGENT": "1"})
	if got != "agora-cli/"+version+" agent/cursor" {
		t.Fatalf("unexpected user agent: %q", got)
	}
}

// TestDecideShouldPromptForLoginYesFlagDoesNotBypassGuards is the
// regression test for the bug where `--yes` / AGORA_NO_INPUT used to
// short-circuit the JSON/CI/non-TTY guards in shouldPromptForLogin and
// silently launch an OAuth browser flow in CI. The new contract: --yes
// only auto-confirms an *already-interactive* prompt. JSON/CI/non-TTY
// runs always return false and let the caller surface the existing
// AUTH_UNAUTHENTICATED error.
func TestDecideShouldPromptForLoginYesFlagDoesNotBypassGuards(t *testing.T) {
	tests := []struct {
		name       string
		mode       outputMode
		ci         bool
		stdinIsTTY bool
		want       bool
	}{
		{name: "interactive pretty TTY", mode: outputPretty, ci: false, stdinIsTTY: true, want: true},
		{name: "json mode never prompts", mode: outputJSON, ci: false, stdinIsTTY: true, want: false},
		{name: "CI never prompts", mode: outputPretty, ci: true, stdinIsTTY: true, want: false},
		{name: "non-TTY never prompts", mode: outputPretty, ci: false, stdinIsTTY: false, want: false},
		{name: "json and CI", mode: outputJSON, ci: true, stdinIsTTY: false, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := decideShouldPromptForLogin(tt.mode, tt.ci, tt.stdinIsTTY)
			if got != tt.want {
				t.Fatalf("decideShouldPromptForLogin(%v, %v, %v) = %v, want %v",
					tt.mode, tt.ci, tt.stdinIsTTY, got, tt.want)
			}
		})
	}
}

// TestPromptForLoginNoInputDoesNotPromptInJSONMode verifies that a
// non-interactive caller that sets AGORA_NO_INPUT=1 still gets the
// structured AUTH_UNAUTHENTICATED error in JSON mode instead of
// triggering an OAuth flow.
func TestPromptForLoginNoInputDoesNotPromptInJSONMode(t *testing.T) {
	t.Setenv("AGORA_NO_INPUT", "1")
	t.Setenv("AGORA_OUTPUT", "json")
	t.Setenv("CI", "true")
	app := &App{
		env:   map[string]string{"AGORA_NO_INPUT": "1", "AGORA_OUTPUT": "json"},
		osEnv: map[string]string{"CI": "true"},
	}
	err := app.promptForLogin()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var structured *cliError
	if !asCliError(err, &structured) || structured.Code != "AUTH_UNAUTHENTICATED" {
		t.Fatalf("expected AUTH_UNAUTHENTICATED cliError, got %T %v", err, err)
	}
}

func asCliError(err error, target **cliError) bool {
	if err == nil {
		return false
	}
	if c, ok := err.(*cliError); ok {
		*target = c
		return true
	}
	return false
}
