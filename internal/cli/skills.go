package cli

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

// skill is the curated, in-binary catalog entry. Field names are stable so
// future dynamic or fetched skills can use the same JSON shape.
//
// Today the catalog is read-only and lives in Go code (no remote fetch,
// no file load). That keeps the surface trivially testable and avoids
// any "where did this skill come from" supply-chain question.
type skill struct {
	ID          string   `json:"id"`
	Title       string   `json:"title"`
	Description string   `json:"description"`
	Category    string   `json:"category"`
	Tags        []string `json:"tags"`
	Steps       []string `json:"steps"`
	NextSteps   []string `json:"nextSteps,omitempty"`
	DocsURL     string   `json:"docsUrl,omitempty"`
}

// skillsCatalog is the canonical curated list. Add new skills here.
// Keep entries small and action-oriented: every skill is a recipe an
// agent can execute end-to-end with the documented steps.
func skillsCatalog() []skill {
	return []skill{
		{
			ID:          "create-nextjs-video-app",
			Title:       "Create a Next.js video app",
			Description: "Scaffold a runnable Next.js video app bound to an Agora project, with credentials wired into .env.local.",
			Category:    "scaffold",
			Tags:        []string{"nextjs", "rtc", "video", "init"},
			Steps: []string{
				"agora login",
				"agora init my-nextjs-demo --template nextjs --json",
				"cd my-nextjs-demo && npm install && npm run dev",
			},
			NextSteps: []string{
				"Open http://localhost:3000 to verify the app boots.",
				"Run agora project doctor --json to confirm RTC is enabled.",
			},
			DocsURL: "https://agoraio.github.io/cli/install.html",
		},
		{
			ID:          "create-python-voice-agent",
			Title:       "Create a Python voice agent (ConvoAI)",
			Description: "Bootstrap a Python ConvoAI voice agent with project metadata and env wiring.",
			Category:    "scaffold",
			Tags:        []string{"python", "convoai", "voice", "init"},
			Steps: []string{
				"agora login",
				"agora init my-voice-agent --template python --feature convoai --json",
				"cd my-voice-agent/server && pip install -r requirements.txt",
			},
			NextSteps: []string{
				"Configure your model provider keys in server/.env (already created with Agora App ID + Certificate).",
				"Run agora project doctor --feature convoai --json before going live.",
			},
		},
		{
			ID:          "create-go-token-service",
			Title:       "Create a Go token service",
			Description: "Stand up a Go server that mints Agora RTC tokens, with project metadata and env wiring.",
			Category:    "scaffold",
			Tags:        []string{"go", "rtc", "token", "backend", "init"},
			Steps: []string{
				"agora login",
				"agora init my-go-token-service --template go --feature rtc --json",
				"cd my-go-token-service/server-go && go run .",
			},
			NextSteps: []string{
				"Curl GET /token to verify the service mints tokens against the bound project.",
			},
		},
		{
			ID:          "rotate-and-export-env",
			Title:       "Rotate project credentials into a running app",
			Description: "Re-export Agora App ID and App Certificate into an existing repo's dotenv files after switching projects.",
			Category:    "ops",
			Tags:        []string{"env", "rotate", "credentials"},
			Steps: []string{
				"agora project use my-other-project",
				"agora project env write apps/web/.env.local --overwrite --json",
				"agora project env --shell  # for ad-hoc shell sourcing",
			},
		},
		{
			ID:          "diagnose-install",
			Title:       "Diagnose a broken Agora CLI install",
			Description: "Run the install doctor and follow its remediation suggestions.",
			Category:    "ops",
			Tags:        []string{"doctor", "diagnose", "ci"},
			Steps: []string{
				"agora doctor --json",
				"agora project doctor --json",
				"agora env-help --json | jq '.data.byCategory.telemetry'",
			},
			NextSteps: []string{
				"If 'auth' fails, run agora login.",
				"If 'network' fails, check proxies / corporate firewall.",
			},
		},
		{
			ID:          "wire-mcp-server",
			Title:       "Expose Agora CLI to an AI agent via MCP",
			Description: "Add Agora CLI as a local Model Context Protocol server so a coding agent can drive it as a tool.",
			Category:    "agent",
			Tags:        []string{"mcp", "cursor", "claude", "windsurf"},
			Steps: []string{
				"agora login  # MCP does not expose OAuth; authenticate on the host first",
				"agora mcp serve  # smoke test that it speaks MCP",
				"In your IDE settings, add a server that runs 'agora mcp serve'.",
			},
			DocsURL: "https://agoraio.github.io/cli/md/agents/README.md",
		},
		{
			ID:          "drop-in-agent-rules",
			Title:       "Drop in agent rules for Cursor / Claude / Windsurf",
			Description: "Write Agora-specific rules into the IDE's known config file with safe append-when-exists semantics.",
			Category:    "agent",
			Tags:        []string{"cursor", "claude", "windsurf", "rules"},
			Steps: []string{
				"agora init my-app --template nextjs --add-agent-rules cursor",
				"# inspect the result",
				"cat .cursor/rules/agora.mdc",
			},
		},
	}
}

