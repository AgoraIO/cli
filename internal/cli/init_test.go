package cli

import (
	"strings"
	"testing"
)

func TestSelectInitProjectFromList(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		project, ok := selectInitProjectFromList(nil)
		if ok {
			t.Fatalf("expected no project, got %+v", project)
		}
	})

	t.Run("default project wins", func(t *testing.T) {
		project, ok := selectInitProjectFromList([]projectSummary{
			{ProjectID: "prj_older", Name: "Older", CreatedAt: "2026-04-01T10:00:00Z", UpdatedAt: "2026-04-01T10:00:00Z"},
			{ProjectID: "prj_default", Name: "Default Project", CreatedAt: "2026-03-01T10:00:00Z", UpdatedAt: "2026-03-01T10:00:00Z"},
			{ProjectID: "prj_newer", Name: "Newest", CreatedAt: "2026-04-10T10:00:00Z", UpdatedAt: "2026-04-10T10:00:00Z"},
		})
		if !ok || project.ProjectID != "prj_default" {
			t.Fatalf("expected default project, got %+v", project)
		}
	})

	t.Run("latest createdAt wins when no default project exists", func(t *testing.T) {
		project, ok := selectInitProjectFromList([]projectSummary{
			{ProjectID: "prj_old", Name: "Older", CreatedAt: "2026-04-01T10:00:00Z", UpdatedAt: "2026-04-01T10:00:00Z"},
			{ProjectID: "prj_new", Name: "Newest", CreatedAt: "2026-04-12T10:00:00Z", UpdatedAt: "2026-04-12T10:00:00Z"},
			{ProjectID: "prj_mid", Name: "Middle", CreatedAt: "2026-04-08T10:00:00Z", UpdatedAt: "2026-04-08T10:00:00Z"},
		})
		if !ok || project.ProjectID != "prj_new" {
			t.Fatalf("expected newest project, got %+v", project)
		}
	})

	t.Run("valid timestamps beat malformed timestamps", func(t *testing.T) {
		project, ok := selectInitProjectFromList([]projectSummary{
			{ProjectID: "prj_bad", Name: "Broken", CreatedAt: "not-a-time", UpdatedAt: "also-not-a-time"},
			{ProjectID: "prj_good", Name: "Good", CreatedAt: "2026-04-12T10:00:00.123Z", UpdatedAt: "2026-04-12T10:00:00.123Z"},
		})
		if !ok || project.ProjectID != "prj_good" {
			t.Fatalf("expected valid timestamp project, got %+v", project)
		}
	})

	t.Run("falls back to first item when all createdAt values are malformed", func(t *testing.T) {
		project, ok := selectInitProjectFromList([]projectSummary{
			{ProjectID: "prj_first", Name: "First", CreatedAt: "bad", UpdatedAt: "bad"},
			{ProjectID: "prj_second", Name: "Second", CreatedAt: "also-bad", UpdatedAt: "still-bad"},
		})
		if !ok || project.ProjectID != "prj_first" {
			t.Fatalf("expected first project fallback, got %+v", project)
		}
	})
}

func TestChooseInitProject(t *testing.T) {
	items := []projectSummary{
		{ProjectID: "prj_old", Name: "Older", CreatedAt: "2026-04-01T10:00:00Z", UpdatedAt: "2026-04-01T10:00:00Z"},
		{ProjectID: "prj_new", Name: "Newest", CreatedAt: "2026-04-12T10:00:00Z", UpdatedAt: "2026-04-12T10:00:00Z"},
		{ProjectID: "prj_mid", Name: "Middle", CreatedAt: "2026-04-08T10:00:00Z", UpdatedAt: "2026-04-08T10:00:00Z"},
	}

	t.Run("enter selects most recent displayed last", func(t *testing.T) {
		var out strings.Builder
		project, action, err := chooseInitProject(strings.NewReader("\n"), &out, items)
		if err != nil {
			t.Fatal(err)
		}
		if action != "reuse" || project.ProjectID != "prj_new" {
			t.Fatalf("expected newest project reuse, got action=%s project=%+v", action, project)
		}
		output := out.String()
		createIndex := strings.Index(output, "Create a new project")
		olderIndex := strings.Index(output, "Older")
		newestIndex := strings.Index(output, "Newest")
		if createIndex < 0 || olderIndex < 0 || newestIndex < 0 || !(createIndex < olderIndex && olderIndex < newestIndex) {
			t.Fatalf("expected create option above projects displayed oldest-to-newest, got:\n%s", output)
		}
	})

	t.Run("new creates a fresh project", func(t *testing.T) {
		var out strings.Builder
		_, action, err := chooseInitProject(strings.NewReader("new\n"), &out, items)
		if err != nil {
			t.Fatal(err)
		}
		if action != "new" {
			t.Fatalf("expected new action, got %s", action)
		}
	})

	t.Run("number selects project", func(t *testing.T) {
		var out strings.Builder
		project, action, err := chooseInitProject(strings.NewReader("3\n"), &out, items)
		if err != nil {
			t.Fatal(err)
		}
		if action != "reuse" || project.ProjectID != "prj_mid" {
			t.Fatalf("expected middle project, got action=%s project=%+v", action, project)
		}
	})
}
