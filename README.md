# Agora CLI

<p><nobr><a href="https://github.com/AgoraIO/cli/actions/workflows/ci.yml"><img src="https://github.com/AgoraIO/cli/actions/workflows/ci.yml/badge.svg" alt="CI"></a>&nbsp;&nbsp;<a href="https://github.com/AgoraIO/cli/releases"><img src="https://img.shields.io/github/v/release/AgoraIO/cli?label=release" alt="Release"></a>&nbsp;&nbsp;<a href="LICENSE"><img src="https://img.shields.io/github/license/AgoraIO/cli" alt="License"></a></nobr></p>

Native Agora CLI for authentication, project management, quickstart setup, and developer onboarding. Use it to go from an Agora account to a runnable app with one command.

## Install

### Requirements

- macOS 12+, Linux (glibc 2.31+ or musl), or Windows 10+ for the prebuilt binaries.
- `git` on `PATH` for `agora init` and `agora quickstart create` (they shell out to `git clone`).
- For the source build, the Go toolchain pinned in [`go.mod`](go.mod).

### Install the CLI

```bash
curl -fsSL https://dl.agora.io/cli/install.sh | sh
```

Run the CLI:

```bash
agora --help
```

The script is served from the Agora CDN (`dl.agora.io`, CloudFront). Binaries download from GitHub by default and automatically fall back to the CDN mirror if GitHub is unreachable; downloads are verified against `checksums.txt` regardless of source.

Windows PowerShell:

```powershell
irm https://dl.agora.io/cli/install.ps1 | iex
```

Alternative install paths (GitHub-hosted; use `install.ps1` for PowerShell):

```bash
# GitHub Pages
curl -fsSL https://agoraio.github.io/cli/install.sh | sh
# raw GitHub
curl -fsSL https://raw.githubusercontent.com/AgoraIO/cli/main/install.sh | sh
```

