# Changelog

All notable changes to Agora CLI are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

When tagging a new release, rename the `[Unreleased]` section to the new version
(e.g. `[0.2.0] - 2026-05-04`), add a fresh empty `[Unreleased]` heading at the top,
and update the link references at the bottom of this file.

When adding a new entry, link the change to the PR or commit that introduced it
using the trailing `([#123](https://github.com/AgoraIO/cli/pull/123))` convention.
Earlier entries pre-date this convention and only carry their version's compare link.

## [Unreleased]

## [0.2.0] - 2026-05-04

### Added

- Add GitHub Pages publishing for generated CLI docs and route `agora open --target docs` to the human CLI docs site, `agora open --target docs-md` to the agent-facing raw Markdown docs under `/md/`, and `product-docs` to Agora product docs.
- Add `docs/site.env` and Pages build-time URL injection so staging docs can publish with different `CLI_DOCS_BASE_URL` / `CLI_DOCS_MD_BASE_URL` values while keeping predictable human and Markdown paths.
- Add a custom GitHub Pages theme for the human docs with responsive layout, system light/dark mode via `prefers-color-scheme`, and no manual theme toggle.
- Add `make docs-preview` for a Ruby/Jekyll local docs preview that builds with localhost-friendly paths, injects localhost docs URLs, and serves both the human site and `/md/` Markdown tree.
- Add global `--yes` / `-y` and `AGORA_NO_INPUT=1` support to accept defaults and suppress prompts.
- Add pretty-mode progress status lines for long-running clone, OAuth, and project creation work.
- Add dynamic shell completions for project names, quickstart templates, and project features, with an on-disk completion cache under `<AGORA_HOME>/cache/projects.json` so `agora project use <TAB>` is instant on warm caches. Configurable via `AGORA_PROJECT_CACHE_TTL_SECONDS` and disable-able via `AGORA_DISABLE_CACHE=1`.
- Add `agora mcp serve` so MCP-capable agents can use local Agora CLI tools, exposing the full surface (`agora.version`, `agora.introspect`, `agora.auth.*`, `agora.config.*`, `agora.telemetry.status`, `agora.upgrade.check`, `agora.project.*` including `create`/`env`/`feature.{list,status,enable}`, `agora.quickstart.*`, and `agora.init`).
- Add drop-in agent rule snippets under `docs/agents/` and `agora init --add-agent-rules` with safe append-when-exists semantics: subsequent runs update only the Agora-managed block between sentinel markers and never destroy pre-existing user content.
- Add `install.sh --uninstall` and `install.ps1 -Uninstall`.
- Add CODEOWNERS, Dependabot, and a scheduled `govulncheck` workflow.
- Add `PROJECT_NAME_REQUIRED` error code for `project create` and the equivalent MCP tool.
- Add `agora project list --refresh-cache` to explicitly refresh the unfiltered first page used by project-name shell completion.
- Infer coarse agent labels for API `User-Agent` when `AGORA_AGENT` is unset; explicit `AGORA_AGENT` still takes precedence.
- Add top-level `agora doctor` command for an install self-test (binary path, PATH resolution, version, AGORA_HOME writability, API/OAuth network reachability, auth state, MCP host detection). Distinct from `agora project doctor` which validates a remote project.
- Add `agora env-help` command listing every `AGORA_*` (and `DO_NOT_TRACK`, `NO_COLOR`) environment variable the CLI honors, grouped by category, with defaults and accepted values. JSON envelope returns the catalog plus a category index.
- Add `agora skills` (list / show / search) curated workflow recipes for humans and AI agents. Read-only catalog shipped in the binary today; future releases can move to fetched skills with the same JSON shape.
- Add `--debug` global flag as the canonical name for runtime log echo (mirrors `AGORA_DEBUG=1`). Matches `gh`, `vercel`, `stripe`, and `supabase` conventions. See **Removed** below for the legacy `--verbose` / `AGORA_VERBOSE` cleanup that landed alongside it.
- Add `agora self-update` as an alias for `agora upgrade` / `agora update`.
- Add `--format envelope|json` to `agora project env` so callers can be explicit about the unified JSON envelope shape; `--format dotenv|shell` continue to render raw stdout for shell sourcing. Unknown formats now produce a typed error listing the valid choices.
- Add Cobra typo suggestions (`SuggestionsMinimumDistance: 2`) so `agora projct doctor` prints "Did you mean this? project". Matches `gh`, `kubectl`, `git` UX.
- Add `SECURITY.md` (private disclosure process, supported versions, safe harbor) and `SUPPORT.md` (channel routing for questions, bugs, security, install issues).
- Add `docs/troubleshooting.md` and link it from the README; add `Troubleshooting` and `Telemetry` to the GitHub Pages nav and sitemap.
- Add `docs/schema/envelope.v1.json` — JSON Schema for the unified envelope so wrappers can generate types or validate at runtime.
- Add curated agent rule snippets for the new `doctor`, `env-help`, and `skills` surfaces in the existing `docs/agents/` rule files via the next sync.
- Make installer shell setup auto-on by default and add granular opt-out flags. `install.sh` now adds the install directory to your shell rc when `agora` isn't already on `PATH` and writes a tab-completion script for the detected shell (bash, zsh, fish); `install.ps1` mirrors the behavior for user PATH and a PowerShell `$PROFILE` completion loader. New flags: `--no-path` / `-NoPath` (skip PATH only), `--no-completion` / `-NoCompletion` (skip completion only), and the umbrella `--skip-shell` / `-SkipShell` (binary only, no shell modifications). Matches modern installer DX (bun, fnm, deno, uv, volta). Auto-PATH wiring is **best-effort**: when the user's shell rc is unwritable (root-owned `~/.zshrc`, read-only mount, UAC denial, etc.) the installer never aborts — it uses bun-style branch wording (`<file> is not writable, so the installer can't add agora to your PATH automatically.`) followed by an indented action block that names the installed binary path and the exact `export PATH=...` (POSIX) or `setx PATH ...` (Windows) line to paste. The same block is reused when the user explicitly opts out via `--no-path`, so the message is identical across both paths.
- Polish installer DX to match bun / uv conventions: softer wording when the shell rc is unwritable (no implicit-failure tone), an `exec $SHELL` (POSIX) / `$env:Path += ';...'` (PowerShell) follow-up so the user can use `agora` in the current shell after install, a bash candidate-list walk that chooses the first writable rc among `~/.bashrc`, `~/.bash_profile`, and `~/.profile`, and a docs URL footer in the manual fallback block. Backed by a new shell-based smoke test (`scripts/test-installer-messages.sh`) wired into CI to prevent regression.
- `agora doctor` now suggests the exact shell-aware command for fixing a missing PATH entry. The `path_resolution` failure suggestion now reads `echo 'export PATH="<dir>:$PATH"' >> ~/.zshrc && source ~/.zshrc` for zsh, the equivalent for bash and `fish_add_path` for fish, the `setx PATH ...` form on Windows, and a generic `~/.profile` fallback for unknown shells. The doctor derives `<dir>` from the running binary's location, so the command is always copy-pasteable.
- Add a curated `Requirements` and `Verifying release artifacts` section to the README, plus links to `SECURITY.md`, `SUPPORT.md`, the new troubleshooting doc, and `docs/schema/envelope.v1.json` from the Docs index.
- Add a telemetry stub (`internal/cli/telemetry.go`) with the `telemetryClient` interface, default no-op sink, redaction helper, and `sentryClient` placeholder so the next release wires Sentry by adding the SDK + replacing one constant. The on/off contract (`agora telemetry`, `AGORA_SENTRY_ENABLED`, `DO_NOT_TRACK`) is fully wired; transport is a no-op until Sentry is connected.
- Add the proposal documents `docs/proposals/supply-chain-hardening.md`, `docs/proposals/ci-matrix-expansion.md`, and `docs/proposals/telemetry-sentry-wireup.md` for the next release.

