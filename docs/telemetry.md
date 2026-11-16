---
title: Telemetry
---

# Telemetry

After explicit consent, Agora CLI can report usage statistics for `recipes list`, `recipes show`, and
`init`, including shell commands and individual MCP tool calls. Events describe
command results, init stages, duration, stable error codes, CLI version, OS,
architecture and coarse Agent classification. Initialization success does not
prove that dependencies were installed or the application ran.

## Activation and controls

Usage collection is off until you explicitly consent. The legacy preference
defaults to enabled, but `telemetryConsent` defaults to `unknown`; a preference
of `true` alone never enables collection. This source build also embeds **no
collector URL**: it sends nothing until an approved collector is configured.
A release can embed `internal/cli.analyticsEndpoint` with Go `-ldflags -X`.
For local verification, set `AGORA_TELEMETRY_URL` to the collector's full URL.
HTTPS is required except for loopback HTTP. The collector remains disabled
until its project token is configured separately. The collector's PostHog host
defaults to `https://ph.agora.io`, overridable with
`CLI_TELEMETRY_POSTHOG_HOST`. This is the BFF's upstream, not the CLI collector URL.

Before the first instrumented command with a configured collector, an interactive
pretty terminal displays the data categories, random installation ID,
documentation link and disable command, then asks
`Allow usage collection? [y/N]:`. Only `y` or `yes` grants consent. Enter or `no`
saves a refusal and suppresses future prompts. EOF, interrupted input or a failed
save never grants consent. The prompt itself sends no event or analytics ID.

JSON, CI, MCP, quiet, non-TTY and `--yes` / `AGORA_NO_INPUT` executions never
prompt or assume consent. Until consent is saved they run normally without
capture, even with `AGORA_TELEMETRY_ENABLED=1`.

Consent is saved locally as `telemetryConsent` in the existing private
`config.json`; locate it with `agora config path --json`. Values are `unknown`
(not decided), `granted` and `denied`. The decision belongs to that user's CLI
configuration directory, not the project repository, and is not uploaded as a
consent event. Legacy configurations without the field stay `unknown`; a legacy
`telemetryEnabled=false` remains disabled and is not prompted again.

Running `agora telemetry enable` or `agora config update --telemetry-enabled=true`
explicitly consents and enables collection, including in noninteractive mode.
Both print the disclosure to stderr before saving; JSON stdout remains a single
envelope. Automation and agents must obtain the user's approval before executing
these commands on the user's behalf. Their disable equivalents save `denied`
and turn the preference off. Unrelated config changes do not grant consent.

```sh
agora telemetry status --json
agora telemetry disable --json
agora telemetry enable --json
DO_NOT_TRACK=1 agora recipes list --json
AGORA_TELEMETRY_ENABLED=0 agora init demo --template nextjs --json
```

Any non-empty `DO_NOT_TRACK`, config `telemetryEnabled=false`,
`AGORA_TELEMETRY_ENABLED=0`, or legacy `AGORA_SENTRY_ENABLED=0` disables capture.
Disabled capture creates no analytics identity and buffers no events.
Existing MCP processes re-read the persisted preference before each operation
and HTTP batch. Disabling capture also discards pending batches; requests already
sent cannot be recalled. Unreadable or invalid persisted configuration stops
capture. Explicit process-level opt-out continues to override file changes.
`telemetry status --json` exposes `transportConfigured` and `captureActive`
separately from the existing preference field `enabled`, plus `consent`.
`captureActive` requires `consent=granted`, an enabled preference, a configured
transport and no runtime opt-out. A previously granted consent never overrides
`DO_NOT_TRACK` or another explicit opt-out.
The Sentry diagnostics transport is still a placeholder; configuring analytics
does not enable raw exception reporting.

## CLI installation identity

The CLI uses a random ID in `analytics-id` under its private config directory
(file mode 0600). This persists across commands and identifies a CLI configuration
directory, not a verified person. Removing the file resets the ID for future events;
it does not delete events already collected. Treat these as linkable usage events,
not irreversibly anonymous aggregate statistics.

CLI activity is not linked to website identities or sessions. Shell commands and
MCP tools do not accept webpage attribution parameters. The collector accepts
only `cli:`-prefixed installation UUIDs and rejects webpage session properties.
`operation_id` identifies one command attempt and changes on retry; `event_id`
identifies one event for delivery deduplication.

## Events

| Event | Meaning |
| --- | --- |
| `cli_recipes_list_completed` | Catalog command ended, with outcome and result count on success |
| `cli_recipes_show_completed` | Detail command ended, with validated public recipe slug on success |
| `cli_init_started` | Init handler entered, before business validation |
| `cli_init_stage_completed` | One init stage ended, successfully or unsuccessfully |
| `cli_init_completed` | Init attempt ended, with outcome, total duration, exit code and failed stage |

Stages: `validate`, `source_resolve`, `project_resolve`, `clone`, `env_write`,
`binding_write`, `context_write`, `agent_rules_write` (when requested).
`validate` covers entry argument checks once per attempt; `source_resolve`
includes validation of the selected source and scaffold target.
`outcome` is `success`, `failure`, or `cancelled`. EOF and unexpected EOF from
backend connections are failures; only explicit cancellation signals or an
`INIT_ABORTED` result are cancellations. Missing final events indicate
unobserved completion, not proven failure. Telemetry is best effort and may be
lost on process termination or network failure.

## Safety and runtime behavior

Only allowlisted properties are constructed. Events never include tokens,
certificates, dotenv contents, complete commands, local paths, project names,
App IDs, prompts, raw environment values, raw errors or request/response bodies.
Agent inference does not prove AI execution; unrecognized callers are `unknown`,
not human. Custom Agent labels are reduced to `custom`.

CLI capture uses an in-memory bounded buffer and a dedicated unauthenticated
HTTP client with redirects and request replay disabled. Sending runs in the
background while the command executes. When a shell command or MCP tool call
finishes, it drains pending batches until their request bodies have been written,
without waiting for HTTP response headers or bodies. Connection setup and writes
have a five-second deadline per request; the final drain also has a total
five-second budget. Unfinished work is cancelled when that budget expires.

A completed write only means the local HTTP transport finished writing the
request; it does not confirm server acceptance or PostHog ingestion. The CLI
does not inspect HTTP status, follow `Retry-After`, retry failed batches, or
replay them on a later invocation. It has no disk queue or background daemon.
Network failures, process termination, server rejection and downstream failures
can lose events. Capture errors do not change command exit codes or the stdout
JSON contract. The BFF still handles its own forwarding response independently.

The anonymous collector validates a strict shared schema, limits size/rate,
deduplicates event IDs, and constructs PostHog events with `app=cli`, explicit
environment, person profile processing disabled and GeoIP enrichment disabled.
It never forwards business authorization or client IP headers. Public project
tokens and installation IDs are not secrets or authentication. A shared PostHog
project still permits forged events through its public capture token.

Before activation, configure the deployment's data region, IP-discard setting,
access control, retention (target 90 days), deletion procedure, edge quotas,
consent requirements and updated privacy notice. A default-enabled legacy
preference alone is not consent to expanded collection. Inspect actual PostHog
queries to verify ingestion; an HTTP acknowledgement alone is insufficient.