// buildSkillsCommand registers `agora skills`. It is intentionally
// read-only in this release: list, show, search. Future releases may
// add `skills run`, `skills install`, and `skills eval` while keeping
// the same JSON shapes documented in docs/automation.md.
func (a *App) buildSkillsCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "skills",
		Short: "Browse curated Agora workflows for humans and AI agents",
		Long: `Skills are short, named, executable recipes that take a developer (human
or AI) from "I want to do X" to a working command sequence.

Today the catalog is curated and shipped in the binary. Future releases
will support fetched skills, evals, and 'agora skills run' to execute
a skill end-to-end. The shape of the JSON output is stable so wrappers
written today keep working when more sources are added.`,
		Example: example(`
  agora skills list
  agora skills list --category scaffold
  agora skills show create-nextjs-video-app
  agora skills search voice
  agora skills list --json
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
			}
			return cmd.Help()
		},
	}
	cmd.AddCommand(a.buildSkillsListCommand())
	cmd.AddCommand(a.buildSkillsShowCommand())
	cmd.AddCommand(a.buildSkillsSearchCommand())
	return cmd
}

func (a *App) buildSkillsListCommand() *cobra.Command {
	var category, tag string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available skills",
		Example: example(`
  agora skills list
  agora skills list --category scaffold
  agora skills list --tag nextjs
  agora skills list --json
`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			items := filterSkills(skillsCatalog(), category, tag)
			return renderResult(cmd, "skills list", map[string]any{
				"action":   "list",
				"items":    items,
				"category": category,
				"tag":      tag,
				"total":    len(items),
			})
		},
	}
	cmd.Flags().StringVar(&category, "category", "", "filter by category (scaffold, ops, agent)")
	cmd.Flags().StringVar(&tag, "tag", "", "filter by tag (e.g. nextjs, rtc, mcp)")
	_ = cmd.RegisterFlagCompletionFunc("category", func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return completeSkillCategories(toComplete), cobra.ShellCompDirectiveNoFileComp
	})
	_ = cmd.RegisterFlagCompletionFunc("tag", func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return completeSkillTags(toComplete), cobra.ShellCompDirectiveNoFileComp
	})
	return cmd
}

func (a *App) buildSkillsShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <id>",
		Short: "Show one skill in detail",
		Example: example(`
  agora skills show create-nextjs-video-app
  agora skills show create-nextjs-video-app --json
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				return errors.New("skill id is required")
			}
			id := strings.TrimSpace(args[0])
			for _, sk := range skillsCatalog() {
				if sk.ID == id {
					return renderResult(cmd, "skills show", map[string]any{
						"action": "show",
						"skill":  sk,
					})
				}
			}
			return &cliError{Message: fmt.Sprintf("no skill with id %q. Run 'agora skills list' to see available IDs.", id), Code: "SKILL_NOT_FOUND"}
		},
		ValidArgsFunction: func(_ *cobra.Command, _ []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completeSkillIDs(toComplete), cobra.ShellCompDirectiveNoFileComp
		},
	}
}

func (a *App) buildSkillsSearchCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "search <query>",
		Short: "Search skills by id, title, description, or tag",
		Example: example(`
  agora skills search voice
  agora skills search nextjs --json
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				return errors.New("search query is required")
			}
			query := strings.ToLower(strings.TrimSpace(args[0]))
			matches := []skill{}
			for _, sk := range skillsCatalog() {
				if skillMatchesQuery(sk, query) {
					matches = append(matches, sk)
				}
			}
			return renderResult(cmd, "skills search", map[string]any{
				"action": "search",
				"query":  args[0],
				"items":  matches,
				"total":  len(matches),
			})
		},
	}
}

func filterSkills(catalog []skill, category, tag string) []skill {
	out := []skill{}
	categoryNorm := strings.ToLower(strings.TrimSpace(category))
	tagNorm := strings.ToLower(strings.TrimSpace(tag))
	for _, sk := range catalog {
		if categoryNorm != "" && strings.ToLower(sk.Category) != categoryNorm {
			continue
		}
		if tagNorm != "" && !skillHasTag(sk, tagNorm) {
			continue
		}
		out = append(out, sk)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func skillHasTag(sk skill, tag string) bool {
	for _, t := range sk.Tags {
		if strings.ToLower(t) == tag {
			return true
		}
	}
	return false
}

func skillMatchesQuery(sk skill, query string) bool {
	if strings.Contains(strings.ToLower(sk.ID), query) {
		return true
	}
	if strings.Contains(strings.ToLower(sk.Title), query) {
		return true
	}
	if strings.Contains(strings.ToLower(sk.Description), query) {
		return true
	}
	if strings.Contains(strings.ToLower(sk.Category), query) {
		return true
	}
	for _, tag := range sk.Tags {
		if strings.Contains(strings.ToLower(tag), query) {
			return true
		}
	}
	return false
}

func completeSkillIDs(toComplete string) []string {
	prefix := strings.ToLower(toComplete)
	out := []string{}
	for _, sk := range skillsCatalog() {
		if strings.HasPrefix(strings.ToLower(sk.ID), prefix) {
			out = append(out, fmt.Sprintf("%s\t%s", sk.ID, sk.Title))
		}
	}
	sort.Strings(out)
	return out
}

func completeSkillCategories(toComplete string) []string {
	prefix := strings.ToLower(toComplete)
	seen := map[string]struct{}{}
	for _, sk := range skillsCatalog() {
		if strings.HasPrefix(strings.ToLower(sk.Category), prefix) {
			seen[sk.Category] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

func completeSkillTags(toComplete string) []string {
	prefix := strings.ToLower(toComplete)
	seen := map[string]struct{}{}
	for _, sk := range skillsCatalog() {
		for _, t := range sk.Tags {
			if strings.HasPrefix(strings.ToLower(t), prefix) {
				seen[t] = struct{}{}
			}
		}
	}
	out := make([]string, 0, len(seen))
	for t := range seen {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}
