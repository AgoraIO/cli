# Automation Contract

This document defines the machine-consumption contract for `agora-cli-go`.

Use this guide for:
- CI jobs
- shell scripts
- agentic workflows
- editor or IDE integrations

## General Rules

- Prefer `--json` for any command consumed by code, scripts, or agents.
- Prefer `agora init` for end-to-end setup.
- Use low-level commands when a workflow must be decomposed, resumed, or partially re-run.
- Use `agora --help --all` to inspect the full command tree (human-readable).
- Use `agora --help --all --json` for a machine-readable command tree with all flags — the primary capability discovery mechanism for agents.
- Use `agora project doctor --json` for readiness checks before continuing with automated setup.
- In JSON mode, both success and failure return the same top-level envelope shape.

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

## JSON Envelope

Commands that support structured output return a JSON envelope in this shape:

```json
{
  "ok": true,
  "command": "init",
  "data": {},
  "meta": {
    "outputMode": "json"
  }
}
```

Stable top-level fields:
- `ok`
  `true` for success and `false` for failure.
- `command`
  Stable command label used by the CLI for the result payload.
- `data`
  Command-specific result payload. This is `null` on failure.
- `error`
  Present on failure with a stable error object.
- `meta.outputMode`
  Currently `json` when `--json` is used.
- `meta.exitCode`
  Present on failures to indicate the process exit code.

Agent guidance:
- branch on `command` and `data`
- branch on `ok` first for success vs failure
- treat pretty output as human-only
- do not parse stderr when `--json` is in use

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

In JSON mode the `meta.exitCode` field in error envelopes carries the same value.

## Stable Result Shapes

The following commands are part of the documented JSON contract.

### `init`

Example:

```bash
./agora init my-nextjs-demo --template nextjs --json
```

Required `data` fields:
- `action`
  Always `init`.
- `template`
  Template ID such as `nextjs`, `python`, or `go`.
- `projectAction`
  `created` or `existing`.
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
  Array of features enabled during this run. Defaults to `rtc` and `convoai` for newly created projects unless overridden with `--feature`. Empty for existing projects since the CLI did not create them in this run.
- `nextSteps`
  Ordered list of suggested follow-up commands for the selected template.
- `status`
  Currently `ready`.

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

Example:

```bash
./agora project create my-agent-demo --feature rtc --feature convoai --json
```

Required `data` fields:
- `action`
  Always `create`.
- `projectId`
- `projectName`
- `appId`
- `region`
- `enabledFeatures`

Safe branch fields:
- `projectId`
- `projectName`
- `region`
- `enabledFeatures`

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

Required `data` fields:
- `action`
  Always `env-write`.
- `projectId`
- `projectName`
- `path`
  Absolute path to the written dotenv file.
- `status`
  One of `created`, `updated`, `appended`, or `overwritten`.
- `keysWritten`
  Ordered list of managed keys that were written.

Safe branch fields:
- `path`
- `status`
- `keysWritten`

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
  Currently `created` or `updated`.

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

Required `data` fields:
- `action`
  Always `status`.
- `authenticated`
- `status`
  `authenticated` or `unauthenticated`.
- `expiresAt`
  May be null when unauthenticated.
- `scope`
  May be null when unauthenticated.

Safe branch fields:
- `authenticated`
- `status`
- `expiresAt`

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
- `verbose`

### `config update`

Example:

```bash
./agora config update --output json --json
./agora config update --telemetry-enabled=false --json
```

Returns the updated config object with the same shape as `config get`. Safe branch fields are the same as `config get`.

## Human vs Machine Output

- Pretty output is optimized for humans.
- JSON output is the supported machine-readable contract.
- For reliable automation, do not parse help text or pretty output.

Recommended pattern:

```bash
./agora project doctor --json
./agora init my-go-demo --template go --json
./agora quickstart env write my-go-demo --json
```
