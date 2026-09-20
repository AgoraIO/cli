package cli

import (
	"os"
	"path/filepath"
	"testing"
)

func TestWorkspaceDoctorSelectionScope(t *testing.T) {
	for _, tc := range []struct {
		name, recipe, template, scenario, manifest, projectID, blockingCode string
		wantWarning                                                         bool
	}{
		{name: "unknown directory", wantWarning: true},
		{name: "recipe ignores quickstart metadata", recipe: "custom", manifest: "invalid", wantWarning: true},
		{name: "unknown directory with wrong project", projectID: "other", blockingCode: "LOCAL_PROJECT_BINDING_MISMATCH", wantWarning: true},
		{name: "recipe with wrong project", recipe: "custom", projectID: "other", blockingCode: "LOCAL_PROJECT_BINDING_MISMATCH", wantWarning: true},
		{name: "declared unknown template", template: "unknown", blockingCode: "QUICKSTART_TEMPLATE_UNKNOWN"},
		{name: "RTC missing manifest", template: "nextjs", scenario: "video-call", blockingCode: "QUICKSTART_MANIFEST_INVALID"},
		{name: "invalid manifest", manifest: "invalid", blockingCode: "QUICKSTART_MANIFEST_INVALID"},
		{name: "conflicting manifest", template: "nextjs", scenario: "video-call", manifest: `{"schemaVersion":1,"template":"nextjs","scenario":"voice-agent"}`, blockingCode: "QUICKSTART_SELECTION_MISMATCH"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			id := tc.projectID
			if id == "" {
				id = "prj_1"
			}
			if err := writeLocalProjectBinding(root, localProjectBinding{ProjectID: id, Recipe: tc.recipe, Template: tc.template, Scenario: tc.scenario}); err != nil {
				t.Fatal(err)
			}
			if tc.manifest != "" {
				if err := os.WriteFile(filepath.Join(root, quickstartManifestFileName), []byte(tc.manifest), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			t.Chdir(root)
			_, _, blocking, warnings := buildWorkspaceDoctorDetails(projectTarget{project: projectDetail{ProjectID: "prj_1"}})
			if tc.blockingCode == "" {
				if len(blocking) != 0 {
					t.Fatalf("unexpected blocking issues: %+v", blocking)
				}
			} else if len(blocking) != 1 || blocking[0].Code != tc.blockingCode {
				t.Fatalf("blocking=%+v want=%s", blocking, tc.blockingCode)
			}
			if (len(warnings) > 0) != tc.wantWarning {
				t.Fatalf("unexpected warnings: %+v", warnings)
			}
			result := projectDoctorResult{Feature: "rtc", BlockingIssues: blocking, Warnings: warnings}
			finalizeDoctorOutcome(&result)
			if tc.blockingCode == "" && (!result.Healthy || result.Status != "warning") {
				t.Fatalf("unexpected outcome: %+v", result)
			}
			if tc.blockingCode != "" && (result.Healthy || result.Status != "not_ready") {
				t.Fatalf("unexpected outcome: %+v", result)
			}
		})
	}
}
