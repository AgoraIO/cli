package cli

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type quickstartTemplate struct {
	ID             string
	Title          string
	Description    string
	Runtime        string
	RepoURL        string
	DocsURL        string
	DetectPaths    []string
	EnvExamplePath string
	EnvTargetPath  string
	InstallCommand string
	RunCommand     string
	EnvDocsSummary string
	SupportsInit   bool
	Available      bool
}

func quickstartTemplates() []quickstartTemplate {
	return []quickstartTemplate{
		{
			ID:             "nextjs",
			Title:          "Conversational AI Next.js Quickstart",
			Description:    "Clone the official Next.js conversational AI quickstart.",
			Runtime:        "node",
			RepoURL:        "https://github.com/AgoraIO-Conversational-AI/agent-quickstart-nextjs",
			DocsURL:        "https://github.com/AgoraIO-Conversational-AI/agent-quickstart-nextjs",
			DetectPaths:    []string{"env.local.example", "app"},
			EnvExamplePath: "env.local.example",
			EnvTargetPath:  ".env.local",
			InstallCommand: "pnpm install",
			RunCommand:     "pnpm dev",
			EnvDocsSummary: "Writes NEXT_PUBLIC_AGORA_APP_ID for the browser and NEXT_AGORA_APP_CERTIFICATE for server-side runtime use.",
			SupportsInit:   true,
			Available:      true,
		},
		{
			ID:             "python",
			Title:          "Conversational AI Python Quickstart",
			Description:    "Clone the official Python conversational AI quickstart.",
			Runtime:        "python",
			RepoURL:        "https://github.com/AgoraIO-Community/agent-quickstart-python",
			DocsURL:        "https://github.com/AgoraIO-Community/agent-quickstart-python",
			DetectPaths:    []string{"server-python/.env.example", "server-python", "web-client"},
			EnvExamplePath: "server-python/.env.example",
			EnvTargetPath:  "server-python/.env.local",
			InstallCommand: "bun install",
			RunCommand:     "bun run dev",
			EnvDocsSummary: "Writes APP_ID and APP_CERTIFICATE into the backend env file under server-python/.",
			SupportsInit:   true,
			Available:      true,
		},
		{
			ID:             "go",
			Title:          "Conversational AI Go Quickstart",
			Description:    "Clone the official Go conversational AI quickstart.",
			Runtime:        "go",
			RepoURL:        "https://github.com/AgoraIO-Conversational-AI/agent-quickstart-go",
			DocsURL:        "https://github.com/AgoraIO-Conversational-AI/agent-quickstart-go",
			DetectPaths:    []string{"server-go/.env.example", "server-go", "web-client"},
			EnvExamplePath: "server-go/.env.example",
			EnvTargetPath:  "server-go/.env.local",
			InstallCommand: "make setup",
			RunCommand:     "make dev",
			EnvDocsSummary: "Writes APP_ID and APP_CERTIFICATE into the backend env file under server-go/.",
			SupportsInit:   true,
			Available:      true,
		},
	}
}

func findQuickstartTemplate(id string) (*quickstartTemplate, bool) {
	for _, template := range quickstartTemplates() {
		if template.ID == id {
			copy := template
			return &copy, true
		}
	}
	return nil, false
}

