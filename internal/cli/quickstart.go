package cli

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

type quickstartTemplate struct {
	ID          string
	Title       string
	Description string
	Runtime     string
	RepoURL     string
	// RepoURLCN / DocsURLCN are the cn-region variants. They currently
	// mirror the global URLs because the conversational-AI quickstarts
	// have no China-hosted mirror yet; set them to the cn URL when one
	// exists and quickstartRepoURLForRegion / quickstartDocsURL will pick
	// it up automatically (an empty value falls back to the global URL).
	RepoURLCN       string
	DocsURL         string
	DocsURLCN       string
	EnvLayouts      []quickstartEnvLayout
	InstallCommand  string
	RunCommand      string
	AdditionalSteps []string
	EnvDocsSummary  string
	SupportsInit    bool
	Available       bool
}

// quickstartEnvLayout describes one supported upstream layout for a
// quickstart template. The first layout is the current upstream default;
// later layouts preserve existing scaffolds created by older CLI versions.
type quickstartEnvLayout struct {
	DetectPaths       []string
	EnvExamplePath    string
	EnvTargetPath     string
	AppIDKey          string
	AppCertificateKey string
}

func quickstartTemplates() []quickstartTemplate {
	return []quickstartTemplate{
		{
			ID:          "nextjs",
			Title:       "Conversational AI Next.js Quickstart",
			Description: "Clone the official Next.js conversational AI quickstart.",
			Runtime:     "node",
			RepoURL:     "https://github.com/AgoraIO-Conversational-AI/agent-quickstart-nextjs",
			RepoURLCN:   "https://github.com/AgoraIO-Conversational-AI/agent-quickstart-nextjs",
			DocsURL:     "https://github.com/AgoraIO-Conversational-AI/agent-quickstart-nextjs",
			DocsURLCN:   "https://github.com/AgoraIO-Conversational-AI/agent-quickstart-nextjs",
			EnvLayouts: []quickstartEnvLayout{{
				DetectPaths:       []string{"env.local.example", "app"},
				EnvExamplePath:    "env.local.example",
				EnvTargetPath:     ".env.local",
				AppIDKey:          "NEXT_PUBLIC_AGORA_APP_ID",
				AppCertificateKey: "NEXT_AGORA_APP_CERTIFICATE",
			}},
			InstallCommand: "pnpm install",
			RunCommand:     "pnpm dev",
			EnvDocsSummary: "Writes NEXT_PUBLIC_AGORA_APP_ID for the browser and NEXT_AGORA_APP_CERTIFICATE for server-side runtime use.",
			SupportsInit:   true,
			Available:      true,
		},
		{
			ID:          "python",
			Title:       "Conversational AI Python Quickstart",
			Description: "Clone the official Python conversational AI quickstart.",
			Runtime:     "python",
			RepoURL:     "https://github.com/AgoraIO-Conversational-AI/agent-quickstart-python",
			RepoURLCN:   "https://github.com/AgoraIO-Conversational-AI/agent-quickstart-python",
			DocsURL:     "https://github.com/AgoraIO-Conversational-AI/agent-quickstart-python",
			DocsURLCN:   "https://github.com/AgoraIO-Conversational-AI/agent-quickstart-python",
			EnvLayouts: []quickstartEnvLayout{
				{
					DetectPaths:       []string{"server/requirements.txt"},
					EnvExamplePath:    "server/.env.example",
					EnvTargetPath:     "server/.env",
					AppIDKey:          "AGORA_APP_ID",
					AppCertificateKey: "AGORA_APP_CERTIFICATE",
				},
				{
					DetectPaths:       []string{"server/env.example"},
					EnvExamplePath:    "server/env.example",
					EnvTargetPath:     "server/.env",
					AppIDKey:          "AGORA_APP_ID",
					AppCertificateKey: "AGORA_APP_CERTIFICATE",
				},
			},
			InstallCommand: "bun run setup",
			RunCommand:     "bun run dev",
			EnvDocsSummary: "Copies server/.env.example to server/.env, then writes AGORA_APP_ID and AGORA_APP_CERTIFICATE.",
			SupportsInit:   true,
			Available:      true,
		},
		{
			ID:          "go",
			Title:       "Conversational AI Go Quickstart",
			Description: "Clone the official Go conversational AI quickstart.",
			Runtime:     "go",
			RepoURL:     "https://github.com/AgoraIO-Conversational-AI/agent-quickstart-go",
			RepoURLCN:   "https://github.com/AgoraIO-Conversational-AI/agent-quickstart-go",
			DocsURL:     "https://github.com/AgoraIO-Conversational-AI/agent-quickstart-go",
			DocsURLCN:   "https://github.com/AgoraIO-Conversational-AI/agent-quickstart-go",
			EnvLayouts: []quickstartEnvLayout{
				{
					DetectPaths:       []string{"server/go.mod"},
					EnvExamplePath:    "server/.env.example",
					EnvTargetPath:     "server/.env",
					AppIDKey:          "AGORA_APP_ID",
					AppCertificateKey: "AGORA_APP_CERTIFICATE",
				},
				{
					DetectPaths:       []string{"server-go/env.example"},
					EnvExamplePath:    "server-go/env.example",
					EnvTargetPath:     "server-go/.env",
					AppIDKey:          "AGORA_APP_ID",
					AppCertificateKey: "AGORA_APP_CERTIFICATE",
				},
			},
			InstallCommand: "make setup",
			RunCommand:     "make dev",
			EnvDocsSummary: "Copies server/.env.example to server/.env, then writes AGORA_APP_ID and AGORA_APP_CERTIFICATE.",
			SupportsInit:   true,
			Available:      true,
		},
		{
			ID:          "android",
			Title:       "Conversational AI Android Quickstart",
			Description: "Clone the official Android client and Python server quickstart.",
			Runtime:     "android",
			RepoURL:     "https://github.com/AgoraIO-Conversational-AI/agent-quickstart-android",
			RepoURLCN:   "https://github.com/AgoraIO-Conversational-AI/agent-quickstart-android",
			DocsURL:     "https://github.com/AgoraIO-Conversational-AI/agent-quickstart-android",
			DocsURLCN:   "https://github.com/AgoraIO-Conversational-AI/agent-quickstart-android",
			EnvLayouts: []quickstartEnvLayout{{
				DetectPaths:       []string{"server/.env.example", "server/requirements-dev.txt", "app/src/main/AndroidManifest.xml"},
				EnvExamplePath:    "server/.env.example",
				EnvTargetPath:     "server/.env.local",
				AppIDKey:          "AGORA_APP_ID",
				AppCertificateKey: "AGORA_APP_CERTIFICATE",
			}},
			InstallCommand: "python3 -m venv server/.venv && server/.venv/bin/pip install -r server/requirements-dev.txt",
			RunCommand:     "./server/run.sh",
			AdditionalSteps: []string{
				"./server/tunnel.sh --provider ngrok",
				"./server/configure-android.sh https://your-public-host",
				"./gradlew :app:assembleDebug",
			},
			EnvDocsSummary: "Copies server/.env.example to server/.env.local and writes server-only Agora credentials; configure local.properties later with the public HTTPS server URL.",
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
		Example: example(`
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
	var details bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List available official quickstarts",
		Long:  "Show the official quickstart templates known to the CLI. By default, only available templates are listed.",
		Example: example(`
  agora quickstart list
  agora quickstart list --show-all
  agora quickstart list --details
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
					"docsUrl":      quickstartDocsURL(template, a.authRegion()),
					"envDocs":      template.EnvDocsSummary,
					"id":           template.ID,
					"repoUrl":      quickstartRepoURLForRegion(template, a.authRegion()),
					"runtime":      template.Runtime,
					"supportsInit": template.SupportsInit,
					"title":        template.Title,
				})
			}
			return renderResult(cmd, "quickstart list", map[string]any{
				"action":  "list",
				"items":   items,
				"details": details,
			})
		},
	}
	cmd.Flags().BoolVar(&showAll, "show-all", false, "include upcoming or unavailable templates in the list")
	cmd.Flags().BoolVar(&details, "details", false, "show repository, runtime, and env details in pretty output")
	return cmd
}

