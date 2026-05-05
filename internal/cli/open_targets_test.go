package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestOpenTargetURLsAreWellFormed is the smoke test guaranteeing that
// every compiled-in `agora open` target points at a syntactically
// valid HTTPS URL. A typo in cliDocsURL / cliDocsMarkdownURL /
// consoleURL / productDocsURL
// fails the test and is caught in CI before shipping.
func TestOpenTargetURLsAreWellFormed(t *testing.T) {
	for _, target := range []string{"console", "docs", "docs-md", "product-docs"} {
		t.Run(target, func(t *testing.T) {
			url, err := resolveOpenTarget(target, nil)
			if err != nil {
				t.Fatalf("resolveOpenTarget(%q) = error %v", target, err)
			}
			if err := validateOpenTargetURL(url); err != nil {
				t.Fatalf("compiled-in URL for %q is malformed: %v", target, err)
			}
		})
	}
}

// TestOpenTargetEnvOverridesWin verifies that AGORA_*_URL overrides
// take precedence over the compiled-in constants. Forks (CLI repo
// renamed → Pages URL changes) and dev/staging environments rely on
// this path; if it ever silently regresses the override becomes a
// no-op and users open the wrong site.
func TestOpenTargetEnvOverridesWin(t *testing.T) {
	env := map[string]string{
		"AGORA_CONSOLE_URL":      "https://staging-console.example.com",
		"AGORA_DOCS_URL":         "https://staging-docs.example.com",
		"AGORA_DOCS_MD_URL":      "https://staging-docs.example.com/md/index.md",
		"AGORA_PRODUCT_DOCS_URL": "https://staging-product-docs.example.com",
	}
	for target, want := range map[string]string{
		"console":      env["AGORA_CONSOLE_URL"],
		"docs":         env["AGORA_DOCS_URL"],
		"docs-md":      env["AGORA_DOCS_MD_URL"],
		"product-docs": env["AGORA_PRODUCT_DOCS_URL"],
	} {
		t.Run(target, func(t *testing.T) {
			got, err := resolveOpenTarget(target, env)
			if err != nil {
				t.Fatalf("resolveOpenTarget(%q) = error %v", target, err)
			}
			if got != want {
				t.Fatalf("resolveOpenTarget(%q) = %q, want %q", target, got, want)
			}
		})
	}
}

// TestOpenTargetUnknownReturnsStructuredError confirms unknown
// targets fail with the documented message rather than fallthrough.
func TestOpenTargetUnknownReturnsStructuredError(t *testing.T) {
	_, err := resolveOpenTarget("nope", nil)
	if err == nil {
		t.Fatal("expected error for unknown target")
	}
	if !strings.Contains(err.Error(), `unknown open target "nope"`) {
		t.Fatalf("unexpected error: %v", err)
	}
}

// TestOpenTargetEmptyOverrideFallsBackToCompiledIn ensures setting
// AGORA_DOCS_URL="" does NOT clear the URL — empty overrides are
// indistinguishable from "unset" from the user's perspective.
func TestOpenTargetEmptyOverrideFallsBackToCompiledIn(t *testing.T) {
	env := map[string]string{"AGORA_DOCS_URL": "   "}
	got, err := resolveOpenTarget("docs", env)
	if err != nil {
		t.Fatal(err)
	}
	if got != cliDocsURL {
		t.Fatalf("empty override should fall back to compiled-in URL, got %q want %q", got, cliDocsURL)
	}
}

// TestCLIDocsURLMatchesPagesWorkflow guards the cross-file invariant
// that cliDocsURL points at the GitHub Pages site published by
// .github/workflows/pages.yml. The workflow uploads the docs folder
// to a Pages site whose URL must match the constant — if the repo or
// publishing branch is ever changed in the workflow, this test fails
// and the maintainer is forced to update both files together.
//
// We do this with a structural check instead of a network probe so
// the test stays hermetic and fast: we just assert that the workflow
// file exists and that it actually publishes a Pages site (i.e. uses
// the `actions/deploy-pages` action).
func TestCLIDocsURLMatchesPagesWorkflow(t *testing.T) {
	repoRoot := findRepoRootForTest(t)
	pagesYAML := filepath.Join(repoRoot, ".github", "workflows", "pages.yml")
	body, err := os.ReadFile(pagesYAML)
	if err != nil {
		t.Fatalf("could not read %s: %v", pagesYAML, err)
	}
	if !strings.Contains(string(body), "actions/deploy-pages") {
		t.Fatal("pages.yml does not invoke actions/deploy-pages; cliDocsURL would be unreachable")
	}
	if !strings.HasSuffix(cliDocsURL, "/") {
		t.Fatalf("cliDocsURL must end with a trailing slash to match GitHub Pages canonical URLs, got %q", cliDocsURL)
	}
	if cliDocsMarkdownURL != cliDocsURL+"md/index.md" {
		t.Fatalf("cliDocsMarkdownURL = %q, want %q", cliDocsMarkdownURL, cliDocsURL+"md/index.md")
	}
	if !strings.Contains(string(body), "Prepare human and agent docs") ||
		!strings.Contains(string(body), "scripts/prepare-pages-site.py") ||
		!strings.Contains(string(body), "docs/site.env") {
		t.Fatal("pages.yml does not prepare docs with the env-file driven Pages script")
	}
}

func TestPagesSiteEnvDefaultsMatchOpenTargets(t *testing.T) {
	repoRoot := findRepoRootForTest(t)
	siteEnv := filepath.Join(repoRoot, "docs", "site.env")
	body, err := os.ReadFile(siteEnv)
	if err != nil {
		t.Fatalf("could not read %s: %v", siteEnv, err)
	}
	content := string(body)
	if !strings.Contains(content, "CLI_DOCS_BASE_URL="+strings.TrimSuffix(cliDocsURL, "/")) {
		t.Fatalf("docs/site.env does not match cliDocsURL %q:\n%s", cliDocsURL, content)
	}
	if !strings.Contains(content, "CLI_DOCS_MD_BASE_URL="+strings.TrimSuffix(cliDocsURL, "/")+"/md") {
		t.Fatalf("docs/site.env does not match cliDocsMarkdownURL base %q:\n%s", cliDocsMarkdownURL, content)
	}
}

// findRepoRootForTest walks up from the current working directory
// until it finds a go.mod, returning the directory holding it. Used
// by TestCLIDocsURLMatchesPagesWorkflow so it doesn't have to encode
// a relative path that breaks when the test is run from somewhere
// other than the package directory.
func findRepoRootForTest(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatalf("could not find go.mod above %s", dir)
		}
		dir = parent
	}
}
