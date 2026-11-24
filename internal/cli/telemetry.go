package cli

import (
	"regexp"
	"strings"
	"time"
)

// telemetryClient is the diagnostics sink interface. Usage analytics uses the
// separate bounded collector in analytics.go; both share the telemetryAllowed
// gate below. This interface is minimal so the sink can be swapped (e.g. for
// the Sentry SDK or a file sink in tests) without touching call sites in
// app.go / commands.go.
//
// Contract for all implementations:
//   - All methods MUST be safe to call from any goroutine.
//   - All methods MUST be cheap when telemetry is disabled (the noop
//     client is the default in that case).
//   - No method may block longer than its bounded timeout. Telemetry
//     must never delay the CLI from returning control to the shell.
//   - All methods MUST redact any field whose key matches the
//     sensitive-field pattern (token, secret, password, api[_-]?key,
//     authorization). The shared redactTelemetryFields helper enforces
//     this so individual call sites cannot forget.
//
// Today this file ships a no-op default. The Sentry-backed sink is
// scaffolded as `sentryClient` below so the next release can flip the
// constructor over without changing the public API. See
// internal-docs/proposals/telemetry-sentry-wireup.md for the wire-up plan.
type telemetryClient interface {
	// Enabled reports whether the underlying sink will actually emit.
	Enabled() bool
	// CaptureException forwards a CLI error to the telemetry sink with
	// optional context fields. Sensitive fields are redacted before
	// transport.
	CaptureException(err error, fields map[string]any)
	// CaptureEvent forwards an arbitrary structured event with optional
	// fields. Used for non-error operational signals (e.g. completion
	// of long-running flows).
	CaptureEvent(name, level string, fields map[string]any)
	// Flush blocks until the sink has flushed pending events or the
	// timeout elapses, whichever comes first. Must be safe to call
	// from defers in main paths.
	Flush(timeout time.Duration) bool
}

// telemetryAllowed is the single gate for every telemetry sink (usage
// analytics and the diagnostics client). preference is the saved
// telemetryEnabled value: the startup snapshot for shell commands, or the
// freshly re-read config for long-lived MCP servers.
func (a *App) telemetryAllowed(preference bool) bool {
	return preference && !a.telemetryOptedOut()
}

// telemetryOptedOut reports a runtime opt-out that overrides the saved
// preference: DO_NOT_TRACK (any non-empty value), AGORA_TELEMETRY_ENABLED=0
// or AGORA_SENTRY_ENABLED=0.
func (a *App) telemetryOptedOut() bool {
	if a == nil || strings.TrimSpace(a.env["DO_NOT_TRACK"]) != "" || a.env["AGORA_TELEMETRY_ENABLED"] == "0" {
		return true
	}
	sentryOverride := a.env["AGORA_SENTRY_ENABLED"]
	if a.cfgState.ConfigFilePath != "" {
		// applyConfigToEnv also writes this key from the saved preference.
		// Only an explicit OS override should mask a preference changed
		// since this (possibly long-lived MCP) process started.
		sentryOverride = a.osEnv["AGORA_SENTRY_ENABLED"]
	}
	return sentryOverride == "0"
}

// agoraSentryDSN is the Sentry project DSN that the CLI ships with.
// Empty string disables Sentry transport entirely (the default until the
// Sentry SDK is wired in for the next release). When set, events go to the
// Agora CLI Sentry project (see internal-docs/proposals/telemetry-sentry-wireup.md).
const agoraSentryDSN = ""

