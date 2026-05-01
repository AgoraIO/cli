package cli

// docgen.go renders docs/commands.md straight from the live cobra tree so
// the published reference can never drift from the binary that ships in
// the same release.
//
// The output is intentionally narrow: command path → description → flags.
// Anything richer (long descriptions, examples) lives next to the command
// in cobra and is reachable via `agora <cmd> --help`. The reference exists
// for skim-reading and for agents that want a single discoverable list.

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// RenderCommandReference writes a Markdown reference of every command and
// global flag rooted at `root` to `out`. The page is generated; never edit
// it by hand. Update the cobra command definitions and re-run
// `make docs-commands` (or rely on the release-time regen) instead.
func RenderCommandReference(out io.Writer, root *cobra.Command) error {
	data := buildIntrospectionData(root)

	// extractTime keeps the generated header informative without making the
	// timestamp diff noisy on every regen — we floor to the hour. Production
	// callers can always look at git history for the exact regen commit.
	stamp := time.Now().UTC().Format("2006-01-02")

	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("title: Command Reference\n")
	b.WriteString("---\n\n")
	b.WriteString("# Agora CLI — Command Reference\n\n")
	b.WriteString("> Generated from `agora introspect --json` on ")
	b.WriteString(stamp)
	b.WriteString(". Do not edit by hand — run `make docs-commands` or rely on the release workflow to regenerate.\n\n")
	b.WriteString("This page lists every enumerable command and its local flags. For long descriptions, examples, and inherited flags, run `agora <command> --help` or read the source in `internal/cli/`.\n\n")

	if version, ok := data["version"].(map[string]string); ok {
		b.WriteString("**CLI version snapshot:** ")
		b.WriteString("`")
		b.WriteString(version["version"])
		b.WriteString("`\n\n")
	}

	b.WriteString("## Global Flags\n\n")
	if globalFlags, ok := data["globalFlags"].([]flagHelpInfo); ok {
		writeFlagsTable(&b, globalFlags)
	}

	b.WriteString("\n## Pseudo Commands\n\n")
	b.WriteString("Pseudo commands are root-level flags that emit their own JSON envelope rather than living in the cobra subcommand tree. Agents should treat the `command` label as a stable identifier when matching JSON envelopes.\n\n")
	if pseudo, ok := data["pseudoCommands"].([]pseudoCommandInfo); ok {
		b.WriteString("| Command | Trigger | Description |\n")
		b.WriteString("|---------|---------|-------------|\n")
		for _, p := range pseudo {
			fmt.Fprintf(&b, "| `%s` | `%s` | %s |\n", escapeMarkdownCell(p.Command), escapeMarkdownCell(p.Trigger), escapeMarkdownCell(p.Short))
		}
	}

	b.WriteString("\n## Commands\n\n")
	if commands, ok := data["commands"].([]commandHelpInfo); ok {
		for _, cmd := range commands {
			fmt.Fprintf(&b, "### `%s`\n\n", cmd.Path)
			if strings.TrimSpace(cmd.Short) != "" {
				b.WriteString(cmd.Short)
				b.WriteString("\n\n")
			}
			if len(cmd.Flags) == 0 {
				b.WriteString("_No local flags. Inherited global flags still apply (see [Global Flags](#global-flags))._\n\n")
				continue
			}
			writeFlagsTable(&b, cmd.Flags)
			b.WriteString("\n")
		}
	}

	b.WriteString("## Enums\n\n")
	if enums, ok := data["enums"].(map[string][]string); ok {
		// Stable order for diff hygiene.
		keys := []string{"features", "outputModes", "doctorStatus"}
		seen := map[string]bool{}
		for _, key := range keys {
			if values, exists := enums[key]; exists {
				writeEnumRow(&b, key, values)
				seen[key] = true
			}
		}
		for key, values := range enums {
			if !seen[key] {
				writeEnumRow(&b, key, values)
			}
		}
	}

	_, err := io.WriteString(out, b.String())
	return err
}

func writeFlagsTable(b *strings.Builder, flags []flagHelpInfo) {
	if len(flags) == 0 {
		return
	}
	b.WriteString("| Flag | Type | Default | Description |\n")
	b.WriteString("|------|------|---------|-------------|\n")
	for _, f := range flags {
		def := f.Default
		if def == "" {
			def = "—"
		} else {
			def = "`" + def + "`"
		}
		fmt.Fprintf(b, "| `--%s` | `%s` | %s | %s |\n", f.Name, f.Type, def, escapeMarkdownCell(f.Usage))
	}
}

func writeEnumRow(b *strings.Builder, name string, values []string) {
	fmt.Fprintf(b, "**`%s`**: ", name)
	for i, v := range values {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(b, "`%s`", v)
	}
	b.WriteString("\n\n")
}

// escapeMarkdownCell hides characters that would break a single-row markdown
// table (mainly `|` and embedded newlines). It is intentionally minimal;
// the CLI's command text is already short and well-behaved.
func escapeMarkdownCell(s string) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "|", "\\|")
	return s
}

// NewRootForDocs builds a fully-wired root cobra command without doing
// any I/O — used by cmd/gendocs to walk the tree without booting the CLI
// for real.
func NewRootForDocs() (*cobra.Command, error) {
	app, err := NewApp()
	if err != nil {
		return nil, err
	}
	return app.root, nil
}
