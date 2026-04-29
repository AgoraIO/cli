package cli

import "testing"

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
