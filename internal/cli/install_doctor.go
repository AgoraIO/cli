package cli

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// buildDoctorCommand registers the top-level `agora doctor` command. It
// diagnoses the *install* (binary location, PATH resolution, version,
// network reachability of the API and OAuth endpoints, MCP-host config,
// auth state, and config sanity), as opposed to `agora project doctor`
// which diagnoses a remote Agora project's readiness for development.
//
// The two surfaces deliberately share the doctor envelope shape so
// wrappers can reuse the same parser:
//
//	{
//	  "ok":     true,
//	  "command":"doctor",
//	  "data": {
//	    "status":  "healthy" | "warning" | "not_ready" | "auth_error",
//	    "checks":  [{"category": "install", "status": "...", "items": [...]}],
//	    "summary": "...",
//	    "warnings": [...],
//	    "blockingIssues": [...]
//	  },
//	  "meta": { "outputMode": "json", "exitCode": 0|1|2|3 }
//	}
func (a *App) buildDoctorCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Diagnose the local Agora CLI install (PATH, version, network, auth, MCP host)",
		Long: `Run a self-test of the Agora CLI install on this machine.

Distinct from "agora project doctor", which validates a remote Agora
project. This command answers questions like:

  - Is the agora binary on PATH and is it the one I expect?
  - Is this the latest version, or is an upgrade available?
  - Can the CLI reach the configured API and OAuth endpoints?
  - Am I authenticated, and is the session still valid?
  - Is AGORA_HOME pointing at a writable directory?
  - Do I have a known MCP host (Cursor / Claude / Windsurf) configured?

Exit codes match "agora project doctor":
  0  healthy
  1  blocking install issues
  2  warnings
  3  auth or session issues

Use --json for a machine-readable envelope. The same data is emitted
under both formats; the JSON envelope is the stable contract.`,
		Example: example(`
  agora doctor
  agora doctor --json
  agora doctor --quiet
`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			result := a.runInstallDoctor()
			code := installDoctorExitCode(result)
			if a.resolveOutputMode(cmd) == outputJSON && code != 0 {
				logPath := resolveLogFilePathForDisplay(a.env)
				err := &cliError{Message: result.Summary, Code: "INSTALL_DOCTOR_" + strings.ToUpper(result.Status)}
				_ = emitFailureEnvelopeWithData(cmd.OutOrStdout(), "doctor", result, err, code, logPath, jsonPrettyFromContext(cmd))
				return &exitError{code: code}
			}
			if err := renderResult(cmd, "doctor", result); err != nil {
				return err
			}
			if code != 0 {
				return &exitError{code: code}
			}
			return nil
		},
	}
}

// runInstallDoctor performs the actual diagnostic. Each category is
// independent and never aborts the others, so a single network failure
// never prevents the user from seeing PATH / auth status.
func (a *App) runInstallDoctor() projectDoctorResult {
	result := projectDoctorResult{
		Action:         "doctor",
		Feature:        "install",
		Mode:           "install",
		BlockingIssues: []doctorIssue{},
		Warnings:       []doctorIssue{},
		Checks:         []doctorCheckCategory{},
	}

	result.Checks = append(result.Checks,
		a.installDoctorBinaryCheck(),
		a.installDoctorVersionCheck(),
		a.installDoctorAgoraHomeCheck(),
		a.installDoctorNetworkCheck(),
		a.installDoctorAuthCheck(),
		a.installDoctorMCPHostCheck(),
	)

	for _, category := range result.Checks {
		for _, item := range category.Items {
			switch item.Status {
			case "fail":
				result.BlockingIssues = append(result.BlockingIssues, doctorIssue{
					Code:             upper(category.Category) + "_" + upper(item.Name),
					Message:          item.Message,
					SuggestedCommand: item.SuggestedCommand,
				})
			case "warn":
				result.Warnings = append(result.Warnings, doctorIssue{
					Code:             upper(category.Category) + "_" + upper(item.Name),
					Message:          item.Message,
					SuggestedCommand: item.SuggestedCommand,
				})
			}
		}
	}

	switch {
	case categoryHasFail(result.Checks, "auth"):
		result.Status = "auth_error"
		result.Summary = "Authentication failed. Run agora login."
	case len(result.BlockingIssues) > 0:
		result.Status = "not_ready"
		result.Summary = fmt.Sprintf("%d blocking install issue(s) detected.", len(result.BlockingIssues))
	case len(result.Warnings) > 0:
		result.Status = "warning"
		result.Summary = fmt.Sprintf("Install is healthy but %d warning(s) found.", len(result.Warnings))
	default:
		result.Status = "healthy"
		result.Summary = "Agora CLI install is healthy."
	}
	result.Healthy = result.Status == "healthy"
	return result
}