### Changed

- Switch npm platform package wiring from scoped `@agoraio/cli-*` packages to unscoped `agoraio-cli-*` packages.
- Standardize README command examples on the installed `agora` command.
- Standardize contributor contact email on `devrel@agora.io`.
- Consolidate the `rtc` / `rtm` / `convoai` feature list into a single source of truth (`internal/cli/features.go`); `init`, `project create`, `project doctor`, `project feature {list,status,enable}`, MCP tools, shell completion, and `--help` text all read from the same catalog so future feature additions only need one entry.
- Default newly created projects to enable `rtc`, `rtm`, and `convoai`, make `convoai` imply `rtm` during project creation, and add `--rtm-data-center` for `init` / `project create` when RTM should be configured for a specific data center.
- Refine `agora init` project selection so `--project` binds explicitly, `--new-project` creates explicitly, `"Default Project"` auto-selects by exact name, and interactive sessions without a default show existing projects plus a create-new option.
- `agora project env write` detects Next.js workspaces and writes `NEXT_PUBLIC_AGORA_APP_ID` / `NEXT_AGORA_APP_CERTIFICATE`, with `--template nextjs|standard` to override auto-detection.
- `project env write` now creates or updates repo-local `.agora/project.json` for the selected project, recording `projectType` (framework/language detection such as `nextjs`, `go`, `python`, `node`, `standard`) and `envPath`, while quickstart-bound repos continue using a single `template` field for template lineage.
- Build and release metadata now target Go 1.26.2, matching the current stable Go toolchain for distributed CLI builds.
- Standardize per-command `Example:` blocks across the entire command tree. Every Cobra command now ships at least one example, including the previously empty telemetry subcommands and the `mcp` parent.
- `agora project env` `--format` is now a typed enum (`dotenv | shell | envelope | json`) with a precedence-aware error message when combined with `--json` / `--shell`.
- Harden the published Docker image: pin Alpine 3.20, run as a non-root `agora` user (uid 10001) with `AGORA_HOME=/home/agora/.agora`, add `tini` as PID 1 for proper signal handling, set OCI labels (`org.opencontainers.image.*`), and default `CMD` to `--help` so `docker run ghcr.io/agoraio/agora-cli:latest` is self-explanatory.
- Rewrite `docs/llms.txt` to fix the outdated command list (`agora init`, `agora project doctor` instead of obsolete labels), document the new `--format envelope` exception for `agora project env`, link the Markdown-mirror docs, and expand the catalog of stable exit codes.
- Rename `agora quickstart list --verbose` to `--details`. The previous flag overloaded the `--verbose` name with a meaning ("show repository, runtime, and env details") completely different from the global `--debug` semantic, so we removed it as part of the broader `--verbose` cleanup. The JSON envelope key emitted under `data` was renamed from `verbose` to `details` to match.
- Rename the `agora config update --verbose` flag to `--debug` and rename the persisted config field from `verbose` to `debug`. Bump the on-disk config schema from version `2` to version `3`. **0.1.x configs auto-migrate**: any existing `verbose` key is silently promoted to `debug` on the first 0.2.0 launch, and the rewritten file no longer contains the legacy key. Users do not need to take any action.

