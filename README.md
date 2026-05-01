# Agora CLI

[![CI](https://github.com/AgoraIO/cli/actions/workflows/ci.yml/badge.svg)](https://github.com/AgoraIO/cli/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/AgoraIO/cli?label=release)](https://github.com/AgoraIO/cli/releases)
[![npm](https://img.shields.io/npm/v/agoraio-cli?label=npm)](https://www.npmjs.com/package/agoraio-cli)
[![License](https://img.shields.io/github/license/AgoraIO/cli)](LICENSE)

Native Agora CLI for authentication, project management, quickstart setup, and developer onboarding. Use it to go from an Agora account to a runnable app with one command.

```bash
agora login
agora init my-nextjs-demo --template nextjs
```

## What You Can Build Quickly

| Goal | Command | What You Get |
|------|---------|--------------|
| Next.js video app | `agora init my-nextjs-demo --template nextjs` | A cloned Next.js quickstart, project binding, and `.env.local` |
| Python voice agent | `agora init my-python-demo --template python` | A Python quickstart with Agora credentials written for the backend |
| Go token service | `agora init my-go-demo --template go` | A Go server quickstart with project metadata and env wiring |

## Install

### Requirements

- macOS 12+, Linux (glibc 2.31+ or musl), or Windows 10+ for the prebuilt binaries.
- `git` on `PATH` for `agora init` and `agora quickstart create` (they shell out to `git clone`).
- For the npm install path, Node.js 18 or newer.
- For the source build, the Go toolchain pinned in [`go.mod`](go.mod).

### Install the CLI

```bash
curl -fsSL https://raw.githubusercontent.com/AgoraIO/cli/main/install.sh | sh
```

Run the CLI:

```bash
agora --help
```

Alternative install paths:

```bash
# npm (Node 18+, signed with npm provenance)
npm install -g agoraio-cli

# Windows PowerShell
irm https://raw.githubusercontent.com/AgoraIO/cli/main/install.ps1 | iex
```

Notes:

- The shell installer supports macOS, Linux, and Windows POSIX shells such as Git Bash. Use `install.ps1` for native PowerShell installs on Windows.
- **Shell setup is auto-on**: the installer wires the install directory onto your `PATH` (when needed) and writes a shell completion script for the detected shell (bash, zsh, fish, or PowerShell). Pass `--no-path`, `--no-completion`, or the umbrella `--skip-shell` (PowerShell: `-NoPath` / `-NoCompletion` / `-SkipShell`) to opt out granularly.
- Installer help is always available with `curl -fsSL https://raw.githubusercontent.com/AgoraIO/cli/main/install.sh | sh -s -- --help`.
- Pinned versions, dry runs, custom install directories, npm details, and source builds are documented in [docs/install.md](docs/install.md).

### Verifying release artifacts

Every release ships with a SHA-256 `checksums.txt` and a Cosign keyless signature. The official installers verify the SHA-256 automatically. You can also verify manually:

```bash
# Verify SHA-256 against the published checksums.txt
curl -fsSLO https://github.com/AgoraIO/cli/releases/download/vX.Y.Z/checksums.txt
sha256sum -c checksums.txt --ignore-missing
```

For Cosign signature verification and the SBOM workflow, see the **Security** section of [docs/install.md](docs/install.md). Vulnerability disclosures: see [SECURITY.md](SECURITY.md).

## First Run

```bash
agora login
agora init my-nextjs-demo --template nextjs
```

## Docs

- Human docs (GitHub Pages): [https://agoraio.github.io/cli/](https://agoraio.github.io/cli/)
- Agent-friendly Markdown mirror: [https://agoraio.github.io/cli/md/](https://agoraio.github.io/cli/md/)
- Release notes: [CHANGELOG.md](CHANGELOG.md)
- Install options (direct installer, Windows, source): [docs/install.md](docs/install.md)
- Full command reference (auto-generated): [docs/commands.md](docs/commands.md)
- Automation and JSON contract: [docs/automation.md](docs/automation.md)
- JSON envelope schema (machine-readable): [docs/schema/envelope.v1.json](docs/schema/envelope.v1.json)
- Stable error codes: [docs/error-codes.md](docs/error-codes.md)
- Telemetry controls: [docs/telemetry.md](docs/telemetry.md)
- Troubleshooting: [docs/troubleshooting.md](docs/troubleshooting.md)
- Security policy: [SECURITY.md](SECURITY.md)
- Support and contact channels: [SUPPORT.md](SUPPORT.md)
- Contributor and agent guide: [AGENTS.md](AGENTS.md), plus [CONTRIBUTING.md](CONTRIBUTING.md)

Command examples use `agora` for the installed CLI. Local source builds are covered in [Build From Source](#build-from-source).

## Command Model

The command model is intentionally layered:

- `init` for the recommended onboarding path
- `quickstart` for standalone starter repos
- `project` for remote Agora resources and env export
- `auth` for login and session inspection
- `config` for local CLI defaults
- `telemetry` for telemetry preferences
- `upgrade` / `update` / `self-update` for in-place upgrade or package-manager-specific guidance
- `open` to open the Console, published CLI docs (human or `/md/` Markdown), or product docs in a browser
- `doctor` for an install self-test (PATH, version, network, auth, MCP host)
- `env-help` to list every `AGORA_*` environment variable the CLI honors
- `skills` to browse curated workflow recipes for humans and AI agents
- `mcp` to run the CLI as a local MCP server (`agora mcp serve`) for agent integrations
- `completion` for shell completion scripts (auto-installed by the installer; see `agora completion --help` for manual setup)

Discover the full command tree:

```bash
agora --help
agora --help --all
agora introspect --json
```

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

### `telemetry`

Reads and updates telemetry preferences. `DO_NOT_TRACK=1` disables telemetry at runtime.

### `open`

Opens curated URLs: Console (`--target console`), human CLI docs on GitHub Pages (`docs`), raw Markdown tree for agents (`docs-md`), and Agora product docs (`product-docs`). Use `--no-browser` to print the resolved URL.

### `mcp`

Runs the CLI as an MCP stdio server so MCP-capable clients can call Agora workflows as tools. Authenticate with `agora login` on the host first; OAuth is not exposed through MCP.

### `version`

Prints build metadata. Release binaries include version, commit, and build date.

## Common Workflows

### Onboard a new demo

```bash
agora login
agora init my-nextjs-demo --template nextjs
```

### Use an existing project with a quickstart

```bash
agora quickstart create my-go-demo --template go --project my-existing-project
agora quickstart env write my-go-demo --project my-existing-project
```

### Update env after changing projects

```bash
agora project use my-agent-demo
agora quickstart env write my-go-demo
```

### Inspect project readiness

```bash
agora project doctor
agora project doctor --json
```

### Use low-level commands directly

```bash
agora project create my-agent-demo --feature rtc --feature convoai
agora quickstart create my-go-demo --template go --project my-agent-demo
agora quickstart env write my-go-demo --project my-agent-demo
```

### Inspect the full command tree

```bash
agora --help --all
```

## Quickstart Env Conventions

`quickstart env write` is different from `project env write`, but both keep dotenv files limited to runtime credentials.

- `project env write` writes only App ID and App Certificate keys for the detected or `--template`-selected layout (for example `AGORA_*` for standard Node, `NEXT_PUBLIC_*` / `NEXT_*` for Next.js, or `APP_ID` / `APP_CERTIFICATE` for Python and Go). See [docs/automation.md](docs/automation.md) for the full matrix and JSON fields.
- `quickstart env write` understands the quickstart type and writes only the App ID and App Certificate variable names the cloned repo expects
- existing `.env` and `.env.local` files are preserved; the CLI appends missing credentials, updates existing credential keys, and comments out duplicate or stale Agora credential aliases for the selected runtime

Template-specific behavior:

- Generic project env writes use `AGORA_APP_ID` plus `AGORA_APP_CERTIFICATE`
- Next.js quickstarts write `.env.local` and use `NEXT_PUBLIC_AGORA_APP_ID` plus `NEXT_AGORA_APP_CERTIFICATE`
- Python quickstarts copy `server/env.example` to `server/.env`, then use `APP_ID` plus `APP_CERTIFICATE`
- Go quickstarts copy `server-go/env.example` to `server-go/.env`, then use `APP_ID` plus `APP_CERTIFICATE`

The CLI also writes repo-local project metadata to:

- `.agora/project.json`

That allows the CLI to detect which Agora project a cloned demo is bound to even when you are working inside the repo later.

## Repo-Local Project Binding

Project resolution precedence is consistent across commands:

1. explicit `--project` or positional project argument
2. repo-local `.agora/project.json` resolved from the target repo path
3. global CLI context from `agora project use`

The `.agora/project.json` file is created or updated by:

- `agora init`
- `agora quickstart create ... --project ...`
- `agora quickstart env write ...`
- `agora project env write ...` (fills missing `projectType` / `envPath` when applicable)

It stores durable non-secret metadata:

- `projectId`
- `projectName`
- `region`
- `template`
- `projectType` (framework hint used for env layout when present)
- `envPath`

Examples:

```bash
# Inside a bound quickstart repo
agora project show --json

# From any directory, target a repo path directly
agora quickstart env write /abs/path/to/my-go-demo --json

# Rebind a repo to a different project
agora quickstart env write /abs/path/to/my-go-demo --project my-other-project --json
```

## Automation / Agent Usage

For scripts, CI, and agentic workflows:

- prefer `--json` for machine consumption
- set `AGORA_HOME` to an isolated temporary directory in CI or multi-agent runs
- prefer `init` for end-to-end setup
- use low-level commands when the workflow must be decomposed or resumed in stages
- use `agora --help --all` to inspect the full command tree
- use `quickstart env write` to re-sync env files after changing project selection
- use `project doctor --json` for readiness checks
- rely on the same JSON envelope for both success and failure

Examples:

```bash
export AGORA_HOME="$(mktemp -d)"
agora init my-nextjs-demo --template nextjs --json
agora quickstart create my-python-demo --template python --project my-project --json
agora quickstart env write my-python-demo --json
agora project doctor --json
agora auth status --json
```

The JSON envelope and stable result shapes are documented in [docs/automation.md](docs/automation.md). `auth status --json` exits `3` with `error.code` set to `AUTH_UNAUTHENTICATED` when no local session exists.

## CI and Releases

GitHub Actions are configured for:

- push and pull request validation on Linux, macOS, and Windows
- automated tag-driven releases for `v*` tags
- cross-platform release artifacts for Linux, macOS, and Windows

Release workflow behavior:

- a pushed tag matching `v*` (for example `v0.2.0`) triggers the release workflow
- the workflow runs tests, builds release binaries, packages them, and publishes a GitHub release automatically
- release artifacts include checksums

## Configuration

The CLI stores config, session, context, and logs under the Agora CLI config directory for the current machine.

Useful commands:

```bash
agora config path
agora config get
```

Built-in default config values are documented in [config.example.json](config.example.json).

## Troubleshooting

For a full troubleshooting guide with diagnostic commands, see [docs/troubleshooting.md](docs/troubleshooting.md). The fastest first step is always:

```bash
agora doctor --json
agora project doctor --json
```

The most common issues:

- **`agora` not found after install**: the installer wires PATH automatically by default; if you ran with `--no-path` or `--skip-shell`, re-run without it (or add the install directory to your shell profile manually).
- **OAuth browser does not open**: `agora login --no-browser` prints the URL so you can open it elsewhere; or `agora config update --browser-auto-open=false`.
- **`git` is missing**: `agora init` and `agora quickstart create` shell out to `git clone`. Install `git` and retry.
- **Project has no app certificate**: `quickstart env write`, `init`, and `project env --with-secrets` need a project with an App Certificate. Pick another project or enable one in [Agora Console](https://console.agora.io).
- **No project selected**: pass `--project <name>`, run `agora project use <name>`, or run from a repo that already has `.agora/project.json`.

Full guide with debug logging, CI tips, completion troubleshooting, and the `--debug` flag: [docs/troubleshooting.md](docs/troubleshooting.md).

## Build From Source

```bash
go build -o agora .
./agora --help
```

Requires the Go toolchain pinned in [go.mod](go.mod). For direct installer options and source install notes, see [docs/install.md](docs/install.md).
