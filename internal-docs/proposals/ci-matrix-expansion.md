---
title: CI matrix expansion proposal
status: proposed
target-release: next
---

# CI matrix expansion proposal

> Status: **proposed**, awaiting approval. This document outlines the
> required steps and the implications of expanding the CI matrix.
> Implementation is deferred to the next release.

The current CI workflow (`.github/workflows/ci.yml`) runs `go test`
once per OS (`ubuntu-latest`, `macos-latest`, `windows-latest`) against
the toolchain pinned in `go.mod`. This proposal adds three missing
signals — race detection, coverage, and a Go version matrix — and
documents the runtime, cost, and maintenance implications.

## Current state

```yaml
# .github/workflows/ci.yml (current)
strategy:
  fail-fast: false
  matrix:
    os: [ubuntu-latest, macos-latest, windows-latest]
- uses: actions/setup-go@v5
  with:
    go-version-file: go.mod
    cache: true
- name: Run tests
  run: go test -count=1 ./...
```

- 3 OS jobs.
- Single Go toolchain per job.
- No `-race`. No `-cover`. No coverage upload.

## Proposed changes

### 1. Add `go test -race` on Linux

The CLI shells out to `git`, parses HTTP responses concurrently in the
project list cache and the OAuth callback server, and serves an MCP
stdio loop. None of this is currently exercised under the race
detector. Linux `-race` is the cheapest, highest-signal addition.

```yaml
- name: Run tests with race detector (Linux)
  if: runner.os == 'Linux'
  run: go test -race -count=1 ./...
```

**Implication:** ~2x the test runtime on Linux, but no extra job and no
parallelism cost. Existing `go test` line stays unchanged for macOS
and Windows because `-race` requires CGO and is markedly slower on
those runners.

### 2. Add `go test -cover` and Codecov upload on Linux

```yaml
- name: Run tests with coverage (Linux)
  if: runner.os == 'Linux'
  run: go test -coverprofile=coverage.out -covermode=atomic ./...

- name: Upload coverage to Codecov
  if: runner.os == 'Linux'
  uses: codecov/codecov-action@v5
  with:
    files: coverage.out
    fail_ci_if_error: false
    token: ${{ secrets.CODECOV_TOKEN }}
```

**Implications:**

- Requires a Codecov account and `CODECOV_TOKEN` secret (Codecov is
  free for public repos but the token avoids rate limits).
- Adds a coverage badge to the README.
- Sets up a soft "no coverage regression" PR check (Codecov default).
  We can switch to hard-fail later once a baseline stabilizes.
- We will need to either combine coverage from `-race` and non-race
  runs, or pick one. Recommendation: run `-race -coverprofile=...` in
  a single step so we get both signals from one binary.

### 3. Add a Go version matrix

```yaml
strategy:
  fail-fast: false
  matrix:
    os: [ubuntu-latest, macos-latest, windows-latest]
    go: [stable, oldstable]
- uses: actions/setup-go@v5
  with:
    go-version: ${{ matrix.go }}
    cache: true
```

**Implications:**

- Doubles the matrix cell count from 3 to 6. With the current ~4
  minute per-cell runtime that adds ~12 minutes of CI time per PR
  (parallelized across runners, wall-clock impact is closer to ~4
  minutes since they run concurrently).
- `go.mod` keeps its pinned version for production builds; CI matrix
  proves the codebase compiles and tests pass against the two toolchain
  lines GitHub-hosted runners support out of the box.
- We should add a `go.work` exclude or build constraint if any test
  uses Go-version-gated APIs (none today).
- Release workflow continues to use `go-version-file: go.mod` so we
  ship one toolchain and one toolchain only.

## Optional follow-ups

These are flagged for discussion but not part of the v1 proposal:

- **Add `go vet -all` as a separate step.** Currently included
  implicitly via `golangci-lint`'s `govet` linter. Explicit step would
  surface `vet` failures separately from lint.
- **Add `gosec` SARIF upload.** Already enabled in `golangci-lint`,
  but standalone SARIF would surface in the Security tab.
- **Run integration tests in a dedicated job with `-tags=integration`.**
  Currently mixed with unit tests.
- **Macos `-race`.** Useful but expensive (~3x slower on Apple
  Silicon runners); defer until we have a concurrency bug that justifies it.

## Implications summary

| Change | New CI minutes / PR (approx) | New required secret | New required action | Risk |
|--------|------------------------------|---------------------|---------------------|------|
| `-race` on Linux | +2 min | none | none | low (catches real bugs) |
| `-cover` + Codecov upload | +1 min | `CODECOV_TOKEN` | Codecov account | low (soft-fail) |
| Go version matrix `[stable, oldstable]` | +12 min total / +4 min wall-clock | none | none | medium (oldstable lag may surface API drift) |

Total wall-clock CI runtime for a PR moves from roughly 4-5 minutes to
roughly 8-10 minutes.

## Rollout plan

1. Land `-race` first as a single-line addition. Watch for new test
   failures over a week.
2. Add `-cover` + Codecov in a follow-up once `CODECOV_TOKEN` is
   provisioned and the README has space for the badge.
3. Add the Go matrix last. If `oldstable` ever holds back development,
   drop back to `[stable]` only and document why in this file.

## Reasoning

| # | Why we need it |
|---|----------------|
| 1 | The CLI runs concurrent goroutines (HTTP, OAuth callback, MCP loop, project cache). Without `-race` we will eventually ship a data race that only manifests for users with high CPU concurrency. |
| 2 | Coverage is the single most useful CI signal for a CLI of this size. We do not need a hard threshold; just visibility on which paths are tested. |
| 3 | `go.mod` pins a single toolchain. Users on slightly older Go versions (common in enterprise) will hit confusing errors if we accidentally use a feature they cannot consume. The matrix catches this on PR. |

## Out of scope

- Self-hosted runners.
- Cross-compilation matrix beyond what GoReleaser already covers in
  the release workflow.
- Mutation testing.
