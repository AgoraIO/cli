package cli

import (
	"net/http"
	"net/http/httptest"
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

func TestInstallDoctorNetworkEndpointsFollowCurrentRegion(t *testing.T) {
	t.Run("global uses global endpoints", func(t *testing.T) {
		app := &App{
			cfg: defaultConfig(),
			env: map[string]string{"XDG_CONFIG_HOME": t.TempDir()},
		}
		endpoints := app.installDoctorNetworkEndpoints()
		if len(endpoints) != 2 {
			t.Fatalf("expected 2 endpoints, got %+v", endpoints)
		}
		if endpoints[0].url != apiBaseURL {
			t.Fatalf("expected global api endpoint, got %q", endpoints[0].url)
		}
		if endpoints[1].url != oauthBaseURL {
			t.Fatalf("expected global oauth endpoint, got %q", endpoints[1].url)
		}
	})

	t.Run("cn uses cn endpoints", func(t *testing.T) {
		env := map[string]string{"XDG_CONFIG_HOME": t.TempDir()}
		if err := saveContext(env, projectContext{CurrentRegion: regionCN}); err != nil {
			t.Fatal(err)
		}
		app := &App{
			cfg: defaultConfig(),
			env: env,
		}
		endpoints := app.installDoctorNetworkEndpoints()
		if len(endpoints) != 2 {
			t.Fatalf("expected 2 endpoints, got %+v", endpoints)
		}
		if endpoints[0].url != apiBaseURLCN {
			t.Fatalf("expected cn api endpoint, got %q", endpoints[0].url)
		}
		if endpoints[1].url != oauthBaseURLCN {
			t.Fatalf("expected cn oauth endpoint, got %q", endpoints[1].url)
		}
	})
}

func TestInstallDoctorPublicJSONAndPrettyOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	baseEnv := map[string]string{
		"AGORA_API_BASE_URL":   server.URL,
		"AGORA_LOG_LEVEL":      "error",
		"AGORA_OAUTH_BASE_URL": server.URL,
	}

	jsonEnv := cloneDoctorEnv(baseEnv)
	jsonEnv["XDG_CONFIG_HOME"] = t.TempDir()
	persistSessionForIntegration(t, jsonEnv["XDG_CONFIG_HOME"])
	jsonResult := runCLI(t, []string{"doctor", "--json"}, cliRunOptions{env: jsonEnv})
	for _, want := range []string{`"command":"doctor"`, `"checks"`, `"category":"network"`, `"category":"auth"`} {
		if !strings.Contains(jsonResult.stdout, want) {
			t.Errorf("doctor JSON output does not contain %q: %s", want, jsonResult.stdout)
		}
	}

	prettyEnv := cloneDoctorEnv(baseEnv)
	prettyEnv["XDG_CONFIG_HOME"] = t.TempDir()
	prettyResult := runCLI(t, []string{"--pretty", "--no-color", "doctor"}, cliRunOptions{env: prettyEnv})
	for _, want := range []string{"Install", "Network", "Auth", "Summary"} {
		if !strings.Contains(prettyResult.stdout, want) {
			t.Errorf("pretty doctor output does not contain %q: %s", want, prettyResult.stdout)
		}
	}
}

func cloneDoctorEnv(input map[string]string) map[string]string {
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}
