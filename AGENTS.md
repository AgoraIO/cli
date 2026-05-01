# Agent and Contributor Guide

## Repo Purpose

This repository contains Agora CLI, the native CLI for Agora developer onboarding. It ships as a single binary with no runtime dependencies and is the primary distribution. The same binary is also published via npm as `agoraio-cli` (a thin shim that runs the native executable).

## Quick Reference

| Task | Command |
|------|---------|
| Build binary | `go build -o agora .` |
| Run all tests | `go test ./...` |
| Inspect full command tree | `./agora --help --all` |
| Machine-readable full command tree | `./agora --help --all --json` |
| Agent introspection artifact | `./agora introspect --json` |
| Machine-readable output | Add `--json` to any command |

## Source Layout

```
main.go                     Entry point — wires the root command and calls Execute()
cmd/
  gendocs/                  Regenerates docs/commands.md from the live cobra tree
internal/cli/
  app.go                    App struct, Execute(), output-mode resolver, env snapshot
  commands.go               Root command tree; subcommand builders for auth/config/upgrade/etc.
  envelope.go               JSON envelope shape, exit-code plumbing, error helpers
  render.go                 Pretty output dispatch (renderResult, printBlock, printDoctor)
  paths.go                  Config / session / context paths, writeSecureJSON
  config.go                 appConfig type, defaults, env injection
  version.go                Build-time version vars, versionInfo, formattedVersion
  introspect.go             agora introspect + buildIntrospectionData (agent discovery contract)
  mcp.go                    agora mcp serve — JSON-RPC MCP stdio transport + tool dispatch
  open_targets.go           Canonical URLs for agora open (docs, Console, product docs)
  features.go               Product feature catalog (rtc/rtm/convoai) shared by doctor, introspect, init defaults
  cache.go                  Short-lived on-disk API caches (project list for shell completion)
  completion.go             Dynamic shell completion helpers
  upgrade.go                agora upgrade self-update logic (download, SHA-256, atomic rename)
  progress.go               NDJSON progress event emitter for long-running JSON-mode commands
  auth.go                   login / logout / whoami / auth status
  projects.go               project create / use / show / env / env write / doctor
  quickstart.go             quickstart create / env write / list
  init.go                   init — one-step: project + quickstart + env
  doctor.go                 project doctor — readiness checks, workspace mode
  install_doctor.go         Top-level agora doctor — install self-test (PATH, network, auth, MCP host)
  env_help.go               agora env-help — authoritative env-var catalog
  skills.go                 agora skills — curated workflow recipes (in-binary catalog)
  telemetry.go              telemetryClient interface + noop sink + Sentry placeholder (wire-up scheduled for next release)
  local_project.go          .agora/project.json read/write; repo-local project binding
  runtime_support.go        Template/runtime detection (nextjs, python, go), CI auto-detect, banner rules
  app_test.go               Unit tests for app init and config
  integration_test.go       Integration tests: build binary, shell out, assert JSON
docs/
  automation.md             Stable JSON output contract — machine-consumption source of truth
  install.md                Direct installer, platform, CI, and security guidance
  _config.yml               Jekyll / GitHub Pages configuration (human docs site)
  _layouts/, assets/        Theme assets for Pages
scripts/
  preview-pages-site.sh      Local Jekyll build + URL injection (`make docs-preview`)
  prepare-pages-site.py      Pages artifact prep (Markdown /md mirror, token expansion)
.github/workflows/
  ci.yml                    Push/PR matrix: Ubuntu, macOS, Windows
  release.yml               Tag-driven cross-platform release
  pages.yml                 Publish docs to GitHub Pages
  apt-repo.yml              Signed apt repository publishing
```

## Command Model

The surface is deliberately layered. Use the highest-level command that covers the workflow:

```
agora
├── init <name>                    Recommended path: reuses existing project (or creates if none); add --new-project to force creation
├── version                        Build version, commit, and date
├── introspect                     Machine-readable command metadata for agents
├── doctor                         Install self-test (PATH, version, network, auth, MCP host); use project doctor for project readiness
├── env-help                       Catalog of every AGORA_* env var the CLI honors
├── skills                         Curated workflow recipes for humans and AI agents (list / show / search)
├── open                           Open Console, CLI docs (human or /md/), or product docs
├── mcp                            MCP stdio server for agent tool integrations
├── telemetry                      Telemetry status/enable/disable
├── upgrade (alias: update, self-update)  In-place self-update on installer-managed installs; otherwise prints upgrade guidance
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
  "meta": { "outputMode": "json", "exitCode": 0 }
}
```