### Removed

- **BREAKING**: Drop the legacy `--verbose` / `-v` global flag and the `AGORA_VERBOSE` environment variable. v0.2.0 ships only `--debug` / `AGORA_DEBUG` for the runtime log-echo control. Rationale: maintaining two names for the same flag indefinitely creates exactly the confusing DX the rename was meant to fix; the 0.1.9 → 0.2.0 boundary is the right place to make the break instead of carrying an alias forward as permanent technical debt. Migration: replace `--verbose` / `-v` with `--debug`, and `AGORA_VERBOSE=1` with `AGORA_DEBUG=1`. Persisted configs are auto-migrated (see Changed above).
- **BREAKING (installer)**: Drop the opt-in `--add-to-path` (`install.sh`) and `-AddToPath` (`install.ps1`) flags. Shell setup is now auto-on by default; opt out with the new `--no-path` / `--no-completion` / `--skip-shell` flags (or `-NoPath` / `-NoCompletion` / `-SkipShell` on Windows). Migration: drop `--add-to-path` from any pinned install command — the bare `curl ... | sh` (and `irm ... | iex`) now does the right thing. CI environments that explicitly want only the binary should switch to `--skip-shell`.
- **BREAKING (installer)**: Drop the `--completion auto|skip|bash|zsh|fish|powershell` flag in favor of `--no-completion` (and the umbrella `--skip-shell`). Auto-detect from `$SHELL` is the only supported wiring path; users who want a completion script for a shell different from `$SHELL` should run `agora completion <shell>` directly.

