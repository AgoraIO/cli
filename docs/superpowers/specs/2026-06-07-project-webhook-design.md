# Project Webhook CLI Design

Date: 2026-06-07

## Context

Agora CLI should support webhook creation, editing, deletion, and viewing for project-scoped developer workflows. The backend API names this resource NCS config, but NCS is internal language. The public CLI should use webhook terminology and reuse the existing feature model (`rtc`, `rtm`, `convoai`) instead of introducing a new user-facing product concept.

Relevant backend endpoints from the Apifox NCS docs:

| CLI need | Backend endpoint |
| --- | --- |
| List webhook events | `GET /api/cli/v1/ncs-events/{feature}` |
| List webhook configs | `GET /api/cli/v1/projects/{projectId}/ncs-configs/{feature}` |
| Create webhook config | `POST /api/cli/v1/projects/{projectId}/ncs-configs/{feature}` |
| Update webhook config | `PUT /api/cli/v1/projects/{projectId}/ncs-configs/{feature}/{configId}` |
| Delete webhook config | `DELETE /api/cli/v1/projects/{projectId}/ncs-configs/{feature}/{configId}` |

Backend `productKey` is an internal implementation detail. For v1, supported public features are the current CLI feature catalog: `rtc`, `rtm`, and `convoai`.

## Goals

- Add `agora project webhook` commands for event discovery, list, show, create, update, and delete.
- Let developers select webhook events by readable CLI event keys instead of requiring numeric event IDs.
- Reuse existing project resolution and feature validation.
- Generate a secure webhook secret by default while allowing explicit override.
- Keep sensitive secrets redacted except when intentionally revealed.
- Preserve existing JSON envelope and documentation contracts.

## Non-Goals

- Do not expose an `ncs` command or backend product terminology.
- Do not add a new public `--product` concept.
- Do not support features outside `rtc`, `rtm`, and `convoai` in v1.
- Do not add webhook health-check or disable commands in v1. The first scope is creation, editing, deletion, viewing, and event discovery.

## Command UX

Add the command group under `project`:

```bash
agora project webhook events <feature>
agora project webhook list <feature> [project]
agora project webhook show <config-id> --feature <feature> [--project <project>] [--with-secret]
agora project webhook create --feature <feature> --url <url> --event <key-or-id>... [--secret <value>] [--delivery-region <region>] [--project <project>]
agora project webhook update <config-id> --feature <feature> [--url <url>] [--event <key-or-id>...] [--delivery-region <region>] [--enabled | --disabled] [--project <project>]
agora project webhook delete <config-id> --feature <feature> [--project <project>] [--yes]
```

`events <feature>` is a discovery command. It fetches available webhook events for the feature and displays CLI event keys, backend event IDs, backend event types, display names, Chinese display names, and payload examples so developers do not need to guess backend event IDs.

Example flow:

```bash
agora project webhook events rtc
agora project webhook create \
  --feature rtc \
  --url https://example.com/webhook \
  --event channel-created \
  --event channel-destroyed
```

Event input rules:

- `--event` accepts CLI event keys as the preferred path.
- Numeric event IDs are accepted as an escape hatch.
- Exact backend `displayName` values are accepted for compatibility, including values with spaces when shell-quoted.
- `create` requires at least one event.
- `update` only replaces event selections when at least one `--event` is provided.
- Unknown event keys return a helpful error suggesting `agora project webhook events <feature>`.

The CLI event key is generated from backend `displayName` by lowercasing it, replacing non-alphanumeric runs with `-`, and trimming leading/trailing separators. For example, `Channel Created` becomes `channel-created`. If two events generate the same key, the key is ambiguous and the user must pass the numeric event ID.

## Delivery Region

Webhook delivery region is separate from the existing CLI `--region` concept. Existing `--region` means login or project control-plane region (`global` or `cn`). Webhooks use backend `urlRegion` with these values:

| Value | Meaning |
| --- | --- |
| `cn` | China |
| `sea` | Asia |
| `na` | North America |
| `eu` | Europe |

Use `--delivery-region <cn|sea|na|eu>` on `create` and `update`.

Default when omitted:

- selected project/control-plane region `global` -> `urlRegion: "na"`
- selected project/control-plane region `cn` -> `urlRegion: "cn"`

Pretty output labels this field `Delivery Region`. JSON output uses `urlRegion` to match the backend field.

## Secret Handling

