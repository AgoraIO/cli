package cli

import (
	"fmt"
	"github.com/spf13/cobra"
	"io"
	"os"
	"path/filepath"
)

const telemetryDisclosure = `Agora CLI collects usage statistics by default when a collector is configured:
recipes/init/project create results, stages, duration, version, OS/architecture,
public recipe/template IDs, Agora App IDs and coarse AI-tool categories.
Events use a random installation ID; website identities are not linked.
No code, conversation content, secrets, project names or local paths are collected.
Details: https://agoraio.github.io/cli/telemetry.html
Disable: agora telemetry disable, DO_NOT_TRACK=1 or AGORA_TELEMETRY_ENABLED=0`

func (a *App) persistTelemetryChoice(enabled bool) error {
	next, valid := a.currentTelemetryConfig()
	if !valid {
		return fmt.Errorf("cannot read telemetry configuration")
	}
	next.TelemetryEnabled = enabled
	path, err := resolveConfigFilePath(a.env)
	if err != nil {
		return err
	}
	if err := writeSecureJSON(path, next); err != nil {
		return err
	}
	a.cfg = next
	a.applyConfigToEnv()
	return nil
}

func (a *App) maybeShowTelemetryNotice(cmd *cobra.Command) {
	a.showTelemetryNotice(cmd.ErrOrStderr(), a.analyticsEnabled())
}

func (a *App) showTelemetryNotice(out io.Writer, enabled bool) {
	if !enabled || a.rootQuiet || a.analyticsURL() == "" {
		return
	}
	dir, err := resolveAgoraDirectory(a.env)
	if err != nil {
		return
	}
	path := filepath.Join(dir, "telemetry-notice-shown")
	if _, err := os.Stat(path); err == nil {
		return
	}
	if _, err := fmt.Fprintln(out, telemetryDisclosure); err != nil {
		return
	}
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	// Never rewrite config.json merely to record that the notice was printed.
	_ = os.WriteFile(path, nil, 0o600)
}
