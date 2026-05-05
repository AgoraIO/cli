package cli

import "fmt"

// Build-time injected version variables. These are populated by ldflags at
// release time:
//
//	go build -ldflags '-X github.com/.../internal/cli.version=v0.2.0
//	                    -X github.com/.../internal/cli.commit=abc1234
//	                    -X github.com/.../internal/cli.date=2026-05-04'
//
// Snapshot/local builds keep the placeholder values below.
var (
	version = "dev"
	commit  = "unknown"
	date    = "unknown"
)

// versionInfo returns the structured version payload used by the JSON
// envelope of `agora version`, `agora introspect`, and the `--upgrade-check`
// pseudo-command.
func versionInfo() map[string]any {
	return map[string]any{
		"commit":  commit,
		"date":    date,
		"version": version,
	}
}

// formattedVersion is the single-line human-readable version string used by
// `agora --version` and the Cobra root `Version` field.
func formattedVersion() string {
	return fmt.Sprintf("agora %s (commit %s, built %s)", version, commit, date)
}
