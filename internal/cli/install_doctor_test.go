package cli

import (
	"runtime"
	"strings"
	"testing"
)

// TestPathFixSuggestionShellAware proves the doctor returns the exact
// shell-rc command the user can paste to fix a missing PATH entry, per
// detected $SHELL. Mirrors install.sh's shell_rc_for_path /
// shell_path_line so the doctor's advice matches what a fresh
// installer run would do automatically.
func TestPathFixSuggestionShellAware(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX shell branches; windows branch is exercised separately")
	}
	const installDir = "/Users/dev/.local/bin"

	cases := []struct {
		name     string
		shell    string
		mustHave []string
	}{
		{
			name:  "zsh writes to ~/.zshrc and sources it",
			shell: "/bin/zsh",
			mustHave: []string{
				`export PATH="` + installDir + `:$PATH"`,
				"~/.zshrc",
				"source ~/.zshrc",
			},
		},
		{
			name:  "bash writes to ~/.bashrc and sources it",
			shell: "/usr/local/bin/bash",
			mustHave: []string{
				`export PATH="` + installDir + `:$PATH"`,
				"~/.bashrc",
				"source ~/.bashrc",
			},
		},
		{
			name:  "fish uses fish_add_path",
			shell: "/opt/homebrew/bin/fish",
			mustHave: []string{
				"fish_add_path " + installDir,
			},
		},
		{
			name:  "unknown shell falls back to ~/.profile and is still actionable",
			shell: "/bin/ksh",
			mustHave: []string{
				installDir,
				"~/.profile",
			},
		},
		{
			name:  "empty shell still emits a copy-pastable command",
			shell: "",
			mustHave: []string{
				installDir,
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			env := map[string]string{"SHELL": tc.shell}
			got := pathFixSuggestion(installDir, env)
			if got == "" {
				t.Fatal("expected non-empty suggestion")
			}
			for _, must := range tc.mustHave {
				if !strings.Contains(got, must) {
					t.Errorf("expected suggestion to contain %q, got %q", must, got)
				}
			}
		})
	}
}

// TestPathFixSuggestionEmptyInstallDirFallsBackToInstallerHint covers
// the case where os.Executable() failed and we have no install dir to
// suggest. The suggestion must still tell the user something actionable
// (re-run the installer) rather than echoing an empty path.
func TestPathFixSuggestionEmptyInstallDirFallsBackToInstallerHint(t *testing.T) {
	got := pathFixSuggestion("", map[string]string{"SHELL": "/bin/zsh"})
	if !strings.Contains(strings.ToLower(got), "installer") {
		t.Fatalf("expected fallback to mention re-running the installer, got %q", got)
	}
	if strings.Contains(got, `""`) || strings.Contains(got, ":$PATH") {
		t.Fatalf("expected no half-built export line when install dir is empty, got %q", got)
	}
}
