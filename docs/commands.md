# Agora CLI — Command Reference

> Generated from `agora introspect --json` on 2026-04-30. Do not edit by hand — run `make docs-commands` or rely on the release workflow to regenerate.

This page lists every enumerable command and its local flags. For long descriptions, examples, and inherited flags, run `agora <command> --help` or read the source in `internal/cli/`.

## Global Flags

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--json` | `bool` | — | shortcut for --output json |
| `--no-color` | `bool` | — | disable ANSI color in pretty output |
| `--output` | `string` | — | output mode for command results: pretty or json |
| `--pretty` | `bool` | — | pretty-print JSON output when used with --json |
| `--quiet` | `bool` | — | suppress success output (both pretty and JSON envelopes); rely on exit code. Errors still print on stderr. |
| `--upgrade-check` | `bool` | — | print non-interactive upgrade guidance and exit |
| `--verbose` | `bool` | — | echo structured logs to stderr (equivalent to AGORA_VERBOSE=1); does not change exit codes or JSON envelopes |
| `--yes` | `bool` | — | accept default answers and suppress interactive prompts (equivalent to AGORA_NO_INPUT=1) |

## Pseudo Commands

Pseudo commands are root-level flags that emit their own JSON envelope rather than living in the cobra subcommand tree. Agents should treat the `command` label as a stable identifier when matching JSON envelopes.

| Command | Trigger | Description |
|---------|---------|-------------|
| `upgrade check` | `agora --upgrade-check` | Print package-manager-specific upgrade guidance and exit (root flag, not a subcommand). |

## Commands

### `agora auth`

Manage Agora authentication

_No local flags. Inherited global flags still apply (see [Global Flags](#global-flags))._

### `agora auth login`

Authenticate with Agora Console

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--no-browser` | `bool` | — | print the login URL instead of auto-opening a browser |
| `--region` | `string` | — | control plane region for login defaults (global or cn) |

### `agora auth logout`

Clear the local Agora session