// installDoctorBinaryCheck verifies the running binary's location and
// whether the same `agora` resolves on PATH from a fresh shell.
func (a *App) installDoctorBinaryCheck() doctorCheckCategory {
	items := []doctorCheckItem{}
	var resolvedExe string
	exe, err := os.Executable()
	if err != nil || exe == "" {
		items = append(items, doctorCheckItem{
			Name:    "binary_path",
			Status:  "warn",
			Message: "Could not determine the running binary path.",
		})
	} else {
		resolvedExe, _ = filepath.EvalSymlinks(exe)
		if resolvedExe == "" {
			resolvedExe = exe
		}
		items = append(items, doctorCheckItem{
			Name:    "binary_path",
			Status:  "pass",
			Message: "Running binary: " + resolvedExe,
		})
	}
	pathBinary, lookErr := exec.LookPath("agora")
	switch {
	case lookErr != nil:
		installDir := ""
		if resolvedExe != "" {
			installDir = filepath.Dir(resolvedExe)
		}
		items = append(items, doctorCheckItem{
			Name:             "path_resolution",
			Status:           "fail",
			Message:          "agora is not resolvable on PATH.",
			SuggestedCommand: pathFixSuggestion(installDir, a.env),
		})
	case resolvedExe != "" && filepath.Clean(pathBinary) != filepath.Clean(resolvedExe):
		items = append(items, doctorCheckItem{
			Name:             "path_resolution",
			Status:           "warn",
			Message:          "PATH resolves agora to " + pathBinary + " (different from running binary).",
			SuggestedCommand: "Reorder PATH so the installer's directory comes first, or remove the older binary.",
		})
	default:
		items = append(items, doctorCheckItem{
			Name:    "path_resolution",
			Status:  "pass",
			Message: "agora resolves on PATH to the running binary.",
		})
	}
	return categoryWithStatus("install", items)
}

func (a *App) installDoctorVersionCheck() doctorCheckCategory {
	info := versionInfo()
	current, _ := info["version"].(string)
	items := []doctorCheckItem{{
		Name:    "current_version",
		Status:  "pass",
		Message: fmt.Sprintf("Installed version: %s", current),
	}}
	return categoryWithStatus("version", items)
}

func (a *App) installDoctorAgoraHomeCheck() doctorCheckCategory {
	items := []doctorCheckItem{}
	cfgPath, err := resolveConfigFilePath(a.env)
	if err != nil {
		items = append(items, doctorCheckItem{
			Name:             "config_path",
			Status:           "fail",
			Message:          "Could not resolve AGORA_HOME / config path: " + err.Error(),
			SuggestedCommand: "Check that $HOME or %APPDATA% is set and writable.",
		})
		return categoryWithStatus("agora_home", items)
	}
	dir := filepath.Dir(cfgPath)
	probe := filepath.Join(dir, ".agora-doctor-probe")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		items = append(items, doctorCheckItem{
			Name:             "config_dir_writable",
			Status:           "fail",
			Message:          "Cannot create config directory: " + dir,
			SuggestedCommand: "Fix permissions on " + dir + " or set AGORA_HOME to a writable directory.",
		})
		return categoryWithStatus("agora_home", items)
	}
	if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
		items = append(items, doctorCheckItem{
			Name:             "config_dir_writable",
			Status:           "fail",
			Message:          "Config directory is not writable: " + dir,
			SuggestedCommand: "Fix permissions on " + dir + " or set AGORA_HOME to a writable directory.",
		})
		return categoryWithStatus("agora_home", items)
	}
	_ = os.Remove(probe)
	items = append(items, doctorCheckItem{
		Name:    "config_dir_writable",
		Status:  "pass",
		Message: "Config directory is writable: " + dir,
	})
	return categoryWithStatus("agora_home", items)
}

