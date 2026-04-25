# Agora CLI Go

`agora-cli-go` is the native Go CLI for managing Agora authentication, remote projects, official quickstarts, and end-to-end onboarding workflows.

It supports both:
- high-level setup with `init`
- explicit low-level workflows with `project` and `quickstart`

## What This CLI Is

Use this CLI when you want to:
- authenticate with Agora Console
- create and manage remote Agora projects
- clone official quickstart repositories for Next.js, Python, or Go
- inject the correct env file for a selected Agora project
- automate onboarding flows with stable JSON output

The command model is intentionally layered:
- `init` for the recommended onboarding path
- `quickstart` for standalone starter repos
- `project` for remote Agora resources and env export
- `auth` for login and session inspection
- `config` for local CLI defaults
- `add` hidden and reserved for future in-place integrations into an existing codebase

## Install / Build

Build the native binary:

```bash
go build -o agora .
```

Runtime dependencies:
- `git` is required for `quickstart create` and `init`
- the cloned quickstart may require `pnpm`, `bun`, `go`, or template-specific tooling to install and run locally

Discover the full command tree:

```bash
./agora --help
./agora --help --all
```

## Quick Start

The primary onboarding path is `init`.

`init` does all of the following:
- creates a new Agora project by default
- enables the default features `rtc` and `convoai`
- clones the selected quickstart
- writes the framework-specific env file
- sets the selected project as the current CLI context

Examples:

```bash
./agora login
./agora init my-nextjs-demo --template nextjs
```

```bash
./agora init my-python-demo --template python
```

```bash
./agora init my-go-demo --template go
```

Use `--project` when you want to bind a quickstart to an existing Agora project instead of creating a new one:

```bash
./agora init my-nextjs-demo --template nextjs --project my-existing-project
```

## Command Model

### `init`

Recommended onboarding command. It creates or binds a project, clones a quickstart, writes env, persists context, and prints next steps.

### `quickstart`

Manages standalone official starter repos and their runtime-specific env files.

Use this when you want to:
- clone a quickstart without creating a project
- bind a quickstart to an existing project
- re-sync env files after changing project selection

### `project`

Manages remote Agora project resources.

Use this when you want to:
- create or inspect projects directly
- switch the default project context
- export project env values
- inspect project readiness with `project doctor`

### `auth`

Handles login, logout, and current session inspection.

### `config`

Reads and updates local CLI defaults such as output mode, log level, and browser behavior.

### `add`

Hidden and reserved for future in-place integrations into an existing application. It is intentionally not part of the normal help surface today.

## Common Workflows

### Onboard a new demo

```bash
./agora login
./agora init my-nextjs-demo --template nextjs
```

### Use an existing project with a quickstart

```bash
./agora quickstart create my-go-demo --template go --project my-existing-project
./agora quickstart env write my-go-demo --project my-existing-project
```

### Update env after changing projects

```bash
./agora project use my-agent-demo
./agora quickstart env write my-go-demo
```

### Inspect project readiness

```bash
./agora project doctor
./agora project doctor --json
```

### Use low-level commands directly

```bash
./agora project create my-agent-demo --feature rtc --feature convoai
./agora quickstart create my-go-demo --template go --project my-agent-demo
./agora quickstart env write my-go-demo --project my-agent-demo
```

### Inspect the full command tree

```bash
./agora --help --all
```

## Quickstart Env Conventions

`quickstart env write` is different from `project env write`.

- `project env write` writes a generic Agora-managed env block to any dotenv file you specify
- `quickstart env write` understands the quickstart type and writes the runtime-specific env file the cloned repo expects

Template-specific behavior:
- Next.js writes `.env.local` and uses `NEXT_PUBLIC_AGORA_APP_ID` plus `NEXT_AGORA_APP_CERTIFICATE`
- Python writes `server-python/.env.local` and uses `APP_ID` plus `APP_CERTIFICATE`
- Go writes `server-go/.env.local` and uses `APP_ID` plus `APP_CERTIFICATE`

The generated quickstart env block also includes project metadata as comments:
- `# Project ID: ...`
- `# Project Name: ...`

The CLI also writes repo-local project metadata to:
- `.agora/project.json`

That allows the CLI to detect which Agora project a cloned demo is bound to even when you are working inside the repo later.

## Automation / Agent Usage

For scripts, CI, and agentic workflows:
- prefer `--json` for machine consumption
- prefer `init` for end-to-end setup
- use low-level commands when the workflow must be decomposed or resumed in stages
- use `./agora --help --all` to inspect the full command tree
- use `quickstart env write` to re-sync env files after changing project selection
- use `project doctor --json` for readiness checks
- rely on the same JSON envelope for both success and failure

Examples:

```bash
./agora init my-nextjs-demo --template nextjs --json
./agora quickstart create my-python-demo --template python --project my-project --json
./agora quickstart env write my-python-demo --json
./agora project doctor --json
./agora auth status --json
```

The JSON envelope and stable result shapes are documented in [docs/automation.md](docs/automation.md).

## CI and Releases

GitHub Actions are configured for:
- push and pull request validation on Linux, macOS, and Windows
- automated tag-driven releases for `v*` tags
- cross-platform release artifacts for Linux, macOS, and Windows

Release workflow behavior:
- a pushed tag like `v0.1.4` triggers the release workflow
- the workflow runs tests, builds release binaries, packages them, and publishes a GitHub release automatically
- release artifacts include checksums

## Configuration

The CLI stores config, session, context, and logs under the Agora CLI config directory for the current machine.

Useful commands:

```bash
./agora config path
./agora config get
```

Built-in default config values are documented in [config.example.json](config.example.json).

## Troubleshooting

### Login or browser issues

Try:

```bash
./agora login --no-browser
```

You can also inspect the current auth state:

```bash
./agora whoami
```

### `git` is missing

`quickstart create` and `init` shell out to `git clone`. Install `git` and retry.

### Quickstart clone failures

Check:
- network access to GitHub
- that the target directory does not already exist
- that the quickstart repo URL is reachable

### Missing app certificate for env injection

Quickstart env injection requires a project with an app certificate. If the selected project has no certificate, `quickstart create --project`, `quickstart env write`, and `init` cannot seed the env file.

### No project selected

If a command needs a project and none is currently selected, either:

```bash
./agora project use my-project
```

or pass `--project` explicitly:

```bash
./agora quickstart env write my-go-demo --project my-project
```

## Migration

This project mirrors the `agora-cli-ts` command surface in a native Go binary so the CLI no longer depends on the Node.js runtime.
