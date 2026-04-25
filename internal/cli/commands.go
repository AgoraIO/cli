package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

func (a *App) buildRoot() *cobra.Command {
	root := &cobra.Command{
		Use:   "agora",
		Short: "Manage Agora auth, projects, quickstarts, and developer workflows",
		Long: `Agora CLI manages three distinct workflows:

  auth        Authenticate this machine with Agora Console
  project     Manage remote Agora project resources and env values
  quickstart  Clone official standalone quickstart repositories
  init        Create a project and quickstart in one onboarding flow

Use "agora init" for the fastest path to a runnable demo.
Use "agora --help --all" to inspect the full command tree, including advanced low-level commands.`,
		Example: strings.TrimSpace(`
  agora login
  agora init my-nextjs-demo --template nextjs
  agora init my-python-demo --template python
  agora init my-go-demo --template go
  agora project doctor --json
  agora --help --all
`),
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			mode := a.resolveOutputMode(cmd)
			ctx := context.WithValue(cmd.Context(), contextKeyOutputMode{}, mode)
			cmd.SetContext(ctx)
			return nil
		},
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.Version = version
	root.PersistentFlags().StringVar(&a.rootOutput, "output", "", "output mode for command results: pretty or json")
	root.PersistentFlags().BoolVar(&a.rootJSON, "json", false, "shortcut for --output json")
	root.PersistentFlags().Bool("all", false, "show the full command tree in help output")
	root.AddCommand(a.buildLoginCommand("login"))
	root.AddCommand(a.buildLogoutCommand("logout"))
	root.AddCommand(a.buildWhoAmICommand())
	root.AddCommand(a.buildAuthCommand())
	root.AddCommand(a.buildConfigCommand())
	root.AddCommand(a.buildProjectCommand())
	root.AddCommand(a.buildQuickstartCommand())
	root.AddCommand(a.buildInitCommand())
	root.AddCommand(a.buildAddCommand())
	defaultHelp := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		defaultHelp(cmd, args)
		if !showAllHelp(cmd) {
			return
		}
		fmt.Fprintln(cmd.OutOrStdout())
		fmt.Fprintln(cmd.OutOrStdout(), "Full Command Tree")
		for _, line := range allCommandPaths(cmd.Root()) {
			fmt.Fprintf(cmd.OutOrStdout(), "- %s\n", line)
		}
	})
	return root
}

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

func allCommandPaths(root *cobra.Command) []string {
	paths := []string{}
	var walk func(*cobra.Command)
	walk = func(cmd *cobra.Command) {
		for _, child := range cmd.Commands() {
			if child.Name() == "help" || child.Name() == "completion" {
				continue
			}
			paths = append(paths, child.CommandPath())
			walk(child)
		}
	}
	walk(root)
	sort.Strings(paths)
	return paths
}

func (a *App) buildLoginCommand(use string) *cobra.Command {
	var noBrowser bool
	var region string
	cmd := &cobra.Command{
		Use:   use,
		Short: "Authenticate with Agora Console",
		Long:  "Open an OAuth login flow in the browser and store the local Agora session for future CLI commands.",
		Example: strings.TrimSpace(`
  agora login
  agora login --no-browser
  agora login --region cn
`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := a.login(noBrowser, region)
			if err != nil {
				return err
			}
			return renderResult(cmd, "login", data)
		},
	}
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "print the login URL instead of auto-opening a browser")
	cmd.Flags().StringVar(&region, "region", "", "control plane region for login defaults (global or cn)")
	return cmd
}

func (a *App) buildLogoutCommand(use string) *cobra.Command {
	return &cobra.Command{
		Use:   use,
		Short: "Clear the local Agora session",
		Long:  "Remove the persisted local session without touching remote Agora resources.",
		Example: strings.TrimSpace(`
  agora logout
  agora auth logout
`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := a.logout()
			if err != nil {
				return err
			}
			return renderResult(cmd, "logout", data)
		},
	}
}

