package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestQuickstartScenarioCompletionFiltersByTemplate(t *testing.T) {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("template", "nextjs", "")
	items, _ := completeQuickstartScenarios(cmd, nil, "")
	joined := strings.Join(items, "\n")
	if !strings.Contains(joined, "voice-agent") || !strings.Contains(joined, "video-call") {
		t.Fatalf("nextjs scenarios = %v", items)
	}

	if err := cmd.Flags().Set("template", "go"); err != nil {
		t.Fatal(err)
	}
	items, _ = completeQuickstartScenarios(cmd, nil, "")
	joined = strings.Join(items, "\n")
	if !strings.Contains(joined, "voice-agent") || strings.Contains(joined, "video-call") {
		t.Fatalf("go scenarios = %v", items)
	}
}

func TestProjectPresetCompletionUsesCatalog(t *testing.T) {
	items, _ := completeProjectPresetIDs(nil, nil, "")
	if strings.Join(items, ",") != "video-call,voice-agent" {
		t.Fatalf("project preset completions = %v", items)
	}
}