func (a *App) buildQuickstartCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "quickstart",
		Short: "Clone official standalone Agora quickstarts",
		Long: `Quickstart commands clone official reference applications into a new directory.

Use this group when you want a standalone demo or onboarding project.`,
		Example: strings.TrimSpace(`
  agora quickstart list
  agora quickstart create my-nextjs-demo --template nextjs
  agora quickstart create my-python-demo --template python --project my-agent-demo
  agora quickstart create my-go-demo --template go --project my-agent-demo
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
			}
			return cmd.Help()
		},
	}
	cmd.AddCommand(a.buildQuickstartList())
	cmd.AddCommand(a.buildQuickstartCreate())
	cmd.AddCommand(a.buildQuickstartEnv())
	return cmd
}

func (a *App) buildQuickstartList() *cobra.Command {
	var showAll bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available official quickstarts",
		Long:  "Show the official quickstart templates known to the CLI. By default, only available templates are listed.",
		Example: strings.TrimSpace(`
  agora quickstart list
  agora quickstart list --show-all
  agora quickstart list --json
`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			items := make([]map[string]any, 0, len(quickstartTemplates()))
			for _, template := range quickstartTemplates() {
				if !showAll && !template.Available {
					continue
				}
				items = append(items, map[string]any{
					"available":    template.Available,
					"description":  template.Description,
					"docsUrl":      template.DocsURL,
					"envDocs":      template.EnvDocsSummary,
					"id":           template.ID,
					"repoUrl":      template.RepoURL,
					"runtime":      template.Runtime,
					"supportsInit": template.SupportsInit,
					"title":        template.Title,
				})
			}
			return renderResult(cmd, "quickstart list", map[string]any{
				"action": "list",
				"items":  items,
			})
		},
	}
	cmd.Flags().BoolVar(&showAll, "show-all", false, "include upcoming or unavailable templates in the list")
	return cmd
}

func (a *App) buildQuickstartCreate() *cobra.Command {
	var templateID string
	var dir string
	var project string
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Clone an official Agora quickstart into a new directory",
		Long: `Clone a standalone quickstart repository into a new directory.

If a current project context exists, or if --project is passed, the CLI also writes the quickstart's expected local env file with Agora credentials where supported.`,
		Example: strings.TrimSpace(`
  agora quickstart create my-nextjs-demo --template nextjs
  agora quickstart create my-python-demo --template python --project my-agent-demo
  agora quickstart create my-go-demo --template go --project my-agent-demo
  agora quickstart create demo --template nextjs --dir apps/demo
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New("quickstart name is required")
			}
			template, ok := findQuickstartTemplate(templateID)
			if !ok {
				return fmt.Errorf("unknown quickstart template %q", templateID)
			}
			targetDir := dir
			if strings.TrimSpace(targetDir) == "" {
				targetDir = args[0]
			}
			result, err := a.quickstartCreate(*template, targetDir, project)
			if err != nil {
				return err
			}
			return renderResult(cmd, "quickstart create", result)
		},
	}
	cmd.Flags().StringVar(&templateID, "template", "", "quickstart template ID from `agora quickstart list`")
	cmd.Flags().StringVar(&dir, "dir", "", "target directory for the cloned quickstart; defaults to <name>")
	cmd.Flags().StringVar(&project, "project", "", "project ID or exact project name to use for env seeding")
	_ = cmd.MarkFlagRequired("template")
	return cmd
}

func (a *App) buildQuickstartEnv() *cobra.Command {
	var templateID string
	var project string
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Write framework-specific env files for a quickstart repo",
		Long: `Update the local env file for a cloned quickstart repository.

The CLI can infer the quickstart type from the repository layout, or you can force it with --template.`,
		Example: strings.TrimSpace(`
  agora quickstart env write
  agora quickstart env write apps/my-nextjs-demo
  agora quickstart env write apps/my-python-demo --project my-agent-demo
  agora quickstart env write apps/my-go-demo --project my-agent-demo
  agora quickstart env write . --template nextjs
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
			}
			return cmd.Help()
		},
	}
	write := &cobra.Command{
		Use:   "write [dir]",
		Short: "Write the quickstart env file for the current or selected project",
		Long: `Write the runtime-specific env file expected by a cloned quickstart repository.

Next.js quickstarts receive NEXT_PUBLIC_* client env vars plus server-only Agora credentials.
Python and Go quickstarts receive backend APP_ID and APP_CERTIFICATE values.`,
		Example: strings.TrimSpace(`
  agora quickstart env write
  agora quickstart env write apps/my-nextjs-demo
  agora quickstart env write apps/my-python-demo --project my-agent-demo
  agora quickstart env write apps/my-go-demo --project my-agent-demo
  agora quickstart env write . --template python
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			targetDir := "."
			if len(args) > 0 && strings.TrimSpace(args[0]) != "" {
				targetDir = args[0]
			}
			result, err := a.quickstartEnvWrite(targetDir, templateID, project)
			if err != nil {
				return err
			}
			return renderResult(cmd, "quickstart env write", result)
		},
	}
	write.Flags().StringVar(&templateID, "template", "", "quickstart template ID; if omitted, the CLI detects it from the repo layout")
	write.Flags().StringVar(&project, "project", "", "project ID or exact project name to use for env seeding")
	cmd.AddCommand(write)
	return cmd
}

func (a *App) quickstartCreate(template quickstartTemplate, targetDir, explicitProject string) (map[string]any, error) {
	if !template.Available || strings.TrimSpace(template.RepoURL) == "" {
		return nil, fmt.Errorf("Quickstart template %q is not available yet.", template.ID)
	}
	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return nil, err
	}
	if info, err := os.Stat(absTarget); err == nil {
		return nil, fmt.Errorf("%s already exists. Choose a new target directory.", absTarget)
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, err
	} else if info != nil {
		return nil, fmt.Errorf("%s already exists. Choose a new target directory.", absTarget)
	}

	var boundProject *projectTarget
	if target, ok, err := a.resolveOptionalProjectTarget(explicitProject, ""); err != nil {
		return nil, err
	} else if ok {
		boundProject = &target
	}

	repoURL := a.quickstartRepoURL(template)
	if err := cloneQuickstartRepo(repoURL, absTarget); err != nil {
		return nil, err
	}

	written := []string{".git"}
	envStatus := "template-only"
	envPath := ""
	if boundProject != nil {
		writtenPath, _, err := seedQuickstartEnv(absTarget, template, boundProject.project)
		if err != nil {
			if cleanupErr := os.RemoveAll(absTarget); cleanupErr != nil {
				return nil, fmt.Errorf("failed to configure quickstart env after clone: %v; cleanup also failed for %s: %v", err, absTarget, cleanupErr)
			}
			return nil, fmt.Errorf("failed to configure quickstart env after clone: %v; removed %s", err, absTarget)
		}
		if err := writeLocalProjectBinding(absTarget, localProjectBinding{
			ProjectID:   boundProject.project.ProjectID,
			ProjectName: boundProject.project.Name,
			Region:      boundProject.region,
			Template:    template.ID,
			EnvPath:     writtenPath,
		}); err != nil {
			if cleanupErr := os.RemoveAll(absTarget); cleanupErr != nil {
				return nil, fmt.Errorf("failed to write .agora project metadata after clone: %v; cleanup also failed for %s: %v", err, absTarget, cleanupErr)
			}
			return nil, fmt.Errorf("failed to write .agora project metadata after clone: %v; removed %s", err, absTarget)
		}
		envStatus = "configured"
		envPath = writtenPath
		written = append(written, writtenPath, filepath.ToSlash(filepath.Join(localAgoraDirName, localProjectFileName)))
	}
	sort.Strings(written)

	result := map[string]any{
		"action":       "create",
		"cloneUrl":     repoURL,
		"docsUrl":      template.DocsURL,
		"envPath":      envPath,
		"envStatus":    envStatus,
		"metadataPath": "",
		"path":         absTarget,
		"projectId":    nil,
		"projectName":  nil,
		"runtime":      template.Runtime,
		"status":       "cloned",
		"template":     template.ID,
		"title":        template.Title,
		"written":      written,
	}
	if boundProject != nil {
		result["projectId"] = boundProject.project.ProjectID
		result["projectName"] = boundProject.project.Name
		result["metadataPath"] = filepath.ToSlash(filepath.Join(localAgoraDirName, localProjectFileName))
	}
	return result, nil
}

func (a *App) quickstartEnvWrite(targetDir, templateID, explicitProject string) (map[string]any, error) {
	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(absTarget)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory.", absTarget)
	}

	template, err := resolveQuickstartTemplateForPath(absTarget, templateID)
	if err != nil {
		return nil, err
	}
	target, ok, err := a.resolveOptionalProjectTarget(explicitProject, absTarget)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errNoProjectSelected
	}

	envPath, status, err := seedQuickstartEnv(absTarget, template, target.project)
	if err != nil {
		return nil, err
	}
	if err := writeLocalProjectBinding(absTarget, localProjectBinding{
		ProjectID:   target.project.ProjectID,
		ProjectName: target.project.Name,
		Region:      target.region,
		Template:    template.ID,
		EnvPath:     envPath,
	}); err != nil {
		return nil, err
	}
	return map[string]any{
		"action":       "env-write",
		"envPath":      envPath,
		"metadataPath": filepath.ToSlash(filepath.Join(localAgoraDirName, localProjectFileName)),
		"path":         absTarget,
		"projectId":    target.project.ProjectID,
		"projectName":  target.project.Name,
		"status":       status,
		"template":     template.ID,
		"title":        template.Title,
	}, nil
}

func (a *App) quickstartRepoURL(template quickstartTemplate) string {
	key := "AGORA_QUICKSTART_" + strings.ToUpper(strings.ReplaceAll(template.ID, "-", "_")) + "_REPO_URL"
	if override := strings.TrimSpace(a.env[key]); override != "" {
		return override
	}
	return template.RepoURL
}

func cloneQuickstartRepo(repoURL, targetDir string) error {
	cmd := exec.Command("git", "clone", "--depth", "1", repoURL, targetDir)
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		hint := "Ensure git is installed and you have network access to GitHub."
		if trimmed == "" {
			return fmt.Errorf("git clone failed for %s. %s", repoURL, hint)
		}
		return fmt.Errorf("git clone failed for %s (%s). %s", repoURL, trimmed, hint)
	}
	return nil
}

func (a *App) resolveOptionalProjectTarget(explicitProject, startPath string) (projectTarget, bool, error) {
	if strings.TrimSpace(explicitProject) != "" {
		target, err := a.resolveProjectTargetFrom(explicitProject, startPath)
		return target, true, err
	}
	target, err := a.resolveProjectTargetFrom("", startPath)
	if err != nil {
		if errors.Is(err, errNoProjectSelected) {
			return projectTarget{}, false, nil
		}
		return projectTarget{}, false, err
	}
	return target, true, nil
}

func resolveQuickstartTemplateForPath(root, explicitTemplate string) (quickstartTemplate, error) {
	if strings.TrimSpace(explicitTemplate) != "" {
		template, ok := findQuickstartTemplate(explicitTemplate)
		if !ok {
			return quickstartTemplate{}, fmt.Errorf("unknown quickstart template %q", explicitTemplate)
		}
		return *template, nil
	}
	for _, template := range quickstartTemplates() {
		if matchesQuickstartTemplate(root, template) {
			return template, nil
		}
	}
	var hints []string
	var ids []string
	for _, t := range quickstartTemplates() {
		if len(t.DetectPaths) > 0 {
			hints = append(hints, fmt.Sprintf("%s (%s)", t.ID, t.DetectPaths[0]))
		}
		ids = append(ids, t.ID)
	}
	return quickstartTemplate{}, fmt.Errorf(
		"could not detect the quickstart type from this directory (looked for %s). Pass --template %s to specify explicitly.",
		strings.Join(hints, ", "),
		strings.Join(ids, "|"),
	)
}

func matchesQuickstartTemplate(root string, template quickstartTemplate) bool {
	if len(template.DetectPaths) == 0 {
		return false
	}
	for _, rel := range template.DetectPaths {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
			return true
		}
	}
	return false
}

func seedQuickstartEnv(root string, template quickstartTemplate, project projectDetail) (string, string, error) {
	if template.EnvTargetPath == "" {
		return "", "", fmt.Errorf("Quickstart template %q does not define an env target yet.", template.ID)
	}
	if project.SignKey == nil || *project.SignKey == "" {
		return "", "", fmt.Errorf("project %q does not have an app certificate. Enable one in Agora Console or use a different project with `agora project use`.", project.Name)
	}

	examplePath := filepath.Join(root, filepath.FromSlash(template.EnvExamplePath))
	targetPath := filepath.Join(root, filepath.FromSlash(template.EnvTargetPath))

	exampleContent := ""
	if raw, err := os.ReadFile(examplePath); err == nil {
		exampleContent = string(raw)
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}

	existingContent := ""
	status := "created"
	if raw, err := os.ReadFile(targetPath); err == nil {
		existingContent = string(raw)
		status = "updated"
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", "", err
	}

	content := mergeQuickstartEnvContent(existingContent, exampleContent, renderQuickstartManagedBlock(template, project))
	if err := os.MkdirAll(filepath.Dir(targetPath), 0o755); err != nil {
		return "", "", err
	}
	if err := os.WriteFile(targetPath, []byte(content), 0o644); err != nil {
		return "", "", err
	}
	return filepath.ToSlash(template.EnvTargetPath), status, nil
}

func renderQuickstartManagedBlock(template quickstartTemplate, project projectDetail) string {
	lines := []string{
		"# BEGIN AGORA CLI QUICKSTART",
		"# Project ID: " + project.ProjectID,
		"# Project Name: " + project.Name,
	}
	switch template.ID {
	case "nextjs":
		lines = append(lines,
			"NEXT_PUBLIC_AGORA_APP_ID="+project.AppID,
			"NEXT_AGORA_APP_CERTIFICATE="+renderDotenvScalar(*project.SignKey),
		)
	case "python":
		lines = append(lines,
			"APP_ID="+project.AppID,
			"APP_CERTIFICATE="+renderDotenvScalar(*project.SignKey),
		)
	case "go":
		lines = append(lines,
			"APP_ID="+project.AppID,
			"APP_CERTIFICATE="+renderDotenvScalar(*project.SignKey),
		)
	default:
		lines = append(lines, "# Template-specific env seeding is not available yet.")
	}
	lines = append(lines, "# END AGORA CLI QUICKSTART")
	return strings.Join(lines, "\n")
}

func mergeQuickstartEnvContent(existing, example, block string) string {
	if existing != "" {
		if strings.Contains(existing, "# BEGIN AGORA CLI QUICKSTART") && strings.Contains(existing, "# END AGORA CLI QUICKSTART") {
			return replaceQuickstartManagedBlock(existing, block)
		}
		return strings.TrimRight(existing, "\r\n") + "\n\n" + block + "\n"
	}
	base := strings.TrimRight(example, "\r\n")
	if base == "" {
		return block + "\n"
	}
	return base + "\n\n" + block + "\n"
}

func replaceQuickstartManagedBlock(existing, block string) string {
	start := strings.Index(existing, "# BEGIN AGORA CLI QUICKSTART")
	end := strings.Index(existing, "# END AGORA CLI QUICKSTART")
	if start == -1 || end == -1 {
		return existing
	}
	end += len("# END AGORA CLI QUICKSTART")
	suffix := strings.TrimLeft(existing[end:], "\r\n")
	if suffix == "" {
		return existing[:start] + block + "\n"
	}
	return existing[:start] + block + "\n" + suffix
}
