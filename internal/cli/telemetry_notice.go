package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

const telemetryDisclosure = `Agora CLI collects usage statistics by default when a collector is configured:
recipes/init/project create results, stages, duration, version, OS/architecture,
public recipe/template IDs, Agora App IDs and coarse AI-tool categories.
Events use a random installation ID with an installation profile in PostHog;
Agora App IDs can be associated with Agora projects and companies.
No code, conversation content, secrets, project names or local paths are collected.
Details: https://agoraio.github.io/cli/telemetry.html
Disable: agora telemetry disable, DO_NOT_TRACK=1 or AGORA_TELEMETRY_ENABLED=0`

// telemetryAggregateDisclosure is shown for aggregate-mode regions.
const telemetryAggregateDisclosure = `Agora CLI collects aggregate usage statistics for this region:
recipes/init/project create results, stages, duration, version, OS/architecture,
public recipe/template IDs, coarse AI-tool categories and region.
No installation ID or Agora App ID is sent; events are not linked across commands.
No code, conversation content, secrets, project names or local paths are collected.
Details: https://agoraio.github.io/cli/telemetry.html
Disable: agora telemetry disable, DO_NOT_TRACK=1 or AGORA_TELEMETRY_ENABLED=0`

// telemetryDisclosureFor returns the disclosure for a capture mode, or ""
// when nothing is captured.
func telemetryDisclosureFor(mode string) string {
	switch mode {
	case analyticsModeFull:
		return telemetryDisclosure
	case analyticsModeAggregate:
		return telemetryAggregateDisclosure
	}
	return ""
}

// telemetryNoticeMarkers records which first-use notice was shown. The modes
// use separate markers: the full notice discloses an installation ID and App
// IDs that the aggregate notice does not, so seeing one must not suppress
// the other when a user moves between regions.
var telemetryNoticeMarkers = map[string]string{
	analyticsModeFull:      "telemetry-notice-shown",
	analyticsModeAggregate: "telemetry-notice-aggregate-shown",
}

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

// telemetryStatus is the shared result for `agora telemetry status|enable|disable`
// and the agora.telemetry.status MCP tool. preference is the saved
// telemetryEnabled value the caller considers current: the startup snapshot
// (already updated by enable/disable) for shell commands, or the re-read config
// for MCP. enabled is the effective value after runtime opt-outs;
// savedPreference is the saved value itself. region, transportConfigured and
// captureMode describe the region an operation starting now would use.
func (a *App) telemetryStatus(action, configPath string, preference bool) map[string]any {
	region, mode, endpoint := a.analyticsCapture(preference)
	return map[string]any{
		"action":              action,
		"region":              region,
		"captureMode":         mode,
		"transportConfigured": endpoint != "",
		"captureActive":       mode != analyticsModeOff,
		"configPath":          configPath,
		"doNotTrack":          strings.TrimSpace(a.env["DO_NOT_TRACK"]) != "",
		"enabled":             a.telemetryAllowed(preference),
		"savedPreference":     preference,
	}
}

// maybeShowTelemetryNotice shows the first-use notice for the mode a shell
// operation starting now would use.
func (a *App) maybeShowTelemetryNotice(cmd *cobra.Command) {
	_, mode, _ := a.analyticsCapture(a.cfg.TelemetryEnabled)
	a.showTelemetryNotice(cmd.ErrOrStderr(), mode)
}

func (a *App) showTelemetryNotice(out io.Writer, mode string) {
	text, marker := telemetryDisclosureFor(mode), telemetryNoticeMarkers[mode]
	if text == "" || a.rootQuiet {
		return
	}
	dir, err := resolveAgoraDirectory(a.env)
	if err != nil {
		return
	}
	path := filepath.Join(dir, marker)
	if _, err := os.Stat(path); err == nil {
		return
	}
	if _, err := fmt.Fprintln(out, text); err != nil {
		return
	}
	if os.MkdirAll(dir, 0o700) != nil {
		return
	}
	// Never rewrite config.json merely to record that the notice was printed.
	_ = os.WriteFile(path, nil, 0o600)
}
