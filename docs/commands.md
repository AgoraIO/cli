---
title: Command Reference
---

# Agora CLI — Command Reference

> Generated from `agora introspect --json`. Do not edit by hand — run `make docs-commands` or rely on the release workflow to regenerate.

This page lists every enumerable command and its local flags. For long descriptions, examples, and inherited flags, run `agora <command> --help` or read the source in `internal/cli/`.

## Global Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--debug` | `bool` | — | echo structured logs to stderr (equivalent to AGORA_DEBUG=1); does not change exit codes or JSON envelopes |
| `--json` | `bool` | — | shortcut for --output json |
| `--no-color` | `bool` | — | disable ANSI color in pretty output |
| `--output` | `string` | — | output mode for command results: pretty or json |
| `--pretty` | `bool` | — | pretty-print JSON output when used with --json |
| `--quiet` | `bool` | — | suppress success output (both pretty and JSON envelopes); rely on exit code. Errors still print on stderr. |
| `--upgrade-check` | `bool` | — | print non-interactive upgrade guidance and exit |
| `--yes` | `bool` | — | assume the default answer to confirmation prompts (equivalent to AGORA_NO_INPUT=1); never starts new interactive flows in JSON/CI/non-TTY contexts |

## Pseudo Commands

Pseudo commands are root-level flags that emit their own JSON envelope rather than living in the cobra subcommand tree. Agents should treat the `command` label as a stable identifier when matching JSON envelopes.

| Command | Trigger | Description |
|---------|---------|-------------|
| `upgrade check` | `agora --upgrade-check` | Print package-manager-specific upgrade guidance and exit (root flag, not a subcommand). |

## Commands

### `agora auth`

Manage Agora authentication

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora auth login`

Authenticate with Agora Console

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--no-browser` | `bool` | — | print the login URL instead of auto-opening a browser |
| `--region` | `string` | `global` | control plane region for login (global or cn) |

### `agora auth logout`

Clear the local Agora session

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora auth status`

Show the current auth status

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora config`

Manage persisted Agora CLI defaults

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora config get`

Read persisted CLI defaults

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora config path`

Show the config file path

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora config update`

Update persisted CLI defaults

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--browser-auto-open` | `bool` | — | persist browser auto-open preference; use --browser-auto-open=false to disable |
| `--debug` | `bool` | — | persist the --debug preference (echo structured logs to stderr); use --debug=false to disable |
| `--log-level` | `string` | `info` | persist default log level |
| `--output` | `output` | `pretty` | persist default output mode (pretty or json) |
| `--telemetry-enabled` | `bool` | — | persist telemetry preference; use --telemetry-enabled=false to disable |

### `agora convoai`

Conversational AI developer tools

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora convoai playground`

Start a local frontend to talk to your Conversational AI agent

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--agent-uid` | `int64` | — | reserved agent uid printed in the wire block (0 = generate) |
| `--channel` | `string` | — | RTC channel to join (required; use the same value for your agent) |
| `--no-open` | `bool` | — | do not open the browser automatically |
| `--port` | `int` | `8787` | local server port (auto-increments if busy unless explicitly set) |
| `--ttl` | `string` | `1h` | token lifetime |
| `--uid` | `int64` | — | browser RTC uid (0 = generate a non-zero uid) |

### `agora doctor`

Diagnose the local Agora CLI install (PATH, version, network, auth, MCP host)

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora env-help`

List every AGORA_* environment variable the CLI honors

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora init`

Create a project, clone a quickstart, and write env in one flow

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--add-agent-rules` | `stringArray` | `[]` | write AI agent rules into the quickstart (repeatable: cursor, claude, windsurf) |
| `--dir` | `string` | — | target directory for the cloned quickstart; defaults to <name> |
| `--feature` | `stringArray` | `[]` | enable a feature on the newly created project (repeatable); defaults to rtc, rtm, convoai; convoai also enables rtm |
| `--new-project` | `bool` | — | always create a new Agora project instead of reusing an existing one |
| `--project` | `string` | — | existing project ID or exact project name to bind to |
| `--rtm-data-center` | `string` | — | RTM data center to configure when rtm is enabled on a newly created project (CN, NA, EU, or AP); defaults to NA |
| `--template` | `string` | — | quickstart template ID to use |

### `agora introspect`

Emit machine-readable command metadata

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora login`

Authenticate with Agora Console

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--no-browser` | `bool` | — | print the login URL instead of auto-opening a browser |
| `--region` | `string` | `global` | control plane region for login (global or cn) |

### `agora logout`

Clear the local Agora session

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora mcp`

Run Agora CLI as a local MCP server

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora mcp serve`

Serve Agora CLI tools over MCP

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora open`

Open Agora Console or CLI docs

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--browser` | `bool` | — | force opening a browser even in CI/non-TTY pretty sessions |
| `--no-browser` | `bool` | — | always print the URL without opening a browser |
| `--target` | `string` | `console` | target to open: console, docs, docs-md, or product-docs |

### `agora project`

Manage remote Agora project resources

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora project create`

Create a new remote Agora project

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | `bool` | — | return the planned project create result without creating remote resources |
| `--feature` | `stringArray` | `[]` | enable one or more features after creation; defaults to rtc, rtm, convoai; convoai also enables rtm |
| `--idempotency-key` | `string` | — | caller-provided key for safe retries when supported by the API |
| `--rtm-data-center` | `string` | — | RTM data center to configure when rtm is enabled (CN, NA, EU, or AP); defaults to NA |
| `--template` | `string` | — | apply a higher-level project preset such as voice-agent |