func (a *App) buildWhoAmICommand() *cobra.Command {
	return &cobra.Command{
		Use:     "whoami",
		Aliases: []string{"status"},
		Short:   "Show the current auth status",
		Long:    "Display whether the CLI is authenticated and which scope and session expiry are currently active.",
		Example: strings.TrimSpace(`
  agora whoami
  agora whoami --json
`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := a.authStatus()
			if err != nil {
				return err
			}
			if auth, ok := data["authenticated"].(bool); ok && !auth {
				cmd.SetContext(context.WithValue(cmd.Context(), exitCodeKey{}, 3))
			}
			if err := renderResult(cmd, "auth status", data); err != nil {
				return err
			}
			return exitIfNeeded(cmd)
		},
	}
}

func (a *App) buildAuthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage Agora authentication",
		Long:  "Authentication helpers for logging in, logging out, and inspecting the current local session.",
		Example: strings.TrimSpace(`
  agora auth login
  agora auth status
  agora auth logout
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
			}
			return cmd.Help()
		},
	}
	cmd.AddCommand(a.buildLoginCommand("login"))
	cmd.AddCommand(a.buildLogoutCommand("logout"))
	cmd.AddCommand(&cobra.Command{
		Use:     "status",
		Aliases: []string{"whoami"},
		Short:   "Show the current auth status",
		Long:    "Display whether the CLI is authenticated and which scope and session expiry are currently active.",
		Example: strings.TrimSpace(`
  agora auth status
  agora auth status --json
`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := a.authStatus()
			if err != nil {
				return err
			}
			if auth, ok := data["authenticated"].(bool); ok && !auth {
				cmd.SetContext(context.WithValue(cmd.Context(), exitCodeKey{}, 3))
			}
			if err := renderResult(cmd, "auth status", data); err != nil {
				return err
			}
			return exitIfNeeded(cmd)
		},
	})
	return cmd
}

func (a *App) buildConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage persisted Agora CLI defaults",
		Long:  "Read and update local CLI defaults such as API endpoints, output mode, log level, and browser behavior.",
		Example: strings.TrimSpace(`
  agora config path
  agora config get
  agora config update --output json --log-level debug
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
			}
			return cmd.Help()
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:     "path",
		Short:   "Show the config file path",
		Long:    "Print the path to the persisted Agora CLI config file on this machine.",
		Example: "  agora config path",
		RunE: func(cmd *cobra.Command, _ []string) error {
			path, err := resolveConfigFilePath(a.env)
			if err != nil {
				return err
			}
			return renderResult(cmd, "config path", map[string]any{"path": path})
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:     "get",
		Short:   "Read persisted CLI defaults",
		Long:    "Print the currently stored CLI defaults after config file loading and migration.",
		Example: "  agora config get",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return renderResult(cmd, "config get", a.cfg)
		},
	})
	var cfg appConfig
	cfg = a.cfg
	var telemetryEnabled, browserAutoOpen, verbose string
	update := &cobra.Command{
		Use:   "update",
		Short: "Update persisted CLI defaults",
		Long:  "Write new default values to the local Agora CLI config file. Environment variables still take precedence at runtime.",
		Example: strings.TrimSpace(`
  agora config update --output json
  agora config update --browser-auto-open false
  agora config update --api-base-url https://agora-cli.agora.io
`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			next := a.cfg
			if cmd.Flags().Changed("api-base-url") {
				next.APIBaseURL = cfg.APIBaseURL
			}
			if cmd.Flags().Changed("oauth-base-url") {
				next.OAuthBaseURL = cfg.OAuthBaseURL
			}
			if cmd.Flags().Changed("oauth-client-id") {
				next.OAuthClientID = cfg.OAuthClientID
			}
			if cmd.Flags().Changed("oauth-scope") {
				next.OAuthScope = cfg.OAuthScope
			}
			if cmd.Flags().Changed("telemetry-enabled") {
				value, err := parseBooleanString(telemetryEnabled, "--telemetry-enabled")
				if err != nil {
					return err
				}
				next.TelemetryEnabled = value
			}
			if cmd.Flags().Changed("browser-auto-open") {
				value, err := parseBooleanString(browserAutoOpen, "--browser-auto-open")
				if err != nil {
					return err
				}
				next.BrowserAutoOpen = value
			}
			if cmd.Flags().Changed("log-level") {
				next.LogLevel = cfg.LogLevel
			}
			if cmd.Flags().Changed("verbose") {
				value, err := parseBooleanString(verbose, "--verbose")
				if err != nil {
					return err
				}
				next.Verbose = value
			}
			if cmd.Flags().Changed("output") {
				next.Output = outputMode(cfg.Output)
			}
			path, err := resolveConfigFilePath(a.env)
			if err != nil {
				return err
			}
			if err := writeSecureJSON(path, next); err != nil {
				return err
			}
			a.cfg = next
			a.applyConfigToEnv()
			return renderResult(cmd, "config update", next)
		},
	}
	update.Flags().StringVar(&cfg.APIBaseURL, "api-base-url", cfg.APIBaseURL, "default CLI API base URL")
	update.Flags().StringVar(&cfg.OAuthBaseURL, "oauth-base-url", cfg.OAuthBaseURL, "default OAuth base URL")
	update.Flags().StringVar(&cfg.OAuthClientID, "oauth-client-id", cfg.OAuthClientID, "default OAuth client ID")
	update.Flags().StringVar(&cfg.OAuthScope, "oauth-scope", cfg.OAuthScope, "default OAuth scope")
	update.Flags().StringVar(&telemetryEnabled, "telemetry-enabled", "", "persist telemetry preference (true or false)")
	update.Flags().StringVar(&browserAutoOpen, "browser-auto-open", "", "persist browser auto-open preference (true or false)")
	update.Flags().StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "persist default log level")
	update.Flags().StringVar(&verbose, "verbose", "", "persist verbose logging preference (true or false)")
	update.Flags().Var(newOutputModeValue((*string)(&cfg.Output)), "output", "persist default output mode (pretty or json)")
	cmd.AddCommand(update)
	return cmd
}