Locked-down environments that block `curl | sh` can download a release archive from GitHub or mirror the binary internally. Every release includes `checksums.txt`, a Cosign keyless signature, and an SBOM; see [docs/install.md](docs/install.md#enterprise--locked-down-environments) for manual tarball, checksum, and Cosign verification steps. The npm distribution is currently paused; the package on npm may be stale and should not be used.

Notes:

- The shell installer supports macOS, Linux, and Windows POSIX shells such as Git Bash. Use `install.ps1` for native PowerShell installs on Windows.
- **Shell setup is auto-on**: the installer wires the install directory onto your `PATH` (when needed) and writes a shell completion script for the detected shell (bash, zsh, fish, or PowerShell). Pass `--no-path`, `--no-completion`, or the umbrella `--skip-shell` (PowerShell: `-NoPath` / `-NoCompletion` / `-SkipShell`) to opt out granularly.
- Installer help is always available with `curl -fsSL https://dl.agora.io/cli/install.sh | sh -s -- --help`.
- Pinned versions, dry runs, custom install directories, and source builds are documented in [docs/install.md](docs/install.md).
- Release artifacts and checksums: [GitHub Releases](https://github.com/AgoraIO/cli/releases). Vulnerability disclosures: [SECURITY.md](SECURITY.md).

### Restricted networks (GitHub blocked)

The default command already fetches the install script from the Agora CDN (`dl.agora.io`), so it works even where GitHub is blocked. Binaries still download from GitHub first (with automatic fallback to the mirror); in a fully-blocked region, add `AGORA_INSTALL_SOURCE=s3` to skip GitHub entirely and avoid the failover delay:

```bash
# macOS, Linux, and Windows POSIX shells
curl -fsSL https://dl.agora.io/cli/install.sh | AGORA_INSTALL_SOURCE=s3 sh

# Windows PowerShell
$env:AGORA_INSTALL_SOURCE = 's3'; irm https://dl.agora.io/cli/install.ps1 | iex
```

Downloads are still SHA-256 verified against `checksums.txt` regardless of source. `AGORA_INSTALL_SOURCE` accepts `auto` (default; GitHub then mirror), `github`, or `s3`. See [docs/install.md](docs/install.md#mirror-fallback-for-restricted-networks) for details.

### Build from source

```bash
go build -o agora .
./agora --help
```

Requires the Go toolchain pinned in [go.mod](go.mod). For direct installer options and source install notes, see [docs/install.md](docs/install.md).

## Quick Start

Recommended path: install the CLI, log in, then run `agora init`. The CLI binds a project, clones an official quickstart, writes the runtime env file (App ID and App Certificate), and creates `.agora/project.json`. You do **not** need to download an env file from Agora Console for this flow.

```bash
# 1) Log in
agora login

# 2) Create a local demo bound to a project
# Interactive (TTY): reuses "Default Project" if present; otherwise prompts to pick or create
agora init my-nextjs-demo --template nextjs
# Non-interactive / scripts / --json / CI: pass an existing project, or force-create one
# agora init my-nextjs-demo --template nextjs --project <project-id-or-name>
# agora init my-nextjs-demo --template nextjs --new-project

# 3) Install deps and start the app (follow nextSteps from init; Next.js example:)
cd my-nextjs-demo
pnpm install
# Newer pnpm may block dependency build scripts (ERR_PNPM_IGNORED_BUILDS). If so:
# pnpm approve-builds --all
# pnpm rebuild
pnpm dev
# Other templates print their own commands, e.g. python: bun run setup && bun run dev
# go: make setup && make dev

# 4) Open the app in a browser (Next.js default: http://localhost:3000)
open http://localhost:3000
# Linux: xdg-open http://localhost:3000
# Windows: start http://localhost:3000

# 5) Optional: check project/workspace readiness
agora project doctor --json
```

`init` also prints template-specific next steps in its output. Refresh credentials or rebind the repo later with:

```bash
cd my-nextjs-demo
agora quickstart env write . --project <project-id-or-name>
```

If a command reports `No project selected`, pass `--project`, run `agora project use <project>`, or work inside a directory that already has `.agora/project.json`. That is expected when none of those contexts exist—not a missing Console env download.

Command examples use `agora` for the installed CLI. Local source builds use `./agora` from the repo root.

## What You Can Build Quickly

| Goal | Command | What You Get |
|------|---------|--------------|
| Next.js video app | `agora init my-nextjs-demo --template nextjs` | A cloned Next.js quickstart, `.agora` binding, and `.env.local` |
| Python voice agent | `agora init my-python-demo --template python` | A Python quickstart with `server/.env.local` credentials |
| Go voice agent | `agora init my-go-demo --template go` | A Go quickstart with `server/.env.local` credentials |

By default `init` reuses a project named `Default Project` when present. In an interactive TTY without that project, it prompts you to pick an existing project or create a new one. Non-interactive/`--json`/CI runs fall back to the most recent project (or create one when none exist). Pass `--project <id-or-name>` or `--new-project` to control selection explicitly. Run `agora quickstart list` to see all available templates.

## Command Model

The command model is intentionally layered:

- `init` for the recommended onboarding path (project + clone + env)
- `quickstart` for local cloned starter repos (requires `git`)
- `project` for remote Agora control-plane resources (does not clone scaffolds)
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

### Which command?

| Goal | Command |
|------|---------|
| New user, one shot | `agora init <name> --template <id>` (reuses `Default Project` / interactive picker; or `--project` / `--new-project`) |
| List available templates | `agora quickstart list` |
| Clone a starter only | `agora quickstart create ...` |
| Re-sync / rebind env in a cloned quickstart | `agora quickstart env write [dir]` (optional `--project` to rebind) |
| Write env to an arbitrary path / non-quickstart repo | `agora project env write <path>` |
| Set machine-wide default project | `agora project use <project>` |
| Install self-test | `agora doctor --json` |
| Project/workspace readiness | `agora project doctor --json` (add `--deep` in a bound repo) |
| Manage feature webhooks | `agora project webhook ... --json` |

### Env-related commands

```
agora init <name>                    # recommended: project + clone + env
├── project
│   ├── env                          Print project env values (no file write)
│   └── env write <path>             Generic dotenv block (AGORA_* or NEXT_*)
└── quickstart
    └── env write [dir]              Template-specific env file and key names
```

Discover the full command tree:

```bash
agora --help
agora --help --all
agora introspect --json
```

### `init`

Recommended onboarding command. By default it reuses a project named `Default Project` when present. In an interactive TTY without that project, it prompts you to pick or create one. Non-interactive/`--json`/CI runs fall back to the most recent project (or create one when none exist). Prefer `--project <id-or-name>` or `--new-project` for explicit selection. It clones a quickstart, writes the template env file from the project API, writes `.agora/project.json`, updates global context, and prints next steps.

### `quickstart`

Manages standalone official starter repos and their runtime-specific env files.

Use this when you want to:

- list available templates with `quickstart list`
- clone a quickstart without creating a project
- bind a quickstart to an existing project
- re-sync env files after changing project selection

### `project`

Manages remote Agora project resources.

Use this when you want to:

- create or inspect projects directly
- switch the default project context
- export project env values with `project env`
- write credentials to a dotenv file with `project env write`
- inspect project readiness with `project doctor`
- manage feature-scoped webhook endpoints with `project webhook`

### `auth`

Handles login, logout, and current session inspection.

### `config`

Reads and updates local CLI defaults such as output mode, log level, and browser behavior.

### `telemetry`

Reads and updates telemetry preferences. `DO_NOT_TRACK=1` disables telemetry at runtime.

### `open`

Opens curated URLs: Console (`--target console`), human CLI docs on GitHub Pages (`docs`), raw Markdown tree for agents (`docs-md`), and Agora product docs (`product-docs`). Use `--no-browser` to print the resolved URL.

### `mcp`

Runs the CLI as a local MCP server so MCP-capable clients can call Agora workflows as tools. Authenticate with `agora login` on the host first; OAuth is not exposed through MCP.

### `version`

Prints build metadata. Release binaries include version, commit, and build date.

## Env Files and Project Binding

Env files hold runtime credentials. Project selection is separate: use `--project`, `.agora/project.json`, or `agora project use`. The CLI writes App ID and App Certificate from the selected project's API response into the template env file. It does **not** download a ready-made dotenv from Agora Console.

Prefer `agora quickstart env write` inside official quickstarts. Use `agora project env write <path>` only when you need a specific dotenv path outside that layout.

| Command | Env path | Key names |
|---------|----------|-----------|
| `agora init` / `quickstart env write` | Template-defined (`.env.local`, `server/.env.local`, etc.) | Template-specific (`NEXT_PUBLIC_*`, `AGORA_*`, …) |
| `agora project env write <path>` | User-supplied path | `AGORA_*` or `NEXT_*` only |

Quickstart template behavior:

- Next.js quickstarts write `.env.local` with `NEXT_PUBLIC_AGORA_APP_ID` plus `NEXT_AGORA_APP_CERTIFICATE`
- Python quickstarts copy `server/.env.example` to `server/.env.local`, then use `AGORA_APP_ID` plus `AGORA_APP_CERTIFICATE`
- Go quickstarts copy `server/.env.example` to `server/.env.local`, then use `AGORA_APP_ID` plus `AGORA_APP_CERTIFICATE`
- Existing Python and Go quickstarts keep their recorded env path and legacy `APP_ID` / `APP_CERTIFICATE` keys when reconfigured.

`project env write` auto-detects Next.js workspaces (or accepts `--template nextjs|standard`) and writes `AGORA_APP_ID` / `AGORA_APP_CERTIFICATE` or the Next.js equivalents.

Existing `.env` and `.env.local` files are preserved: the CLI appends missing credentials, updates existing credential keys, and comments out duplicate or stale Agora credential aliases for the selected runtime.

See [docs/automation.md](docs/automation.md) for JSON fields and the full credential matrix.

### Repo-local binding

`.agora/project.json` is the **repo-local** project binding (not the env file). It lets the CLI know which Agora project a cloned demo uses when you work inside that repo later. `agora project use` only sets a **machine-wide** default and does not rewrite `.agora/project.json`.

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

## Common Workflows

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

### Use low-level commands directly

```bash
agora project create my-agent-demo --feature rtc --feature convoai
agora quickstart create my-go-demo --template go --project my-agent-demo
agora quickstart env write my-go-demo --project my-agent-demo
```

## Automation and Agents

For scripts, CI, and agentic workflows:

- prefer `--json` for machine consumption
- set `AGORA_HOME` to an isolated temporary directory in CI or multi-agent runs
- prefer `init` for end-to-end setup; decompose with lower-level commands when a workflow must be resumed in stages
- use `agora introspect --json` and [AGENTS.md](AGENTS.md) for agent discovery; [docs/automation.md](docs/automation.md) for the JSON envelope contract

Example:

```bash
export AGORA_HOME="$(mktemp -d)"
agora init my-nextjs-demo --template nextjs --json
agora quickstart create my-python-demo --template python --project my-project --json
agora quickstart env write my-python-demo --json
agora project doctor --json
agora auth status --json
```

`auth status --json` exits `3` with `error.code` set to `AUTH_UNAUTHENTICATED` when no local session exists.

## Configuration

The CLI stores config, session, context, and logs under the Agora CLI config directory for the current machine.

Useful commands:

```bash
agora config path
agora config get
```

Built-in default config values are documented in [config.example.json](config.example.json).

## Troubleshooting

For a full troubleshooting guide with diagnostic commands, see [docs/troubleshooting.md](docs/troubleshooting.md).

Start with the right doctor command:

- **`agora doctor --json`** — install self-test (PATH, version, network, auth, MCP host)
- **`agora project doctor --json`** — project and workspace readiness (credentials, `.agora` binding, env consistency; add `--deep` for repo-local checks)

The most common issues:

- **`agora` not found after install**: the installer wires PATH automatically by default; if you ran with `--no-path` or `--skip-shell`, re-run without it (or add the install directory to your shell profile manually).
- **OAuth browser does not open**: `agora login --no-browser` prints the URL so you can open it elsewhere; or `agora config update --browser-auto-open=false`.
- **`git` is missing**: `agora init` and `agora quickstart create` shell out to `git clone`. Install `git` and retry.
- **Project has no app certificate**: `quickstart env write`, `init`, and `project env --with-secrets` need a project with an App Certificate. Pick another project or enable one in [Agora Console](https://console.agora.io).
- **No project selected**: the command has no project context. Pass `--project <name>`, run `agora project use <name>`, or work inside a repo with `.agora/project.json` (created by `init` / `quickstart env write`). Do not expect the CLI to infer a project from `.env.local` alone.
- **Wrong or stale credentials in a quickstart**: re-run `agora quickstart env write . --project <id>` from the demo directory instead of pasting a Console-downloaded env file.

Full guide with debug logging, CI tips, completion troubleshooting, and the `--debug` flag: [docs/troubleshooting.md](docs/troubleshooting.md).

## Documentation

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
