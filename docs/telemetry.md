---
title: Telemetry
---

# Telemetry

Agora CLI can report Recipes usage, init stages/results and project creation
results to the CLI BFF, which forwards allowlisted events to PostHog. Capture is
enabled by default when a valid collector URL is configured, unless the user
opts out. The embedded collector URL is currently empty; deployment and release
configuration are required before production capture is active.

Events contain a random local installation ID, a per-operation ID, CLI version,
OS/architecture, coarse AI-tool categories, public recipe/template IDs, timings
and structured results. Project/init events can include validated Agora App IDs.
An installation profile can be created in PostHog. Agora App IDs can be
associated with Agora projects and companies. OAuth tokens, App Certificates, project names, source code, conversation
content, local paths and raw errors are not included. This describes the
default `full` mode used for the global region; see [Regions](#regions).

Recipe init attempts are labeled as Recipe usage even if the catalog lookup
fails. Only attempts with a confirmed public catalog entry include a Recipe ID.
When reporting per-Recipe results, show unconfirmed attempts separately.
An existing target directory is reported as a local validation failure.

Project creation (`agora project create`, and the project step of `agora init`)
is reported when the create API call returns, and its duration covers only that
call. A project that is created successfully is recorded as a success even if a
later step, such as enabling features, fails; the outer command result still
reports that failure. A `project create` that fails before reaching the API
(for example, invalid flags) is reported as a failure when the command
finishes.

## Regions

Each operation is captured under the control-plane region the CLI targets
when it starts. That is the region saved in the local CLI context by
`agora login --region`, or `cn` when `AGORA_API_BASE_URL` points at the
built-in CN API host. `agora mcp serve` resolves the region again for every
tool call, so a login into another region applies to the next call.

Each region has a capture mode:

| Mode | Behavior |
|------|----------|
| `full` | The events and properties described above. |
| `aggregate` | The same events without a persistent identity: the installation ID file is neither read nor created, each operation gets a fresh random `distinct_id`, and Agora App IDs are never sent. |
| `off` | Nothing is sent. The installation ID file is neither read nor created, and no first-use notice is shown. |

The global region uses `full` and the global collector. The CN region defaults
to `off`. CN events go only to a separately configured CN collector and never
to the global collector, so CN usage is not transferred across borders. CN
capture is active only when both a CN mode other than `off` and a valid CN
collector URL are configured (`AGORA_TELEMETRY_CN_MODE` and
`AGORA_TELEMETRY_CN_URL` for local verification). Otherwise it is `off`.
Events sent to the CN collector carry `region: "cn"`. Global events do not
carry a region property. All opt-outs below apply to both regions.

If the local CLI context cannot be read, the region is unknown and nothing is
captured for that operation. A user who has never logged in and has no CN API
override is treated as global. This is a known gap: such a user may be in
China, but the CLI has no region signal until login.

The first-use notice for `aggregate` mode is shorter and says that no
installation ID or App ID is sent. It is tracked separately from the `full`
notice, so a user who later uses a `full` region still sees the full notice.

## Delivery

Each instrumented shell command collects its events in memory and flushes them
when the operation finishes. MCP flushes after each instrumented tool call,
independently of other calls and without waiting for the MCP server to exit.

A shell command passes one JSON batch through stdin to a detached sender
subprocess, which outlives the command. The business command waits only for a
local handoff of at most 100 ms, not for the network response. Because
`agora mcp serve` is long-lived, it sends each tool call's batch from a
background goroutine in the same process instead of spawning a subprocess; the
tool response never waits for it. When the MCP host closes stdin, the server
waits at most one second for in-flight batches before exiting. Both paths
apply the same validation and make one HTTP request with a ten-second timeout.
Telemetry failures do not change business output or exit codes.

The complete request is limited to 64 KiB. There is no separate event-count or
per-event size cap. If an operation exceeds the request limit, its entire batch
is dropped. There is no disk queue or retry. A crash or forced termination before
handoff (or, for MCP, before the request completes) can lose all events for
that operation; long-running operations are not visible in telemetry until they
finish. Successful delivery is best effort and does not prove that PostHog has
made the events available for queries.

The sender honors `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY` and their lowercase
forms, as the business HTTP client does. It also preserves `SSL_CERT_FILE` and
`SSL_CERT_DIR` for custom certificate authorities. The shell sender subprocess
does not inherit other parent environment variables. Business authorization headers are not attached to
telemetry requests, and redirects are not followed.

## Inspect or change the setting

```bash
agora telemetry status --json
agora telemetry disable --json
agora telemetry enable --json
```

`savedPreference` reports the configured preference. `enabled` also accounts
for runtime opt-outs. `region` is `global`, `cn` or `unknown`.
`transportConfigured` reports whether a valid collector URL is configured for
that region. `captureMode` is the effective mode (`off`, `aggregate` or
`full`). `captureActive` is true when `captureMode` is not `off`. It tracks
whether capture can start.

Shell commands print a first-use disclosure to stderr unless quiet mode
suppresses it or the operation's mode is `off`. MCP includes the disclosure for
the current region's effective mode in its initialization instructions, and
omits it when that mode is `off`. If the region is unknown at initialization,
MCP uses the global rules for the disclosure; each tool call is still gated
on its own region. MCP hosts decide how to present instructions.
The disclosure provides notice. The first instrumented call can collect events
when capture is active.

Any of these signals disables capture:

| Signal | Effect |
|--------|--------|
| `DO_NOT_TRACK=<any non-empty>` | Runtime opt-out. Also suppresses local file logs. |
| `AGORA_TELEMETRY_ENABLED=0` | Runtime opt-out for usage capture. |
| `AGORA_SENTRY_ENABLED=0` | Runtime opt-out honored by usage capture. |
| `agora telemetry disable` | Saves `telemetryEnabled: false`. |
| `agora config update --telemetry-enabled=false` | Saves the same preference. |

Shell commands use their startup preference. A running MCP server rereads the
saved preference when each tool call starts. A preference change does not alter
an operation that has already started.

## Collector configuration

`AGORA_TELEMETRY_URL` overrides the release-embedded global collector URL. It
never applies to CN operations. `AGORA_TELEMETRY_CN_URL` and
`AGORA_TELEMETRY_CN_MODE` override the release-embedded CN collector URL
(empty by default) and CN mode (`off` by default). Only HTTPS URLs, or HTTP
URLs on loopback for local verification, are accepted. URLs with credentials,
query parameters or fragments are rejected. A CN mode other than `off`,
`aggregate` or `full` means `off`.

The existing operational-diagnostics sink is separate from these usage events;
its Sentry transport remains a placeholder. Usage capture does not require an
OAuth login or a client-side PostHog API key.

The configuration location is available with:

```bash
agora config path
```