func (a *App) buildProjectCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "project",
		Short: "Manage remote Agora project resources",
		Long: `Project commands work against remote Agora Console resources.

Use this group to create projects, switch the current project context, inspect feature state, and export project env values.
These commands do not clone local application code. Use "agora quickstart" for standalone starter repos or "agora init" for the recommended end-to-end onboarding flow.`,
		Example: strings.TrimSpace(`
  agora project create my-agent-demo --feature rtc --feature convoai
  agora project list
  agora project use my-agent-demo
  agora project show
  agora project env write .env.local
  agora project doctor --deep
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
			}
			return cmd.Help()
		},
	}
	cmd.AddCommand(a.buildProjectCreate())
	cmd.AddCommand(a.buildProjectList())
	cmd.AddCommand(a.buildProjectUse())
	cmd.AddCommand(a.buildProjectShow())
	cmd.AddCommand(a.buildProjectEnv())
	cmd.AddCommand(a.buildProjectFeature())
	cmd.AddCommand(a.buildProjectDoctor())
	return cmd
}

func (a *App) buildAddCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "add",
		Short: "Integrate Agora features into an existing codebase",
		Long: `The add command group is reserved for future in-place integrations into an existing application.

Use "agora quickstart create" when you want a standalone starter repo.
Use "agora init" when you want the recommended end-to-end onboarding flow.
Use "agora project" when you want to manage remote Agora resources.`,
		Example: strings.TrimSpace(`
  agora add --help
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
			}
			return cmd.Help()
		},
	}
	cmd.Hidden = true
	return cmd
}