### Fixed

- Fix `--yes` / `-y` / `AGORA_NO_INPUT=1` so it never silently launches an OAuth browser flow in JSON, CI, or non-TTY contexts. Industry convention for `-y` is "accept the default for confirmation prompts", not "spawn a brand-new interactive flow"; those contexts now consistently fail fast with `AUTH_UNAUTHENTICATED`.
- Fix the MCP server's stdio scanner to allow JSON-RPC frames up to 4 MiB (was 64 KiB) so large `tools/call` payloads no longer truncate the loop.
- Fix the MCP `agora.init` tool to never read from `os.Stdin` (the JSON-RPC transport stream) or write to `os.Stderr`; `initProject` is now invoked with an empty in-memory reader and a discarded prompt writer.
- Fix the MCP server's notification handling to match JSON-RPC 2.0: any frame without an `id` is treated as a notification and produces no response (previously notifications without the `notifications/` method prefix received an `id: null` reply).
- Fix `printBlock` value-column truncation, which used to silently no-op because `COLUMNS` is a shell-internal variable and is rarely exported to child processes. The CLI now consults `COLUMNS` first (so users and tests can override) and falls back to `golang.org/x/term.GetSize` against stderr / stdout, with a "no terminal detected → don't truncate" safe default for log scrapers and CI build logs.
- Fix `agora open --target docs` URL resolution to be configurable: each target now reads from `AGORA_CONSOLE_URL` / `AGORA_DOCS_URL` / `AGORA_PRODUCT_DOCS_URL` (when set), falling back to the compiled-in canonical URLs. A new smoke test asserts every compiled-in URL parses, uses HTTPS, and has a host, and that `cliDocsURL` and `.github/workflows/pages.yml` stay in sync.
- Fix project-name shell completion so the on-disk cache is ignored when the local session is missing, empty, or locally expired.
- Fix bug report template references to use `agora project doctor --json`.
- Return structured `INIT_NAME_REQUIRED`, `AUTH_OAUTH_EXCHANGE_FAILED`, and `AUTH_OAUTH_RESPONSE_INVALID` errors for previously unclassified paths.

### Documentation

- Document the MCP transport caveat that `agora init`, `agora quickstart create`, `agora project create`, and `agora login` collapse their NDJSON progress event stream into the final `tools/call` result over MCP, since stdout is the JSON-RPC transport.
- README updates land in logical sections (Install requirements, Verifying releases, Docs index, Command Model additions, Troubleshooting redirect) without disturbing the marketing-first opening.
- `CONTRIBUTING.md` documents the branching model (`main` is releasable, topic branches off `main`), the commit-message convention, the optional DCO sign-off path, and the per-command example requirement for new commands.
- `docs/automation.md` adds a section documenting the `agora project env` raw-stdout exception and links the new envelope JSON Schema.
- `docs/proposals/` introduces a new directory for deferred-implementation proposals (supply-chain hardening, CI matrix expansion, Sentry wire-up). Each proposal carries a `status: proposed`, `target-release: next` front-matter so contributors can see what's planned without bisecting branches.

## [0.1.9] - 2026-04-30

### Changed

