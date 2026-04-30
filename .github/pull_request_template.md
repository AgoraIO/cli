<!--
Thanks for contributing to Agora CLI!

If this is your first PR here, please skim CONTRIBUTING.md and AGENTS.md
before opening a review. The CLI's JSON envelope, exit codes, and error.code
values are public contracts — changes that affect them must be called out
below and reflected in the changelog.
-->

## Summary

<!--
What does this change do, and why? 1-3 sentences.
Link the related issue(s): "Fixes #123" / "Refs #456".
-->

## Type of change

- [ ] Bug fix (non-breaking)
- [ ] New feature (non-breaking, additive)
- [ ] Behavior change to an existing command (potentially user-visible)
- [ ] Breaking change (CLI flag, exit code, JSON shape, or `error.code` rename/removal)
- [ ] Documentation only
- [ ] CI / packaging / tooling
- [ ] Refactor (no behavior change)

## Public-contract impact

- [ ] No public-contract impact.
- [ ] Adds or changes a JSON envelope shape — described below.
- [ ] Adds a new `error.code` — added to `docs/error-codes.md`.
- [ ] Renames or removes an `error.code` — flagged as breaking, included in CHANGELOG.
- [ ] Changes an exit code for an existing command — flagged as breaking.
- [ ] Adds or changes a CLI flag — documented in help text and (if user-facing) `docs/automation.md`.

<!--
For any "Adds or changes" item above, paste the before/after JSON envelope
or pretty output here so reviewers can spot the contract delta at a glance.
-->

## Test plan

<!--
Describe what you ran (and any new tests you added). At minimum:

  go test ./...
  make lint

Include before/after CLI output samples when the change affects pretty
output, errors, or progress events.
-->

- [ ] `go test ./...` passes locally.
- [ ] `make lint` passes locally (`gofmt`, `golangci-lint`, error-code coverage audit).
- [ ] New behavior is covered by a JSON-mode integration test in `internal/cli/integration_test.go`.
- [ ] Edge cases are covered by unit tests in `internal/cli/app_test.go` (where applicable).

## Documentation

- [ ] `CHANGELOG.md` updated under `## Unreleased` (Added / Changed / Deprecated / Removed / Fixed / Security).
- [ ] `docs/automation.md` updated for any user-facing JSON shape, env var, or flag change.
- [ ] `docs/error-codes.md` updated for any new `error.code` (or N/A).
- [ ] `README.md` updated if the command tree, install path, or quickstart changed.
- [ ] `AGENTS.md` updated if engineering or release process changed.

## Security checklist

- [ ] No credentials, App Certificates, tokens, or PII added to fixtures, logs, or test output.
- [ ] No new outbound network call without timeout / context cancellation.
- [ ] No new file written under user `$HOME` without `0o600` perms when it can contain credentials (e.g. session, config).
- [ ] No new `unsafe` import.

## Additional notes

<!--
Anything reviewers should pay extra attention to (tricky migration, behavior
under CI auto-detect, locked Cobra alias, etc.). Optional.
-->
