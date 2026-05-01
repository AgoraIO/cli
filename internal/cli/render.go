package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
	"golang.org/x/term"
)

// renderResult is the single dispatch point for command output. In JSON
// mode it always emits a jsonEnvelope; in pretty mode it dispatches to a
// hand-tuned printer per command label. --quiet suppresses the success
// envelope in BOTH modes (NDJSON progress events emitted earlier are
// observability and stay).
//
// New command labels go in the switch below; the default branch dumps the
// raw map so unforeseen shapes still produce some output during development.
func renderResult(cmd *cobra.Command, command string, data any) error {
	out := cmd.OutOrStdout()
	quiet, _ := cmd.Context().Value(contextKeyQuiet{}).(bool)
	if aMode := cmd.Context().Value(contextKeyOutputMode{}); aMode != nil && aMode.(outputMode) == outputJSON {
		if quiet {
			return nil
		}
		return emitEnvelope(out, command, data, jsonPrettyFromContext(cmd))
	}
	if quiet {
		return nil
	}
	switch command {
	case "login":
		m := data.(map[string]any)
		printBlock(out, "Login", [][2]string{{"Status", asString(m["status"])}, {"Scope", asString(m["scope"])}, {"Expires At", asString(m["expiresAt"])}})
	case "logout":
		m := data.(map[string]any)
		printBlock(out, "Logout", [][2]string{{"Status", asString(m["status"])}, {"Session Cleared", asString(m["clearedSession"])}})
	case "auth status":
		m := data.(map[string]any)
		printBlock(out, "Auth", [][2]string{{"Status", asString(m["status"])}, {"Authenticated", asString(m["authenticated"])}, {"Scope", asString(m["scope"])}, {"Expires At", asString(m["expiresAt"])}})
	case "project create":
		m := data.(map[string]any)
		features := "-"
		if list, ok := m["enabledFeatures"].([]string); ok {
			features = strings.Join(list, ", ")
		}
		printBlock(out, "Project", [][2]string{{"Name", asString(m["projectName"])}, {"Project ID", asString(m["projectId"])}, {"App ID", asString(m["appId"])}, {"Region", asString(m["region"])}, {"Features", features}})
	case "project use":
		m := data.(map[string]any)
		printBlock(out, "Current Project", [][2]string{{"Name", asString(m["projectName"])}, {"Project ID", asString(m["projectId"])}, {"Region", asString(m["region"])}})
	case "project show":
		m := data.(map[string]any)
		printBlock(out, "Project", [][2]string{{"Name", asString(m["projectName"])}, {"Project ID", asString(m["projectId"])}, {"App ID", asString(m["appId"])}, {"App Certificate", redactSensitive(m["appCertificate"])}, {"Region", asString(m["region"])}, {"Token Enabled", asString(m["tokenEnabled"])}})
	case "project env write":
		m := data.(map[string]any)
		printBlock(out, "Project Env", [][2]string{{"Project", asString(m["projectName"])}, {"Project ID", asString(m["projectId"])}, {"Path", asString(m["path"])}, {"Status", asString(m["status"])}})
	case "project env":
		m := data.(map[string]any)
		valuesText := renderProjectEnv(m["values"].(map[string]any), envDotenv)
		printBlock(out, "Project Env", [][2]string{{"Project", asString(m["projectName"])}, {"Project ID", asString(m["projectId"])}, {"Region", asString(m["region"])}})
		fmt.Fprintln(out)
		fmt.Fprint(out, valuesText)
	case "quickstart list":
		m := data.(map[string]any)
		fmt.Fprintln(out, "Quickstarts")
		if items, ok := m["items"].([]map[string]any); ok {
			for _, item := range items {
				fmt.Fprintf(out, "- %s: %s\n", asString(item["id"]), asString(item["title"]))
				if verbose, _ := m["verbose"].(bool); verbose {
					fmt.Fprintf(out, "  Available: %s\n", asString(item["available"]))
					fmt.Fprintf(out, "  Runtime: %s\n", asString(item["runtime"]))
					fmt.Fprintf(out, "  Supports Init: %s\n", asString(item["supportsInit"]))
					fmt.Fprintf(out, "  Env: %s\n", asString(item["envDocs"]))
					fmt.Fprintf(out, "  Repo: %s\n", asString(item["repoUrl"]))
				}
			}
		}
	case "quickstart create":
		m := data.(map[string]any)
		printBlock(out, "Quickstart", [][2]string{{"Template", asString(m["template"])}, {"Path", asString(m["path"])}, {"Project", asString(m["projectName"])}, {"Env", asString(m["envStatus"])}, {"Metadata", asString(m["metadataPath"])}, {"Status", asString(m["status"])}})
		if steps, ok := m["nextSteps"].([]string); ok && len(steps) > 0 {
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Next Steps")
			for _, step := range steps {
				fmt.Fprintf(out, "- %s\n", step)
			}
		}
	case "quickstart env write":
		m := data.(map[string]any)
		printBlock(out, "Quickstart Env", [][2]string{{"Template", asString(m["template"])}, {"Project", asString(m["projectName"])}, {"Path", asString(m["path"])}, {"Env Path", asString(m["envPath"])}, {"Metadata", asString(m["metadataPath"])}, {"Status", asString(m["status"])}})
	case "init":
		m := data.(map[string]any)
		features := "-"
		if list, ok := m["enabledFeatures"].([]string); ok && len(list) > 0 {
			features = strings.Join(list, ", ")
		}
		printBlock(out, "Init", [][2]string{{"Template", asString(m["template"])}, {"Project", asString(m["projectName"])}, {"Project ID", asString(m["projectId"])}, {"Project Action", asString(m["projectAction"])}, {"Region", asString(m["region"])}, {"Path", asString(m["path"])}, {"Env Path", asString(m["envPath"])}, {"Metadata", asString(m["metadataPath"])}, {"Features", features}, {"Status", asString(m["status"])}})
		if steps, ok := m["nextSteps"].([]string); ok && len(steps) > 0 {
			fmt.Fprintln(out)
			fmt.Fprintln(out, "Next Steps")
			for _, step := range steps {
				fmt.Fprintf(out, "- %s\n", step)
			}
		}
	case "project feature list":
		m := data.(map[string]any)
		fmt.Fprintf(out, "Project Features: %s\n", asString(m["projectName"]))
		if items, ok := m["items"].([]featureItem); ok {
			for _, item := range items {
				fmt.Fprintf(out, "- %s: %s (%s)\n", item.Feature, item.Status, item.Message)
			}
		}
	case "project feature status", "project feature enable":
		m := data.(map[string]any)
		printBlock(out, "Feature", [][2]string{{"Feature", asString(m["feature"])}, {"Project", asString(m["projectName"])}, {"Status", asString(m["status"])}, {"Message", asString(m["message"])}})
	case "project list":
		m := data.(map[string]any)
		total, _ := m["total"].(int)
		page, _ := m["page"].(int)
		pageSize, _ := m["pageSize"].(int)
		if pageSize <= 0 {
			pageSize = 20
		}
		totalPages := (total + pageSize - 1) / pageSize
		if totalPages == 0 {
			totalPages = 1
		}
		count := 0
		if items, ok := m["items"].([]projectSummary); ok {
			count = len(items)
		}
		printBlock(out, "Projects", [][2]string{
			{"Total", asString(total)},
			{"Page", fmt.Sprintf("%d of %d (showing %d)", page, totalPages, count)},
		})
		fmt.Fprintln(out)
		if items, ok := m["items"].([]projectSummary); ok {
			for _, item := range items {
				fmt.Fprintln(out, item.Name)
				printBlock(out, "", [][2]string{{"Project ID", item.ProjectID}, {"Type", item.ProjectType}, {"Status", item.Status}})
				fmt.Fprintln(out)
			}
		}
	case "project doctor":
		noColor, _ := cmd.Context().Value(contextKeyNoColor{}).(bool)
		return printDoctor(out, data.(projectDoctorResult), noColor || strings.TrimSpace(os.Getenv("NO_COLOR")) != "")
	case "version":
		m := data.(map[string]any)
		printBlock(out, "Version", [][2]string{{"Version", asString(m["version"])}, {"Commit", asString(m["commit"])}, {"Built", asString(m["date"])}})
	case "telemetry":
		m := data.(map[string]any)
		printBlock(out, "Telemetry", [][2]string{{"Enabled", asString(m["enabled"])}, {"Config Path", asString(m["configPath"])}, {"DO_NOT_TRACK", asString(m["doNotTrack"])}})
	case "upgrade":
		m := data.(map[string]any)
		printBlock(out, "Upgrade", [][2]string{{"Status", asString(m["status"])}, {"Install Method", asString(m["installMethod"])}, {"Command", asString(m["command"])}})
	case "open":
		m := data.(map[string]any)
		printBlock(out, "Open", [][2]string{{"Target", asString(m["target"])}, {"URL", asString(m["url"])}, {"Status", asString(m["status"])}})
	default:
		encoded, _ := json.MarshalIndent(data, "", "  ")
		fmt.Fprintf(out, "%s\n%s\n", command, string(encoded))
	}
	return nil
}

