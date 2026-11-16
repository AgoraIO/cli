package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"
)

const (
	telemetryConsentUnknown = "unknown"
	telemetryConsentGranted = "granted"
	telemetryConsentDenied  = "denied"
)

const telemetryDisclosure = `Agora CLI can collect usage statistics to improve the product:
recipes/init command results, stages, duration, version, OS/architecture,
public recipe/template IDs and coarse AI-tool categories. Events use a random
installation ID for CLI usage only; website identities are not linked.
No code, conversation content, secrets, project names or local paths are collected.
Details: https://agoraio.github.io/cli/telemetry.html
You can stop collection at any time with: agora telemetry disable`

func normalizeTelemetryConsent(value string) string {
	switch value {
	case telemetryConsentGranted, telemetryConsentDenied:
		return value
	default:
		return telemetryConsentUnknown
	}
}

func (a *App) currentTelemetryConsent() string {
	cfg, valid := a.currentTelemetryConfig()
	if !valid {
		return telemetryConsentUnknown
	}
	return normalizeTelemetryConsent(cfg.TelemetryConsent)
}

func (a *App) persistTelemetryChoice(enabled bool) error {
	next, valid := a.currentTelemetryConfig()
	if !valid {
		return fmt.Errorf("cannot read telemetry configuration")
	}
	next.TelemetryEnabled = enabled
	next.TelemetryConsent = telemetryConsentDenied
	if enabled {
		next.TelemetryConsent = telemetryConsentGranted
	}
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

func (a *App) shouldPromptTelemetryConsent(cmd *cobra.Command, stdinTTY, stderrTTY bool) bool {
	cfg, valid := a.currentTelemetryConfig()
	return valid && cfg.TelemetryEnabled && normalizeTelemetryConsent(cfg.TelemetryConsent) == telemetryConsentUnknown &&
		!a.analyticsOptedOut() && a.analyticsURL() != "" &&
		a.resolveOutputMode(cmd) != outputJSON && !isCIEnvironment(a.osEnv) &&
		!a.noInput() && !a.rootQuiet && stdinTTY && stderrTTY
}

func (a *App) maybePromptTelemetryConsent(cmd *cobra.Command) {
	if !a.shouldPromptTelemetryConsent(cmd, isTTY(os.Stdin), isTTY(os.Stderr)) {
		return
	}
	if err := a.promptTelemetryConsent(cmd.InOrStdin(), cmd.ErrOrStderr()); err != nil {
		fmt.Fprintln(cmd.ErrOrStderr(), "Telemetry consent was not saved; usage collection remains off.")
	}
}

func (a *App) promptTelemetryConsent(in io.Reader, out io.Writer) error {
	if _, err := fmt.Fprintln(out, telemetryDisclosure); err != nil {
		return err
	}
	for {
		if _, err := fmt.Fprint(out, "Allow usage collection? [y/N]: "); err != nil {
			return err
		}
		// Read one byte at a time so a later login/init prompt keeps its input.
		var answer strings.Builder
		for {
			var b [1]byte
			if _, err := io.ReadFull(in, b[:]); err != nil {
				return err // EOF or interrupted input never grants consent.
			}
			if b[0] == '\n' {
				break
			}
			if answer.Len() >= 256 {
				return fmt.Errorf("telemetry answer is too long")
			}
			answer.WriteByte(b[0])
		}
		switch strings.ToLower(strings.TrimSpace(answer.String())) {
		case "y", "yes":
			return a.persistTelemetryChoice(true)
		case "", "n", "no":
			return a.persistTelemetryChoice(false)
		default:
			if _, err := fmt.Fprintln(out, "Please answer y or n."); err != nil {
				return err
			}
		}
	}
}