func (a *App) buildProjectCreate() *cobra.Command {
	var region, template string
	var features []string
	cmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a new remote Agora project",
		Long:  "Create a new Agora project resource in the selected control-plane region and optionally enable features after creation.",
		Example: strings.TrimSpace(`
  agora project create my-app
  agora project create my-agent-demo --region global --feature rtc --feature convoai
  agora project create my-voice-agent --template voice-agent
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				return errors.New("Project name is required for agora-cli 0.1.3.")
			}
			data, err := a.projectCreate(args[0], region, template, features)
			if err != nil {
				return err
			}
			return renderResult(cmd, "project create", data)
		},
	}
	cmd.Flags().StringVar(&region, "region", "", "control plane region for the project context (global or cn)")
	cmd.Flags().StringVar(&template, "template", "", "apply a higher-level project preset such as voice-agent")
	cmd.Flags().StringArrayVar(&features, "feature", nil, "enable one or more features after creation")
	return cmd
}

func (a *App) buildProjectList() *cobra.Command {
	var page, pageSize int
	var keyword string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List projects available to the current account",
		Long:  "List remote Agora projects visible to the authenticated account, with optional filtering and pagination.",
		Example: strings.TrimSpace(`
  agora project list
  agora project list --keyword demo
  agora project list --page 2 --page-size 50
`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			res, err := a.listProjects(keyword, page, pageSize)
			if err != nil {
				return err
			}
			return renderResult(cmd, "project list", map[string]any{"items": res.Items, "page": res.Page, "pageSize": res.PageSize, "total": res.Total})
		},
	}
	cmd.Flags().IntVar(&page, "page", 1, "page number to request")
	cmd.Flags().IntVar(&pageSize, "page-size", 20, "number of projects per page")
	cmd.Flags().StringVar(&keyword, "keyword", "", "filter by exact or partial project name or project ID")
	return cmd
}

func (a *App) buildProjectUse() *cobra.Command {
	return &cobra.Command{
		Use:   "use <project>",
		Short: "Set the current project context",
		Long:  "Select the default project used by commands such as project show, project env, project feature, and quickstart env seeding.",
		Example: strings.TrimSpace(`
  agora project use my-agent-demo
  agora project use prj_123456
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) != 1 {
				return errors.New("project id or exact project name is required")
			}
			data, err := a.projectUse(args[0])
			if err != nil {
				return err
			}
			return renderResult(cmd, "project use", data)
		},
	}
}

