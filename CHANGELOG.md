# Changelog

All notable changes to Agora CLI are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

When tagging a new release, rename the `[Unreleased]` section to the new version
(e.g. `[0.2.0] - 2026-04-30`), add a fresh empty `[Unreleased]` heading at the top,
and update the link references at the bottom of this file.

When adding a new entry, link the change to the PR or commit that introduced it
using the trailing `([#123](https://github.com/AgoraIO/cli/pull/123))` convention.
Earlier entries pre-date this convention and only carry their version's compare link.

## [Unreleased]

## [0.2.0] - 2026-04-30

### Added

- Add GitHub Pages publishing for generated CLI docs and route `agora open --target docs` to the human CLI docs site, `agora open --target docs-md` to the agent-facing raw Markdown docs under `/md/`, and `product-docs` to Agora product docs.
- Add `docs/site.env` and Pages build-time URL injection so staging docs can publish with different `CLI_DOCS_BASE_URL` / `CLI_DOCS_MD_BASE_URL` values while keeping predictable human and Markdown paths.
- Add a custom GitHub Pages theme for the human docs with responsive layout, system light/dark mode via `prefers-color-scheme`, and no manual theme toggle.
- Add `make docs-preview` for a Ruby/Jekyll local docs preview that builds with localhost-friendly paths, injects localhost docs URLs, and serves both the human site and `/md/` Markdown tree.
- Add global `--yes` / `-y` and `AGORA_NO_INPUT=1` support to accept defaults and suppress prompts.
- Add pretty-mode progress status lines for long-running clone, OAuth, and project creation work.
- Add dynamic shell completions for project names, quickstart templates, and project features, with an on-disk completion cache under `<AGORA_HOME>/cache/projects.json` so `agora project use <TAB>` is instant on warm caches. Configurable via `AGORA_PROJECT_CACHE_TTL_SECONDS` and disable-able via `AGORA_DISABLE_CACHE=1`.
- Add `agora mcp serve --transport stdio` so MCP-capable agents can use local Agora CLI tools, exposing the full surface (`agora.version`, `agora.introspect`, `agora.auth.*`, `agora.config.*`, `agora.telemetry.status`, `agora.upgrade.check`, `agora.project.*` including `create`/`env`/`feature.{list,status,enable}`, `agora.quickstart.*`, and `agora.init`).
- Add drop-in agent rule snippets under `docs/agents/` and `agora init --add-agent-rules` with safe append-when-exists semantics: subsequent runs update only the Agora-managed block between sentinel markers and never destroy pre-existing user content.
- Add `install.sh --uninstall` and `install.ps1 -Uninstall`.
- Add CODEOWNERS, Dependabot, and a scheduled `govulncheck` workflow.
- Add `PROJECT_NAME_REQUIRED` error code for `project create` and the equivalent MCP tool.
- Add `agora project list --refresh-cache` to explicitly refresh the unfiltered first page used by project-name shell completion.
- Infer coarse agent labels for API `User-Agent` when `AGORA_AGENT` is unset; explicit `AGORA_AGENT` still takes precedence.

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
