---
title: Automation Contract
---

# Automation Contract

This document defines the machine-consumption contract for Agora CLI.

Use this guide for:
- CI jobs
- shell scripts
- agentic workflows
- editor or IDE integrations

## Contents

- [General Rules](#general-rules)
- [Project Resolution Precedence](#project-resolution-precedence)
- [Config Isolation](#config-isolation)
- [JSON Envelope](#json-envelope)
- [Progress Events (NDJSON Stream)](#progress-events-ndjson-stream)
- [Exit Codes](#exit-codes)
- [Stable Result Shapes](#stable-result-shapes)
- [Human vs Machine Output](#human-vs-machine-output)

## General Rules

- Prefer `--json` for any command consumed by code, scripts, or agents.
- Prefer `agora init` for end-to-end setup.
- Use low-level commands when a workflow must be decomposed, resumed, or partially re-run.
- Use `agora --help --all` to inspect the full command tree (human-readable).
- Use `agora --help --all --json` for a machine-readable command tree with all flags — the primary capability discovery mechanism for agents.
- Use `agora introspect --json` when you need command labels, enums, and version metadata in a single stable artifact.
- Use `agora project doctor --json` for readiness checks before continuing with automated setup.
- Use `agora whoami --plain` for shell `if` chains that only need `authenticated` / `unauthenticated`.
- In JSON mode, both success and failure return the same top-level envelope shape.
- Use `--json --pretty` when a human needs to inspect JSON directly. Scripts should keep the default single-line JSON.
- Use `--quiet` to suppress the success envelope in **both** pretty and JSON modes; the exit code becomes the only result. Errors are still printed on stderr (and as a JSON envelope on stdout when `--json` is set without `--quiet`). NDJSON progress events are still emitted because they are observability, not results.
- Use `--debug` (equivalent to `AGORA_DEBUG=1`) to echo structured log records to stderr. The flag does not change exit codes, JSON envelope shape, or NDJSON progress events; it only mirrors the entries that would normally be written to the log file. Pair with `--json` for fully machine-parseable runs that also surface internal events to your CI logs. v0.2.0 dropped the legacy `--verbose` / `-v` alias and the `AGORA_VERBOSE` env var; persisted configs that contain a `verbose` key are auto-promoted to `debug` on first load.
- Use `--yes` (or `-y`) / `AGORA_NO_INPUT=1` to assume the default answer to confirmation prompts. Following industry convention for `-y` (apt-style), the flag never starts brand-new interactive flows: in JSON, CI, or non-TTY contexts the CLI still fails fast with the same `AUTH_UNAUTHENTICATED` error you would have seen without `--yes`, instead of silently launching an OAuth browser flow.
- Interactive login prompts only appear in interactive pretty-mode TTY runs. Automation should authenticate up front with `agora login`; `--json`, `AGORA_OUTPUT=json`, detected CI environments, and non-TTY stdin all skip the prompt and fail with `AUTH_UNAUTHENTICATED`.
- Output mode precedence is: explicit CLI flag (`--json` or `--output`) first, user-set `AGORA_OUTPUT` second, then user-customized config file value, then **CI auto-detect → JSON** (see below), then pretty.
- Set `AGORA_AGENT=<tool-name>` in automated environments to explicitly label agent traffic in the API `User-Agent`. When unset, the CLI may infer a coarse label such as `cursor`, `claude-code`, `cline`, `windsurf`, `codex`, or `aider` from known agent environment markers. Set `AGORA_AGENT_DISABLE_INFER=1` to disable inference.
- Use `agora mcp serve` to expose local Agora CLI tools to MCP-capable agents. The full surface is exposed: `agora.version`, `agora.introspect`, `agora.auth.{status,logout}`, `agora.config.{path,get}`, `agora.telemetry.status`, `agora.upgrade.check`, `agora.project.{list,show,use,create,doctor,env,env_write}`, `agora.project.feature.{list,status,enable}`, `agora.quickstart.{list,create,env_write}`, and `agora.init`. Authentication is intentionally **not** exposed via MCP because OAuth requires an interactive browser; run `agora login` once on the host first.
- Use `agora open --target docs` for the human GitHub Pages docs and `agora open --target docs-md` for the agent-facing raw Markdown index. The Markdown tree is published under predictable `/md/` URLs, for example `/md/commands.md`, `/md/automation.md`, and `/md/error-codes.md`.
- Docs publishing reads `docs/site.env` for `CLI_DOCS_BASE_URL` and `CLI_DOCS_MD_BASE_URL`; staging Pages builds can override those environment variables at workflow time without changing docs content. The resolved values are published as `/docs.env` for transparency.
- The CLI maintains a short-lived on-disk completion cache for `agora project use <TAB>` under `<AGORA_HOME>/cache/projects.json`. The cache is only used for completions when a **local unexpired session exists** (`session.json` with a non-empty access token and a future `expiresAt`, when present), so Tab does not suggest stale project names after logout or local session expiry. The cache TTL is 5 minutes by default; override with `AGORA_PROJECT_CACHE_TTL_SECONDS=<seconds>` (set to `0` to disable). Cache files older than 24 h are pruned at every CLI startup. Set `AGORA_DISABLE_CACHE=1` to drop the cache on the next startup. The cache is invalidated automatically by `agora logout` and `agora project create` (the latter clears the file; it does not embed the new project until the next successful list fetch). To **force-refresh** the cached completion page, run `agora project list --refresh-cache` while authenticated; that command fetches the unfiltered first page used by completion and rewrites `projects.json` when it succeeds.

### CI auto-detect

When the CLI detects it is running inside a CI environment, it automatically:

- Switches the default output mode to `--output json` (so build logs stay machine-parseable).
- Suppresses the first-run "Config initialized" banner on stderr.
- Skips interactive prompts (login confirmation, project reuse confirmation, template picker).

CI is detected when **any** of the following environment variables is present (and `CI` is not literally `false`/`0`/empty):

| Variable          | Vendor             |
| ----------------- | ------------------ |
| `CI`              | de-facto universal |
| `GITHUB_ACTIONS`  | GitHub Actions     |
| `GITLAB_CI`       | GitLab CI          |
| `BUILDKITE`       | Buildkite          |
| `CIRCLECI`        | CircleCI           |
| `JENKINS_URL`     | Jenkins            |
| `TF_BUILD`        | Azure Pipelines    |

You can always override:

- `--output pretty` — force pretty output even in CI.
- `AGORA_OUTPUT=pretty` — same, via env (user-set values always win over auto-detect).
- `AGORA_DISABLE_CI_DETECT=1` — disable CI detection entirely (useful when iterating on a CI script locally).

Primary command groups:
- `init`
- `quickstart`
- `project`
- `auth`
- `config`

## Project Resolution Precedence

Commands that require a project resolve context in this order:
1. explicit `--project` or positional project argument
2. repo-local `.agora/project.json` from the target repo path
3. global CLI context selected by `agora project use`

Agent guidance:
- prefer explicit `--project` for deterministic cross-repo operations
- rely on repo-local binding when operating repeatedly inside one bound quickstart
- keep `metadataPath` from command results if you need to validate or audit project bindings

## Config Isolation

The CLI creates or migrates its config directory on startup. In CI, tests, and multi-agent runs, isolate config and session state with `AGORA_HOME` so concurrent jobs do not mutate a shared developer profile:

```bash
export AGORA_HOME="$(mktemp -d)"
agora auth status --json
agora init my-nextjs-demo --template nextjs --project my-project --json
```

`AGORA_HOME` points directly at the Agora CLI config directory. `XDG_CONFIG_HOME` is also supported, but `AGORA_HOME` is the most explicit option for short-lived automation.

## JSON Envelope

Commands that support structured output return a JSON envelope in this shape:

```json
{
  "ok": true,
  "command": "init",
  "data": {},
  "meta": {
    "outputMode": "json",
    "exitCode": 0
  }
}
```

Stable top-level fields:
- `ok`
  `true` for success and `false` for failure.
- `command`
  Stable command label used by the CLI for the result payload.
- `data`
  Command-specific result payload. This is usually `null` on failure. `project doctor --json` keeps its diagnostic payload on readiness failures so agents can inspect blocking issues while still branching on `ok: false`.
- `error`
  Present on failure with a stable error object.
  Known structured failures may include `error.code`, `error.httpStatus`, and `error.requestId` in addition to `error.message`.
- `meta.outputMode`
  Currently `json` when `--json` is used.
- `meta.exitCode`
  Process exit code for this result. Success envelopes use `0`; error envelopes use the nonzero process exit code.

Agent guidance:
- branch on `command` and `data`
- branch on `ok` first for success vs failure
- treat pretty output as human-only
- do not parse stderr when `--json` is in use

A **JSON Schema** for this envelope is published at
[`docs/schema/envelope.v1.json`](schema/envelope.v1.json) (also available
at the live URL `https://agoraio.github.io/cli/schema/envelope.v1.json`).
Wrappers that want compile-time type safety can generate types from the
schema with `quicktype`, `datamodel-code-generator`, or any
JSON-Schema-aware tool.

### One documented exception: `agora project env`

`agora project env` is the only command whose **default** (non-JSON)
output is raw stdout — without the unified envelope — so it can be used
with shell substitution:

```bash
source <(agora project env --shell)
eval "$(agora project env --format shell)"
```

To explicitly request a specific format, pass `--format`:

| Flag                  | Output                                    |
|-----------------------|-------------------------------------------|
| `--format dotenv`     | `KEY=value` lines (default; `>> .env`)    |
| `--format shell`      | shell `export KEY=value` statements       |
| `--format envelope`   | unified JSON envelope (alias of `--json`) |
| `--format json`       | same as `--format envelope`               |
| `--shell`             | back-compat alias of `--format shell`     |
| `--json`              | unified JSON envelope                     |

For automation, prefer `--json` (or `--format envelope`) so the result
has the same shape as every other command. `agora project env write`
already emits the unified envelope under all output modes — only the
read path has the raw-stdout exception above.

## Progress Events (NDJSON Stream)

Long-running commands emit one or more **progress events** to stdout ahead of the final envelope when `--json` is set. The wire format is **NDJSON** (newline-delimited JSON): one complete JSON object per line, terminated by `\n`.

Agents must therefore parse stdout line-by-line, not as a single JSON document. The terminal envelope is always the **last** line and is the only object with the top-level `ok` field.

### Event shape

```json
{"event":"progress","command":"init","stage":"clone:start","message":"Cloning quickstart repository","timestamp":"2026-04-29T22:30:00.123Z","repoUrl":"https://github.com/AgoraIO/...","targetPath":"/abs/path","ref":""}
```

Stable top-level fields on every progress event:

- `event` — always the literal string `"progress"`. Use this to distinguish progress events from the terminal envelope (which has `"ok"` instead).
- `command` — the same stable command label used by the terminal envelope.
- `stage` — a stable enum-like string identifying the milestone. See the table below.
- `message` — short human-readable description suitable for a log line.
- `timestamp` — RFC3339Nano UTC timestamp.

Additional stage-specific fields may appear (for example `repoUrl`, `projectId`, `loginUrl`). These are documented per-stage but agents should treat unknown extra fields as opaque metadata.

### Stable stage taxonomy

| Stage | Emitted by | When | Stage-specific fields |
|-------|------------|------|------------------------|
| `clone:start` | `quickstart create`, `init` | Before the `git clone` shell-out | `repoUrl`, `targetPath`, `ref` |
| `clone:complete` | `quickstart create`, `init` | After `git clone` succeeds | `targetPath` |
| `project:create` | `init` | Before creating a new Agora project | `projectName`, `features` |
| `project:created` | `init` | After the project is ready | `projectId`, `projectName` |
| `project:reuse` | `init` | When binding to an existing project | `projectId`, `projectName` |
| `oauth:waiting` | `login`, `auth login` | After the localhost callback server is listening and the URL is printed | `loginUrl`, `redirectUri`, `timeoutMs` |
| `oauth:received` | `login`, `auth login` | When the browser callback delivers an authorization code | (none) |
| `oauth:complete` | `login`, `auth login` | After the access token is stored locally | (none) |

### Sample stream (init)

```text
{"event":"progress","command":"init","stage":"project:reuse","message":"Reusing existing Agora project","timestamp":"...","projectId":"prj_abc","projectName":"Default Project"}
{"event":"progress","command":"init","stage":"clone:start","message":"Cloning quickstart repository","timestamp":"...","repoUrl":"...","targetPath":"...","ref":""}
{"event":"progress","command":"init","stage":"clone:complete","message":"Quickstart repository cloned","timestamp":"...","targetPath":"..."}
{"ok":true,"command":"init","data":{},"meta":{"exitCode":0,"outputMode":"json"}}
```

### Parsing rules for agents

1. Read stdout line by line until EOF.
2. JSON-decode each line independently.
3. Discard or log lines that do not parse as JSON (defensive).
4. The line where `event === "progress"` is a progress event; `stage` and `message` are the most useful fields.
5. The first (and only) line where `ok` is set is the terminal envelope.
6. Process exit code is also reported in `meta.exitCode` of the terminal envelope; treat the OS exit code as the source of truth.

Stages are stable identifiers; new stages may be added over time. Agents should treat unknown stages as benign progress information rather than failing on them.

### MCP transport caveat: progress events collapse into the tool result

When the same long-running commands (`agora init`, `agora quickstart create`, `agora project create`, `agora login`) are invoked **through the MCP server** (`agora mcp serve`), the NDJSON progress event stream is **not** surfaced to the MCP client. MCP clients receive the final result payload only — JSON-stringified into the standard `content[0].text` slot of the `tools/call` response.

Why: MCP's stdio transport uses stdout for JSON-RPC framing, so any extra newline-delimited objects emitted before the final response would corrupt the transport. The progress events are still produced internally but are intentionally swallowed before they reach the client.

Implications for agent authors:

- Do not rely on `stage`-based hooks (e.g. progress UIs, mid-flight cancellation prompts) when calling these tools over MCP. Use them only when shelling out to `agora ... --json` directly.
- Plan for higher tail latency on MCP `tools/call` for the affected tools; the user-visible "nothing is happening" gap may be tens of seconds for git clones or project creation. Consider surfacing your own "running tool: agora.init…" UI in the host agent.
- The terminal payload shape returned in `content[0].text` is identical to the CLI's terminal `data` envelope, so existing JSON-shape handlers continue to work unchanged.

When future MCP transports support server-initiated `notifications/progress` (an open spec area), the CLI can add native MCP progress forwarding without changing the existing tool result shape.

Failure example:

```json
{
  "ok": false,
  "command": "project env write",
  "data": null,
  "error": {
    "message": "path/to/.env.custom already exists. Use --append to append it or --overwrite to replace it.",
    "logFilePath": "/path/to/agora-cli.log"
  },
  "meta": {
    "outputMode": "json",
    "exitCode": 1
  }
}
```

## Exit Codes

| Code | Meaning | Commands |
|------|---------|----------|
| 0 | Success | all commands |
| 1 | General error or blocking issue | most commands; `project doctor` when blocking issues found |
| 2 | Non-blocking warning | `project doctor` when only warnings found |
| 3 | Auth or session error | `project doctor` when not authenticated; `auth status` / `whoami` when unauthenticated |

In JSON mode the `meta.exitCode` field carries the same value as the process exit code.

Known `error.code` values are cataloged in [error-codes.md](error-codes.md).

## Stable Result Shapes

The following commands are part of the documented JSON contract.

### `init`

Example:

```bash
./agora init my-nextjs-demo --template nextjs --json
./agora init my-nextjs-demo --template nextjs --new-project --json
```

By default `init` reuses an existing project — preferring one named exactly `"Default Project"`. If no default exists, interactive sessions show existing projects with a create-new option and default to the most recently created project; JSON, CI, and non-TTY runs select the most recent project automatically. Pass `--new-project` to force creation. Use `--project <name|id>` to bind to a specific project.

Required `data` fields:
- `action`
  Always `init`.
- `template`
  Template ID such as `nextjs`, `python`, or `go`.
- `projectAction`
  `created` or `existing`.
- `reusedExistingProject`
  Boolean mirror for agents that branch on init reuse.
- `projectId`
- `projectName`
- `region`
- `path`
  Absolute path to the cloned quickstart.
- `envPath`
  Path of the env file relative to the cloned quickstart root.
- `metadataPath`
  Repo-local project binding file path, currently `.agora/project.json`.
- `enabledFeatures`
  Array of features enabled during this run. Defaults to `rtc`, `rtm`, and `convoai` for newly created projects unless overridden with `--feature`. Empty for existing projects since the CLI did not create them in this run.
- `nextSteps`
  Ordered list of suggested follow-up commands for the selected template.
- `status`
  Currently `ready`.

Optional fields:
- `rtmDataCenter`
  RTM data center configured on the new project when RTM was enabled. Defaults to `NA` when `--rtm-data-center` is omitted.

Display-oriented fields:
- `title`

Safe branch fields:
- `template`
- `projectAction`
- `projectId`
- `path`
- `envPath`
- `status`

### `project create`

Automation notes:
- `--dry-run` returns the planned envelope (`status: "planned"`, `dryRun: true`) without creating remote resources.
- `--idempotency-key <key>` is forwarded to the API body for retry-safe project creation where supported.

Example:

```bash
./agora project create my-agent-demo --json
./agora project create my-agent-demo --rtm-data-center EU --json
./agora project create my-agent-demo --feature rtc --feature convoai --json
```

Required `data` fields (success):
- `action`
  Always `create`.
- `projectId`
- `projectName`
- `appId`
- `region`
- `enabledFeatures`
  Array of features that were enabled on the new project. Defaults to `["rtc", "rtm", "convoai"]` when no `--feature` flags are passed. Explicit `convoai` requests also include `rtm`.

Optional fields:
- `rtmDataCenter`
  RTM data center configured when RTM was enabled. Defaults to `NA` when `--rtm-data-center` is omitted.

Required `data` fields (`--dry-run`):
- `action`
  Always `create`.
- `dryRun`
  Always `true`.
- `status`
  Always `planned`.
- `projectName`
- `region`
- `enabledFeatures`
- `template`
  Project preset that would be applied (empty when not requested).
- `idempotencyKey`
  Echoes the caller-provided `--idempotency-key` value (empty when not provided).

Optional fields (`--dry-run`):
- `rtmDataCenter`
  Uppercased data center value (`CN`, `NA`, `EU`, or `AP`) when RTM is enabled. Defaults to `NA` when omitted.

Safe branch fields:
- `projectId` (success only)
- `projectName`
- `region`
- `enabledFeatures`
- `status` (`--dry-run` only)
- `dryRun` (`--dry-run` only)

### `project use`

Example:

```bash
./agora project use my-agent-demo --json
```

Required `data` fields:
- `action`
  Always `use`.
- `projectId`
- `projectName`
- `region`
- `status`
  Currently `selected`.

Safe branch fields:
- `projectId`
- `projectName`
- `region`
- `status`

### `project show`

Example:

```bash
./agora project show --json
```

Required `data` fields:
- `action`
  Always `show`.
- `projectId`
- `projectName`
- `appId`
- `region`
- `tokenEnabled`

Optional fields:
- `appCertificate`
  The app certificate (signing key). Present when the project has one configured. Sensitive — redacted in pretty output; available in JSON mode.

Display-oriented fields:
- `appCertificate`

Safe branch fields:
- `projectId`
- `projectName`
- `appId`
- `region`
- `tokenEnabled`

### `project env write`

Example:

```bash
./agora project env write apps/web/.env.local --json
```

Optional `data` fields:
- `credentialLayout`
  Either `standard` (AGORA_* keys) or `nextjs` (`NEXT_PUBLIC_AGORA_APP_ID` and `NEXT_AGORA_APP_CERTIFICATE`) when the workspace is detected or overridden as Next.js.

Required `data` fields:
- `action`
  Always `env-write`.
- `projectId`
- `projectName`
- `path`
  Absolute path to the written dotenv file.
- `projectType`
  Detected workspace type used for future repo metadata (`nextjs`, `go`, `python`, `node`, or `standard`).
- `status`
  One of `created`, `updated`, `appended`, or `overwritten`.
- `keysWritten`
  Ordered list of credential keys that were written. By default `project env write` uses `AGORA_APP_ID` and `AGORA_APP_CERTIFICATE`. Next.js workspaces (detected via `package.json` / `next.config.*` / `env.local.example` / repo `.agora` `projectType` / `template: nextjs`, or forced with `--template nextjs`) use `NEXT_PUBLIC_AGORA_APP_ID` and `NEXT_AGORA_APP_CERTIFICATE` instead. Non-secret project metadata stays in `.agora/project.json`.

Optional `data` fields (present when the CLI updates or creates repo metadata):
- `metadataUpdated`
  `true` when `.agora/project.json` was created or updated for the selected project (including `projectType` and `envPath` when missing).
- `metadataPath`
  Relative path `.agora/project.json` from the repo root when `metadataUpdated` is true.

Pass `--template standard` to force AGORA_* keys when auto-detection would pick Next.js.

Write behavior:
- existing `.env` and `.env.local` files are preserved; missing credential keys are appended and existing credential keys are updated
- existing legacy Agora-managed blocks are replaced with plain credential assignments
- duplicate credential assignments are commented out after the first updated key
- explicit non-standard env files still require `--append` or `--overwrite` when they do not already contain managed credentials

Safe branch fields:
- `path`
- `status`
- `keysWritten`
- `credentialLayout`
- `projectType`
- `metadataUpdated` (when repo binding was updated)
- `metadataPath` (when `metadataUpdated` is true)

### `project env`

Example:

```bash
./agora project env --json
```

Required `data` fields:
- `action`
  Always `env`.
- `format`
  Currently `json`.
- `projectId`
- `projectName`
- `region`
- `values`
  Object containing the rendered env key/value pairs.

Safe branch fields:
- `projectId`
- `projectName`
- `region`
- `values`

### `quickstart list`

Example:

```bash
./agora quickstart list --json
```

Required `data` fields:
- `action`
  Always `list`.
- `items`
  Array of template objects.

Each item currently includes:
- `id`
- `title`
- `description`
- `runtime`
- `repoUrl`
- `docsUrl`
- `available`
- `envDocs`
- `supportsInit`

Safe branch fields:
- `items[].id`
- `items[].runtime`
- `items[].repoUrl`
- `items[].available`
- `items[].supportsInit`

Display-oriented fields:
- `title`
- `description`
- `docsUrl`
- `envDocs`

### `quickstart create`

Automation notes:
- `--ref <branch|tag|ref>` pins the cloned quickstart source for workshops and reproducible demos.

Example:

```bash
./agora quickstart create my-python-demo --template python --project my-project --json
```

Required `data` fields:
- `action`
  Always `create`.
- `template`
- `title`
- `runtime`
- `cloneUrl`
- `docsUrl`
- `path`
  Absolute path to the cloned quickstart.
- `envStatus`
  `template-only` or `configured`.
- `envPath`
  Empty when no project was bound during creation.
- `metadataPath`
  `.agora/project.json` when the quickstart was bound to a project during creation.
- `status`
  Currently `cloned`.
- `written`
  Files or managed outputs written by the command.

Optional fields:
- `projectId`
- `projectName`

Safe branch fields:
- `template`
- `path`
- `envStatus`
- `envPath`
- `status`
- `projectId`

### `quickstart env write`

Example:

```bash
./agora quickstart env write /abs/path/to/my-python-demo --json
```

Required `data` fields:
- `action`
  Always `env-write`.
- `template`
- `title`
- `path`
  Absolute path to the quickstart root.
- `envPath`
  Env file path relative to the quickstart root.
- `metadataPath`
  Repo-local project binding file path, currently `.agora/project.json`.
- `projectId`
- `projectName`
- `status`
  Currently `created`, `updated`, or `appended`.

Env write behavior:
- quickstart env files contain only the App ID and App Certificate variable names required by the template
- Next.js uses `NEXT_PUBLIC_AGORA_APP_ID` and `NEXT_AGORA_APP_CERTIFICATE`
- Python and Go use `APP_ID` and `APP_CERTIFICATE`
- project metadata such as project ID, project name, region, template, projectType, and env path is stored in `.agora/project.json`
- existing quickstart env files are preserved; missing credential keys are appended and existing credential keys are updated
- stale Agora credential aliases for another runtime are commented out to avoid ambiguous dotenv resolution; for example, a Next.js quickstart prefers `NEXT_PUBLIC_AGORA_APP_ID` and comments out old `AGORA_APP_ID` / `APP_ID` entries when replacing them

Safe branch fields:
- `template`
- `path`
- `envPath`
- `projectId`
- `projectName`
- `status`

### `project doctor`

Example:

```bash
./agora project doctor --json
```

Required `data` fields:
- `action`
  Always `doctor`.
- `healthy`
- `mode`
  `default` or `deep`.
- `status`
  One of `healthy`, `warning`, `not_ready`, or `auth_error`.
- `summary`
- `checks`
  Array of category objects.
- `blockingIssues`
  Array of blocking issue objects.
- `warnings`
  Array of warning issue objects.

Optional fields:
- `project`
  Nil during auth or project-selection failure paths.
- `workspace`
  Present in deep mode with repo-local binding and env consistency details.

Safe branch fields:
- `healthy`
- `mode`
- `status`
- `summary`
- `blockingIssues`
- `warnings`

Recommended agent behavior:
- branch first on `status`
- use `healthy` as a fast readiness boolean
- inspect `blockingIssues[].suggestedCommand` for recovery suggestions
- for repo-bound validation, run `project doctor --deep --json`

### `auth status`

Examples:

```bash
./agora auth status --json
./agora whoami --json
```

When authenticated, this command returns a success envelope with these required `data` fields:
- `action`
  Always `status`.
- `authenticated`
- `status`
  `authenticated`.
- `expiresAt`
- `scope`

Safe branch fields:
- `authenticated`
- `status`
- `expiresAt`

When unauthenticated, this command returns exit code `3` and an error envelope:

```json
{
  "ok": false,
  "command": "auth status",
  "data": null,
  "error": {
    "message": "No local Agora session found. Run `agora login` first.",
    "code": "AUTH_UNAUTHENTICATED",
    "logFilePath": "/path/to/agora-cli.log"
  },
  "meta": {
    "outputMode": "json",
    "exitCode": 3
  }
}
```

Agent guidance:
- branch on `ok` before reading `data`
- handle `error.code == "AUTH_UNAUTHENTICATED"` as the unauthenticated state
- run `agora login` before continuing with commands that require a session

### `auth login`

Example:

```bash
./agora login --json
./agora auth login --json
```

Required `data` fields:
- `action`
  Always `login`.
- `status`
  Currently `authenticated`.
- `scope`
- `expiresAt`

Safe branch fields:
- `status`
- `expiresAt`

### `auth logout`

Example:

```bash
./agora logout --json
./agora auth logout --json
```

Required `data` fields:
- `action`
  Always `logout`.
- `status`
  Currently `logged-out`.
- `clearedSession`
  `true` if a session file was removed; `false` if no session existed.

Safe branch fields:
- `status`
- `clearedSession`

### `project list`

Example:

```bash
./agora project list --json
./agora project list --keyword demo --page 2 --json
./agora project list --refresh-cache --json
```

Required `data` fields:
- `items`
  Array of project summary objects.
- `page`
  Current page number (1-based).
- `pageSize`
  Number of items per page.
- `total`
  Total number of matching projects across all pages.
- `cacheRefreshed`
  Boolean. `true` only when `--refresh-cache` successfully refreshed the unfiltered first-page project completion cache.

Each item includes: `projectId`, `name`, `appId`, `projectType`, `status`, `region`, `createdAt`, `updatedAt`.

Safe branch fields:
- `items[].projectId`
- `items[].name`
- `total`
- `page`
- `pageSize`

### `project feature list`

Example:

```bash
./agora project feature list --json
./agora project feature list my-project --json
```

Required `data` fields:
- `action`
  Always `feature-list`.
- `projectId`
- `projectName`
- `items`
  Array of feature status objects.

Each item includes: `feature` (one of `rtc`, `rtm`, `convoai`), `status` (one of `enabled`, `disabled`, `included`, `provisioning`), `message`.

Safe branch fields:
- `projectId`
- `items[].feature`
- `items[].status`

### `project feature status`

Example:

```bash
./agora project feature status convoai --json
```

Required `data` fields:
- `action`
  Always `feature-status`.
- `feature`
- `status`
  One of `enabled`, `disabled`, `included`, `provisioning`.
- `message`
- `projectId`
- `projectName`

Safe branch fields:
- `feature`
- `status`
- `projectId`

### `project feature enable`

Example:

```bash
./agora project feature enable convoai --json
```

Required `data` fields:
- `action`
  Always `feature-enable`.
- `feature`
- `status`
  One of `enabled`, `included`.
- `message`
- `projectId`
- `projectName`

Safe branch fields:
- `feature`
- `status`
- `projectId`

### `config path`

Example:

```bash
./agora config path --json
```

Required `data` fields:
- `path`
  Absolute path to the config file on disk.

Safe branch fields:
- `path`

### `config get`

Example:

```bash
./agora config get --json
```

Returns the current resolved config object. Safe branch fields:
- `apiBaseUrl`
- `oauthBaseUrl`
- `output`
- `logLevel`
- `browserAutoOpen`
- `telemetryEnabled`
- `debug` (renamed from legacy `verbose` in v0.2.0; legacy key is migrated on first load)

### `config update`

Example:

```bash
./agora config update --output json --json
./agora config update --telemetry-enabled=false --json
```

Returns the updated config object with the same shape as `config get`. Safe branch fields are the same as `config get`.

### `upgrade`

Example:

```bash
./agora upgrade --json
./agora upgrade --check --json
./agora --upgrade-check --json
```

Required `data` fields:
- `action`
  `upgrade` for the subcommand and `upgrade-check` for the root `--upgrade-check` pseudo-command.
- `installMethod`
  One of `installer`, `npm`, `homebrew`, `scoop`, `chocolatey`, `winget`, or `unknown`.
- `installSource`
  `install.sh` / `install.ps1` when read from a valid direct-installer receipt, `path` when inferred from the resolved executable path, or `fallback` when no durable source was available.
- `installedPath`
  Resolved executable path used for receipt validation and path inference.
- `upgradeCommand`
  The user-facing command for the owning install channel.
- `command`
  Backwards-compatible alias for `upgradeCommand`.
- `status`
  One of `manual`, `dry-run`, `up-to-date`, or `upgraded`.

Optional fields:
- `receiptPath`
  Path to the validated `agora.install.json` receipt when present.
- `currentVersion`
  Version of the running binary for `agora upgrade`.
- `latestVersion`
  Latest resolved release version when the command resolves GitHub release metadata.
- `version`
  Structured version payload for `agora --upgrade-check`.
- `receiptWarning`
  Nonfatal warning when a direct-installer self-update succeeded but the CLI could not refresh `agora.install.json`.

Upgrade behavior:
- direct-installer installs (`installMethod: "installer"`) self-update in place after downloading the GitHub release archive and verifying it against `checksums.txt`
- package-manager installs return `status: "manual"` with the owning package-manager command; agents should run that command only after user approval
- `unknown` means the CLI could not verify the install channel, usually because the binary is a development/test build

Safe branch fields:
- `installMethod`
- `installSource`
- `upgradeCommand`
- `status`

## Human vs Machine Output

- Pretty output is optimized for humans.
- JSON output is the supported machine-readable contract.
- For reliable automation, do not parse help text or pretty output.
- Use `--output pretty` to force human output when `AGORA_OUTPUT=json` is set in the environment.

Recommended pattern:

```bash
./agora project doctor --json
./agora init my-go-demo --template go --json
./agora quickstart env write my-go-demo --json
```