Backend webhook secrets must match `^[A-Za-z0-9_-]{7,32}$`. `create` generates a secure random secret when `--secret` is omitted. The generated value is 32 base64url characters without padding, produced from 24 random bytes. `--secret <value>` overrides the generated value and is validated against the backend pattern before the request is sent.

Secret output rules:

- `create` includes the secret in the command result because the developer needs to store it.
- `list` redacts secrets by default.
- `show` redacts secrets by default.
- `show --with-secret` reveals the secret if the backend returns it.
- Webhook secret rotation is not supported by the update endpoint. Users who need a new secret must create a replacement webhook and delete the old one.

Empty webhook secrets are not supported in v1.

## Data Model

Add typed structs in a focused implementation file, `internal/cli/webhooks.go`.

Normalized CLI event shape:

```go
type webhookEvent struct {
    ID            int    `json:"id"`
    Key           string `json:"key"`
    DisplayName   string `json:"displayName"`
    DisplayNameCn string `json:"displayNameCn,omitempty"`
    EventType     int    `json:"eventType"`
    Payload       string `json:"payload,omitempty"`
}
```

Normalized CLI config shape:

```go
type webhookConfig struct {
    ConfigID        int            `json:"configId"`
    URL             string         `json:"url"`
    URLRegion       string         `json:"urlRegion"`
    Enabled         bool           `json:"enabled"`
    EventIDs        []int          `json:"eventIds"`
    Events          []webhookEvent `json:"events,omitempty"`
    Retry           *bool          `json:"retry,omitempty"`
    UseIPWhitelist  bool           `json:"useIpWhitelist"`
    Secret          string         `json:"secret,omitempty"`
}
```

The backend response may include additional fields such as `appId`, `productId`, `projectId`, `vendorId`, `internal`, `createdAt`, and `updatedAt`. The CLI preserves stable fields needed by automation and support, but pretty output stays focused on project, feature, config ID, URL, delivery region, enabled state, and event names.

`retry` is read-only in v1. The CLI includes `retry` in output when the backend returns it, but create and update requests do not send a `retry` field and do not expose a retry flag.

The event API returns an `items` array. Each backend event has `eventId`, `displayName`, `displayNameCn`, `eventType`, and `payload`. The CLI uses `eventId` for config `eventIds`; `eventType` is retained only as event metadata for now because the config create/update schema labels `eventIds` as `event.id`. The implementation must not send `eventType` in config bodies.

Create requests send all backend-required fields:

```json
{
  "enabled": true,
  "eventIds": [1],
  "secret": "generated-or-user-provided",
  "url": "https://example.com/webhook",
  "urlRegion": "na",
  "useIpWhitelist": false
}
```

Create defaults are `enabled: true` and `useIpWhitelist: false`.

Update requests also send every backend-required PUT field: `enabled`, `eventIds`, `url`, `urlRegion`, and `useIpWhitelist`. Because users may update only one flag, update must do read-modify-write:

1. List existing configs for the feature.
2. Select the config with the requested `configId`.
3. Merge provided flags onto the existing config.
4. Send the complete PUT body.

The update endpoint has no `secret` field. `update` must not expose `--secret`.

List, create, and update responses are `NcsConfigListResponse` objects with an `items` array, not single config objects. The adapter extracts the relevant config:

- `list` returns all normalized items.
- `show` lists configs and selects the requested `configId`.
- `update` selects the requested `configId` from the PUT response.
- `create` selects the item matching the requested URL, delivery region, event IDs, and secret. If multiple items match, choose the newest item by `updatedAt` when present, otherwise the highest `configId`.

## JSON Contract

All commands use the existing JSON envelope. Stable command labels:

- `project webhook events`
- `project webhook list`
- `project webhook show`
- `project webhook create`
- `project webhook update`
- `project webhook delete`

Example create data:

```json
{
  "action": "webhook-create",
  "projectId": "prj_123",
  "projectName": "my-agent-demo",
  "feature": "rtc",
  "configId": 42,
  "status": "enabled",
  "url": "https://example.com/webhook",
  "urlRegion": "na",
  "eventIds": [1001],
  "events": [
    {
      "id": 1001,
      "key": "channel-created",
      "displayName": "Channel Created",
      "displayNameCn": "频道创建",
      "eventType": 1,
      "payload": "{...}"
    }
  ],
  "retry": true,
  "useIpWhitelist": false,
  "secret": "pUkA4FzTdI8iGtLA6m3o2qR9x_Nb7sYc"
}
```