func (a *App) buildProjectShow() *cobra.Command {
	return &cobra.Command{
		Use:   "show [project]",
		Short: "Show one project",
		Long:  "Display details for the current project or for a project provided explicitly by name or ID.",
		Example: strings.TrimSpace(`
  agora project show
  agora project show my-agent-demo
  agora project show prj_123456 --json
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			project := ""
			if len(args) > 0 {
				project = args[0]
			}
			data, err := a.projectShow(project)
			if err != nil {
				return err
			}
			return renderResult(cmd, "project show", data)
		},
	}
}

func (a *App) buildProjectEnv() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Export project environment variables",
		Long: `Render environment variables for a project in dotenv, shell, or JSON form.

Use "project env write" when you want to persist the values into a managed dotenv file.`,
		Example: strings.TrimSpace(`
  agora project env
  agora project env --shell
  agora project env --with-secrets --json
  agora project env --project my-agent-demo
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
			}
			format, err := resolveProjectEnvOutputFormat(a.projectEnvFormat, a.projectEnvShell, a.resolveOutputMode(cmd))
			if err != nil {
				return err
			}
			values, err := a.projectEnvValues(a.projectEnvProject, a.projectEnvSecrets)
			if err != nil {
				return err
			}
			_, err = fmt.Fprint(os.Stdout, renderProjectEnv(values, format))
			return err
		},
	}
	cmd.Flags().StringVar(&a.projectEnvProject, "project", "", "project ID or exact project name; defaults to the current project context")
	cmd.Flags().StringVar(&a.projectEnvFormat, "format", "", "output format: dotenv or shell; use --json for JSON output")
	cmd.Flags().BoolVar(&a.projectEnvShell, "shell", false, "render shell export statements instead of dotenv lines")
	cmd.Flags().BoolVar(&a.projectEnvSecrets, "with-secrets", false, "include sensitive values such as the app certificate")
	write := &cobra.Command{
		Use:   "write [path]",
		Short: "Write project environment variables to a dotenv file",
		Long: `Write a managed Agora CLI env block to a dotenv file.

If no path is provided, the CLI chooses the default target using the existing env files in the working directory.`,
		Example: strings.TrimSpace(`
  agora project env write
  agora project env write .env.local
  agora project env write apps/web/.env.local --overwrite
  agora project env write .env --append --project my-agent-demo
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if cmd.Flags().Changed("append") && cmd.Flags().Changed("overwrite") {
				appendFlag, _ := cmd.Flags().GetBool("append")
				overwriteFlag, _ := cmd.Flags().GetBool("overwrite")
				if appendFlag && overwriteFlag {
					return errors.New("`--append` and `--overwrite` cannot be used together.")
				}
			}
			path := ""
			if len(args) > 0 {
				path = args[0]
			}
			appendFlag, _ := cmd.Flags().GetBool("append")
			overwriteFlag, _ := cmd.Flags().GetBool("overwrite")
			values, err := a.projectEnvValues(a.projectEnvProject, a.projectEnvSecrets)
			if err != nil {
				return err
			}
			target, err := a.resolveProjectTarget(a.projectEnvProject)
			if err != nil {
				return err
			}
			file, err := writeProjectEnvFile(path, values, appendFlag, overwriteFlag)
			if err != nil {
				return err
			}
			return renderResult(cmd, "project env write", map[string]any{"action": "env-write", "keysWritten": projectEnvKeys(values), "path": file.Path, "projectId": target.project.ProjectID, "projectName": target.project.Name, "status": file.Status})
		},
	}
	write.Flags().Bool("overwrite", false, "replace the target file with a fresh managed Agora block")
	write.Flags().Bool("append", false, "append a managed Agora block to the target file when no managed block exists")
	cmd.AddCommand(write)
	return cmd
}

func (a *App) buildProjectFeature() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "feature",
		Short: "Manage project feature state",
		Long:  "Inspect and enable product features such as rtc, rtm, and convoai for a remote Agora project.",
		Example: strings.TrimSpace(`
  agora project feature list
  agora project feature status convoai
  agora project feature enable rtm my-agent-demo
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
			}
			return cmd.Help()
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "list [project]",
		Short: "List feature status for a project",
		Example: strings.TrimSpace(`
  agora project feature list
  agora project feature list my-agent-demo
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			project := ""
			if len(args) > 0 {
				project = args[0]
			}
			target, err := a.resolveProjectTarget(project)
			if err != nil {
				return err
			}
			items, err := a.listProjectFeatures(target.project, target.region)
			if err != nil {
				return err
			}
			return renderResult(cmd, "project feature list", map[string]any{"action": "feature-list", "items": items, "projectId": target.project.ProjectID, "projectName": target.project.Name})
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "status <feature> [project]",
		Short: "Show one feature status",
		Example: strings.TrimSpace(`
  agora project feature status convoai
  agora project feature status rtm my-agent-demo
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("feature name is required")
			}
			project := ""
			if len(args) > 1 {
				project = args[1]
			}
			data, err := a.projectFeatureStatus(args[0], project)
			if err != nil {
				return err
			}
			return renderResult(cmd, "project feature status", data)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "enable <feature> [project]",
		Short: "Enable one feature for a project",
		Example: strings.TrimSpace(`
  agora project feature enable convoai
  agora project feature enable rtm my-agent-demo
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return errors.New("feature name is required")
			}
			project := ""
			if len(args) > 1 {
				project = args[1]
			}
			data, err := a.projectFeatureEnable(args[0], project)
			if err != nil {
				return err
			}
			return renderResult(cmd, "project feature enable", data)
		},
	})
	return cmd
}

func (a *App) buildProjectDoctor() *cobra.Command {
	var deep bool
	cmd := &cobra.Command{
		Use:   "doctor [project]",
		Short: "Diagnose whether a project is ready for ConvoAI development",
		Long: `Run a readiness check for a project, including auth state, project context, and required feature configuration.

Exit codes:
  0  healthy
  1  blocking project issues
  2  warnings
  3  auth or session issues`,
		Example: strings.TrimSpace(`
  agora project doctor
  agora project doctor --deep
  agora project doctor my-agent-demo --json
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			project := ""
			if len(args) > 0 {
				project = args[0]
			}
			result := a.projectDoctor(project, deep)
			if err := renderResult(cmd, "project doctor", result); err != nil {
				return err
			}
			code := 0
			switch result.Status {
			case "auth_error":
				code = 3
			case "not_ready":
				code = 1
			case "warning":
				code = 2
			}
			if code != 0 {
				return &exitError{code: code}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&deep, "deep", false, "run deeper preflight checks when supported")
	cmd.Flags().String("feature", "convoai", "target feature to evaluate")
	cmd.Flags().Bool("fix", false, "reserved for future safe automatic fixes")
	cmd.Flags().Bool("verbose", false, "reserved for future verbose pretty output")
	return cmd
}
