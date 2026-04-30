package cli

// Golden-file tests for the most stable agentic surfaces.
//
// Why golden files? Several JSON envelopes form part of the public agent
// contract — agents and CI scripts will memoize their shape. We do not
// want to learn we broke that contract from a downstream Slack ping; we
// want a single line of test diff to scream at us in PR review.
//
// Golden coverage is intentionally narrow: only structures that should
// almost never change. For each envelope we normalize host-specific
// noise (paths, timestamps, dev version metadata) before comparing.
//
// To regenerate after an intentional change:
//
//	go test ./internal/cli/ -run Golden -update
//
// Then commit the updated testdata/golden/*.json files alongside the
// code change so reviewers see the contract diff in the same PR.

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

var updateGolden = flag.Bool("update", false, "regenerate golden files in internal/cli/testdata/golden")

// normalizeForGolden walks a JSON value and rewrites any field whose
// value is host-specific (paths, timestamps, build metadata) to a stable
// placeholder so the comparison only fails on contract changes.
func normalizeForGolden(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		result := make(map[string]any, len(typed))
		for key, child := range typed {
			switch key {
			case "logFilePath":
				result[key] = "<path>"
			case "version":
				if str, ok := child.(string); ok {
					result[key] = redactVersionString(str)
				} else {
					result[key] = normalizeForGolden(child)
				}
			case "commit", "date":
				if _, ok := child.(string); ok {
					result[key] = "<ignored>"
				} else {
					result[key] = normalizeForGolden(child)
				}
			default:
				result[key] = normalizeForGolden(child)
			}
		}
		return result
	case []any:
		out := make([]any, len(typed))
		for i, child := range typed {
			out[i] = normalizeForGolden(child)
		}
		return out
	default:
		return typed
	}
}

func redactVersionString(s string) string {
	if s == "dev" || s == "" {
		return "<dev>"
	}
	return "<version>"
}

// assertGolden compares the normalized JSON `actual` to the golden file
// stored at testdata/golden/<name>. With -update set, it overwrites the
// golden file instead of failing.
func assertGolden(t *testing.T, name string, actual []byte) {
	t.Helper()
	var parsed any
	if err := json.Unmarshal(actual, &parsed); err != nil {
		t.Fatalf("invalid JSON for golden %s: %v\n%s", name, err, string(actual))
	}
	normalized := normalizeForGolden(parsed)
	canonical, err := json.MarshalIndent(normalized, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	canonical = append(canonical, '\n')

	path := filepath.Join("testdata", "golden", name)
	if *updateGolden {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, canonical, 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("missing golden %s (run `go test ./internal/cli -run Golden -update` to create it): %v", path, err)
	}
	// Windows checkouts may materialize text fixtures with CRLF line endings.
	// The renderer always produces LF, so normalize before comparing the JSON
	// contract itself.
	expected = bytes.ReplaceAll(expected, []byte("\r\n"), []byte("\n"))
	if !bytes.Equal(canonical, expected) {
		t.Fatalf("golden %s mismatch — diff below.\nIf this change is intentional, regenerate with `go test ./internal/cli -run Golden -update` and commit testdata/golden/%s.\n--- expected\n%s\n--- actual\n%s",
			name, name, string(expected), string(canonical))
	}
}

// extractField pulls a sub-tree out of an envelope so each golden file
// can focus on a single contract surface (e.g. pseudoCommands) instead
// of the entire payload.
func extractField(t *testing.T, raw []byte, path ...string) []byte {
	t.Helper()
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, string(raw))
	}
	var current any = doc
	for _, key := range path {
		mapVal, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("expected map at %v, got %T", path, current)
		}
		current = mapVal[key]
	}
	out, err := json.Marshal(current)
	if err != nil {
		t.Fatal(err)
	}
	return out
}

func TestGoldenIntrospectPseudoCommands(t *testing.T) {
	result := runCLI(t, []string{"introspect", "--json"}, cliRunOptions{
		env: map[string]string{"AGORA_HOME": t.TempDir()},
	})
	if result.exitCode != 0 {
		t.Fatalf("introspect failed: exit=%d stderr=%s", result.exitCode, result.stderr)
	}
	pseudo := extractField(t, []byte(result.stdout), "data", "pseudoCommands")
	assertGolden(t, "introspect-pseudo-commands.json", pseudo)
}

func TestGoldenIntrospectGlobalFlags(t *testing.T) {
	result := runCLI(t, []string{"introspect", "--json"}, cliRunOptions{
		env: map[string]string{"AGORA_HOME": t.TempDir()},
	})
	if result.exitCode != 0 {
		t.Fatalf("introspect failed: exit=%d stderr=%s", result.exitCode, result.stderr)
	}
	globalFlags := extractField(t, []byte(result.stdout), "data", "globalFlags")
	assertGolden(t, "introspect-global-flags.json", globalFlags)
}

func TestGoldenIntrospectEnums(t *testing.T) {
	result := runCLI(t, []string{"introspect", "--json"}, cliRunOptions{
		env: map[string]string{"AGORA_HOME": t.TempDir()},
	})
	if result.exitCode != 0 {
		t.Fatalf("introspect failed: exit=%d stderr=%s", result.exitCode, result.stderr)
	}
	enums := extractField(t, []byte(result.stdout), "data", "enums")
	assertGolden(t, "introspect-enums.json", enums)
}

func TestGoldenAuthStatusUnauthenticated(t *testing.T) {
	configHome := t.TempDir()
	result := runCLI(t, []string{"auth", "status", "--json"}, cliRunOptions{
		env: map[string]string{"XDG_CONFIG_HOME": configHome, "AGORA_HOME": t.TempDir()},
	})
	if result.exitCode != 3 {
		t.Fatalf("auth status without session should exit 3, got %d stderr=%s stdout=%s", result.exitCode, result.stderr, result.stdout)
	}
	// Drop the trailing newline — runCLI captures whatever the binary
	// printed verbatim, but the envelope itself is one JSON document.
	stdout := strings.TrimSpace(result.stdout)
	assertGolden(t, "auth-status-unauthenticated.json", []byte(stdout))
}
