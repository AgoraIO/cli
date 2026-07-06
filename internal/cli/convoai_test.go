package cli

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestValidateChannelName(t *testing.T) {
	if err := validateChannelName(""); err == nil {
		t.Fatal("empty channel must be rejected")
	}
	if err := validateChannelName(strings.Repeat("a", 65)); err == nil {
		t.Fatal("channel over 64 bytes must be rejected")
	}
	if err := validateChannelName("bad space"); err == nil {
		t.Fatal("space must be rejected")
	}
	if err := validateChannelName("my-dev_room.1"); err != nil {
		t.Fatalf("valid channel rejected: %v", err)
	}
}

func TestResolveUIDGeneratesNonZero(t *testing.T) {
	if got := resolveUID(0); got == 0 {
		t.Fatal("generated uid must be non-zero")
	}
	if got := resolveUID(42); got != 42 {
		t.Fatalf("explicit uid must pass through, got %d", got)
	}
}

func TestResolveDisjointAgentUID(t *testing.T) {
	got := resolveAgentUID(0)
	if got < 10000000 || got > 99999999 {
		t.Fatalf("generated agent uid out of reserved range: %d", got)
	}
}

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
