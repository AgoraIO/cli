package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestWriteAgentRulesCreatesFileWhenMissing(t *testing.T) {
	root := t.TempDir()
	results, err := writeAgentRules(root, []string{"claude"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Status != "created" {
		t.Fatalf("expected one created result, got %+v", results)
	}
	body, err := os.ReadFile(filepath.Join(root, "CLAUDE.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), agentRuleStartMarker) || !strings.Contains(string(body), agentRuleEndMarker) {
		t.Fatalf("expected managed markers in created file, got:\n%s", body)
	}
	if !strings.Contains(string(body), "Agora CLI Agent Rules") {
		t.Fatalf("expected rule body in created file, got:\n%s", body)
	}
}

func TestWriteAgentRulesAppendsToExistingFileWithoutClobber(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "CLAUDE.md")
	prior := "# Project Notes\n\nThese are critical instructions from the user.\n"
	if err := os.WriteFile(dest, []byte(prior), 0o644); err != nil {
		t.Fatal(err)
	}
	results, err := writeAgentRules(root, []string{"claude"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Status != "appended" {
		t.Fatalf("expected one appended result, got %+v", results)
	}
	body, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(body), prior) {
		t.Fatalf("user content was modified or removed, got:\n%s", body)
	}
	if !strings.Contains(string(body), agentRuleStartMarker) {
		t.Fatalf("expected managed marker after append, got:\n%s", body)
	}
}

func TestWriteAgentRulesUpdatesExistingManagedBlockInPlace(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "CLAUDE.md")
	header := "# Project Notes\n\nThese are critical instructions from the user.\n"
	footer := "\n\n## After-block notes\n\nRemember to run tests.\n"
	staleBlock := agentRuleStartMarker + "\nOLD AGORA BODY\n" + agentRuleEndMarker + "\n"
	prior := header + staleBlock + footer
	if err := os.WriteFile(dest, []byte(prior), 0o644); err != nil {
		t.Fatal(err)
	}
	results, err := writeAgentRules(root, []string{"claude"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Status != "updated" {
		t.Fatalf("expected updated result, got %+v", results)
	}
	body, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), header) {
		t.Fatalf("header content missing, got:\n%s", body)
	}
	if !strings.Contains(string(body), footer) {
		t.Fatalf("footer content missing, got:\n%s", body)
	}
	if strings.Contains(string(body), "OLD AGORA BODY") {
		t.Fatalf("stale block was not replaced, got:\n%s", body)
	}
	if !strings.Contains(string(body), "Agora CLI Agent Rules") {
		t.Fatalf("expected fresh rule body, got:\n%s", body)
	}
	if strings.Count(string(body), agentRuleStartMarker) != 1 {
		t.Fatalf("expected exactly one start marker, got:\n%s", body)
	}
}

func TestWriteAgentRulesCursorPreservesFrontmatter(t *testing.T) {
	root := t.TempDir()
	results, err := writeAgentRules(root, []string{"cursor"})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Status != "created" {
		t.Fatalf("expected one created result, got %+v", results)
	}
	body, err := os.ReadFile(filepath.Join(root, ".cursor", "rules", "agora.mdc"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(body), "---\n") {
		t.Fatalf("Cursor .mdc must start with YAML frontmatter, got:\n%s", body)
	}
	frontMatterEnd := strings.Index(string(body), "\n---\n")
	if frontMatterEnd <= 0 {
		t.Fatalf("Cursor .mdc must close YAML frontmatter, got:\n%s", body)
	}
	startMarkerPos := strings.Index(string(body), agentRuleStartMarker)
	if startMarkerPos <= frontMatterEnd {
		t.Fatalf("start marker must come after YAML frontmatter, body:\n%s", body)
	}
}

func TestWriteAgentRulesUnknownTargetReturnsError(t *testing.T) {
	root := t.TempDir()
	_, err := writeAgentRules(root, []string{"copilot"})
	if err == nil {
		t.Fatal("expected error for unknown target")
	}
	if !strings.Contains(err.Error(), "unknown agent rules target") {
		t.Fatalf("unexpected error: %v", err)
	}
}