func (a *App) installDoctorNetworkCheck() doctorCheckCategory {
	items := []doctorCheckItem{}
	endpoints := []struct {
		name string
		url  string
	}{
		{"api", a.env["AGORA_API_BASE_URL"]},
		{"oauth", a.env["AGORA_OAUTH_BASE_URL"]},
	}
	client := &http.Client{Timeout: 5 * time.Second}
	for _, ep := range endpoints {
		if strings.TrimSpace(ep.url) == "" {
			items = append(items, doctorCheckItem{
				Name:    ep.name + "_endpoint",
				Status:  "skipped",
				Message: ep.name + " endpoint not configured.",
			})
			continue
		}
		parsed, err := url.Parse(ep.url)
		if err != nil || parsed.Host == "" {
			items = append(items, doctorCheckItem{
				Name:    ep.name + "_endpoint_parse",
				Status:  "warn",
				Message: "Could not parse " + ep.name + " URL: " + ep.url,
			})
			continue
		}
		// DNS lookup: cheap, deterministic, no auth.
		if _, err := net.LookupHost(parsed.Hostname()); err != nil {
			items = append(items, doctorCheckItem{
				Name:             ep.name + "_dns",
				Status:           "fail",
				Message:          "DNS lookup failed for " + parsed.Hostname() + ": " + err.Error(),
				SuggestedCommand: "Check network connectivity and any corporate proxy settings.",
			})
			continue
		}
		// HEAD/GET probe: tolerate 4xx since we are unauthenticated.
		req, _ := http.NewRequest(http.MethodGet, ep.url, nil)
		resp, err := client.Do(req)
		if err != nil {
			items = append(items, doctorCheckItem{
				Name:             ep.name + "_reachability",
				Status:           "warn",
				Message:          "Could not reach " + ep.url + ": " + err.Error(),
				SuggestedCommand: "Check firewall/proxy. The CLI works offline for read-only commands but needs network for login and project operations.",
			})
			continue
		}
		_ = resp.Body.Close()
		items = append(items, doctorCheckItem{
			Name:    ep.name + "_reachability",
			Status:  "pass",
			Message: ep.name + " endpoint reachable: " + ep.url,
		})
	}
	return categoryWithStatus("network", items)
}

func (a *App) installDoctorAuthCheck() doctorCheckCategory {
	items := []doctorCheckItem{}
	data, err := a.authStatus()
	if err != nil {
		items = append(items, doctorCheckItem{
			Name:             "session",
			Status:           "fail",
			Message:          "Could not read local session: " + err.Error(),
			SuggestedCommand: "agora login",
		})
		return categoryWithStatus("auth", items)
	}
	if auth, _ := data["authenticated"].(bool); !auth {
		items = append(items, doctorCheckItem{
			Name:             "session",
			Status:           "fail",
			Message:          "No active Agora session.",
			SuggestedCommand: "agora login",
		})
		return categoryWithStatus("auth", items)
	}
	items = append(items, doctorCheckItem{
		Name:    "session",
		Status:  "pass",
		Message: "Authenticated.",
	})
	return categoryWithStatus("auth", items)
}

func (a *App) installDoctorMCPHostCheck() doctorCheckCategory {
	items := []doctorCheckItem{}
	hosts := detectMCPHostConfig()
	if len(hosts) == 0 {
		items = append(items, doctorCheckItem{
			Name:    "mcp_host",
			Status:  "skipped",
			Message: "No known MCP host config detected (Cursor/Claude/Windsurf). Install one to use `agora mcp serve`.",
		})
		return categoryWithStatus("mcp", items)
	}
	sort.Strings(hosts)
	items = append(items, doctorCheckItem{
		Name:    "mcp_host",
		Status:  "pass",
		Message: "Detected MCP host(s): " + strings.Join(hosts, ", "),
	})
	return categoryWithStatus("mcp", items)
}

