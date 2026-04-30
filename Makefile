.PHONY: build test lint lint-go lint-fmt snapshot snapshot-error-codes release release-snapshot docs-commands docs-commands-check

# Build a local agora binary with stripped paths.
build:
	go build -trimpath -o agora .

# Run the full Go test suite.
test:
	go test ./...

# Aggregate lint target. Runs gofmt, golangci-lint (if installed), the
# error-code coverage audit, and the docs/commands.md drift check. Use
# this in CI and pre-commit.
lint: lint-fmt lint-go snapshot-error-codes docs-commands-check

lint-fmt:
	@unformatted="$$(gofmt -l .)"; \
	if [ -n "$$unformatted" ]; then \
		echo "::error::gofmt found unformatted files:"; \
		echo "$$unformatted"; \
		exit 1; \
	fi

# Run golangci-lint with the project config (.golangci.yml).
# CI uses v1.64.8 (matched in .github/workflows/ci.yml). To install locally:
#   curl -sSfL https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh \
#     | sh -s -- -b "$$(go env GOPATH)/bin" v1.64.8
# Set LINT_GO_STRICT=1 to fail when golangci-lint is missing (CI default).
lint-go:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run --timeout=5m ./...; \
	elif [ "$${LINT_GO_STRICT:-0}" = "1" ]; then \
		echo "::error::golangci-lint not installed. Install: https://golangci-lint.run/usage/install/" >&2; \
		exit 1; \
	else \
		echo "golangci-lint not installed; skipping. Install: https://golangci-lint.run/usage/install/"; \
	fi

# Snapshot the introspect envelope (mainly used to spot-check schema changes).
snapshot:
	go run . introspect --json

# Audit that docs/error-codes.md documents every Code: literal in internal/cli/*.go.
# Fails non-zero if any code is undocumented. Wired into CI via `make lint`.
snapshot-error-codes:
	./scripts/check-error-codes.sh

# Run a full GoReleaser snapshot release locally (no publish, no GitHub release).
# Useful for verifying .goreleaser.yaml changes before tagging.
release-snapshot:
	@if command -v goreleaser >/dev/null 2>&1; then \
		goreleaser release --snapshot --clean; \
	else \
		echo "::error::goreleaser not installed. Install: https://goreleaser.com/install/"; \
		exit 1; \
	fi

# Alias for release-snapshot to match common Make conventions.
release: release-snapshot

# Regenerate docs/commands.md from the live cobra command tree. Run this
# whenever you add, remove, rename, or rewrap a command/flag.
docs-commands:
	go run ./cmd/gendocs

# Drift check used in CI: fails non-zero if docs/commands.md does not match
# the current command tree. The fix is to run `make docs-commands` and
# commit the result alongside the code change.
docs-commands-check:
	go run ./cmd/gendocs -check
