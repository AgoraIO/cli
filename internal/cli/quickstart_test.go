package cli

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGitQuickstartCloneArgs(t *testing.T) {
	args := gitQuickstartCloneArgs("https://github.com/AgoraIO/example", "/tmp/example", "")
	want := []string{"-c", "credential.helper=", "clone", "--depth", "1", "https://github.com/AgoraIO/example", "/tmp/example"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("unexpected clone args:\n got: %#v\nwant: %#v", args, want)
	}

	args = gitQuickstartCloneArgs("https://github.com/AgoraIO/example", "/tmp/example", " release/v1 ")
	want = []string{"-c", "credential.helper=", "clone", "--depth", "1", "--branch", "release/v1", "https://github.com/AgoraIO/example", "/tmp/example"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("unexpected clone args with ref:\n got: %#v\nwant: %#v", args, want)
	}
}

func TestCloneQuickstartRepoLocal(t *testing.T) {
	repo := createLocalGitRepo(t, map[string]string{
		"README.md": "# Quickstart\n",
	})
	target := filepath.Join(t.TempDir(), "quickstart")

	if err := cloneQuickstartRepo(repo, target, ""); err != nil {
		t.Fatalf("cloneQuickstartRepo failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, ".git")); err != nil {
		t.Fatalf("expected cloned git repo: %v", err)
	}
}