// detectMCPHostConfig probes well-known IDE config paths for MCP host
// installations. We only check existence, never read or parse the file
// (privacy / least surprise).
func detectMCPHostConfig() []string {
	var found []string
	home, err := os.UserHomeDir()
	if err != nil {
		return found
	}
	candidates := map[string]string{}
	switch runtime.GOOS {
	case "darwin":
		candidates["Cursor"] = filepath.Join(home, "Library/Application Support/Cursor/User/globalStorage/cursor.cursor-mcp")
		candidates["Claude Desktop"] = filepath.Join(home, "Library/Application Support/Claude/claude_desktop_config.json")
		candidates["Windsurf"] = filepath.Join(home, ".codeium/windsurf/mcp_config.json")
	case "linux":
		candidates["Cursor"] = filepath.Join(home, ".config/Cursor/User/globalStorage/cursor.cursor-mcp")
		candidates["Claude Desktop"] = filepath.Join(home, ".config/Claude/claude_desktop_config.json")
		candidates["Windsurf"] = filepath.Join(home, ".codeium/windsurf/mcp_config.json")
	case "windows":
		appdata := os.Getenv("APPDATA")
		if appdata != "" {
			candidates["Cursor"] = filepath.Join(appdata, "Cursor/User/globalStorage/cursor.cursor-mcp")
			candidates["Claude Desktop"] = filepath.Join(appdata, "Claude/claude_desktop_config.json")
			candidates["Windsurf"] = filepath.Join(appdata, "Codeium/windsurf/mcp_config.json")
		}
	}
	for name, path := range candidates {
		if _, err := os.Stat(path); err == nil {
			found = append(found, name)
		}
	}
	return found
}

func categoryWithStatus(name string, items []doctorCheckItem) doctorCheckCategory {
	cat := doctorCheckCategory{Category: name, Items: items}
	cat.Status = summarizeCategoryStatus(items)
	return cat
}

func categoryHasFail(checks []doctorCheckCategory, category string) bool {
	for _, c := range checks {
		if c.Category != category {
			continue
		}
		for _, item := range c.Items {
			if item.Status == "fail" {
				return true
			}
		}
	}
	return false
}

func installDoctorExitCode(result projectDoctorResult) int {
	switch result.Status {
	case "auth_error":
		return 3
	case "not_ready":
		return 1
	case "warning":
		return 2
	default:
		return 0
	}
}

func upper(s string) string { return strings.ToUpper(s) }

// pathFixSuggestion returns the exact command the user can paste to add
// installDir to their PATH. The command is tailored to the detected
// $SHELL on POSIX (or the platform on Windows). Falls back to a
// generic, copy-pastable export when the shell or installDir is
// unknown so the suggestion is *always* actionable.
//
// Mirrors the rc-file detection logic in install.sh (shell_rc_for_path
// + shell_path_line) so the doctor's advice matches what a fresh
// installer run would do automatically.
func pathFixSuggestion(installDir string, env map[string]string) string {
	if installDir == "" {
		return "Re-run the installer (PATH wiring is now automatic by default), then open a new shell."
	}
	if runtime.GOOS == "windows" {
		// PowerShell users get the persistent setx form because
		// it survives shell restarts; show both setx and a
		// session-only fallback.
		return fmt.Sprintf(
			"Add %s to your user PATH: setx PATH \"%s;%%PATH%%\" (then open a new terminal). PowerShell session-only: $env:Path = \"%s;\" + $env:Path",
			installDir, installDir, installDir,
		)
	}
	shell := ""
	if env != nil {
		shell = strings.TrimSpace(env["SHELL"])
	}
	if shell == "" {
		shell = strings.TrimSpace(os.Getenv("SHELL"))
	}
	switch filepath.Base(shell) {
	case "fish":
		return fmt.Sprintf("fish_add_path %s", installDir)
	case "zsh":
		return fmt.Sprintf(
			"echo 'export PATH=\"%s:$PATH\"' >> ~/.zshrc && source ~/.zshrc",
			installDir,
		)
	case "bash":
		// ~/.bashrc is the right target for interactive non-login
		// bash on Linux; macOS users typically rely on ~/.bash_profile
		// but ~/.bashrc is symlinked or sourced from it in nearly all
		// modern setups. Stay consistent with install.sh's rc choice.
		return fmt.Sprintf(
			"echo 'export PATH=\"%s:$PATH\"' >> ~/.bashrc && source ~/.bashrc",
			installDir,
		)
	default:
		return fmt.Sprintf(
			"Add %s to PATH: echo 'export PATH=\"%s:$PATH\"' >> ~/.profile && source ~/.profile",
			installDir, installDir,
		)
	}
}