_No local flags. Inherited global flags still apply (see [Global Flags](#global-flags))._

### `agora auth status`

Show the current auth status

_No local flags. Inherited global flags still apply (see [Global Flags](#global-flags))._

### `agora config`

Manage persisted Agora CLI defaults

_No local flags. Inherited global flags still apply (see [Global Flags](#global-flags))._

### `agora config get`

Read persisted CLI defaults

_No local flags. Inherited global flags still apply (see [Global Flags](#global-flags))._

### `agora config path`

Show the config file path

_No local flags. Inherited global flags still apply (see [Global Flags](#global-flags))._

### `agora config update`

Update persisted CLI defaults

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--api-base-url` | `string` | `https://agora-cli.agora.io` | default CLI API base URL |
| `--browser-auto-open` | `bool` | — | persist browser auto-open preference; use --browser-auto-open=false to disable |
| `--log-level` | `string` | `info` | persist default log level |
| `--oauth-base-url` | `string` | `https://sso2.agora.io` | default OAuth base URL |
| `--oauth-client-id` | `string` | `agora_web_cli` | default OAuth client ID |
| `--oauth-scope` | `string` | `basic_info,console` | default OAuth scope |
| `--output` | `output` | `pretty` | persist default output mode (pretty or json) |
| `--telemetry-enabled` | `bool` | — | persist telemetry preference; use --telemetry-enabled=false to disable |
| `--verbose` | `bool` | — | persist verbose logging preference; use --verbose=false to disable |

### `agora init`

Create a project, clone a quickstart, and write env in one flow

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--add-agent-rules` | `stringArray` | `[]` | write AI agent rules into the quickstart (repeatable: cursor, claude, windsurf) |
| `--dir` | `string` | — | target directory for the cloned quickstart; defaults to <name> |
| `--feature` | `stringArray` | `[]` | enable a feature on the newly created project (repeatable); defaults to rtc, rtm, and convoai; convoai also enables rtm |
| `--new-project` | `bool` | — | always create a new Agora project instead of reusing an existing one |
| `--project` | `string` | — | existing project ID or exact project name to bind to |
| `--region` | `string` | — | control plane region for newly created projects (global or cn) |
| `--rtm-data-center` | `string` | — | RTM data center to configure when rtm is enabled on a newly created project (CN, NA, EU, or AP); defaults to NA |
| `--template` | `string` | — | quickstart template ID to use |

### `agora introspect`

Emit machine-readable command metadata

_No local flags. Inherited global flags still apply (see [Global Flags](#global-flags))._

### `agora login`

Authenticate with Agora Console

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--no-browser` | `bool` | — | print the login URL instead of auto-opening a browser |
| `--region` | `string` | — | control plane region for login defaults (global or cn) |

### `agora logout`

Clear the local Agora session

_No local flags. Inherited global flags still apply (see [Global Flags](#global-flags))._

### `agora mcp`

Run Agora CLI as a local MCP server

_No local flags. Inherited global flags still apply (see [Global Flags](#global-flags))._

### `agora mcp serve`

Serve Agora CLI tools over MCP

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--transport` | `string` | `stdio` | MCP transport: stdio |

### `agora open`

Open Agora Console or CLI docs

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--no-browser` | `bool` | — | print the URL without opening a browser |
| `--target` | `string` | `console` | target to open: console, docs, or product-docs |

### `agora project`

Manage remote Agora project resources

_No local flags. Inherited global flags still apply (see [Global Flags](#global-flags))._

### `agora project create`

Create a new remote Agora project

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--dry-run` | `bool` | — | return the planned project create result without creating remote resources |
| `--feature` | `stringArray` | `[]` | enable one or more features after creation; defaults to rtc, rtm, and convoai; convoai also enables rtm |
| `--idempotency-key` | `string` | — | caller-provided key for safe retries when supported by the API |
| `--region` | `string` | — | control plane region for the project context (global or cn) |
| `--rtm-data-center` | `string` | — | RTM data center to configure when rtm is enabled (CN, NA, EU, or AP); defaults to NA |
| `--template` | `string` | — | apply a higher-level project preset such as voice-agent |

### `agora project doctor`

Diagnose whether a project is ready for selected feature development

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--deep` | `bool` | — | run deeper repo-local checks for .agora metadata and quickstart env consistency |
| `--feature` | `string` | `convoai` | target feature readiness to evaluate: rtc, rtm, or convoai |

### `agora project env`

Export project environment variables

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--format` | `string` | — | output format: dotenv or shell; use --json for JSON output |
| `--project` | `string` | — | project ID or exact project name; defaults to the current project context |
| `--shell` | `bool` | — | render shell export statements instead of dotenv lines |
| `--with-secrets` | `bool` | — | include sensitive values such as the app certificate |

### `agora project env write`

Write project environment variables to a dotenv file

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--append` | `bool` | — | append Agora App ID and App Certificate values when no existing values are present |
| `--overwrite` | `bool` | — | replace the target file with only Agora App ID and App Certificate values |
| `--template` | `string` | — | credential key layout: nextjs or standard; if omitted, detect Next.js from the workspace |

### `agora project feature`

Manage project feature state

_No local flags. Inherited global flags still apply (see [Global Flags](#global-flags))._

### `agora project feature enable`

Enable one feature for a project

_No local flags. Inherited global flags still apply (see [Global Flags](#global-flags))._

### `agora project feature list`

List feature status for a project

_No local flags. Inherited global flags still apply (see [Global Flags](#global-flags))._

### `agora project feature status`

Show one feature status

_No local flags. Inherited global flags still apply (see [Global Flags](#global-flags))._

### `agora project list`

List projects available to the current account

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--keyword` | `string` | — | filter by exact or partial project name or project ID |
| `--page` | `int` | `1` | page number to request |
| `--page-size` | `int` | `20` | number of projects per page |

### `agora project show`

Show one project

_No local flags. Inherited global flags still apply (see [Global Flags](#global-flags))._

### `agora project use`

Set the current project context

_No local flags. Inherited global flags still apply (see [Global Flags](#global-flags))._

### `agora quickstart`

Clone official standalone Agora quickstarts

_No local flags. Inherited global flags still apply (see [Global Flags](#global-flags))._

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

_No local flags. Inherited global flags still apply (see [Global Flags](#global-flags))._

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
| `--show-all` | `bool` | — | include upcoming or unavailable templates in the list |
| `--verbose` | `bool` | — | show repository, runtime, and env details in pretty output |

### `agora telemetry`

Inspect or update telemetry preferences

_No local flags. Inherited global flags still apply (see [Global Flags](#global-flags))._

### `agora telemetry disable`

Disable telemetry

_No local flags. Inherited global flags still apply (see [Global Flags](#global-flags))._

### `agora telemetry enable`

Enable telemetry

_No local flags. Inherited global flags still apply (see [Global Flags](#global-flags))._

### `agora telemetry status`

Show telemetry status

_No local flags. Inherited global flags still apply (see [Global Flags](#global-flags))._

### `agora upgrade`

Upgrade Agora CLI in place when installer-managed; otherwise print upgrade guidance

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--check` | `bool` | — | resolve the latest release and report what would happen without writing anything |

### `agora version`

Show Agora CLI build information

_No local flags. Inherited global flags still apply (see [Global Flags](#global-flags))._

### `agora whoami`

Show the current auth status

| Flag | Type | Default | Description |
|------|------|---------|-------------|
| `--plain` | `bool` | — | print only authenticated or unauthenticated for shell scripts |

## Enums

**`features`**: `rtc`, `rtm`, `convoai`

**`outputModes`**: `pretty`, `json`

**`doctorStatus`**: `healthy`, `warning`, `not_ready`, `auth_error`