// asString converts heterogeneous payload values into the human-friendly
// string used by printBlock. nil / empty string become "-"; bool becomes
// "yes"/"no"; everything else falls back to fmt.Sprint.
func asString(v any) string {
	switch x := v.(type) {
	case nil:
		return "-"
	case string:
		if x == "" {
			return "-"
		}
		return x
	case bool:
		if x {
			return "yes"
		}
		return "no"
	default:
		return fmt.Sprint(v)
	}
}

// redactSensitive returns "[hidden]" for any non-empty string value and
// "-" for empty / nil. Used for fields like App Certificate that should
// never appear in pretty output.
func redactSensitive(v any) string {
	switch x := v.(type) {
	case nil:
		return "-"
	case *string:
		if x == nil || *x == "" {
			return "-"
		}
		return "[hidden]"
	case string:
		if x == "" {
			return "-"
		}
		return "[hidden]"
	default:
		return "-"
	}
}

// printBlock renders a key-value block with right-padded labels. An empty
// title suppresses the header row, useful when stacking multiple blocks
// under a single section.
func printBlock(out io.Writer, title string, rows [][2]string) {
	width := 0
	for _, row := range rows {
		if len(row[0]) > width {
			width = len(row[0])
		}
	}
	if title != "" {
		fmt.Fprintln(out, title)
	}
	for _, row := range rows {
		value := row[1]
		if max := terminalValueWidth(width); max > 0 && len(value) > max {
			value = value[:max-1] + "..."
		}
		fmt.Fprintf(out, "%-*s : %s\n", width, row[0], value)
	}
}