| Field | Stable | Notes |
|-------|--------|-------|
| `ok` | yes | Branch on this first |
| `command` | yes | Stable command label |
| `data` | yes | `null` on failure |
| `error.message` | yes | Present on failure |
| `error.code` | yes | Present on known structured failures |
| `meta.outputMode` | yes | Always `"json"` |
| `meta.exitCode` | yes | Process exit code for success and failure |

`auth status`, `whoami`, and API-touching commands return exit code `3` plus `ok: false` with `error.code == "AUTH_UNAUTHENTICATED"` when no local session exists. Treat that as the unauthenticated state and run `agora login` before commands that require a session.

Set `AGORA_AGENT=<tool-name>` in agent runs to explicitly label API requests in `User-Agent`. If it is unset, the CLI infers a coarse label from known agent environment markers (Cursor, Claude Code, Cline, Windsurf, Codex, Aider) unless `AGORA_AGENT_DISABLE_INFER=1` is set.

## Testing

```bash
go test ./...
```

- `app_test.go` — unit tests for app initialization and config
- `integration_test.go` — builds the binary, shells out, asserts JSON output shapes

## Linting

```bash
make lint            # gofmt + golangci-lint + error-code audit
golangci-lint run    # standalone (config: .golangci.yml)
```

CI uses `golangci-lint v1.64.8`, installed via `go install` so the linter is built with the same Go version as `go.mod`. Install locally to match:

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8
```

Alternatively, download the release binary (must be built with a Go version ≥ `go.mod`; if config load fails, prefer `go install` above):

```bash
curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
  | sh -s -- -b "$(go env GOPATH)/bin" v1.64.8
```

The ruleset is intentionally conservative (errcheck, govet, staticcheck, ineffassign, unused, gosec, bodyclose, errorlint, misspell, unconvert). When a finding is a false positive, prefer narrowing the rule via `.golangci.yml` `exclude-rules` over adding inline `//nolint` directives.

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
6. Call out breaking JSON or exit-code changes in `CHANGELOG.md` and migration notes in `docs/automation.md` when behavior is intentional.

## CI and Release

| Workflow | Trigger | What it does |
|----------|---------|--------------|
| `ci.yml` | push, PR | `go test ./...` + `gofmt`, `golangci-lint`, error-code coverage audit on Ubuntu, macOS, Windows |
| `release.yml` | `v*` tag | Builds cross-platform binaries, publishes GitHub release and package channels |
| `apt-repo.yml` | published release | Updates the signed apt repository |

Tagging `v0.2.0` (or any `v*` semver tag) triggers the release workflow automatically.

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
  agoraio-cli-darwin-arm64/ ← one unscoped package per platform
  agoraio-cli-darwin-x64/
  agoraio-cli-linux-arm64/
  agoraio-cli-linux-x64/
  agoraio-cli-win32-x64/
  agoraio-cli-win32-arm64/
    package.json            ← os/cpu fields restrict install to matching platform
    bin/                    ← .gitignored; populated by CI at release time
```

**How it works:**
1. `npm install -g agoraio-cli` installs the shim + the matching platform package via `optionalDependencies`
2. `bin/agora.js` resolves `agoraio-cli-<platform>/bin/agora` and `spawnSync`s it with all args inherited
3. If the platform package is missing, the shim prints a helpful error pointing to Homebrew or GitHub releases

**Release flow (automated and active):** the `publish-npm` job in `release.yml`:
1. Downloads release archives and `checksums.txt` from the GitHub release
2. Verifies SHA-256 of every archive against `checksums.txt`; fails the job on mismatch
3. Extracts the binary for each platform into the corresponding package's `bin/`
4. Stamps the tag version into all `package.json` files (wrapper + 6 platform packages, including `optionalDependencies` values)
5. Publishes all 6 platform packages, then the wrapper package, all with `npm publish --provenance` (sigstore-backed supply-chain attestations)
6. Smoke-tests the published wrapper with `npx --yes agoraio-cli@<tag> --version` (retry/backoff for registry propagation)

**Prerequisites:**
- `NPM_TOKEN` secret in the repo, with publish access to `agoraio-cli` and all unscoped `agoraio-cli-*` platform packages.
- `id-token: write` workflow permission (already set in `release.yml`) — required for npm provenance.

**Manual dry-run:** the workflow exposes `workflow_dispatch` with a `dry_run` input that runs `npm publish --dry-run` against a synthetic version, validating packaging without publishing.

**Installing from npm (users):**
```bash
npm install -g agoraio-cli   # installs shim + native binary for current platform
npx agoraio-cli --help       # or via npx without global install
```

The shell installer remains the primary install mechanism. npm is a convenience path for developers already in a Node.js ecosystem and benefits from the supply-chain provenance attestations attached at publish time.
