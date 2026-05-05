package cli

import (
	"fmt"
	"net/url"
	"strings"
)

// Canonical URLs for `agora open --target <name>`.
//
// IMPORTANT — keep these in sync with infrastructure outside this
// package:
//
//   - cliDocsURL must match the GitHub Pages site published by
//     .github/workflows/pages.yml. If the publishing repo, branch, or
//     custom-domain CNAME ever changes, update both here AND in
//     pages.yml in the same commit.
//   - cliDocsMarkdownURL must point at the raw Markdown copy of the
//     same docs tree. Pages publishes these files under /md/ after
//     rendering the human HTML site, giving agents stable source URLs
//     such as /md/commands.md and /md/automation.md.
//   - consoleURL is the public Agora Console front door.
//   - productDocsURL is the public product documentation site.
//
// A smoke test in open_targets_test.go validates that each URL
// parses, uses HTTPS, and is non-empty so a typo here surfaces in CI.
//
// Forks and dev/staging environments can override any of these at
// runtime via environment variables (see openTargetEnv) without
// editing or rebuilding the CLI.
const (
	cliDocsURL         = "https://agoraio.github.io/cli/"
	cliDocsMarkdownURL = "https://agoraio.github.io/cli/md/index.md"
	consoleURL         = "https://console.agora.io"
	productDocsURL     = "https://docs.agora.io"
)

// openTargetEnv maps each open-target name to the environment variable
// that overrides its compiled-in URL. This is the supported escape
// hatch for forks (CLI repo renamed → Pages URL changes), local dev
// against a staging Console, and CI environments that prefer to point
// at preview docs builds.
var openTargetEnv = map[string]string{
	"console":      "AGORA_CONSOLE_URL",
	"docs":         "AGORA_DOCS_URL",
	"docs-md":      "AGORA_DOCS_MD_URL",
	"product-docs": "AGORA_PRODUCT_DOCS_URL",
}

// resolveOpenTarget returns the URL the `agora open` command should
// open or print for the given target name. Resolution order:
//
//  1. Per-target environment override from openTargetEnv (when set
//     and non-empty), e.g. AGORA_DOCS_URL=https://staging-docs.example/
//  2. Compiled-in canonical URL.
//
// Returns a structured error for unknown targets so the message stays
// consistent with the rest of the CLI's input-validation errors.
func resolveOpenTarget(target string, env map[string]string) (string, error) {
	envKey, known := openTargetEnv[target]
	if !known {
		return "", fmt.Errorf("unknown open target %q. Use console, docs, docs-md, or product-docs.", target)
	}
	if env != nil {
		if override := strings.TrimSpace(env[envKey]); override != "" {
			return override, nil
		}
	}
	switch target {
	case "console":
		return consoleURL, nil
	case "docs":
		return cliDocsURL, nil
	case "docs-md":
		return cliDocsMarkdownURL, nil
	case "product-docs":
		return productDocsURL, nil
	}
	// Unreachable: openTargetEnv keys and switch cases are kept in sync.
	return "", fmt.Errorf("unknown open target %q. Use console, docs, docs-md, or product-docs.", target)
}

// validateOpenTargetURL is the predicate used by the smoke test (and
// by any future runtime URL-loaded path) to assert that a URL string
// is well-formed enough to hand to a browser: parses, uses HTTPS,
// has a host, and contains no whitespace. Exposed so tests can
// validate both the compiled-in constants and override env vars.
func validateOpenTargetURL(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return fmt.Errorf("URL must not be empty")
	}
	if strings.ContainsAny(raw, " \t\r\n") {
		return fmt.Errorf("URL must not contain whitespace: %q", raw)
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("URL %q failed to parse: %w", raw, err)
	}
	if parsed.Scheme != "https" {
		return fmt.Errorf("URL %q must use https", raw)
	}
	if parsed.Host == "" {
		return fmt.Errorf("URL %q must include a host", raw)
	}
	return nil
}