func (a *App) buildQuickstartCreate() *cobra.Command {
	var templateID string
	var dir string
	var project string
	var ref string
	var templateOnly bool
	cmd := &cobra.Command{
		Use:   "create <name>",
		Short: "Clone an official Agora quickstart into a new directory",
		Long: `Clone a standalone quickstart repository into a new directory.

If a current project context exists, or if --project is passed, the CLI also writes the quickstart's expected local env file with Agora credentials where supported. Without a resolved project, interactive runs prompt for an existing project; non-interactive runs require --template-only to clone without credentials.`,
		Example: example(`
  agora quickstart create my-nextjs-demo --template nextjs
  agora quickstart create my-python-demo --template python --project my-agent-demo
  agora quickstart create my-go-demo --template go --project my-agent-demo
  agora quickstart create template-source --template python --template-only
  agora quickstart create demo --template nextjs --dir apps/demo
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New("quickstart name is required")
			}
			template, ok := findQuickstartTemplate(templateID)
			if !ok {
				return &cliError{Message: fmt.Sprintf("unknown quickstart template %q. Run `agora quickstart list` to see available templates.", templateID), Code: "QUICKSTART_TEMPLATE_UNKNOWN"}
			}
			targetDir := dir
			if strings.TrimSpace(targetDir) == "" {
				targetDir = args[0]
			}
			promptForProject := !templateOnly &&
				!a.noInput() &&
				a.resolveOutputMode(cmd) != outputJSON &&
				!isCIEnvironment(a.osEnv) &&
				isTTY(os.Stdin)
			progress := jsonProgressFor(a, cmd, "quickstart create")
			result, err := a.quickstartCreate(*template, targetDir, project, templateOnly, promptForProject, cmd.ErrOrStderr(), os.Stdin, ref, progress)
			if err != nil {
				return err
			}
			return renderResult(cmd, "quickstart create", result)
		},
	}
	cmd.Flags().StringVar(&templateID, "template", "", "quickstart template ID from `agora quickstart list`")
	cmd.Flags().StringVar(&dir, "dir", "", "target directory for the cloned quickstart; defaults to <name>")
	cmd.Flags().StringVar(&project, "project", "", "project ID or exact project name to use for env seeding")
	cmd.Flags().StringVar(&ref, "ref", "", "git branch, tag, or ref to clone for pinned workshops")
	cmd.Flags().BoolVar(&templateOnly, "template-only", false, "clone without resolving a project or writing credentials")
	cmd.MarkFlagsMutuallyExclusive("project", "template-only")
	_ = cmd.MarkFlagRequired("template")
	_ = cmd.RegisterFlagCompletionFunc("template", completeQuickstartTemplateIDs)
	_ = cmd.RegisterFlagCompletionFunc("project", a.completeProjectNames)
	return cmd
}

func chooseQuickstartProject(in io.Reader, out io.Writer, items []projectSummary) (projectSummary, string, error) {
	choices := initProjectChoiceItems(items)
	if len(choices) == 0 {
		return projectSummary{}, "none", nil
	}
	reader := bufio.NewReader(in)
	templateOnlyIndex := len(choices) + 1
	cancelIndex := len(choices) + 2
	for {
		if _, err := fmt.Fprintln(out, "Choose an Agora project:"); err != nil {
			return projectSummary{}, "", err
		}
		for index, item := range choices {
			suffix := ""
			if index == len(choices)-1 {
				suffix = " (most recent)"
			}
			if _, err := fmt.Fprintf(out, "  %d. %s (%s)%s\n", index+1, item.Name, item.ProjectID, suffix); err != nil {
				return projectSummary{}, "", err
			}
		}
		if _, err := fmt.Fprintf(out, "  %d. Clone template only\n  %d. Cancel\nProject [%d]: ", templateOnlyIndex, cancelIndex, len(choices)); err != nil {
			return projectSummary{}, "", err
		}
		answer, readErr := reader.ReadString('\n')
		if readErr != nil && !errors.Is(readErr, io.EOF) {
			return projectSummary{}, "", readErr
		}
		trimmed := strings.TrimSpace(answer)
		switch strings.ToLower(trimmed) {
		case "":
			return choices[len(choices)-1], "project", nil
		case "template-only", "template", "t":
			return projectSummary{}, "template-only", nil
		case "cancel", "abort", "q", "quit":
			return projectSummary{}, "abort", nil
		}
		if index, err := strconv.Atoi(trimmed); err == nil {
			switch {
			case index >= 1 && index <= len(choices):
				return choices[index-1], "project", nil
			case index == templateOnlyIndex:
				return projectSummary{}, "template-only", nil
			case index == cancelIndex:
				return projectSummary{}, "abort", nil
			}
		}
		for _, item := range choices {
			if strings.EqualFold(item.ProjectID, trimmed) || strings.EqualFold(item.Name, trimmed) {
				return item, "project", nil
			}
		}
		if _, err := fmt.Fprintf(out, "Please choose 1-%d, enter a project name/id, type template-only, or cancel.\n", cancelIndex); err != nil {
			return projectSummary{}, "", err
		}
		if errors.Is(readErr, io.EOF) {
			return projectSummary{}, "abort", nil
		}
	}
}

func (a *App) buildQuickstartEnv() *cobra.Command {
	var templateID string
	var project string
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Write framework-specific env files for a quickstart repo",
		Long: `Update the local env file for a cloned quickstart repository.

This is the canonical env-write command for official Agora quickstarts. The CLI selects the template-specific path, seeds its example file, normalizes legacy credential names, and updates quickstart metadata.

The CLI can infer the quickstart type from the repository layout, or you can force it with --template. Use ` + "`agora project env write <file>`" + ` instead for an explicit file in a custom repository.`,
		Example: example(`
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
Python and Go quickstarts receive backend AGORA_APP_ID and AGORA_APP_CERTIFICATE values.`,
		Example: example(`
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
	_ = write.RegisterFlagCompletionFunc("template", completeQuickstartTemplateIDs)
	_ = write.RegisterFlagCompletionFunc("project", a.completeProjectNames)
	cmd.AddCommand(write)
	return cmd
}

func (a *App) quickstartCreate(template quickstartTemplate, targetDir, explicitProject string, templateOnly, promptForProject bool, promptOut io.Writer, promptIn io.Reader, ref string, progress progressEmitter) (map[string]any, error) {
	if !template.Available || strings.TrimSpace(template.RepoURL) == "" {
		return nil, &cliError{Message: fmt.Sprintf("Quickstart template %q is not available yet.", template.ID), Code: "QUICKSTART_TEMPLATE_UNAVAILABLE"}
	}
	absTarget, err := resolveScaffoldTarget(targetDir)
	if err != nil {
		return nil, err
	}

	var boundProject *projectTarget
	if !templateOnly {
		if target, ok, err := a.resolveOptionalProjectTarget(explicitProject, ""); err != nil {
			return nil, err
		} else if ok {
			boundProject = &target
		} else if !promptForProject {
			return nil, &cliError{Message: "No project selected. Pass `--project <id-or-name>`, set a current project with `agora project use`, or pass `--template-only` to clone without credentials.", Code: "QUICKSTART_PROJECT_REQUIRED"}
		} else {
			ctx, items, err := a.listInitProjects()
			if err != nil {
				return nil, err
			}
			selected, action, err := chooseQuickstartProject(promptIn, promptOut, items)
			if err != nil {
				return nil, err
			}
			switch action {
			case "none":
				return nil, &cliError{Message: "No Agora projects are available. Use `agora init` or `agora project create` to create one, or pass `--template-only` to clone without credentials.", Code: "QUICKSTART_PROJECT_REQUIRED"}
			case "template-only":
				// Leave boundProject nil so the clone continues without env injection.
			case "abort":
				return nil, &cliError{Message: "Quickstart creation aborted by user.", Code: "QUICKSTART_CREATE_ABORTED"}
			default:
				resolved, err := a.resolveInitProject(ctx, selected)
				if err != nil {
					return nil, err
				}
				boundProject = &resolved
			}
		}
	}

	repoURL, overrideKey, err := a.quickstartRepoURL(template)
	if err != nil {
		return nil, err
	}
	if overrideKey != "" {
		progress.emit("clone:override", fmt.Sprintf("Using repo override from %s", overrideKey), map[string]any{"repoUrl": repoURL, "envVar": overrideKey})
	}
	if err := cloneScaffoldRepo(repoURL, absTarget, ref, progress); err != nil {
		return nil, err
	}

	written := []string{}
	envStatus := "template-only"
	envPath := ""
	if boundProject != nil {
		layout, ok := template.defaultEnvLayout()
		if !ok {
			return nil, &cliError{Message: fmt.Sprintf("Quickstart template %q does not define an env target yet.", template.ID), Code: "QUICKSTART_TEMPLATE_ENV_UNSUPPORTED"}
		}
		writtenPath, _, err := seedQuickstartEnv(absTarget, template, layout, boundProject.project)
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
		"docsUrl":      quickstartDocsURL(template, a.authRegion()),
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
		"nextSteps":    initNextSteps(template, absTarget),
		"ref":          ref,
	}
	if boundProject != nil {
		result["projectId"] = boundProject.project.ProjectID
		result["projectName"] = boundProject.project.Name
		result["metadataPath"] = filepath.ToSlash(filepath.Join(localAgoraDirName, localProjectFileName))
	}
	return result, nil
}

func resolveScaffoldTarget(targetDir string) (string, error) {
	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return "", err
	}
	if _, err := os.Stat(absTarget); err == nil {
		return "", &cliError{Message: fmt.Sprintf("%s already exists. Choose a new target directory.", absTarget), Code: "QUICKSTART_TARGET_EXISTS"}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return absTarget, nil
}

func cloneScaffoldRepo(repoURL, absTarget, ref string, progress progressEmitter) error {
	progress.emit("clone:start", "Cloning scaffold repository", map[string]any{"repoUrl": repoURL, "targetPath": absTarget, "ref": ref})
	if err := cloneQuickstartRepo(repoURL, absTarget, ref); err != nil {
		return err
	}
	progress.emit("clone:complete", "Scaffold repository cloned", map[string]any{"targetPath": absTarget})
	if err := stripClonedGitMetadata(absTarget); err != nil {
		if cleanupErr := os.RemoveAll(absTarget); cleanupErr != nil {
			return fmt.Errorf("failed to remove scaffold git metadata after clone: %v; cleanup also failed for %s: %v", err, absTarget, cleanupErr)
		}
		return fmt.Errorf("failed to remove scaffold git metadata after clone: %v; removed %s", err, absTarget)
	}
	progress.emit("clone:strip-git", "Removed scaffold repository history", map[string]any{"targetPath": absTarget})
	return nil
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

	template, layout, err := resolveQuickstartEnvWriteTarget(absTarget, templateID)
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

	envPath, status, err := seedQuickstartEnv(absTarget, template, layout, target.project)
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

// quickstartRepoOverrideKey returns the env var name that overrides the
// clone URL for a given template. Power users (workshops, internal forks,
// CLI integration tests) set this; everyday users never do.
func quickstartRepoOverrideKey(templateID string) string {
	return "AGORA_QUICKSTART_" + strings.ToUpper(strings.ReplaceAll(templateID, "-", "_")) + "_REPO_URL"
}

// quickstartRepoURLForRegion returns the default quickstart repository URL
// for the active login region, falling back to the global URL when no
// cn-specific repository is configured.
func quickstartRepoURLForRegion(template quickstartTemplate, region string) string {
	if normalizeContextRegion(region) == regionCN && strings.TrimSpace(template.RepoURLCN) != "" {
		return template.RepoURLCN
	}
	return template.RepoURL
}

// quickstartDocsURL returns the default quickstart documentation URL for
// the active login region, falling back to the global URL when no
// cn-specific docs page is configured.
func quickstartDocsURL(template quickstartTemplate, region string) string {
	if normalizeContextRegion(region) == regionCN && strings.TrimSpace(template.DocsURLCN) != "" {
		return template.DocsURLCN
	}
	return template.DocsURL
}

// quickstartRepoURL resolves the clone URL for a template, honoring an
// env override if present. Returns the URL, the env var name that
// supplied the override (empty when none was used), and an error if the
// override is set to a malformed value.
func (a *App) quickstartRepoURL(template quickstartTemplate) (string, string, error) {
	key := quickstartRepoOverrideKey(template.ID)
	if override := strings.TrimSpace(a.env[key]); override != "" {
		if err := validateRepoOverrideURL(override); err != nil {
			return "", "", &cliError{
				Message: fmt.Sprintf("%s is set to an invalid value (%s): %v. Set it to an https://, ssh://, git://, file://, git@host:path, or absolute local path URL.", key, override, err),
				Code:    "QUICKSTART_REPO_OVERRIDE_INVALID",
			}
		}
		return override, key, nil
	}
	return quickstartRepoURLForRegion(template, a.authRegion()), "", nil
}

func stripClonedGitMetadata(targetDir string) error {
	return os.RemoveAll(filepath.Join(targetDir, ".git"))
}

func cloneQuickstartRepo(repoURL, targetDir, ref string) error {
	if err := validateGitRef(ref); err != nil {
		return &cliError{
			Message: fmt.Sprintf("--ref %q is invalid: %v.", ref, err),
			Code:    "QUICKSTART_REF_INVALID",
		}
	}
	if _, err := exec.LookPath("git"); err != nil {
		return &cliError{
			Message: "git was not found on PATH. Install git from https://git-scm.com/downloads and retry.",
			Code:    "QUICKSTART_GIT_MISSING",
		}
	}
	// #nosec G204 -- git is invoked without a shell; repoURL and targetDir
	// follow "--" so git cannot parse them as flags.
	cmd := exec.Command("git", gitQuickstartCloneArgs(repoURL, targetDir, ref)...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		trimmed := strings.TrimSpace(string(output))
		hint := "Check network access to the host and that the ref exists."
		if trimmed == "" {
			return fmt.Errorf("git clone failed for %s. %s", repoURL, hint)
		}
		return fmt.Errorf("git clone failed for %s (%s). %s", repoURL, trimmed, hint)
	}
	return nil
}

// validateGitRef rejects ref values that would either confuse git's
// option parser or are obviously malformed. Empty refs are allowed and
// mean "default branch."
func validateGitRef(ref string) error {
	trimmed := strings.TrimSpace(ref)
	if trimmed == "" {
		return nil
	}
	if strings.HasPrefix(trimmed, "-") {
		return fmt.Errorf("must not start with '-'")
	}
	for _, r := range trimmed {
		if r == ' ' || r == '\t' || r < 0x20 || r == 0x7f {
			return fmt.Errorf("must not contain whitespace or control characters")
		}
	}
	return nil
}

// validateRepoOverrideURL accepts the URL forms git itself accepts for
// remotes plus absolute local paths used by the integration tests. It
// rejects values that begin with '-' to keep them from being parsed as
// git options even before "--" terminates argv.
func validateRepoOverrideURL(s string) error {
	if s == "" {
		return fmt.Errorf("must not be empty")
	}
	if strings.HasPrefix(s, "-") {
		return fmt.Errorf("must not start with '-'")
	}
	for _, scheme := range []string{"http://", "https://", "ssh://", "git://", "file://"} {
		if strings.HasPrefix(s, scheme) {
			return nil
		}
	}
	if at := strings.Index(s, "@"); at > 0 {
		if strings.Contains(s[at+1:], ":") {
			return nil
		}
	}
	if filepath.IsAbs(s) {
		return nil
	}
	return fmt.Errorf("unrecognized URL form")
}

func gitQuickstartCloneArgs(repoURL, targetDir, ref string) []string {
	// Disable credential helpers for this invocation so non-TTY agent/CI runs
	// do not consult macOS keychain for public HTTPS repos.
	args := []string{"-c", "credential.helper=", "clone", "--depth", "1"}
	if strings.TrimSpace(ref) != "" {
		args = append(args, "--branch", strings.TrimSpace(ref))
	}
	// "--" stops git from treating repoURL/targetDir as flags if they start with "-".
	args = append(args, "--", repoURL, targetDir)
	return args
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
			return quickstartTemplate{}, &cliError{Message: fmt.Sprintf("unknown quickstart template %q. Run `agora quickstart list` to see available templates.", explicitTemplate), Code: "QUICKSTART_TEMPLATE_UNKNOWN"}
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
		if layout, ok := t.defaultEnvLayout(); ok && len(layout.DetectPaths) > 0 {
			hints = append(hints, fmt.Sprintf("%s (%s)", t.ID, layout.DetectPaths[0]))
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
	_, ok := quickstartEnvLayoutForPath(root, template)
	return ok
}

func (template quickstartTemplate) defaultEnvLayout() (quickstartEnvLayout, bool) {
	if len(template.EnvLayouts) == 0 {
		return quickstartEnvLayout{}, false
	}
	return template.EnvLayouts[0], true
}

func quickstartEnvLayoutForPath(root string, template quickstartTemplate) (quickstartEnvLayout, bool) {
	for _, layout := range template.EnvLayouts {
		if matchesQuickstartEnvLayout(root, layout) {
			return layout, true
		}
	}
	return quickstartEnvLayout{}, false
}

func matchesQuickstartEnvLayout(root string, layout quickstartEnvLayout) bool {
	if len(layout.DetectPaths) == 0 {
		return false
	}
	for _, rel := range layout.DetectPaths {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err == nil {
			continue
		}
		return false
	}
	return true
}

func quickstartEnvLayoutForEnvPath(template quickstartTemplate, envPath string) (quickstartEnvLayout, bool) {
	wanted := filepath.ToSlash(filepath.Clean(envPath))
	for _, layout := range template.EnvLayouts {
		if filepath.ToSlash(filepath.Clean(layout.EnvTargetPath)) == wanted {
			return layout, true
		}
	}
	return quickstartEnvLayout{}, false
}

func resolveQuickstartEnvWriteTarget(root, explicitTemplate string) (quickstartTemplate, quickstartEnvLayout, error) {
	binding, foundBinding, bindingRoot, err := detectLocalProjectBindingFrom(root)
	if err != nil {
		return quickstartTemplate{}, quickstartEnvLayout{}, err
	}

	var template quickstartTemplate
	if strings.TrimSpace(explicitTemplate) == "" && foundBinding && bindingRoot == root && strings.TrimSpace(binding.Template) != "" {
		found, ok := findQuickstartTemplate(binding.Template)
		if !ok {
			return quickstartTemplate{}, quickstartEnvLayout{}, &cliError{Message: fmt.Sprintf("unknown quickstart template %q. Run `agora quickstart list` to see available templates.", binding.Template), Code: "QUICKSTART_TEMPLATE_UNKNOWN"}
		}
		template = *found
	} else {
		resolved, resolveErr := resolveQuickstartTemplateForPath(root, explicitTemplate)
		if resolveErr != nil {
			return quickstartTemplate{}, quickstartEnvLayout{}, resolveErr
		}
		template = resolved
	}

	if foundBinding && bindingRoot == root && strings.TrimSpace(binding.EnvPath) != "" {
		if layout, ok := quickstartEnvLayoutForEnvPath(template, binding.EnvPath); ok {
			return template, layout, nil
		}
	}
	if layout, ok := quickstartEnvLayoutForPath(root, template); ok {
		return template, layout, nil
	}
	layout, ok := template.defaultEnvLayout()
	if !ok {
		return quickstartTemplate{}, quickstartEnvLayout{}, &cliError{Message: fmt.Sprintf("Quickstart template %q does not define an env target yet.", template.ID), Code: "QUICKSTART_TEMPLATE_ENV_UNSUPPORTED"}
	}
	return template, layout, nil
}

func seedQuickstartEnv(root string, template quickstartTemplate, layout quickstartEnvLayout, project projectDetail) (string, string, error) {
	if layout.EnvTargetPath == "" {
		return "", "", &cliError{Message: fmt.Sprintf("Quickstart template %q does not define an env target yet.", template.ID), Code: "QUICKSTART_TEMPLATE_ENV_UNSUPPORTED"}
	}
	targetPath := filepath.Join(root, filepath.FromSlash(layout.EnvTargetPath))
	examplePath := ""
	if layout.EnvExamplePath != "" {
		examplePath = filepath.Join(root, filepath.FromSlash(layout.EnvExamplePath))
	}
	written, err := writeCredentialEnv(credentialEnvTarget{
		Path:              targetPath,
		ExamplePath:       examplePath,
		AppIDKey:          layout.AppIDKey,
		AppCertificateKey: layout.AppCertificateKey,
	}, project, credentialEnvWriteOptions{AllowAppend: true})
	if err != nil {
		return "", "", err
	}
	return filepath.ToSlash(layout.EnvTargetPath), written.Status, nil
}
