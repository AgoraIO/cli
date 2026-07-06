package cli

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestConvoaiPlaygroundIsRegistered(t *testing.T) {
	// Use newTestApp (defined in mcp_test.go) to boot a real App and obtain
	// its fully-wired root cobra command, matching the pattern used by sibling
	// test files in this package.
	a := newTestApp(t)
	root := a.buildRoot()

	cmd, _, err := root.Find([]string{"convoai", "playground"})
	if err != nil {
		t.Fatalf("convoai playground not found: %v", err)
	}
	if cmd.Name() != "playground" {
		t.Fatalf("expected playground, got %q", cmd.Name())
	}
	for _, name := range []string{"channel", "port", "uid", "agent-uid", "ttl", "no-open"} {
		if cmd.Flag(name) == nil {
			t.Fatalf("missing --%s flag", name)
		}
	}
	// --channel must be marked required.
	if ann := cmd.Flag("channel").Annotations[cobra.BashCompOneRequiredFlag]; len(ann) == 0 {
		t.Fatalf("--channel must be required")
	}
}
