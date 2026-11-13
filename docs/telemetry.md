---
title: Telemetry
---

# Telemetry

Agora CLI can report anonymous usage of `recipes list`, `recipes show`, and
`init`, including shell commands and individual MCP tool calls. Events describe
command results, init stages, duration, stable error codes, CLI version, OS,
architecture and coarse Agent classification. Initialization success does not
prove that dependencies were installed or the application ran.

## Activation and controls

The local preference defaults to enabled, but this source build embeds **no
collector URL**: it sends nothing until an approved collector is configured.
A release can embed `internal/cli.analyticsEndpoint` with Go `-ldflags -X`.
For local verification, set `AGORA_TELEMETRY_URL` to the collector's full URL.
HTTPS is required except for loopback HTTP. The collector remains disabled
until its PostHog host and project token are configured separately.

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
separately from the existing preference field `enabled`.
The Sentry diagnostics transport is still a placeholder; configuring analytics
does not enable raw exception reporting.

## Anonymous identity and optional webpage attribution

Without web context, the CLI uses a random ID in `analytics-id` under its private
config directory (file mode 0600). This identifies a CLI configuration directory,
not a verified person. Removing that file resets future local attribution.

Recipes can pass its anonymous browser and source session IDs for one command:

```sh
agora init demo --recipe hotel-reception \
  --source-distinct-id <browser-id> \
  --source-session-id <web-session-id> --json
```

The same optional flags are available on `recipes list` and `recipes show`.
MCP tools `agora.init`, `agora.recipes.list`, and `agora.recipes.show` accept
`sourceDistinctId` and `sourceSessionId`. The parameters are not persisted.
Missing/invalid paired values fall back to the CLI anonymous ID without blocking
the command. Invalid shell context produces a fixed stderr notice with no IDs.

With valid source context, events use the browser distinct ID and store the
web session as `source_session_id`. They do **not** set PostHog `$session_id`
or pretend the browser remained active while CLI commands ran. Join webpage
`$session_id` to CLI `source_session_id`. Deduplicate repeated copies by
`(distinct_id, webpage session, recipe ID)`; use a defined conversion window
(recommended: seven days). `operation_id` identifies a command attempt and
changes on retry. `event_id` identifies an event for delivery deduplication.
A start event may precede recipe validation; join subsequent events by
`operation_id` to obtain the validated recipe identity.

Web IDs can appear in copied prompts, AI conversations and shell history.
Forwarded prompts retain the original source. Neither these IDs nor inferred
Agent labels authenticate users, clients or resource ownership. The website
must not attach IDs when analytics is unavailable or opted out, and must only
add these flags once the installed CLI version supports them. Website copy
adaptation is a separate release from this CLI implementation.

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

CLI capture uses an in-memory bounded buffer, a dedicated unauthenticated HTTP
client with redirects disabled, and a total exit-flush budget of one second.
On HTTP 429 it discards remaining events for that operation. Later calls in
the same MCP process pause until `Retry-After` (seconds or HTTP date); missing
or invalid headers default to one minute. No sleeping/retrying delays the
business command, and cooldown is not persisted across separate CLI processes.
It has no disk queue, background daemon or unbounded retry. Capture errors do
not change command exit codes or the stdout JSON contract.

The anonymous collector validates a strict shared schema, limits size/rate,
deduplicates event IDs, and constructs PostHog events with `app=cli`, explicit
environment, person profile processing disabled and GeoIP enrichment disabled.
It never forwards business authorization or client IP headers. Public project
tokens and browser IDs are not secrets or authentication. A shared PostHog
project still permits forged events through its public capture token.

Before activation, configure the deployment's data region, IP-discard setting,
access control, retention (target 90 days), deletion procedure, edge quotas,
consent requirements and updated privacy notice. A default-enabled legacy
preference alone is not consent to expanded collection. Inspect actual PostHog
queries to verify ingestion; an HTTP acknowledgement alone is insufficient.
