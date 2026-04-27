package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"
)

func defaultInitFeatures() []string {
	return []string{"rtc", "convoai"}
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
	var features []string
	cmd := &cobra.Command{
		Use:   "init <name>",
		Short: "Create a project, clone a quickstart, and write env in one flow",
		Long: `Init is the recommended onboarding command.

By default it creates a new Agora project, enables rtc and convoai, clones the official quickstart repository, and writes the expected local env file.

Use --project to bind to an existing Agora project instead of creating a new one.
Use --feature to specify which features to enable instead of the defaults (repeatable).`,
		Example: strings.TrimSpace(`
  agora init my-nextjs-demo --template nextjs
  agora init my-python-demo --template python
  agora init my-go-demo --template go --project my-existing-project
  agora init my-rtm-demo --template nextjs --feature rtc --feature rtm
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 || strings.TrimSpace(args[0]) == "" {
				return fmt.Errorf("project name is required")
			}
			template, ok := findQuickstartTemplate(templateID)
			if !ok {
				return fmt.Errorf("unknown quickstart template %q", templateID)
			}
			targetDir := dir
			if strings.TrimSpace(targetDir) == "" {
				targetDir = args[0]
			}
			result, err := a.initProject(args[0], targetDir, *template, existingProject, region, features)
			if err != nil {
				return err
			}
			return renderResult(cmd, "init", result)
		},
	}
	cmd.Flags().StringVar(&templateID, "template", "", "quickstart template ID to use")
	cmd.Flags().StringVar(&dir, "dir", "", "target directory for the cloned quickstart; defaults to <name>")
	cmd.Flags().StringVar(&existingProject, "project", "", "existing project ID or exact project name to use instead of creating a new project")
	cmd.Flags().StringVar(&region, "region", "", "control plane region for newly created projects (global or cn)")
	cmd.Flags().StringArrayVar(&features, "feature", nil, "enable a feature on the newly created project (repeatable); defaults to rtc and convoai")
	_ = cmd.MarkFlagRequired("template")
	return cmd
}

func (a *App) initProject(name, targetDir string, template quickstartTemplate, existingProject, region string, features []string) (map[string]any, error) {
	var target projectTarget
	projectAction := "existing"
	enabledFeatures := []string{}

	if strings.TrimSpace(existingProject) != "" {
		resolved, err := a.resolveProjectTarget(existingProject)
		if err != nil {
			return nil, err
		}
		target = resolved
	} else {
		featuresToEnable := features
		if len(featuresToEnable) == 0 {
			featuresToEnable = defaultInitFeatures()
		}
		projectResult, err := a.projectCreate(name, region, "", featuresToEnable)
		if err != nil {
			return nil, err
		}
		projectAction = "created"
		if list, ok := projectResult["enabledFeatures"].([]string); ok {
			enabledFeatures = list
		}
		resolved, err := a.resolveProjectTarget(asString(projectResult["projectId"]))
		if err != nil {
			return nil, err
		}
		target = resolved
	}

	quickstartResult, err := a.quickstartCreate(template, targetDir, target.project.ProjectID)
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
		"action":          "init",
		"enabledFeatures": enabledFeatures,
		"envPath":         quickstartResult["envPath"],
		"metadataPath":    filepath.ToSlash(filepath.Join(localAgoraDirName, localProjectFileName)),
		"nextSteps":       initNextSteps(template, asString(quickstartResult["path"])),
		"path":            quickstartResult["path"],
		"projectAction":   projectAction,
		"projectId":       target.project.ProjectID,
		"projectName":     target.project.Name,
		"region":          target.region,
		"status":          "ready",
		"template":        template.ID,
		"title":           template.Title,
	}
	return result, nil
}
