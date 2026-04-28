# Agora CLI Go

Native Agora CLI for authentication, project management, quickstart setup, and developer onboarding.

## Install

```bash
curl -fsSL https://raw.githubusercontent.com/AgoraIO-Community/cli/main/install.sh | sh -s -- --add-to-path
```

Run the CLI:

```bash
agora --help
```

Notes:

- The shell installer supports macOS, Linux, and Windows POSIX shells such as Git Bash. Use `install.ps1` for native PowerShell installs on Windows.
- If the installer says `agora` is not on your PATH, re-run with `--add-to-path` or add the printed install directory to your shell profile.
- Installer help is always available with `curl -fsSL https://raw.githubusercontent.com/AgoraIO-Community/cli/main/install.sh | sh -s -- --help`.
- Pinned versions, dry runs, custom install directories, and source builds are documented in [docs/install.md](docs/install.md).

## First Run

```bash
agora login
agora init my-nextjs-demo --template nextjs
```

## Docs

- Install options (direct installer, Windows, source): [docs/install.md](docs/install.md)
- Automation and JSON contract: [docs/automation.md](docs/automation.md)

## Command Model

The command model is intentionally layered:

- `init` for the recommended onboarding path
- `quickstart` for standalone starter repos
- `project` for remote Agora resources and env export
- `auth` for login and session inspection
- `config` for local CLI defaults

Discover the full command tree:

```bash
./agora --help
./agora --help --all
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

## Repo-Local Project Binding

Project resolution precedence is consistent across commands:

1. explicit `--project` or positional project argument
2. repo-local `.agora/project.json` resolved from the target repo path
3. global CLI context from `agora project use`

The `.agora/project.json` file is created or updated by:

- `agora init`
- `agora quickstart create ... --project ...`
- `agora quickstart env write ...`

It stores durable non-secret metadata:

- `projectId`
- `projectName`
- `region`
- `template`
- `envPath`

Examples:

```bash
# Inside a bound quickstart repo
./agora project show --json

# From any directory, target a repo path directly
./agora quickstart env write /abs/path/to/my-go-demo --json

# Rebind a repo to a different project
./agora quickstart env write /abs/path/to/my-go-demo --project my-other-project --json
```

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
./agora quickstart env write my-go-demo --project my-project
./agora project use my-project
```

or run it inside a repo that already has `.agora/project.json`.

## Build From Source

```bash
go build -o agora .
./agora --help
```

Requires the Go toolchain pinned in [go.mod](go.mod). For direct installer options and source install notes, see [docs/install.md](docs/install.md).

## Migration

This project mirrors the `agora-cli-ts` command surface in a native Go binary so the CLI no longer depends on the Node.js runtime.
