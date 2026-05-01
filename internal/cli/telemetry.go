package cli

import (
	"regexp"
	"strings"
	"time"
)

// telemetryClient is the abstract interface every telemetry sink in the
// CLI implements. It is intentionally minimal so the production sink can
// be swapped (e.g. for the Sentry SDK or a file sink in tests) without
// touching call sites in app.go / commands.go.
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
// docs/proposals/telemetry-sentry-wireup.md for the wire-up plan.
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

// agoraSentryDSN is the Sentry project DSN that the CLI ships with.
// Empty string disables Sentry transport entirely (the default until the
// Sentry SDK is wired in for the next release). Mirrors the value in
// agora-cli-ts apps/agora-cli/src/telemetry.ts so the two surfaces report
// to the same project once enabled.
const agoraSentryDSN = ""

// initTelemetry returns the telemetry client appropriate for the current
// runtime. The decision tree mirrors the TS predecessor for parity:
//
//  1. DO_NOT_TRACK is set → noop (Console-style hard opt-out).
//  2. config.telemetryEnabled is false → noop.
//  3. AGORA_SENTRY_ENABLED=0 in the environment → noop.
//  4. agoraSentryDSN is empty (default in the current build) → noop.
//  5. Otherwise → Sentry-backed client (placeholder until SDK wired).
//
// initTelemetry never returns nil; callers can rely on a usable
// interface value without a nil check.
func initTelemetry(configEnabled bool, env map[string]string, _ versionInformation) telemetryClient {
	if strings.TrimSpace(env["DO_NOT_TRACK"]) != "" {
		return noopTelemetry{}
	}
	if !configEnabled {
		return noopTelemetry{}
	}
	if strings.TrimSpace(env["AGORA_SENTRY_ENABLED"]) == "0" {
		return noopTelemetry{}
	}
	if agoraSentryDSN == "" {
		// Sentry SDK not compiled in for this build. The wire-up is
		// scheduled for the next release; until then this is the
		// expected path. See docs/proposals/telemetry-sentry-wireup.md.
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

func (noopTelemetry) Enabled() bool                              { return false }
func (noopTelemetry) CaptureException(_ error, _ map[string]any) {}
func (noopTelemetry) CaptureEvent(_, _ string, _ map[string]any) {}
func (noopTelemetry) Flush(_ time.Duration) bool                 { return true }

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

// telemetrySensitiveFieldPattern is shared with sanitizeFields and
// matches the TS implementation in agora-cli-ts. Any field whose key
// matches this pattern is replaced with the literal "[REDACTED]" before
// it leaves the process.
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