// initTelemetry returns the telemetry client appropriate for the current
// runtime. Decision order:
//
//  1. allowed is false (see App.telemetryAllowed: saved preference off,
//     DO_NOT_TRACK, AGORA_TELEMETRY_ENABLED=0 or AGORA_SENTRY_ENABLED=0) → noop.
//  2. agoraSentryDSN is empty (default in the current build) → noop.
//  3. Otherwise → Sentry-backed client (placeholder until SDK wired).
//
// initTelemetry never returns nil; callers can rely on a usable
// interface value without a nil check.
func initTelemetry(allowed bool, env map[string]string, _ versionInformation) telemetryClient {
	if !allowed {
		return noopTelemetry{}
	}
	if agoraSentryDSN == "" {
		// Sentry SDK not compiled in for this build. The wire-up is
		// scheduled for the next release; until then this is the
		// expected path. See internal-docs/proposals/telemetry-sentry-wireup.md.
		return noopTelemetry{}
	}
	return newSentryClient(agoraSentryDSN, env)
}

// versionInformation is a narrow alias used by initTelemetry so the
// telemetry constructor signature does not couple to the full
// versionInfo() return shape.
type versionInformation = map[string]any

// noopTelemetry is the default and the implementation used whenever
// telemetry is disabled, opted out, or not compiled in.
type noopTelemetry struct{}

func (noopTelemetry) Enabled() bool { return false }
func (noopTelemetry) CaptureException(_ error, fields map[string]any) {
	// Contract: redact before any sink transports fields; keep call so
	// redactTelemetryFields stays covered until Sentry wiring lands.
	_ = redactTelemetryFields(fields)
}
func (noopTelemetry) CaptureEvent(_, _ string, fields map[string]any) {
	_ = redactTelemetryFields(fields)
}
func (noopTelemetry) Flush(_ time.Duration) bool { return true }

// sentryClient is the placeholder for the Sentry-backed sink. Until the
// Sentry SDK is wired in, every method is a no-op so the surface and
// envelope contract stay stable. The next release will:
//
//  1. Add `github.com/getsentry/sentry-go` to go.mod.
//  2. Replace the unexported fields below with a real *sentry.Client.
//  3. Implement CaptureException using sentry.CaptureException with a
//     scope carrying the redacted fields.
//  4. Implement Flush by delegating to sentry.Flush(timeout).
//
// The CLI today already exposes the on/off contract (`agora telemetry
// enable|disable`, `AGORA_SENTRY_ENABLED`, `DO_NOT_TRACK`) and the
// documented field shape, so flipping to the real SDK is a one-file
// change with no observable contract break.
type sentryClient struct {
	dsn         string
	environment string
	release     string
}

func newSentryClient(dsn string, env map[string]string) *sentryClient {
	environment := strings.TrimSpace(env["AGORA_SENTRY_ENVIRONMENT"])
	if environment == "" {
		environment = "production"
	}
	release := strings.TrimSpace(env["AGORA_RELEASE"])
	return &sentryClient{
		dsn:         dsn,
		environment: environment,
		release:     release,
	}
}

func (c *sentryClient) Enabled() bool { return c != nil && c.dsn != "" }

func (c *sentryClient) CaptureException(_ error, _ map[string]any) {
	// Wire to sentry.CaptureException once SDK is added. Today: noop.
}

func (c *sentryClient) CaptureEvent(_, _ string, _ map[string]any) {
	// Wire to sentry.CaptureEvent once SDK is added. Today: noop.
}

func (c *sentryClient) Flush(_ time.Duration) bool {
	// Wire to sentry.Flush once SDK is added. Today: trivially true.
	return true
}

// telemetrySensitiveFieldPattern is shared with sanitizeFields. Any field
// whose key matches this pattern is replaced with the literal "[REDACTED]"
// before it leaves the process.
var telemetrySensitiveFieldPattern = regexp.MustCompile(`(?i)token|secret|password|api[_-]?key|authorization`)

// redactTelemetryFields returns a copy of fields with any sensitive key
// replaced by the redaction sentinel. Telemetry sinks MUST call this
// before transporting fields off the host.
func redactTelemetryFields(fields map[string]any) map[string]any {
	if fields == nil {
		return nil
	}
	out := make(map[string]any, len(fields))
	for k, v := range fields {
		if telemetrySensitiveFieldPattern.MatchString(k) {
			out[k] = "[REDACTED]"
			continue
		}
		out[k] = v
	}
	return out
}
