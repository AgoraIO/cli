---
title: Telemetry
---

# CLI usage telemetry

Collection is enabled by default when a valid collector URL is configured and
the saved preference permits it. This source build embeds no collector URL;
changing the default does not activate production collection. Sentry diagnostics
is a separate placeholder and is not expanded by this feature.

## Notice and controls

Before the first eligible command, CLI prints the scope, this documentation
link and opt-out instructions to stderr. It never requests input. JSON, CI,
Agent and MCP use the same policy; MCP prints at service startup. Quiet mode
skips the notice and its marker, but does not disable capture. Help, version,
completion, internal sending, dry-run and telemetry disable do not trigger it.

The private `telemetry-notice-shown` file under the resolved Agora configuration
directory records best-effort disclosure, separately from config.json. It is
not consent. Concurrent first commands may repeat it. Output or marker write
failure does not fail the business command or change the saved switch.

```bash
agora telemetry disable
DO_NOT_TRACK=1 agora recipes list
AGORA_TELEMETRY_ENABLED=0 agora recipes list
agora telemetry enable
```

Any nonempty DO_NOT_TRACK and explicit AGORA_SENTRY_ENABLED=0 also opt out.
Environment opt-out overrides enable. Saved false remains false after upgrades.
`telemetryEnabled` is the saved preference; no consent field or consent migration
is used. Enable prints the notice under the same eligibility/marker rules.
CLI snapshots its preference at startup; MCP reads config.json once per tool
call and skips capture if that read fails. An operation already started can
continue sending after a later disable. The child never reads config/session.
This also applies with a custom AGORA_HOME.

`telemetry status` returns saved `enabled`, effective `captureActive` and local
`transportConfigured`. The last field only proves a valid local URL, not remote
availability, activation or ingestion. `consent` and `telemetryConsent` are
removed from status/config output. Business stdout and exit codes are unchanged.

## Events and identifiers

| Operation | Events |
| --- | --- |
| recipes list | cli_recipes_list_completed |
| recipes show | cli_recipes_show_completed |
| init | cli_init_started, cli_init_stage_completed, cli_init_completed |
| project create, including creation inside init | cli_project_create_completed |

Events include outcome, elapsed time, stable error/exit codes, result count
on successful list, public Recipe/template identifiers, CLI version, OS,
architecture, coarse AI-tool category and shell/MCP invocation channel.
Init stages remain validate, source_resolve, project_resolve, clone, env_write,
binding_write, context_write and agent_rules_write.

`distinct_id` is a random installation identifier privately stored in
`analytics-id`; it is not a verified person. Deleting the file creates a new
identifier, not deletion of previously captured data. `operation_id` joins
one command/tool call; retries use new IDs. `event_id` deduplicates only inside
the collector process and is never forwarded as PostHog UUID or property.

`app_id` accepts only a 32-character hexadecimal Agora project App ID from the
actual created/resolved target. It is not the OAuth client's appid. Each
operation has at most one target. Earlier events omit it and are never
backfilled; operation_id joins them with later events. Recipes omit app_id.
No user/company identity lookup, region, project_id, session_id, website
identity linking, code, conversations, secrets, paths, project names, complete
commands or original error messages are collected.

Creation is observed once at the shared createProject call boundary, including
internal authentication/token refresh/retry, excluding later feature activation,
context writes and cloning. Both shell/MCP and Recipe/Quickstart paths use it.
Dry-run, validation before that call and reuse emit no creation event. Success
means that call returned successfully. Later initialization failures do not
rewrite it; valid App ID is captured immediately, before later work can fail.
Failure can occur during authentication, transport or response handling; it
does not prove a request was sent or that no server project was created.
Count successful creation calls, or deduplicate successful events by app_id
for observed distinct projects. Telemetry is not a project ledger.

## Background transport and loss

The first event starts one detached copy of the current executable at a private
entry point; later events reuse it. Events pass through stdin as NDJSON, never
command-line arguments. Parent queues at most 20 events and writes asynchronously.
Child reading and HTTP sending are independent, with another 20-event queue.
Full queues drop new events. Each event is allowlisted and limited to 4 KiB;
oversized events are dropped independently. No disk queue, persistent service,
retry, replay, or future-run recovery is used.

Each HTTP POST carries exactly `{"events":[event]}`. The child uses its own
unauthenticated client, refuses redirects and body replay, and ignores business
cancellation. Requests have a three-second timeout. There is no child lifetime
limit while the parent pipe is open; after EOF, remaining work has at most three
seconds. The parent submits its final event, waits at most 100 ms for local
handoff, closes the pipe and returns without waiting for the network. Debug
mode does not change this. Child stdout/stderr go to the null device.

At most two sender children are in flight per parent process. If full at an
operation's first event, that operation is dropped without later startup retries.
The parent asynchronously waits/reaps children while alive. OS-specific detach
flags support macOS/Linux/Windows; CI containers, process-group cleanup or
Windows Job Objects can still terminate children. These are runtime budgets,
not hard guarantees across system suspension. Actual runtime validation per
platform must be recorded separately from cross-compilation.

Crashes, termination, startup/pipe/network errors, refusal, queue overflow and
budgets can lose events. A missing final event is not proof of business failure.
A successful write or HTTP response is not proof of PostHog query availability.

## Collector and activation

CLI → SSP anonymous collector → PostHog. SSP keeps its 20-event/64 KiB batch
limits, 4 KiB event limit, strict schema, rate limits, environment label and
per-process deduplication. No auth or client IP is forwarded. Person profiles
and GeoIP are disabled. Cross-replica/restart duplicates remain possible.
PostHog is sent no uuid or event_id property; it must assign its own UUID.
Before activation, test the actual deployment accepts omitted uuid and query
results have generated UUIDs. Local mock success cannot establish that.
Production endpoint configuration, deployment, release and real ingestion
verification require separate actions; this implementation does not activate them.