- Add direct-installer provenance receipts (`agora.install.json`) and make `agora upgrade` use receipt-first install-method detection before falling back to package-manager path inference.

## [0.1.8] - 2026-04-30

### Fixed

- Preserve OAuth PKCE query parameters on Windows by opening browser login URLs through `rundll32 url.dll,FileProtocolHandler` instead of `cmd /c start`.
- Accept OAuth callbacks on both IPv4 and IPv6 localhost loopback addresses so Windows `localhost` resolution does not strand successful browser sign-ins.
- Update the release workflow output wiring to avoid self-referencing step outputs during dry-run and publish-mode setup.

## [0.1.7] - 2026-04-30

### Added

- Auto-detect CI environments (`CI`, `GITHUB_ACTIONS`, `GITLAB_CI`, `BUILDKITE`, `CIRCLECI`, `JENKINS_URL`, `TF_BUILD`) and automatically default `--output` to `json`, suppress the first-run config banner, and short-circuit interactive prompts. Explicit `--output` flags, user-set `AGORA_OUTPUT`, and `AGORA_DISABLE_CI_DETECT=1` always take precedence.
- Add a `.golangci.yml` ruleset (errcheck, govet, staticcheck, ineffassign, unused, gosec, bodyclose, errorlint, misspell, unconvert) and wire `golangci-lint v1.64.8` into the Linux CI matrix. The `make lint` target now runs `gofmt`, `golangci-lint`, and the error-code coverage audit together.
- Add an interactive sign-in prompt for human CLI sessions when an account connection is required and no local session exists. The prompt defaults to yes on Enter and launches the existing OAuth login flow.
- Re-enable the npm distribution channel (`agoraio-cli` wrapper plus six platform packages). The release workflow now downloads the GitHub release archives, verifies them against `checksums.txt` (SHA-256), stages binaries into platform packages, stamps the tag version into every `package.json`, and publishes all packages with `npm publish --provenance` (sigstore-backed supply-chain attestations).
- Add a post-publish smoke test that runs `npx --yes agoraio-cli@<tag> --version` with retry/backoff to catch registry-propagation or platform-package-mismatch bugs before users hit them.
- Add a `workflow_dispatch` trigger to the release workflow with a `dry_run` input so maintainers can validate npm packaging end-to-end without minting a real release.
- Enrich every npm `package.json` (wrapper + 6 platform packages) with `repository`, `homepage`, `bugs`, `license`, `author`, `keywords`, and `publishConfig.provenance` for a higher-quality npmjs.com listing and supply-chain attestation.
- Inject version, commit, and build date at release time and surface them through `agora version` and `--version`.
- Add `agora introspect`, `agora telemetry`, `agora upgrade` (alias `update`), and `agora open` for agent and human workflows.
- Add global `--pretty`, `--quiet`, and `--no-color` flags, plus `agora whoami --plain` for shell-friendly auth checks.
- Add `AGORA_AGENT` propagation into the API `User-Agent`, `project create --dry-run` / `--idempotency-key`, and `quickstart create --ref`.
- Add `quickstart list --verbose` for richer template details in pretty output.
- Honor `DO_NOT_TRACK=1` to disable telemetry without editing config.
- Add this changelog so users can review notable CLI changes from version to version.
- Add golden-file tests (`internal/cli/golden_test.go` + `testdata/golden/*.json`) for stable agent envelopes (`introspect` pseudoCommands, globalFlags, enums; `auth status` AUTH_UNAUTHENTICATED). Golden files can be regenerated with `go test ./internal/cli -run Golden -update` and must be committed alongside any contract change.
- Add an auto-generated CLI command reference at `docs/commands.md`. A new `cmd/gendocs` Go program walks the cobra tree and renders Markdown; `make docs-commands` regenerates it locally. CI fails on drift, and the release workflow attaches the regenerated reference as a GitHub release asset so the published doc never lies about the binary in the same tag.
- Generate SPDX 2.3 SBOMs (per archive + per Linux package) and Cosign keyless signatures for the `checksums.txt` file and every published Docker image. New verification recipes in `docs/install.md` show users how to verify with `cosign verify-blob` / `cosign verify` and audit dependencies with Grype against the published SBOMs.
- Add a global `--verbose` persistent flag that mirrors the existing `AGORA_VERBOSE=1` behavior — echoes structured log entries to stderr alongside the log file. Exit codes, JSON envelope shape, and NDJSON progress events are unchanged.
- `project doctor` now attaches a `suggestedCommand` to the two remaining blocking issues that were missing one (`APP_CREDENTIALS_MISSING` → `agora project show --project <id>`; `WORKSPACE_ENV_READ_FAILED` → `agora quickstart env write . --project <id> --overwrite`), so every blocking issue carries an actionable recovery hint for both human and agentic consumers.

