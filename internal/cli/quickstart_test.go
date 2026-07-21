package cli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestGitQuickstartCloneArgs(t *testing.T) {
	args := gitQuickstartCloneArgs("https://github.com/AgoraIO/example", "/tmp/example", "")
	want := []string{"-c", "credential.helper=", "clone", "--depth", "1", "--", "https://github.com/AgoraIO/example", "/tmp/example"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("unexpected clone args:\n got: %#v\nwant: %#v", args, want)
	}

	args = gitQuickstartCloneArgs("https://github.com/AgoraIO/example", "/tmp/example", " release/v1 ")
	want = []string{"-c", "credential.helper=", "clone", "--depth", "1", "--branch", "release/v1", "--", "https://github.com/AgoraIO/example", "/tmp/example"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("unexpected clone args with ref:\n got: %#v\nwant: %#v", args, want)
	}

	args = gitQuickstartCloneArgs("-https://evil.example/repo", "/tmp/example", "")
	want = []string{"-c", "credential.helper=", "clone", "--depth", "1", "--", "-https://evil.example/repo", "/tmp/example"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("unexpected clone args with dash-prefixed url:\n got: %#v\nwant: %#v", args, want)
	}
}

func TestStripClonedGitMetadata(t *testing.T) {
	repo := createLocalGitRepo(t, map[string]string{
		"README.md": "# Quickstart\n",
	})
	target := filepath.Join(t.TempDir(), "quickstart")
	if err := cloneQuickstartRepo(repo, target, ""); err != nil {
		t.Fatalf("cloneQuickstartRepo failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, ".git")); err != nil {
		t.Fatalf("expected cloned git repo before strip: %v", err)
	}
	if err := stripClonedGitMetadata(target); err != nil {
		t.Fatalf("stripClonedGitMetadata failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, ".git")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected .git removed after strip, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "README.md")); err != nil {
		t.Fatalf("expected README to remain after strip: %v", err)
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

func TestCloneQuickstartRepoRejectsBadRef(t *testing.T) {
	target := filepath.Join(t.TempDir(), "quickstart")
	err := cloneQuickstartRepo("https://example.invalid/repo.git", target, "-fexploit")
	if err == nil {
		t.Fatal("expected error for dash-prefixed ref, got nil")
	}
	var cliErr *cliError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected *cliError, got %T: %v", err, err)
	}
	if cliErr.Code != "QUICKSTART_REF_INVALID" {
		t.Fatalf("expected code QUICKSTART_REF_INVALID, got %q", cliErr.Code)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("expected target not created on validation failure, stat err = %v", statErr)
	}
}

func TestValidateGitRef(t *testing.T) {
	cases := []struct {
		name string
		ref  string
		ok   bool
	}{
		{"empty allowed", "", true},
		{"whitespace allowed (treated as empty)", "  ", true},
		{"normal branch", "main", true},
		{"slash branch", "release/v1", true},
		{"tag with dots", "v1.2.3", true},
		{"leading dash rejected", "-fexploit", false},
		{"embedded space rejected", "release v1", false},
		{"embedded tab rejected", "release\tv1", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateGitRef(tc.ref)
			if tc.ok && err != nil {
				t.Fatalf("expected ref %q to be valid, got error: %v", tc.ref, err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("expected ref %q to be rejected", tc.ref)
			}
		})
	}
}

func TestValidateRepoOverrideURL(t *testing.T) {
	localAbsPath := filepath.Join(os.TempDir(), "example-repo")
	cases := []struct {
		name string
		url  string
		ok   bool
	}{
		{"https", "https://github.com/AgoraIO/example", true},
		{"http", "http://example.com/repo.git", true},
		{"ssh", "ssh://git@github.com/AgoraIO/example.git", true},
		{"git", "git://github.com/AgoraIO/example.git", true},
		{"file", "file:///srv/mirror/example.git", true},
		{"ssh shorthand", "git@github.com:AgoraIO/example.git", true},
		{"absolute local path", localAbsPath, true},
		{"empty rejected", "", false},
		{"dash prefix rejected", "-https://evil/repo", false},
		{"unrecognized form rejected", "example", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRepoOverrideURL(tc.url)
			if tc.ok && err != nil {
				t.Fatalf("expected %q to be valid, got error: %v", tc.url, err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("expected %q to be rejected", tc.url)
			}
		})
	}
}

func TestQuickstartRepoOverrideKey(t *testing.T) {
	if got := quickstartRepoOverrideKey("nextjs"); got != "AGORA_QUICKSTART_NEXTJS_REPO_URL" {
		t.Fatalf("unexpected key for nextjs: %q", got)
	}
	if got := quickstartRepoOverrideKey("my-template"); got != "AGORA_QUICKSTART_MY_TEMPLATE_REPO_URL" {
		t.Fatalf("unexpected key for my-template: %q", got)
	}
}

func TestQuickstartRepoURLOverride(t *testing.T) {
	tmpl := quickstartTemplate{
		ID:        "nextjs",
		RepoURL:   "https://default.example/repo",
		RepoURLCN: "https://cn.example/repo",
	}
	app := &App{env: map[string]string{"XDG_CONFIG_HOME": t.TempDir()}}

	url, override, err := app.quickstartRepoURL(tmpl)
	if err != nil || override != "" || url != tmpl.RepoURL {
		t.Fatalf("default path: url=%q override=%q err=%v", url, override, err)
	}

	app.env = map[string]string{
		"AGORA_QUICKSTART_NEXTJS_REPO_URL": "https://fork.example/repo",
		"XDG_CONFIG_HOME":                  t.TempDir(),
	}
	url, override, err = app.quickstartRepoURL(tmpl)
	if err != nil || override != "AGORA_QUICKSTART_NEXTJS_REPO_URL" || url != "https://fork.example/repo" {
		t.Fatalf("override path: url=%q override=%q err=%v", url, override, err)
	}

	app.env = map[string]string{
		"AGORA_QUICKSTART_NEXTJS_REPO_URL": "-fexploit",
		"XDG_CONFIG_HOME":                  t.TempDir(),
	}
	if _, _, err := app.quickstartRepoURL(tmpl); err == nil {
		t.Fatal("expected error for invalid override")
	} else {
		var cliErr *cliError
		if !errors.As(err, &cliErr) || cliErr.Code != "QUICKSTART_REPO_OVERRIDE_INVALID" {
			t.Fatalf("expected QUICKSTART_REPO_OVERRIDE_INVALID, got %v", err)
		}
	}
}

func TestQuickstartRepoURLForRegion(t *testing.T) {
	tmpl := quickstartTemplate{
		RepoURL:   "https://global.example/repo",
		RepoURLCN: "https://cn.example/repo",
	}
	if got := quickstartRepoURLForRegion(tmpl, regionGlobal); got != tmpl.RepoURL {
		t.Fatalf("global repo url = %q, want %q", got, tmpl.RepoURL)
	}
	if got := quickstartRepoURLForRegion(tmpl, regionCN); got != tmpl.RepoURLCN {
		t.Fatalf("cn repo url = %q, want %q", got, tmpl.RepoURLCN)
	}
}

func TestQuickstartDocsURLForRegion(t *testing.T) {
	tmpl := quickstartTemplate{
		DocsURL:   "https://global.example/docs",
		DocsURLCN: "https://cn.example/docs",
	}
	if got := quickstartDocsURL(tmpl, regionGlobal); got != tmpl.DocsURL {
		t.Fatalf("global docs url = %q, want %q", got, tmpl.DocsURL)
	}
	if got := quickstartDocsURL(tmpl, regionCN); got != tmpl.DocsURLCN {
		t.Fatalf("cn docs url = %q, want %q", got, tmpl.DocsURLCN)
	}
}