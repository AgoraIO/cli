package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// agentRuleSentinel is a unique marker the CLI writes alongside the
// rule body so we can recognize an Agora-managed block on subsequent
// runs and update it in place without duplicating content. The marker
// is wrapped in HTML comments so it stays invisible in rendered
// Markdown views (Cursor, GitHub, etc.).
const (
	agentRuleStartMarker = "<!-- agora-cli:agent-rules:start -->"
	agentRuleEndMarker   = "<!-- agora-cli:agent-rules:end -->"
)

// agentRuleBody is the shared rule body written into every supported
// agent's configuration file. The body lives between sentinel markers
// inside the destination file so subsequent `agora init --add-agent-rules`
// runs can update only the Agora-managed block.
const agentRuleBody = "# Agora CLI Agent Rules\n\n" +
	"- Use `agora --help --all --json` or `agora introspect --json` for command discovery.\n" +
	"- Prefer `--json` for commands whose output you need to parse.\n" +
	"- Set `AGORA_HOME` to an isolated temporary directory in CI or multi-agent runs.\n" +
	"- Use explicit `--project <id-or-name>` when working outside a bound quickstart.\n" +
	"- Run `agora project doctor --json` before assuming a project is ready.\n" +
	"- Do not parse pretty output; it is for humans.\n" +
	"- Do not print App Certificates, session tokens, or `.env` secret values.\n"

// agentRuleWriteResult records what writeAgentRules did for one target
// so callers can surface accurate, non-destructive feedback to the user.
type agentRuleWriteResult struct {
	Target string `json:"target"`
	Path   string `json:"path"`
	// Status is one of: "created", "appended", "updated".
	//   created  — file did not exist and we created it
	//   appended — file existed and did not contain an Agora-managed
	//              block; we appended a new block, preserving every
	//              prior line of the user's content
	//   updated  — file existed and already contained an Agora-managed
	//              block from a previous run; we replaced only the
	//              block between the markers
	Status string `json:"status"`
}

// writeAgentRules writes (or refreshes) the Agora rules block in the
// configured destination for each requested agent target. It NEVER
// destroys pre-existing user content: existing files are appended to,
// and previously-managed blocks are updated in place between sentinel
// markers. Unknown targets return an error before any file is touched.
func writeAgentRules(root string, targets []string) ([]agentRuleWriteResult, error) {
	results := []agentRuleWriteResult{}
	for _, target := range targets {
		target = strings.ToLower(strings.TrimSpace(target))
		if target == "" {
			continue
		}
		path, body, err := agentRuleTargetSpec(root, target)
		if err != nil {
			return results, err
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return results, err
		}
		status, err := writeOrAppendAgentRuleBlock(path, body, target)
		if err != nil {
			return results, err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		results = append(results, agentRuleWriteResult{Target: target, Path: rel, Status: status})
	}
	return results, nil
}

// agentRuleTargetSpec returns the destination path and the full block
// body (including any front-matter) for a given target.
func agentRuleTargetSpec(root, target string) (string, string, error) {
	switch target {
	case "cursor":
		path := filepath.Join(root, ".cursor", "rules", "agora.mdc")
		body := "---\n" +
			"description: Use Agora CLI safely in this project\n" +
			"globs: \"**/*\"\n" +
			"alwaysApply: true\n" +
			"---\n\n" +
			agentRuleBody
		return path, body, nil
	case "claude":
		return filepath.Join(root, "CLAUDE.md"), agentRuleBody, nil
	case "windsurf":
		return filepath.Join(root, ".windsurf", "rules", "agora.md"), agentRuleBody, nil
	default:
		return "", "", fmt.Errorf("unknown agent rules target %q. Use cursor, claude, or windsurf.", target)
	}
}

// writeOrAppendAgentRuleBlock writes the Agora-managed rule block to
// path, taking three possible paths depending on the file's prior state:
//
//  1. The file does not exist                 → create with the marked block.
//  2. The file exists but has no markers      → append a separator and the
//     marked block. We preserve every line of the user's content.
//  3. The file exists and has prior markers   → replace ONLY the content
//     between the markers. Everything before the start marker and after
//     the end marker (including any user notes the agent added in the
//     same file) survives unchanged.
//
// In every case the destination ends with the canonical Agora-managed
// block surrounded by `agentRuleStartMarker` / `agentRuleEndMarker`,
// which is what the next `agora init --add-agent-rules` run looks for.
func writeOrAppendAgentRuleBlock(path, body, target string) (string, error) {
	managed := buildManagedAgentRuleBlock(body, target)
	existing, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		// Cursor's `.mdc` files require front-matter at the very top
		// of the file, so we write the managed block as the entire
		// file when the file did not exist.
		if err := os.WriteFile(path, []byte(managed), 0o644); err != nil {
			return "", err
		}
		return "created", nil
	}
	if err != nil {
		return "", err
	}
	if start, end, ok := locateManagedAgentRuleBlock(existing); ok {
		next := append([]byte{}, existing[:start]...)
		next = append(next, []byte(managed)...)
		next = append(next, existing[end:]...)
		if err := os.WriteFile(path, next, 0o644); err != nil {
			return "", err
		}
		return "updated", nil
	}
	separator := "\n"
	if len(existing) > 0 && existing[len(existing)-1] != '\n' {
		separator = "\n\n"
	} else if len(existing) >= 2 && string(existing[len(existing)-2:]) != "\n\n" {
		separator = "\n"
	} else {
		separator = ""
	}
	next := append([]byte{}, existing...)
	next = append(next, []byte(separator)...)
	next = append(next, []byte(managed)...)
	if err := os.WriteFile(path, next, 0o644); err != nil {
		return "", err
	}
	return "appended", nil
}

// buildManagedAgentRuleBlock wraps the raw rule body with stable
// sentinel markers and a generation header. The header is
// human-readable so users browsing the file understand who owns this
// block and how to refresh it. Cursor `.mdc` front-matter must stay at
// the very top of the file, so the start marker comes after the front
// matter when present.
func buildManagedAgentRuleBlock(body, target string) string {
	const note = "<!-- This block is managed by `agora init --add-agent-rules " +
		"<target>`. Edits inside the block will be overwritten on the " +
		"next run; lines outside the markers are preserved. -->\n"
	if strings.HasPrefix(body, "---\n") {
		// Split off the leading YAML front-matter so the start marker
		// stays inside the body of the file.
		end := strings.Index(body[4:], "\n---")
		if end >= 0 {
			split := 4 + end + len("\n---\n")
			if split <= len(body) {
				front := body[:split]
				rest := body[split:]
				return front + agentRuleStartMarker + "\n" + note + rest + agentRuleEndMarker + "\n"
			}
		}
	}
	return agentRuleStartMarker + "\n" + note + body + agentRuleEndMarker + "\n"
}

// locateManagedAgentRuleBlock returns the byte offsets of the prior
// Agora-managed block, including its surrounding markers, so the caller
// can splice a refreshed block in place.
func locateManagedAgentRuleBlock(content []byte) (int, int, bool) {
	startIdx := strings.Index(string(content), agentRuleStartMarker)
	if startIdx < 0 {
		return 0, 0, false
	}
	endIdx := strings.Index(string(content[startIdx:]), agentRuleEndMarker)
	if endIdx < 0 {
		return 0, 0, false
	}
	endIdx += startIdx + len(agentRuleEndMarker)
	if endIdx < len(content) && content[endIdx] == '\n' {
		endIdx++
	}
	return startIdx, endIdx, true
}