// terminalValueWidth returns the maximum number of value-column
// characters that fit on one terminal line, given the label-column
// width plus the " : " separator.
//
// Resolution order:
//
//  1. COLUMNS env var when set and parseable. Lets users and tests
//     override the detected width without a real TTY (and lets
//     containers/CI runners that *do* export COLUMNS opt in).
//  2. golang.org/x/term.GetSize against stderr, then stdout. Both
//     are tried because pretty output goes to stdout but stderr is
//     more often a TTY when stdout is being piped (the common case
//     for `agora ... | jq`).
//  3. 0, meaning "do not truncate". Honoring "no terminal detected
//     => never truncate" is the safest default for log scrapers and
//     CI build logs.
//
// A returned 0 (no width info) means the caller MUST NOT truncate.
// A nonzero value is the byte width available for the value column;
// callers should treat values longer than this as truncation
// candidates. Values below 20 characters of available room are
// suppressed because narrower truncation produces unreadable output.
func terminalValueWidth(labelWidth int) int {
	columns := detectTerminalColumns()
	if columns <= 0 {
		return 0
	}
	available := columns - labelWidth - len(" : ")
	if available < 20 {
		return 0
	}
	return available
}

// detectTerminalColumns is the resolution helper for
// terminalValueWidth. Split out so tests can drive it directly.
func detectTerminalColumns() int {
	if raw := strings.TrimSpace(os.Getenv("COLUMNS")); raw != "" {
		if columns, err := strconv.Atoi(raw); err == nil && columns > 0 {
			return columns
		}
	}
	for _, fd := range []uintptr{os.Stderr.Fd(), os.Stdout.Fd()} {
		if width, _, err := term.GetSize(int(fd)); err == nil && width > 0 {
			return width
		}
	}
	return 0
}

// printDoctor prints a structured diagnostic report including per-category
// items, suggested recovery commands, and a status summary line. noColor
// swaps Unicode glyphs for ASCII so the output is safe for log scrapers.
func printDoctor(out io.Writer, result projectDoctorResult, noColor bool) error {
	if m, ok := result.Project.(map[string]any); ok {
		fmt.Fprintf(out, "Checking project: %s\n", asString(m["name"]))
		mode := "Mode: " + asString(result.Feature)
		if result.Mode == "deep" {
			mode += " (deep)"
		}
		fmt.Fprintf(out, "%s\n\n", mode)
	}
	for _, category := range result.Checks {
		fmt.Fprintf(out, "%s\n", strings.ToUpper(category.Category[:1])+category.Category[1:])
		for _, item := range category.Items {
			marker := doctorMarker(item.Status, noColor)
			fmt.Fprintf(out, "  %s %s\n", marker, item.Message)
			if item.SuggestedCommand != "" {
				fmt.Fprintf(out, "    Run: %s\n", item.SuggestedCommand)
			}
		}
		fmt.Fprintln(out)
	}
	fmt.Fprintln(out, "Summary")
	marker := "✗"
	if result.Healthy {
		marker = "✓"
	} else if result.Status == "warning" {
		marker = "!"
	}
	if noColor {
		marker = doctorMarker(map[bool]string{true: "pass", false: "fail"}[result.Healthy], noColor)
		if result.Status == "warning" {
			marker = doctorMarker("warn", noColor)
		}
	}
	fmt.Fprintf(out, "  %s %s\n", marker, result.Summary)
	return nil
}

func doctorMarker(status string, noColor bool) string {
	if noColor {
		return map[string]string{"pass": "OK", "warn": "!", "skipped": "-", "fail": "X"}[status]
	}
	return map[string]string{"pass": "✓", "warn": "!", "skipped": "-", "fail": "✗"}[status]
}
