# Agent and Contributor Guide

## Repo Purpose

`agora-cli-go` is the next-generation native CLI for Agora developer onboarding. It ships as a single binary with no runtime dependencies and is the primary distribution going forward. It is feature-parity with (and successor to) the TypeScript CLI `agoraio-cli`.

## Quick Reference

| Task | Command |
|------|---------|
| Build binary | `go build -o agora .` |
| Run all tests | `go test ./...` |
| Inspect full command tree | `./agora --help --all` |
| Machine-readable full command tree | `./agora --help --all --json` |
| Machine-readable output | Add `--json` to any command |

## Source Layout

```
main.go                     Entry point — wires the root command and calls Execute()
internal/cli/
  app.go                    App struct, config loading, logger and output setup
  commands.go               Root command tree; every subcommand is registered here
  auth.go                   login / logout / whoami / auth status
  projects.go               project create / use / show / env / env write / doctor
  quickstart.go             quickstart create / env write / list
  init.go                   init — one-step: project + quickstart + env
  doctor.go                 project doctor — readiness checks, workspace mode
  local_project.go          .agora/project.json read/write; repo-local project binding
  runtime_support.go        Template/runtime detection (nextjs, python, go)
  app_test.go               Unit tests for app init and config
  integration_test.go       Integration tests: build binary, shell out, assert JSON
docs/
  automation.md             Stable JSON output contract — machine-consumption source of truth
  devex-recommendations.md  Devex audit and backlog
  parity-matrix.json        Feature parity tracking vs agora-cli-ts
  homebrew.md               Homebrew distribution
scripts/
  homebrew/generate.sh      Homebrew formula generation
.github/workflows/
  ci.yml                    Push/PR matrix: Ubuntu, macOS, Windows
  release.yml               Tag-driven cross-platform release
  homebrew-tap.yml          Homebrew tap update after release
```

## Command Model

The surface is deliberately layered. Use the highest-level command that covers the workflow:

```
agora
├── init <name>                    Recommended path: project + quickstart + env in one step
├── project
│   ├── create <name>              Create a remote Agora project (control-plane only)
│   ├── use <name>                 Set global project context
│   ├── show                       Print selected project details
│   ├── env                        Print project env values (no file write)
│   ├── env write <path>           Write a dotenv block to a file
│   └── doctor                     Readiness check; --deep for workspace-level checks
├── quickstart
│   ├── create <name>              Clone an official quickstart repo
│   ├── env write <name|path>      Write the template-specific env file
│   └── list                       List available quickstart templates
├── auth
│   ├── login   (alias: agora login)   OAuth login via browser or manual URL
│   ├── logout                         Clear session
│   └── status  (alias: agora whoami)  Print current session state
└── config
    ├── path    Print the config file path
    ├── get     Print current config values
    └── update  Update a config value
```

**Design rules — do not break these:**
- `project` = remote Agora control-plane resource; it never scaffolds local files
- `quickstart` = local repo clone; requires `git` on the PATH
- `init` = the only command that composes both
- The `add` namespace is reserved; keep it hidden and return a command-not-found error if invoked

## Project Resolution Precedence

Commands that need a project resolve context in this order:

1. **Explicit `--project` flag or positional argument** — use in all pipeline and cross-directory operations
2. **Repo-local `.agora/project.json`** — auto-detected from the target directory tree
3. **Global CLI context** — set by `agora project use`

**Agent rule:** always prefer explicit `--project` for deterministic, reproducible operations. Use repo-local binding only when operating repeatedly inside a bound quickstart.

Repo-local detection correctly traverses upward from the provided target path argument. Running `quickstart env write /abs/path/to/demo` from any working directory will find `.agora/project.json` inside that path.

## JSON Output Contract

Every command accepts `--json`. In automated contexts always use `--json` — never parse human/pretty output.

```bash
./agora init my-demo --template nextjs --json
./agora project doctor --json
./agora auth status --json
```

The full stable contract with all result shapes is in [`docs/automation.md`](docs/automation.md).

**Envelope:**
```json
{
  "ok": true,
  "command": "init",
  "data": { ... },
  "meta": { "outputMode": "json" }
}
```