### Changed

- `--quiet` now suppresses the success envelope in **both** pretty and JSON modes (previously it only suppressed pretty output). Errors still print on stderr; NDJSON progress events are still emitted because they are observability rather than results. Updated the flag help to reflect the new semantics.
- Standardize unauthenticated failures across API-touching commands to return exit code `3` with `error.code == "AUTH_UNAUTHENTICATED"` in JSON mode.
- Return `project doctor --json` readiness failures as `ok: false` with matching `meta.exitCode`, while preserving the diagnostic `data` payload.
- Improve project resolution to try project-ID lookups directly and paginate name searches, surfacing ambiguous matches instead of silently picking one.
- Return stable `error.code` values for project and quickstart failures (`PROJECT_NOT_SELECTED`, `PROJECT_NOT_FOUND`, `PROJECT_NO_CERTIFICATE`, `PROJECT_AMBIGUOUS`, `QUICKSTART_TEMPLATE_UNKNOWN`, `QUICKSTART_TARGET_EXISTS`, etc.) so scripts and agents can branch on them.
- Replace the OAuth callback page with a branded success view after sign-in.
- Prompt for an `init` template in interactive pretty-mode runs when `--template` is omitted, while keeping JSON, CI, and non-TTY runs strict.
- Print quickstart next steps from `quickstart create` and include `reusedExistingProject` in `init` results.
- Limit env file writes to runtime credential keys only, keeping project metadata in `.agora/project.json` and preserving existing `.env` / `.env.local` content.
- Update installer, README, install docs, and Homebrew formula references from `AgoraIO-Community/cli` to `AgoraIO/cli`.
- Keep automation non-interactive when auth is missing. JSON output, `AGORA_OUTPUT=json`, CI, and non-TTY runs still fail fast with the existing login-required error instead of prompting.
- Update `agora init` project reuse to prefer a project named `Default Project`, then the project with the latest `createdAt` value from the current results page.

### Fixed

- OAuth callback HTTP server now sets `ReadHeaderTimeout` (gosec G112 — Slowloris mitigation), even though it only listens on `127.0.0.1`.
- `agora upgrade` extraction (tar.gz and zip) now caps decompressed binary size at 256 MiB to defend against malicious release archives (gosec G110).

### Refactor

- Split `internal/cli/app.go` (1029 lines) into focused files for contributor velocity: `envelope.go` (JSON envelope + exit codes), `render.go` (pretty output dispatch), `paths.go` (config/session/context paths and `writeSecureJSON`), `config.go` (`appConfig` + defaults + env injection), `version.go` (build-time version vars). `app.go` now contains only the `App` struct, `Execute`, the output-mode resolver, and core helpers (378 lines, a 63% reduction). Behavior is unchanged; all existing tests pass.
- Extract introspection helpers (`buildIntrospectionData`, `buildCommandTree`, `commandHelpInfo`, `flagHelpInfo`, `pseudoCommandInfo`, `showAllHelp`, `nonTrivialDefault`) plus `buildIntrospectCommand` from `commands.go` into `introspect.go` so the agent-discovery surface lives in one file.
- Split `internal/cli/integration_test.go` (1330 lines) into command-area files (`integration_help_test.go`, `integration_quickstart_test.go`, `integration_init_test.go`, `integration_auth_test.go`, `integration_project_test.go`). `integration_test.go` now contains only shared helpers (`runCLI`, `fakeOAuthServer`, `fakeCLIBFF`, `createLocalGitRepo`, `parseAuthURL`, `persistSessionForIntegration`).
- Correct the npm wrapper's error-path URLs to `AgoraIO/cli`, matching the rest of the repo.
- Fix Cobra example formatting so the first example line keeps its indentation in command help.

