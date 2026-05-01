---
title: Sentry telemetry wire-up plan
status: proposed
target-release: next
---

# Sentry telemetry wire-up plan

> Status: **proposed**, target release: next.
>
> This document describes how to take the telemetry stub introduced in
> [`internal/cli/telemetry.go`](../../internal/cli/telemetry.go) from a
> no-op interface to a fully wired Sentry-backed client. Until this lands,
> the CLI exposes the full opt-in/opt-out contract (`agora telemetry
> enable|disable`, `AGORA_SENTRY_ENABLED`, `DO_NOT_TRACK`) but does not
> actually transmit any events. Treat this proposal as the contract a
> reviewer should validate against during the next release PR.

## Context

The TS predecessor (`agora-cli-ts`) ships Sentry. Concretely:

- `agora-cli-ts/packages/cli-telemetry/package.json` depends on
  `@sentry/node ^10.18.0`.
- `agora-cli-ts/apps/agora-cli/src/telemetry.ts` initializes Sentry
  with the project DSN
  `https://07bf9b5275eef5259abebe89fa247cec@o4510955723292672.ingest.us.sentry.io/4511189164687360`,
  reads the `AGORA_SENTRY_ENABLED` env var, redacts sensitive fields,
  and skips `command.failed` to avoid double reporting.
- The Go CLI mirrors the on/off contract in
  [`internal/cli/config.go`](../../internal/cli/config.go) (the
  `applyConfigToEnv` map sets `AGORA_SENTRY_ENABLED` from
  `cfg.TelemetryEnabled`) but had no actual transport. The new
  `telemetry.go` adds a `telemetryClient` interface and a noop
  default; callsites in `app.go` already invoke `CaptureException` on
  error.

## Goals

1. The Go CLI reports the same operational diagnostics to the same
   Sentry project the TS CLI reports to, so the migration does not lose
   error visibility.
2. The contract surface (`agora telemetry`, env vars, log fields) does
   not change. Existing wrappers and CI configs keep working.
3. Field redaction is enforced inside the sink (defense in depth) in
   addition to whatever the call site does.
4. Telemetry never blocks the CLI from returning control to the shell.
   Bounded flush, no panic propagation, no synchronous network calls
   on the hot path.

## Implementation steps

### Step 1: Add the SDK dependency

```bash
go get github.com/getsentry/sentry-go@latest
go mod tidy
```

Verify the resolved version is `>=v0.27.0` so we get the `WithContext`
helpers and the modern `BeforeSend` signature.

### Step 2: Replace the `agoraSentryDSN` const

In [`internal/cli/telemetry.go`](../../internal/cli/telemetry.go):

```go
const agoraSentryDSN = "https://07bf9b5275eef5259abebe89fa247cec@o4510955723292672.ingest.us.sentry.io/4511189164687360"
```

This single change activates the Sentry path through `initTelemetry`.

### Step 3: Replace the `sentryClient` placeholder methods

Replace the `sentryClient` struct so it owns a real Sentry hub and
implements the interface against the SDK. The full implementation
should look approximately like:

```go
import sentry "github.com/getsentry/sentry-go"

type sentryClient struct {
	hub *sentry.Hub
}

func newSentryClient(dsn string, env map[string]string) *sentryClient {
	options := sentry.ClientOptions{
		Dsn:              dsn,
		Environment:      defaultString(env["AGORA_SENTRY_ENVIRONMENT"], "production"),
		Release:          firstNonEmpty(env["AGORA_RELEASE"], version),
		AttachStacktrace: true,
		SendDefaultPII:   false,
		BeforeSend: func(event *sentry.Event, _ *sentry.EventHint) *sentry.Event {
			// Defense-in-depth: redact sensitive fields again, in case
			// a call site forgot.
			if event.Extra != nil {
				event.Extra = redactTelemetryFields(event.Extra)
			}
			return event
		},
	}
	client, err := sentry.NewClient(options)
	if err != nil {
		return nil
	}
	scope := sentry.NewScope()
	scope.SetTag("app", "agora-cli")
	return &sentryClient{hub: sentry.NewHub(client, scope)}
}

func (c *sentryClient) CaptureException(err error, fields map[string]any) {
	if c == nil || c.hub == nil {
		return
	}
	c.hub.WithScope(func(scope *sentry.Scope) {
		for k, v := range redactTelemetryFields(fields) {
			scope.SetExtra(k, v)
		}
		c.hub.CaptureException(err)
	})
}

func (c *sentryClient) Flush(timeout time.Duration) bool {
	if c == nil || c.hub == nil {
		return true
	}
	return c.hub.Flush(timeout)
}
```