| Field | Stable | Notes |
|-------|--------|-------|
| `ok` | yes | Branch on this first |
| `command` | yes | Stable command label |
| `data` | yes | `null` on failure |
| `error.message` | yes | Present on failure |
| `meta.outputMode` | yes | Always `"json"` |
| `meta.exitCode` | yes | Present on failure |

## Testing

```bash
go test ./...
```

- `app_test.go` — unit tests for app initialization and config
- `integration_test.go` — builds the binary, shells out, asserts JSON output shapes

When adding a command:
1. Register it in `commands.go`
2. Add a happy-path JSON test in `integration_test.go`
3. Add edge-case unit tests in `app_test.go` for non-trivial logic

## Adding a New Command

1. Create `internal/cli/<noun>.go` with business logic on `*App`
2. Register the command in `commands.go` inside `buildRoot()`
3. Accept `--json` via `a.resolveOutputMode(cmd)`; return results through `renderResult(cmd, "command label", data)`
4. Add the command to the README command model
5. Add a stable JSON result shape to `docs/automation.md`
6. Update `docs/parity-matrix.json` if it corresponds to a TypeScript command

## CI and Release

| Workflow | Trigger | What it does |
|----------|---------|--------------|
| `ci.yml` | push, PR | `go test ./...` on Ubuntu, macOS, Windows |
| `release.yml` | `v*` tag | Builds cross-platform binaries, publishes GitHub release |
| `homebrew-tap.yml` | after release | Updates Homebrew formula |

Tagging `v0.1.4` triggers the release workflow automatically.

## Gotchas

| Issue | Detail |
|-------|--------|
| `git` required | `quickstart create` and `init` shell out to `git clone` |
| Headless OAuth | Use `--no-browser` to print a URL instead of opening a browser |
| `quickstart env write` ≠ `project env write` | Template-aware paths and variable names vs generic dotenv block |
| `add` namespace | Reserved and hidden; must behave as not-found from the user's perspective |
| `doctor --deep` | Flag exists but deep checks are not fully implemented; don't document as stable yet |
| App certificate required | `quickstart env write` and `init` fail env injection if the project has no certificate |

## npm Distribution (Node Wrapper)

The Go binary is also distributed via npm as `agoraio-cli`. The packaging lives entirely in this repo under `packaging/npm/`.

**Structure:**
```
packaging/npm/
  agoraio-cli/              ← the published npm package (Node shim only)
    bin/agora.js            ← entry point: resolves platform binary and spawns it
    package.json            ← optionalDependencies for all 6 platforms
  @agoraio/
    cli-darwin-arm64/       ← one package per platform
    cli-darwin-x64/
    cli-linux-arm64/
    cli-linux-x64/
    cli-win32-x64/
    cli-win32-arm64/
      package.json          ← os/cpu fields restrict install to matching platform
      bin/                  ← .gitignored; populated by CI at release time
```

**How it works:**
1. `npm install -g agoraio-cli` installs the shim + the matching platform package via `optionalDependencies`
2. `bin/agora.js` resolves `@agoraio/cli-<platform>/bin/agora` and `spawnSync`s it with all args inherited
3. If the platform package is missing, the shim prints a helpful error pointing to Homebrew or GitHub releases

**Release flow (automated):** the `publish-npm` job in `release.yml`:
1. Downloads build artifacts from the `build` job
2. Extracts the binary for each platform into the corresponding package's `bin/`
3. Stamps the tag version into all `package.json` files
4. Publishes all 6 platform packages, then publishes the wrapper package

**Prerequisites:** `NPM_TOKEN` secret must be set in the repo with publish access to the `@agoraio` scope and `agoraio-cli`.

**Installing from npm (users):**
```bash
npm install -g agoraio-cli   # installs shim + native binary for current platform
npx agoraio-cli --help       # or via npx without global install
```

Homebrew remains the primary install mechanism. npm is a convenience path for developers already in a Node.js ecosystem.

## Parity with agora-cli-ts

Feature parity is tracked in [`docs/parity-matrix.json`](docs/parity-matrix.json). When implementing or modifying a command, verify it matches the TypeScript behavior for JSON field names, project resolution precedence, error messages, and exit codes. The TS implementation is the reference for existing behavior; the Go CLI is the reference going forward.