Safe branch fields for automation:

- `projectId`
- `feature`
- `configId`
- `status`
- `urlRegion`
- `eventIds`

## Error Handling

Use existing envelope behavior and classify errors where the CLI can provide stable codes:

| Case | Error code |
| --- | --- |
| Missing URL on create | `WEBHOOK_URL_REQUIRED` |
| Missing events on create | `WEBHOOK_EVENTS_REQUIRED` |
| Unknown event name | `WEBHOOK_EVENT_UNKNOWN` |
| Duplicate or ambiguous event name | `WEBHOOK_EVENT_AMBIGUOUS` |
| Invalid delivery region | `WEBHOOK_DELIVERY_REGION_INVALID` |
| Invalid secret format | `WEBHOOK_SECRET_INVALID` |
| Missing config ID | `WEBHOOK_CONFIG_ID_REQUIRED` |
| Config not found when classifiable | `WEBHOOK_CONFIG_NOT_FOUND` |

Feature validation reuses the existing feature catalog and error style so accepted values stay in one place.

Delete confirmation:

- In interactive pretty mode, prompt before deletion unless `--yes` is passed.
- In JSON, CI, or non-TTY contexts, fail fast unless `--yes` is passed.
- Follow the existing `--yes` convention: it confirms destructive actions but does not start unrelated interactive flows.

## Rendering

Use `renderResult` and `printBlock` for pretty output, following existing project commands.

Single webhook operations render as:

```text
Webhook
  Project          my-agent-demo
  Feature          rtc
  Config ID        42
  URL              https://example.com/webhook
  Events           channel-created, channel-destroyed
  Delivery Region  North America (na)
  Enabled          true
  Retry            true
  Secret           pUkA4FzTdI8iGtLA6m3o2qR9x_Nb7sYc
```

For create, print a short note after the block:

```text
Store this secret now. It may not be shown again.
```

For list and show without `--with-secret`, render `Secret` as redacted if displayed at all.

## MCP Parity

Add MCP tools for agent workflows because existing project feature/env/init commands are mirrored through MCP:

- `agora.project.webhook.events`
- `agora.project.webhook.list`
- `agora.project.webhook.show`
- `agora.project.webhook.create`
- `agora.project.webhook.update`
- `agora.project.webhook.delete`

MCP inputs use feature names, event keys or event IDs, config IDs, URL, delivery region, and project. Secrets follow the same redaction and explicit reveal rules as CLI JSON output.

## Implementation Plan Outline

1. Add webhook constants and validation helpers:
   - supported delivery regions
   - control-plane to delivery-region default mapping
   - secure secret generation
   - secret pattern validation
   - event key generation
   - event name/ID resolution
2. Add typed API helpers in `internal/cli/webhooks.go`.
3. Register `project webhook` commands in `commands.go`.
4. Add pretty render cases in `render.go`.
5. Add MCP descriptors and dispatch handlers in `mcp.go`.
6. Extend fake CLI BFF with NCS event/config endpoints.
7. Add integration and unit tests.
8. Update `docs/automation.md`, `README.md`, `docs/llms.txt` if MCP surface changes, and regenerate `docs/commands.md`.

## Tests

Integration tests should cover:

- `webhook events rtc --json` returns event keys plus backend event metadata.
- `webhook create` with event keys resolves IDs and generates a backend-valid secret.
- `webhook create --secret` forwards explicit secret.
- `webhook create --secret` rejects values that do not match `^[A-Za-z0-9_-]{7,32}$`.
- `webhook create` defaults delivery region to `na` for `global` project context.
- `webhook create` defaults delivery region to `cn` for `cn` project context.
- `webhook create` sends `enabled: true` and `useIpWhitelist: false`.
- `webhook list` redacts secret by default.
- `webhook show --with-secret` reveals secret when backend returns it.
- `webhook update` performs list-merge-put and preserves omitted required fields.
- `webhook update` updates URL, events, delivery region, and enabled state.
- `webhook update --secret` is rejected as an unknown flag.
- `webhook delete` requires `--yes` in JSON/non-TTY runs.
- Auth errors continue to return exit code `3` with `AUTH_UNAUTHENTICATED`.

Unit tests should cover:

- delivery-region validation and defaulting
- feature validation reuse
- event key generation, event ID resolution, unknown event suggestions, and ambiguous keys
- eventId versus eventType mapping for config `eventIds`
- generated secret format, backend pattern compliance, and entropy length
- redaction behavior