### Documentation

- Add standard contributor surfaces: `CONTRIBUTING.md`, `CODE_OF_CONDUCT.md` (Contributor Covenant 2.1), `.github/pull_request_template.md`, and `.github/ISSUE_TEMPLATE/{config.yml,bug_report.yml,feature_request.yml}` so first-time contributors land on the standard GitHub forms instead of a blank issue.
- Document the new CI auto-detect behavior, the precedence order, and the escape hatch in `docs/automation.md`.
- Document the npm channel as `Available` in `docs/install.md` with install, pin, and update examples.
- Document the active npm release flow, the `NPM_TOKEN` and `id-token: write` requirements, the dry-run workflow_dispatch path, the pre-tag checklist, and the npm rollback procedure in `RELEASING.md`.
- Update `AGENTS.md` to reflect that npm publishing is active and to describe the checksum verification, provenance, and smoke-test additions.
- Add `npm install -g agoraio-cli` as an alternative install one-liner in the README.
- Document the interactive-auth behavior and `init` default-project fallback in `docs/automation.md`.
- Add `docs/error-codes.md` cataloguing stable `error.code` values and `docs/telemetry.md` covering telemetry controls and `DO_NOT_TRACK`.

## [0.1.6] - 2026-04-28

### Fixed

- Update GoReleaser Docker image and manifest templates to lowercase the GitHub repository owner before publishing to GHCR, which requires lowercase registry paths.

## [0.1.5] - 2026-04-28

### Changed

- Scope the release workflow to installer-supported artifacts while npm, Homebrew tap, and Scoop bucket publishing remain disabled.
- Keep GoReleaser archive naming stable for shell and PowerShell installers.
- Keep Docker image publishing through GoReleaser with per-architecture images and manifests.

## [0.1.4] - 2026-04-28

### Added

- Provide the native Agora CLI command model for auth, project management, quickstart setup, and the composed `init` onboarding flow.
- Support OAuth login and logout through `agora login`, `agora auth login`, `agora logout`, and `agora auth logout`.
- Support session inspection through `agora whoami` and `agora auth status`.
- Support project creation, selection, env export, env file writes, and readiness checks through the `project` command group.
- Support official quickstart cloning and template-specific env file generation through the `quickstart` command group.
- Support `agora init` as the recommended end-to-end onboarding command that creates or reuses an Agora project, clones a quickstart, writes env, persists context, and prints next steps.
- Support machine-readable JSON output for automation and agent workflows.
- Ship automated release packaging through GoReleaser, including cross-platform archives, Linux packages, Homebrew, Scoop, npm wrapper packages, Docker images, and install scripts.

[Unreleased]: https://github.com/AgoraIO/cli/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/AgoraIO/cli/compare/v0.1.9...v0.2.0
[0.1.9]: https://github.com/AgoraIO/cli/compare/v0.1.8...v0.1.9
[0.1.8]: https://github.com/AgoraIO/cli/compare/v0.1.7...v0.1.8
[0.1.7]: https://github.com/AgoraIO/cli/compare/v0.1.6...v0.1.7
[0.1.6]: https://github.com/AgoraIO/cli/compare/v0.1.5...v0.1.6
[0.1.5]: https://github.com/AgoraIO/cli/compare/v0.1.4...v0.1.5
[0.1.4]: https://github.com/AgoraIO/cli/releases/tag/v0.1.4
