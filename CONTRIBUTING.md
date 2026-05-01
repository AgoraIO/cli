# Contributing to Agora CLI

Thanks for your interest in improving the Agora CLI! This document covers the
quick path: how to set up, what to run before opening a PR, where to find more
detailed guidance, and what we expect from contributions.

## Code of conduct

This project adheres to the [Contributor Covenant](CODE_OF_CONDUCT.md). By
participating, you agree to uphold its standards. Please report unacceptable
behavior to <devrel@agora.io>.

## Where to read first

For day-to-day development, the most important reference is **[AGENTS.md](AGENTS.md)**
in the repo root. It documents:

- The CLI's design contracts (JSON envelopes, exit codes, error code catalog).
- Project layout (`internal/cli/`, `cmd/`, `docs/`, `packaging/`).
- How to add a new command end-to-end (registration, JSON shape, tests, docs).
- The CI/release pipeline.
- The lint and test workflow.

`AGENTS.md` is the contract for both human contributors and AI coding agents,
so it is kept current as the canonical engineering guide.

For end-user behavior and machine-readable contracts, see:

- [`README.md`](README.md) — install, getting-started, command tree.
- [Published docs](https://agoraio.github.io/cli/) — human-readable CLI documentation (GitHub Pages).
- [`docs/automation.md`](docs/automation.md) — JSON envelope, agent guidance,
  output mode precedence (including CI auto-detect).
- [`docs/error-codes.md`](docs/error-codes.md) — every stable `error.code`.
- [`docs/install.md`](docs/install.md) — installer / package channel matrix.

## Quick setup

Requirements:

- **Go** 1.26.2+ (see `go.mod`). Release builds intentionally track the current stable Go toolchain; this distributed CLI does not target older Go compiler support.
- **Git**.
- (Optional) `golangci-lint` v1.64.8 — install matches CI; instructions in
  the next section.

```bash
git clone https://github.com/AgoraIO/cli.git
cd cli/agora-cli-go

go build -trimpath -o agora .
./agora --help
```

## Tests, lint, and the pre-PR checklist

Run the full local check suite before opening a PR:

```bash
make test            # go test ./...
make lint            # gofmt + golangci-lint + error-code coverage audit
```

Or run pieces individually:

```bash
go test ./...                              # unit + integration tests
gofmt -l .                                  # must print nothing
golangci-lint run --timeout=5m              # uses .golangci.yml
./scripts/check-error-codes.sh              # docs/error-codes.md drift check
go run ./cmd/gendocs -check                 # docs/commands.md drift check
make docs-preview                           # optional: local Jekyll site + /md preview (requires Ruby/Jekyll)
```

Documentation work:

- Run `make docs-commands` after command-tree changes; CI uses `go run ./cmd/gendocs -check`.
- For GitHub Pages content, use `make docs-preview` (see `scripts/preview-pages-site.sh`). Published docs resolve `@@CLI_DOCS_*@@` tokens via `scripts/prepare-pages-site.py` and `docs/site.env` as documented in `docs/automation.md`.

Install `golangci-lint` **v1.64.8** (matches CI). CI builds it with `go install` against your toolchain; locally prefer:

```bash
go install github.com/golangci/golangci-lint/cmd/golangci-lint@v1.64.8
```

Or use the [install script](https://golangci-lint.run/welcome/install/) if the binary supports your `go.mod` Go version.

When changing release packaging, also run a snapshot release:

```bash
make release-snapshot   # goreleaser release --snapshot --clean
```

## Adding a new command

1. Create `internal/cli/<noun>.go` with business logic on `*App`.
2. Register the command in `commands.go` inside `buildRoot()`.
3. Use `a.resolveOutputMode(cmd)` to honor `--json` / `--output` / CI auto-detect.
4. Return results through `renderResult(cmd, "<command label>", data)`.
5. Document the JSON shape and any new `error.code` values in `docs/automation.md`.
6. Run `make docs-commands` to refresh `docs/commands.md`. CI fails if it drifts.
7. Add a happy-path JSON test in the appropriate `internal/cli/integration_*_test.go` file (helpers live in `integration_test.go`).
8. Add edge-case unit tests in `internal/cli/app_test.go` for non-trivial logic.
9. If a new `error.code` was introduced, add it to `docs/error-codes.md`. The
   `make lint` audit will fail otherwise.

## Adding or changing an `error.code`

Every literal `Code:` value emitted from `internal/cli/*.go` must be
documented in `docs/error-codes.md`. Dynamic prefixes (e.g. `FEATURE_<NAME>_PROVISIONING`)
must have a documented prefix entry. CI runs `scripts/check-error-codes.sh`
on every PR.

Codes are part of the public CLI contract. Renaming a code is a breaking
change; prefer adding a new code and deprecating the old one over a rename.

## Coding standards

- **Formatting:** `gofmt` — enforced in CI.
- **Lint:** `golangci-lint` with the project `.golangci.yml`. Prefer narrowing
  rules in the config file over inline `//nolint` directives.
- **Errors:** wrap with `%w` (`fmt.Errorf("doing X: %w", err)`); use
  `errors.As` / `errors.Is` rather than type assertions.
- **JSON shapes:** the JSON envelope (`ok`, `command`, `data`, `error`, `meta`)
  is a public contract. Any breaking change must be called out in the PR
  description and the changelog.
- **Logging:** use `appendAppLog` for structured logs. Never log secrets — the
  `sensitiveFieldPattern` redactor catches obvious cases but is not a
  substitute for thinking about what you're emitting.
- **Tests:** prefer integration tests in `integration_test.go` for behavior
  that is part of the public contract (JSON shape, exit code, stderr text).
  Use `app_test.go` for isolated helper logic.

## Branching model

- `main` is always releasable. CI must be green before merge.
- Feature work happens on short-lived topic branches off `main` named
  `feat/<scope>`, `fix/<scope>`, `docs/<scope>`, or `chore/<scope>`
  (matches the conventional-commits prefixes). Avoid long-running
  branches; rebase on `main` instead of merging it back into your
  topic branch.
- Releases are cut from `main` by tagging `vX.Y.Z`. The release
  workflow handles building, signing, publishing, and Homebrew /
  Scoop / npm bumps. See [docs/install.md](docs/install.md) for the
  release matrix.

## Commit hygiene

- Keep commits focused. One logical change per commit is preferred.
- Write present-tense imperative subjects ("Add CI auto-detect", not
  "Added CI auto-detect"). We prefer (but do not strictly require)
  the [Conventional Commits](https://www.conventionalcommits.org/)
  prefixes (`feat:`, `fix:`, `docs:`, `chore:`, `refactor:`,
  `test:`, `build:`).
- Reference issues with `Fixes #123` / `Refs #456` in the body when relevant.
- We do not require [DCO](https://developercertificate.org/) sign-off
  today, but contributors are welcome to sign their commits with
  `git commit -s`. If we adopt mandatory sign-off in the future, we
  will announce it here and add a CI check.

## Pull requests

- Fill in the [pull request template](.github/pull_request_template.md).
- Make sure `make test && make lint` pass locally.
- Include changelog entries under the `## [Unreleased]` section of `CHANGELOG.md`
  for user-facing changes (new commands, behavior changes, breaking changes,
  CLI exit code changes, error code additions). When cutting a release, move
  those bullets into a dated `## [x.y.z] - YYYY-MM-DD` section per the note at
  the top of `CHANGELOG.md` (for example, v0.2.0 shipped as `## [0.2.0] - 2026-05-01`).
- For UI/UX-affecting changes (pretty output, prompts, progress events,
  errors), include before/after copy-paste samples in the PR description.
- New commands MUST include a per-command example block in the Cobra
  `Example:` field. See "Adding a new command" above and the existing
  `agora skills`, `agora doctor`, and `agora env-help` builders for the
  current style.

## Reporting bugs and requesting features

Use the GitHub issue templates:

- [Bug report](https://github.com/AgoraIO/cli/issues/new?template=bug_report.yml)
- [Feature request](https://github.com/AgoraIO/cli/issues/new?template=feature_request.yml)

For **support** (questions, "how do I", install help) see [SUPPORT.md](SUPPORT.md).

For **security** issues, see [SECURITY.md](SECURITY.md). Email
<security@agora.io> rather than filing a public issue. Do not include
credentials or App Certificates in any public report.

## License

By contributing, you agree that your contributions will be licensed under
the project's MIT license.
