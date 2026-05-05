package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// defaultInitFeatures returns the features enabled on a freshly created
// Agora project when no `--feature` flags are passed. Sourced from the
// canonical feature catalog so adding a new feature in features.go
// flows here automatically.
func defaultInitFeatures() []string {
	return featureIDs()
}

func initNextSteps(template quickstartTemplate, targetDir string) []string {
	dir := filepath.Base(targetDir)
	steps := []string{"cd " + dir}
	if template.InstallCommand != "" {
		steps = append(steps, template.InstallCommand)
	}
	if template.RunCommand != "" {
		steps = append(steps, template.RunCommand)
	}
	return steps
}

func (a *App) buildInitCommand() *cobra.Command {
	var templateID string
	var dir string
	var existingProject string
	var region string
	var rtmDataCenter string
	var features []string
	var agentRules []string
	var newProject bool
	cmd := &cobra.Command{
		Use:   "init <name>",
		Short: "Create a project, clone a quickstart, and write env in one flow",
		Long: `Init is the recommended onboarding command.

By default it reuses your existing Agora project — preferring one named "Default Project". In interactive sessions without a Default Project, init shows your existing projects and a create-new option. Non-interactive runs fall back to the most recent project. A new project is created when no projects exist yet or when --new-project is passed.

Use --project to bind to a specific existing project by name or ID.
Use --new-project to always create a fresh project regardless of existing ones.
Use --feature to specify which features to enable on a newly created project (repeatable).`,
		Example: example(`
  agora init my-nextjs-demo --template nextjs
  agora init my-python-demo --template python
  agora init my-go-demo --template go --project my-existing-project
  agora init my-rtm-demo --template nextjs --new-project --rtm-data-center AP
  agora init my-rtm-demo --template nextjs --new-project --feature rtc --feature rtm
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				return &cliError{Message: "directory name is required", Code: "INIT_NAME_REQUIRED"}
			}
			if strings.TrimSpace(templateID) == "" {
				selected, err := a.selectInitTemplate(cmd)
				if err != nil {
					return err
				}
				templateID = selected
			}
			template, ok := findQuickstartTemplate(templateID)
			if !ok {
				return &cliError{Message: fmt.Sprintf("unknown quickstart template %q. Run `agora quickstart list` to see available templates.", templateID), Code: "QUICKSTART_TEMPLATE_UNKNOWN"}
			}
			targetDir := dir
			if strings.TrimSpace(targetDir) == "" {
				targetDir = args[0]
			}
			// Interactive reuse confirmation: only when TTY+pretty+not-CI, no
			// explicit --project, and not --new-project. Silent reuse stays the
			// default for --json / CI / non-TTY agent runs.
			promptForReuse := strings.TrimSpace(existingProject) == "" &&
				!newProject &&
				!a.noInput() &&
				a.resolveOutputMode(cmd) != outputJSON &&
				!isCIEnvironment(a.osEnv) &&
				isTTY(os.Stdin)
			progress := jsonProgressFor(a, cmd, "init")
			result, err := a.initProject(args[0], targetDir, *template, existingProject, region, features, rtmDataCenter, newProject, promptForReuse, cmd.ErrOrStderr(), os.Stdin, progress)
			if err != nil {
				return err
			}
			if len(agentRules) > 0 {
				written, err := writeAgentRules(asString(result["path"]), agentRules)
				if err != nil {
					return err
				}
				result["agentRules"] = written
			}
			return renderResult(cmd, "init", result)
		},
	}
	cmd.Flags().StringVar(&templateID, "template", "", "quickstart template ID to use")
	cmd.Flags().StringVar(&dir, "dir", "", "target directory for the cloned quickstart; defaults to <name>")
	cmd.Flags().StringVar(&existingProject, "project", "", "existing project ID or exact project name to bind to")
	cmd.Flags().StringVar(&region, "region", "", "control plane region for newly created projects (global or cn)")
	cmd.Flags().StringVar(&rtmDataCenter, "rtm-data-center", "", "RTM data center to configure when rtm is enabled on a newly created project (CN, NA, EU, or AP); defaults to NA")
	cmd.Flags().StringArrayVar(&features, "feature", nil, fmt.Sprintf("enable a feature on the newly created project (repeatable); defaults to %s; convoai also enables rtm", featureListString()))
	cmd.Flags().StringArrayVar(&agentRules, "add-agent-rules", nil, "write AI agent rules into the quickstart (repeatable: cursor, claude, windsurf)")
	cmd.Flags().BoolVar(&newProject, "new-project", false, "always create a new Agora project instead of reusing an existing one")
	return cmd
}

func (a *App) selectInitTemplate(cmd *cobra.Command) (string, error) {
	if a.noInput() {
		for _, template := range quickstartTemplates() {
			if template.Available && template.SupportsInit {
				return template.ID, nil
			}
		}
		return "", &cliError{Message: "no init-compatible quickstart templates are available.", Code: "QUICKSTART_TEMPLATE_UNAVAILABLE"}
	}
	if a.resolveOutputMode(cmd) == outputJSON || isCIEnvironment(a.osEnv) || !isTTY(os.Stdin) {
		return "", &cliError{Message: "quickstart template is required. Pass `--template` or run `agora quickstart list`.", Code: "QUICKSTART_TEMPLATE_REQUIRED"}
	}
	templates := []quickstartTemplate{}
	for _, template := range quickstartTemplates() {
		if template.Available && template.SupportsInit {
			templates = append(templates, template)
		}
	}
	if len(templates) == 0 {
		return "", &cliError{Message: "no init-compatible quickstart templates are available.", Code: "QUICKSTART_TEMPLATE_UNAVAILABLE"}
	}
	out := cmd.ErrOrStderr()
	fmt.Fprintln(out, "Choose a quickstart template:")
	for index, template := range templates {
		fmt.Fprintf(out, "  %d. %s (%s)\n", index+1, template.ID, template.Title)
	}
	fmt.Fprint(out, "Template: ")
	reader := bufio.NewReader(os.Stdin)
	answer, err := reader.ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return "", err
	}
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return templates[0].ID, nil
	}
	if index, err := strconv.Atoi(answer); err == nil && index >= 1 && index <= len(templates) {
		return templates[index-1].ID, nil
	}
	if _, ok := findQuickstartTemplate(answer); ok {
		return answer, nil
	}
	return "", &cliError{Message: fmt.Sprintf("unknown quickstart template %q. Run `agora quickstart list` to see available templates.", answer), Code: "QUICKSTART_TEMPLATE_UNKNOWN"}
}

func parseInitProjectTimestamp(value string) (time.Time, bool) {
	if strings.TrimSpace(value) == "" {
		return time.Time{}, false
	}
	for _, layout := range []string{time.RFC3339Nano, time.RFC3339} {
		if parsed, err := time.Parse(layout, value); err == nil {
			return parsed, true
		}
	}
	return time.Time{}, false
}

func selectInitProjectFromList(items []projectSummary) (projectSummary, bool) {
	if len(items) == 0 {
		return projectSummary{}, false
	}
	if item, ok := selectDefaultInitProjectFromList(items); ok {
		return item, true
	}
	selected := items[0]
	selectedCreated, selectedCreatedOK := parseInitProjectTimestamp(selected.CreatedAt)
	selectedUpdated, selectedUpdatedOK := parseInitProjectTimestamp(selected.UpdatedAt)
	for _, item := range items[1:] {
		itemCreated, itemCreatedOK := parseInitProjectTimestamp(item.CreatedAt)
		switch {
		case itemCreatedOK && !selectedCreatedOK:
			selected = item
			selectedCreated = itemCreated
			selectedCreatedOK = true
			selectedUpdated, selectedUpdatedOK = parseInitProjectTimestamp(item.UpdatedAt)
			continue
		case !itemCreatedOK || !selectedCreatedOK:
			continue
		case itemCreated.After(selectedCreated):
			selected = item
			selectedCreated = itemCreated
			selectedUpdated, selectedUpdatedOK = parseInitProjectTimestamp(item.UpdatedAt)
			continue
		case !itemCreated.Equal(selectedCreated):
			continue
		}
		itemUpdated, itemUpdatedOK := parseInitProjectTimestamp(item.UpdatedAt)
		switch {
		case itemUpdatedOK && !selectedUpdatedOK:
			selected = item
			selectedCreated = itemCreated
			selectedUpdated = itemUpdated
			selectedUpdatedOK = true
		case itemUpdatedOK && selectedUpdatedOK && itemUpdated.After(selectedUpdated):
			selected = item
			selectedCreated = itemCreated
			selectedUpdated = itemUpdated
		case itemUpdatedOK == selectedUpdatedOK && item.ProjectID > selected.ProjectID:
			selected = item
			selectedCreated = itemCreated
			selectedUpdated = itemUpdated
			selectedUpdatedOK = itemUpdatedOK
		}
	}
	return selected, true
}

func selectDefaultInitProjectFromList(items []projectSummary) (projectSummary, bool) {
	for _, item := range items {
		if item.Name == "Default Project" {
			return item, true
		}
	}
	return projectSummary{}, false
}

func initProjectChoiceItems(items []projectSummary) []projectSummary {
	choices := append([]projectSummary{}, items...)
	sort.SliceStable(choices, func(i, j int) bool {
		leftCreated, leftCreatedOK := parseInitProjectTimestamp(choices[i].CreatedAt)
		rightCreated, rightCreatedOK := parseInitProjectTimestamp(choices[j].CreatedAt)
		switch {
		case leftCreatedOK && rightCreatedOK && !leftCreated.Equal(rightCreated):
			return leftCreated.Before(rightCreated)
		case leftCreatedOK != rightCreatedOK:
			return !leftCreatedOK
		}
		leftUpdated, leftUpdatedOK := parseInitProjectTimestamp(choices[i].UpdatedAt)
		rightUpdated, rightUpdatedOK := parseInitProjectTimestamp(choices[j].UpdatedAt)
		switch {
		case leftUpdatedOK && rightUpdatedOK && !leftUpdated.Equal(rightUpdated):
			return leftUpdated.Before(rightUpdated)
		case leftUpdatedOK != rightUpdatedOK:
			return !leftUpdatedOK
		}
		return choices[i].ProjectID < choices[j].ProjectID
	})
	return choices
}

// chooseInitProject prompts a human user to either create a fresh project or bind
// the new quickstart to one of the existing projects. The default selection is
// the most recently created project, displayed last.
func chooseInitProject(in io.Reader, out io.Writer, items []projectSummary) (projectSummary, string, error) {
	choices := initProjectChoiceItems(items)
	if len(choices) == 0 {
		return projectSummary{}, "new", nil
	}
	defaultIndex := len(choices) + 1
	reader := bufio.NewReader(in)
	for {
		if _, err := fmt.Fprintln(out, "Choose an Agora project:"); err != nil {
			return projectSummary{}, "", err
		}
		if _, err := fmt.Fprintln(out, "  1. Create a new project"); err != nil {
			return projectSummary{}, "", err
		}
		for index, item := range choices {
			suffix := ""
			if index == len(choices)-1 {
				suffix = " (most recent)"
			}
			if _, err := fmt.Fprintf(out, "  %d. %s (%s)%s\n", index+2, item.Name, item.ProjectID, suffix); err != nil {
				return projectSummary{}, "", err
			}
		}
		if _, err := fmt.Fprintf(out, "Project [%d]: ", defaultIndex); err != nil {
			return projectSummary{}, "", err
		}
		answer, err := reader.ReadString('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return projectSummary{}, "", err
		}
		trimmed := strings.ToLower(strings.TrimSpace(answer))
		switch trimmed {
		case "":
			return choices[len(choices)-1], "reuse", nil
		case "1", "new", "c", "create":
			return projectSummary{}, "new", nil
		case "n", "no", "abort", "q", "quit":
			return projectSummary{}, "abort", nil
		}
		if index, parseErr := strconv.Atoi(trimmed); parseErr == nil && index >= 2 && index <= len(choices)+1 {
			return choices[index-2], "reuse", nil
		}
		for _, item := range choices {
			if strings.EqualFold(item.ProjectID, strings.TrimSpace(answer)) || strings.EqualFold(item.Name, strings.TrimSpace(answer)) {
				return item, "reuse", nil
			}
		}
		if _, err := fmt.Fprintf(out, "Please choose 1-%d, enter a project name/id, or type new.\n", len(choices)+1); err != nil {
			return projectSummary{}, "", err
		}
		if errors.Is(err, io.EOF) {
			return projectSummary{}, "abort", nil
		}
	}
}

func (a *App) listInitProjects() (projectContext, []projectSummary, error) {
	ctx, err := loadContext(a.env)
	if err != nil {
		return projectContext{}, nil, err
	}
	list, err := a.listProjects("", 1, 100)
	if err != nil {
		return projectContext{}, nil, err
	}
	return ctx, list.Items, nil
}

func (a *App) resolveInitProject(ctx projectContext, item projectSummary) (projectTarget, error) {
	project, err := a.getProject(item.ProjectID)
	if err != nil {
		return projectTarget{}, err
	}
	region := ctx.CurrentRegion
	if region == "" {
		region = "global"
	}
	if item.Region != nil && *item.Region != "" {
		region = *item.Region
	}
	if project.Region != nil && *project.Region != "" {
		region = *project.Region
	}
	return projectTarget{project: project, region: region}, nil
}

func (a *App) initProject(name, targetDir string, template quickstartTemplate, existingProject, region string, features []string, rtmDataCenter string, newProject bool, promptForReuse bool, promptOut io.Writer, promptIn io.Reader, progress progressEmitter) (map[string]any, error) {
	var target projectTarget
	projectAction := "existing"
	enabledFeatures := []string{}
	needsCreate := false
	createdRTMDataCenter := ""

	switch {
	case strings.TrimSpace(existingProject) != "":
		resolved, err := a.resolveProjectTarget(existingProject)
		if err != nil {
			return nil, err
		}
		target = resolved
	case newProject:
		needsCreate = true
	default:
		ctx, items, err := a.listInitProjects()
		if err != nil {
			return nil, err
		}
		if len(items) == 0 {
			needsCreate = true
			break
		}
		if item, ok := selectDefaultInitProjectFromList(items); ok {
			resolved, err := a.resolveInitProject(ctx, item)
			if err != nil {
				return nil, err
			}
			target = resolved
			break
		}
		if promptForReuse {
			selected, action, err := chooseInitProject(promptIn, promptOut, items)
			if err != nil {
				return nil, err
			}
			switch action {
			case "abort":
				return nil, &cliError{Message: "init aborted by user.", Code: "INIT_ABORTED"}
			case "new":
				needsCreate = true
			default:
				resolved, err := a.resolveInitProject(ctx, selected)
				if err != nil {
					return nil, err
				}
				target = resolved
			}
		} else {
			item, ok := selectInitProjectFromList(items)
			if !ok {
				needsCreate = true
				break
			}
			resolved, err := a.resolveInitProject(ctx, item)
			if err != nil {
				return nil, err
			}
			target = resolved
		}
	}

	if needsCreate {
		featuresToEnable := normalizeProjectCreateFeatures(features)
		progress.emit("project:create", "Creating Agora project", map[string]any{"projectName": name, "features": featuresToEnable})
		projectResult, err := a.projectCreate(name, region, "", featuresToEnable, rtmDataCenter, "")
		if err != nil {
			return nil, err
		}
		projectAction = "created"
		if list, ok := projectResult["enabledFeatures"].([]string); ok {
			enabledFeatures = list
		}
		createdRTMDataCenter = asString(projectResult["rtmDataCenter"])
		resolved, err := a.resolveProjectTarget(asString(projectResult["projectId"]))
		if err != nil {
			return nil, err
		}
		target = resolved
		progress.emit("project:created", "Agora project ready", map[string]any{"projectId": target.project.ProjectID, "projectName": target.project.Name})
	} else {
		progress.emit("project:reuse", "Reusing existing Agora project", map[string]any{"projectId": target.project.ProjectID, "projectName": target.project.Name})
	}

	quickstartResult, err := a.quickstartCreate(template, targetDir, target.project.ProjectID, "", progress)
	if err != nil {
		return nil, err
	}

	ctx, err := loadContext(a.env)
	if err != nil {
		return nil, err
	}
	ctx.CurrentProjectID = &target.project.ProjectID
	ctx.CurrentProjectName = &target.project.Name
	ctx.CurrentRegion = target.region
	if ctx.PreferredRegion == "" {
		ctx.PreferredRegion = target.region
	}
	if err := saveContext(a.env, ctx); err != nil {
		return nil, err
	}

	result := map[string]any{
		"action":                "init",
		"enabledFeatures":       enabledFeatures,
		"envPath":               quickstartResult["envPath"],
		"metadataPath":          filepath.ToSlash(filepath.Join(localAgoraDirName, localProjectFileName)),
		"nextSteps":             initNextSteps(template, asString(quickstartResult["path"])),
		"path":                  quickstartResult["path"],
		"projectAction":         projectAction,
		"projectId":             target.project.ProjectID,
		"projectName":           target.project.Name,
		"region":                target.region,
		"reusedExistingProject": projectAction == "existing",
		"status":                "ready",
		"template":              template.ID,
		"title":                 template.Title,
	}
	if createdRTMDataCenter != "" {
		result["rtmDataCenter"] = createdRTMDataCenter
	}
	return result, nil
}
