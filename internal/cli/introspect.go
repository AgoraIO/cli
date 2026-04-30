package cli

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// commandHelpInfo is the per-command record returned by buildCommandTree
// (used by both `agora --help --all` and `agora introspect --json`).
//
//	Path    — full command path with `agora` prefix, e.g. "agora project create"
//	Command — stable command label (Path minus the `agora` prefix), suitable
//	          for matching against the `command` field of a JSON envelope
//	Short   — one-line description from the cobra command
//	Flags   — local flags (inherited flags omitted to keep the tree compact)
type commandHelpInfo struct {
	Path    string         `json:"path"`
	Command string         `json:"command"`
	Short   string         `json:"short"`
	Flags   []flagHelpInfo `json:"flags"`
}

// flagHelpInfo describes a single command-local flag in machine-readable
// form. nonTrivialDefault filters out the noisy "false" / "0" / "" defaults
// to keep agent payloads short.
type flagHelpInfo struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Default string `json:"default,omitempty"`
	Usage   string `json:"usage"`
}

// pseudoCommandInfo describes synthetic commands that are not enumerable in
// the Cobra tree (typically root flags that emit their own envelope, e.g.
// --upgrade-check). Listing them here lets agents discover every `command`
// label they may see in envelopes.
type pseudoCommandInfo struct {
	Command string `json:"command"`
	Trigger string `json:"trigger"`
	Short   string `json:"short"`
}

func buildPseudoCommands() []pseudoCommandInfo {
	return []pseudoCommandInfo{
		{
			Command: "upgrade check",
			Trigger: "agora --upgrade-check",
			Short:   "Print package-manager-specific upgrade guidance and exit (root flag, not a subcommand).",
		},
	}
}

// buildIntrospectionData is the single source of truth for the data payload
// returned by both `agora introspect` and `agora --help --all --json`.
// Adding a key here makes it appear on both surfaces.
//
// The shape is part of the public agent contract — see
// docs/automation.md → "JSON Envelope" → "introspect data shape".
func buildIntrospectionData(root *cobra.Command) map[string]any {
	globalFlags := make([]flagHelpInfo, 0)
	root.PersistentFlags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Name == "help" || f.Name == "all" {
			return
		}
		globalFlags = append(globalFlags, flagHelpInfo{
			Name:    f.Name,
			Type:    f.Value.Type(),
			Default: nonTrivialDefault(f.DefValue),
			Usage:   f.Usage,
		})
	})
	return map[string]any{
		"commands":       buildCommandTree(root),
		"globalFlags":    globalFlags,
		"pseudoCommands": buildPseudoCommands(),
		"enums": map[string][]string{
			"features":     featureIDs(),
			"outputModes":  {"pretty", "json"},
			"doctorStatus": {"healthy", "warning", "not_ready", "auth_error"},
		},
		"version": versionInfo(),
	}
}

// showAllHelp reports whether either the in-flight command or any inherited
// flag set has --all set. We accept both because `--all` is a persistent
// root flag that may not be visible on Cobra's flag stack at every nesting
// level.
func showAllHelp(cmd *cobra.Command) bool {
	if flag := cmd.Flags().Lookup("all"); flag != nil {
		value, err := cmd.Flags().GetBool("all")
		if err == nil {
			return value
		}
	}
	if flag := cmd.InheritedFlags().Lookup("all"); flag != nil {
		value, err := cmd.InheritedFlags().GetBool("all")
		if err == nil {
			return value
		}
	}
	return false
}

// buildCommandTree walks the cobra tree (skipping help/completion) and
// returns a sorted, machine-readable list of all enumerable commands.
// Sort order makes diffs across releases easy to read.
func buildCommandTree(root *cobra.Command) []commandHelpInfo {
	var result []commandHelpInfo
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, child := range cmd.Commands() {
			if child.Name() == "help" || child.Name() == "completion" {
				continue
			}
			result = append(result, commandHelpInfo{
				Path:    child.CommandPath(),
				Command: strings.TrimSpace(strings.TrimPrefix(child.CommandPath(), root.CommandPath())),
				Short:   child.Short,
				Flags:   localFlagInfos(child),
			})
			walk(child)
		}
	}
	walk(root)
	sort.Slice(result, func(i, j int) bool { return result[i].Path < result[j].Path })
	return result
}

func localFlagInfos(cmd *cobra.Command) []flagHelpInfo {
	inherited := cmd.InheritedFlags()
	flags := make([]flagHelpInfo, 0)
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		if f.Hidden || f.Name == "help" || inherited.Lookup(f.Name) != nil {
			return
		}
		flags = append(flags, flagHelpInfo{
			Name:    f.Name,
			Type:    f.Value.Type(),
			Default: nonTrivialDefault(f.DefValue),
			Usage:   f.Usage,
		})
	})
	return flags
}

// nonTrivialDefault hides Cobra's noisy default-value strings ("", "false",
// "0") so the introspect payload only includes interesting defaults.
func nonTrivialDefault(v string) string {
	if v == "" || v == "false" || v == "0" {
		return ""
	}
	return v
}

func (a *App) buildIntrospectCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "introspect",
		Short: "Emit machine-readable command metadata",
		Long:  "Emit command paths, flag metadata, stable command labels, pseudo commands, and known enums for agent tooling. Equivalent to `agora --help --all --json`.",
		Example: example(`
  agora introspect --json
  agora --help --all --json
`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return renderResult(cmd, "introspect", buildIntrospectionData(cmd.Root()))
		},
	}
}
