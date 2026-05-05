---
title: Telemetry
---

# Telemetry

Agora CLI telemetry is limited to operational diagnostics such as command
failures and local log metadata. It **never** includes OAuth tokens, app
certificates, dotenv secrets, or project env values. Field redaction is
enforced by `redactTelemetryFields` in
[`internal/cli/telemetry.go`](https://github.com/AgoraIO/cli/blob/main/internal/cli/telemetry.go),
which matches the `token | secret | password | api[_-]?key | authorization`
key pattern (case-insensitive) and replaces the value with the literal
`[REDACTED]`.

> **Status (current release):** the on/off contract below is fully
> wired. The transport (Sentry SDK) is not yet linked into the binary,
> so all telemetry calls are no-ops at runtime. The next release will
> wire Sentry per
> [`docs/proposals/telemetry-sentry-wireup.md`](proposals/telemetry-sentry-wireup.md);
> the surface and field schema will not change.

## Inspect or change the setting

Telemetry is enabled by default in the local config.

```bash
agora telemetry status
agora telemetry disable
agora telemetry enable
```

For scripts, prefer JSON:

```bash
agora telemetry disable --json
```

## Opt-out signals

The CLI honors **all** of the following — any one of them disables
telemetry for the current process:

| Signal | Notes |
|--------|-------|
| `DO_NOT_TRACK=<any non-empty>` | Standard cross-tool convention. Also suppresses local file logs for that process. |
| `AGORA_SENTRY_ENABLED=0` | Hard env override. Wins over the config file. |
| `agora telemetry disable` | Persists `telemetryEnabled: false` to the config. |
| `agora config update --telemetry-enabled=false` | Equivalent to `agora telemetry disable`. |

```bash
DO_NOT_TRACK=1 agora project list --json
AGORA_SENTRY_ENABLED=0 agora init my-app --template nextjs --json
```

## Field schema

The wire-up plan documents the **exact** field set the Sentry-backed
sink will send. Until then the table below is forward-looking; treat it
as the specification, not the production behavior:

| Field           | Type   | Example                            | Notes |
|-----------------|--------|------------------------------------|-------|
| `command`       | string | `"project create"`                 | Stable label from `agora introspect --json`. |
| `exitCode`      | int    | `1`                                | Process exit code at failure. |
| `commitSha`     | string | `"abc1234"`                        | Build-time injected. |
| `os`            | string | `"darwin/arm64"`                   | Runtime, not host-specific. |
| `installMethod` | string | `"installer"` / `"npm"` / `"brew"` | From the install provenance receipt. |
| `agentLabel`    | string | `"cursor"`                         | From `agent_infer.go`. |

Explicitly **never** sent:

- OAuth tokens or session refresh tokens.
- Agora App Certificate values.
- Project names or App IDs (use opaque hashes if needed in the future).
- Local file paths beyond the rotating log file path basename.

## Where the config lives

```bash
agora config path
```

The same directory holds the rotating log file (`agora-cli.log`) and
the project list cache used by shell completion.