### `agora project doctor`

Diagnose whether a project is ready for selected feature development

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--deep` | `bool` | — | run deeper repo-local checks for .agora metadata and quickstart env consistency |
| `--feature` | `string` | `convoai` | target feature readiness to evaluate: rtc, rtm, convoai |

### `agora project env`

Export project environment variables

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--format` | `string` | — | output format: dotenv \| shell \| envelope \| json (default dotenv; envelope/json emit the unified JSON envelope) |
| `--project` | `string` | — | project ID or exact project name; defaults to the current project context |
| `--shell` | `bool` | — | render shell export statements instead of dotenv lines |
| `--with-secrets` | `bool` | — | include sensitive values such as the app certificate |

### `agora project env write`

Write project environment variables to a dotenv file

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--append` | `bool` | — | append Agora App ID and App Certificate values when no existing values are present |
| `--overwrite` | `bool` | — | replace the target file with only Agora App ID and App Certificate values |
| `--project` | `string` | — | project ID or exact project name; defaults to the current project context |
| `--template` | `string` | — | credential key layout: nextjs or standard; if omitted, detect Next.js from the workspace |

### `agora project feature`

Manage project feature state

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora project feature enable`

Enable one feature for a project

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora project feature list`

List feature status for a project

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora project feature status`

Show one feature status

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora project list`

List projects available to the current account

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--keyword` | `string` | — | filter by exact or partial project name or project ID |
| `--page` | `int` | `1` | page number to request |
| `--page-size` | `int` | `20` | number of projects per page |
| `--refresh-cache` | `bool` | — | force-refresh the unfiltered first-page project completion cache after listing |

### `agora project show`

Show one project

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora project use`

Set the current project context

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora project webhook`

Manage project webhook configurations

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--feature` | `string` | — | project feature for webhook operations: rtc, rtm, or convoai |

### `agora project webhook create`

Create a webhook configuration

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--delivery-region` | `string` | — | webhook delivery region: cn, sea, na, or eu |
| `--events` | `string` | — | comma-separated webhook event keys, display names, or numeric IDs |
| `--project` | `string` | — | project ID or exact project name; defaults to the current project context |
| `--secret` | `string` | — | webhook signing secret; generated when omitted |
| `--url` | `string` | — | webhook endpoint URL |

### `agora project webhook delete`

Delete a webhook configuration

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--project` | `string` | — | project ID or exact project name; defaults to the current project context |

### `agora project webhook events`

List available webhook events for a feature

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora project webhook list`

List webhook configurations for a project feature

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--project` | `string` | — | project ID or exact project name; defaults to the current project context |

### `agora project webhook show`

Show one webhook configuration

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--project` | `string` | — | project ID or exact project name; defaults to the current project context |
| `--with-secret` | `bool` | — | include the webhook secret in the response |

### `agora project webhook update`

Update a webhook configuration

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--delivery-region` | `string` | — | new webhook delivery region: cn, sea, na, or eu |
| `--disabled` | `bool` | — | disable the webhook configuration |
| `--enabled` | `bool` | — | enable the webhook configuration |
| `--events` | `string` | — | comma-separated replacement webhook event keys, display names, or numeric IDs |
| `--project` | `string` | — | project ID or exact project name; defaults to the current project context |
| `--url` | `string` | — | new webhook endpoint URL |

### `agora quickstart`

Clone official standalone Agora quickstarts

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora quickstart create`

Clone an official Agora quickstart into a new directory

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dir` | `string` | — | target directory for the cloned quickstart; defaults to <name> |
| `--project` | `string` | — | project ID or exact project name to use for env seeding |
| `--ref` | `string` | — | git branch, tag, or ref to clone for pinned workshops |
| `--template` | `string` | — | quickstart template ID from `agora quickstart list` |

### `agora quickstart env`

Write framework-specific env files for a quickstart repo

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora quickstart env write`

Write the quickstart env file for the current or selected project

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--project` | `string` | — | project ID or exact project name to use for env seeding |
| `--template` | `string` | — | quickstart template ID; if omitted, the CLI detects it from the repo layout |

### `agora quickstart list`

List available official quickstarts

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--details` | `bool` | — | show repository, runtime, and env details in pretty output |
| `--show-all` | `bool` | — | include upcoming or unavailable templates in the list |

### `agora skills`

Browse curated Agora workflows for humans and AI agents

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora skills list`

List available skills

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--category` | `string` | — | filter by category (scaffold, ops, agent) |
| `--tag` | `string` | — | filter by tag (e.g. nextjs, rtc, mcp) |

### `agora skills search`

Search skills by id, title, description, or tag

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora skills show`

Show one skill in detail

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora telemetry`

Inspect or update telemetry preferences

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora telemetry disable`

Disable telemetry

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora telemetry enable`

Enable telemetry

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora telemetry status`

Show telemetry status

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora upgrade`

Upgrade Agora CLI in place when installer-managed; otherwise print upgrade guidance

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--check` | `bool` | — | resolve the latest release and report what would happen without writing anything |

### `agora version`

Show Agora CLI build information

_No local flags. Inherited parent and global flags still apply; run `agora <command> --help` for the full flag set._

### `agora whoami`

Show the current auth status

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--plain` | `bool` | — | print only authenticated or unauthenticated for shell scripts |

## Enums

**`features`**: `rtc`, `rtm`, `convoai`

**`outputModes`**: `pretty`, `json`

**`doctorStatus`**: `healthy`, `warning`, `not_ready`, `auth_error`
