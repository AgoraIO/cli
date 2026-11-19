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
An installation profile can be created in PostHog; website identities are not
linked. OAuth tokens, App Certificates, project names, source code, conversation
content, local paths and raw errors are not included.

## Delivery

Each instrumented shell command collects its events in memory and flushes them
when the operation finishes. MCP flushes after each instrumented tool call,
independently of other calls and without waiting for the MCP server to exit.

The CLI passes one JSON batch through stdin to a detached sender subprocess.
The sender makes one HTTP request with a ten-second timeout. The business
command waits only for a local handoff of at most 100 ms, not for the network
response. Telemetry failures do not change business output or exit codes.

The complete request is limited to 64 KiB. There is no separate event-count or
per-event size cap. If an operation exceeds the request limit, its entire batch
is dropped. There is no disk queue or retry. A crash or forced termination before
handoff can lose all events for that operation; long-running operations are not
visible in telemetry until they finish. Successful delivery is best effort and
does not prove that PostHog has made the events available for queries.

The sender honors `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY` and their lowercase
forms, as the business HTTP client does. It also preserves `SSL_CERT_FILE` and
`SSL_CERT_DIR` for custom certificate authorities. Other parent environment
variables are not inherited. Business authorization headers are not attached to
telemetry requests, and redirects are not followed.

## Inspect or change the setting

```bash
agora telemetry status --json
agora telemetry disable --json
agora telemetry enable --json
```

`enabled` reports the saved preference. `transportConfigured` reports whether a
valid collector URL is configured. `captureActive` combines the preference,
runtime opt-out and transport configuration; it does not confirm delivery.
A first-use disclosure is printed to stderr unless quiet mode suppresses it.

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

`AGORA_TELEMETRY_URL` overrides the release-embedded collector URL. Only HTTPS
URLs, or HTTP URLs on loopback for local verification, are accepted. URLs with
credentials, query parameters or fragments are rejected.

The existing operational-diagnostics sink is separate from these usage events;
its Sentry transport remains a placeholder. Usage capture does not require an
OAuth login or a client-side PostHog API key.

The configuration location is available with:

```bash
agora config path
```
