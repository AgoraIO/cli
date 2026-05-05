package cli

import (
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

// agoraEnvVar describes a single environment variable the CLI honors.
// The catalog below is the authoritative list; do not add new env vars
// without an entry here, otherwise `agora env-help` and the auto-
// generated docs will silently drift.
type agoraEnvVar struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Default     string `json:"default,omitempty"`
	Category    string `json:"category"`
	Effect      string `json:"effect"`
}

func agoraEnvCatalog() []agoraEnvVar {
	return []agoraEnvVar{
		// Output / runtime UX
		{Name: "AGORA_OUTPUT", Category: "output", Description: "Default output mode when --output / --json is not passed.", Default: "pretty", Effect: "pretty | json"},
		{Name: "AGORA_DEBUG", Category: "output", Description: "When 1, mirror structured log records to stderr (equivalent to --debug). v0.2.0 dropped the legacy AGORA_VERBOSE alias.", Default: "0", Effect: "0 | 1"},
		{Name: "AGORA_LOG_LEVEL", Category: "output", Description: "Minimum log level written to the rotating log file.", Default: "info", Effect: "debug | info | warn | error"},
		{Name: "AGORA_LOG_ENABLED", Category: "output", Description: "When 0, disable file logging entirely.", Default: "1", Effect: "0 | 1"},
		{Name: "AGORA_LOG_MAX_BYTES", Category: "output", Description: "Per-file size before log rotation.", Default: "1000000", Effect: "positive integer"},
		{Name: "AGORA_LOG_MAX_FILES", Category: "output", Description: "Number of rotated log files to keep.", Default: "5", Effect: "positive integer"},
		{Name: "NO_COLOR", Category: "output", Description: "Standard NO_COLOR convention; disables ANSI color in pretty output.", Effect: "any non-empty value"},
		// Interaction
		{Name: "AGORA_NO_INPUT", Category: "interaction", Description: "When set, accept default for confirmation prompts (alias of --yes). Never starts a new interactive OAuth flow in JSON/CI/non-TTY contexts.", Default: "0", Effect: "0 | 1 | true | yes | y"},
		{Name: "AGORA_BROWSER_AUTO_OPEN", Category: "interaction", Description: "When 0, never auto-open a browser for OAuth login (forces --no-browser semantics).", Default: "1", Effect: "0 | 1"},
		{Name: "AGORA_LOGIN_TIMEOUT_MS", Category: "interaction", Description: "How long to wait for the OAuth callback before giving up.", Default: "300000", Effect: "milliseconds"},
		// Storage / paths
		{Name: "AGORA_HOME", Category: "storage", Description: "Override the directory the CLI uses for config, session, context, cache, and logs.", Effect: "absolute path"},
		{Name: "AGORA_DISABLE_CACHE", Category: "storage", Description: "When 1, disable the on-disk project list cache used for shell completion.", Default: "0", Effect: "0 | 1"},
		{Name: "AGORA_PROJECT_CACHE_TTL_SECONDS", Category: "storage", Description: "TTL for the project list cache.", Default: "60", Effect: "seconds"},
		// Endpoints / OAuth
		{Name: "AGORA_API_BASE_URL", Category: "endpoints", Description: "Override the Agora API base URL.", Default: "https://agora-cli.agora.io"},
		{Name: "AGORA_OAUTH_BASE_URL", Category: "endpoints", Description: "Override the OAuth authorization server.", Default: "https://sso2.agora.io"},
		{Name: "AGORA_OAUTH_CLIENT_ID", Category: "endpoints", Description: "Override the OAuth client ID.", Default: "agora_web_cli"},
		{Name: "AGORA_OAUTH_SCOPE", Category: "endpoints", Description: "Override the OAuth scope set requested at login.", Default: "basic_info,console"},
		{Name: "AGORA_CONSOLE_URL", Category: "endpoints", Description: "Override the URL used by `agora open --target console`."},
		{Name: "AGORA_DOCS_URL", Category: "endpoints", Description: "Override the URL used by `agora open --target docs`."},
		{Name: "AGORA_PRODUCT_DOCS_URL", Category: "endpoints", Description: "Override the URL used by `agora open --target product-docs`."},
		// Telemetry
		{Name: "DO_NOT_TRACK", Category: "telemetry", Description: "Standard DO_NOT_TRACK convention; hard opt-out of telemetry and file logging.", Effect: "any non-empty value"},
		{Name: "AGORA_SENTRY_ENABLED", Category: "telemetry", Description: "When 0, disable telemetry transport even if config has telemetryEnabled=true.", Default: "1", Effect: "0 | 1"},
		{Name: "AGORA_SENTRY_ENVIRONMENT", Category: "telemetry", Description: "Override the environment tag used in telemetry events.", Default: "production"},
		{Name: "AGORA_RELEASE", Category: "telemetry", Description: "Override the release tag used in telemetry events.", Default: "<built-in version>"},
		// Agent integrations
		{Name: "AGORA_AGENT", Category: "agent", Description: "Coarse agent label appended to the User-Agent header. Auto-inferred from CURSOR / CLAUDE_AGENT / etc. when unset.", Effect: "free-form string"},
	}
}

// buildEnvHelpCommand exposes the catalog as `agora env-help`. It mirrors
// gh's `gh env-help` and Stripe CLI's `stripe env`. JSON mode emits an
// envelope so wrappers can enumerate every variable the CLI reads.
func (a *App) buildEnvHelpCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "env-help",
		Short: "List every AGORA_* environment variable the CLI honors",
		Long: `Print the canonical list of environment variables that affect the CLI.

This is the authoritative reference: any variable read by the CLI must
appear here, otherwise the env-help drift check (run via "make lint")
fails. Use this command instead of grepping the source for AGORA_*.

Use --json for a machine-readable envelope grouped by category.`,
		Example: example(`
  agora env-help
  agora env-help --json
`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			catalog := agoraEnvCatalog()
			sort.SliceStable(catalog, func(i, j int) bool {
				if catalog[i].Category != catalog[j].Category {
					return catalog[i].Category < catalog[j].Category
				}
				return catalog[i].Name < catalog[j].Name
			})
			grouped := groupEnvByCategory(catalog)
			return renderResult(cmd, "env-help", map[string]any{
				"action":     "env-help",
				"catalog":    catalog,
				"byCategory": grouped,
				"summary":    summarizeEnvCatalog(catalog),
			})
		},
	}
}

func groupEnvByCategory(catalog []agoraEnvVar) map[string][]agoraEnvVar {
	out := map[string][]agoraEnvVar{}
	for _, v := range catalog {
		out[v.Category] = append(out[v.Category], v)
	}
	return out
}

func summarizeEnvCatalog(catalog []agoraEnvVar) string {
	categories := map[string]struct{}{}
	for _, v := range catalog {
		categories[v.Category] = struct{}{}
	}
	names := make([]string, 0, len(categories))
	for c := range categories {
		names = append(names, c)
	}
	sort.Strings(names)
	return "Documented " +
		strconv.Itoa(len(catalog)) + " environment variable(s) across " +
		strconv.Itoa(len(categories)) + " category(ies): " + strings.Join(names, ", ")
}