`newSentryClient` returning nil on init failure means
`(*sentryClient).Enabled` returns false, the CLI drops back to noop
behavior, and nothing else changes. This is the contract `initTelemetry`
already expects.

### Step 4: Document fields

Update [`docs/telemetry.md`](../telemetry.md) with the **exact**
field schema we send. Suggested initial event vocabulary (mirrors TS):

| Field           | Type   | Example                          | Notes |
|----------------|--------|----------------------------------|-------|
| `command`      | string | `"project create"`               | Stable label from `introspect`. |
| `exitCode`     | int    | `1`                              | Process exit code at failure. |
| `commitSha`    | string | `"abc1234"`                      | Build-time injected. |
| `os`           | string | `"darwin/arm64"`                 | Runtime, not host-specific. |
| `installMethod`| string | `"installer"` / `"npm"` / `"brew"` | From provenance receipt. |
| `agentLabel`   | string | `"cursor"`                       | From `agent_infer.go`. |

Explicitly call out what we **never** send:

- OAuth tokens or session refresh tokens.
- Agora App Certificate values.
- Project names or App IDs (use opaque hashes if needed).
- Local file paths beyond the log file path basename.

### Step 5: Wire the consent banner

Add a one-time interactive prompt on first run when stderr is a TTY,
not in CI, and not in JSON mode:

> "Agora CLI sends anonymous error reports to help us fix bugs. You
> can disable this any time with `agora telemetry disable`. Continue?
> [Y/n]"

The default is "yes" to match the current `cfg.TelemetryEnabled: true`
default and the TS predecessor. Persist the answer to config so the
prompt never re-appears.

### Step 6: Add tests

- `telemetry_test.go` covering each branch of `initTelemetry`
  (DO_NOT_TRACK, config off, env=0, empty DSN, normal).
- A round-trip test that asserts `redactTelemetryFields` zeroes out
  every key matching the documented pattern.
- A test asserting `Flush` honors a 100 ms timeout deterministically
  (using a fake hub).

### Step 7: Update CHANGELOG

Under `[Unreleased] / Added`:

> - Wire Agora CLI telemetry to Sentry. Telemetry is on by default, can
>   be disabled with `agora telemetry disable`,
>   `agora config update --telemetry-enabled=false`, `DO_NOT_TRACK=1`,
>   or `AGORA_SENTRY_ENABLED=0`. Field schema is documented in
>   [docs/telemetry.md](docs/telemetry.md). No tokens, app certificates,
>   or project identifiers are transmitted.

## Reasoning

| # | Why |
|---|-----|
| 1 | TS already shipped Sentry. Migrating without it would silently regress error visibility for the same user base. |
| 2 | `internal/cli/telemetry.go` already exposes the right interface. Wire-up is one constant + one struct change. |
| 3 | `BeforeSend` redaction in the sink is a belt-and-braces guarantee: even if a future call site forgets to redact, fields never leave the host. |
| 4 | A documented one-time consent prompt aligns with the industry direction (Homebrew flipped to opt-in in 2024, npm honors `DO_NOT_TRACK`). We keep opt-out as the default but add explicit acknowledgement. |

## Risks / open questions

- **DSN exposure.** The TS DSN is already in a public npm package, so
  embedding the same DSN in the Go binary does not change the threat
  model. (Sentry DSNs are intentionally public; rate-limiting and
  project-side filtering are the controls.)
- **Default opt-in vs opt-out.** Industry is shifting; we should
  explicitly decide for v1 of the Go CLI rather than inheriting the TS
  default by accident.
- **Sentry SDK size.** `sentry-go` adds ~3 MB to the static binary.
  Acceptable for a CLI; document in the release notes.

## Out of scope

- Custom event sinks beyond Sentry (file sink, OTel exporter). Add
  later if customer demand emerges.
- PII review beyond the field schema documented in step 4.
