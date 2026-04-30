package cli

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
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
Use "agora --help --all" to inspect the full command tree with descriptions and flags.
Use "agora --help --all --json" for a machine-readable command tree (agent tooling).`,
		Example: example(`
  agora login
  agora whoami
  agora logout
  agora init my-nextjs-demo --template nextjs
  agora init my-python-demo --template python
  agora init my-go-demo --template go
  agora project doctor --json
  agora --help --all
  agora --help --all --json
`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if a.rootUpgradeCheck {
				provenance := detectInstallProvenance(a.env)
				return renderResult(cmd, "upgrade check", map[string]any{
					"action":         "upgrade-check",
					"command":        provenance.UpgradeCommand,
					"installMethod":  provenance.Method,
					"installSource":  provenance.Source,
					"installedPath":  provenance.InstalledPath,
					"status":         "manual",
					"upgradeCommand": provenance.UpgradeCommand,
					"version":        versionInfo(),
				})
			}
			return cmd.Help()
		},
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			mode := a.resolveOutputMode(cmd)
			// --verbose mirrors AGORA_VERBOSE=1: echoes structured logs to
			// stderr in addition to writing them to the log file. Setting
			// it on a.env is enough because appendAppLog reads from there.
			if a.rootVerbose {
				a.env["AGORA_VERBOSE"] = "1"
			}
			ctx := context.WithValue(cmd.Context(), contextKeyOutputMode{}, mode)
			ctx = context.WithValue(ctx, contextKeyJSONPretty{}, a.rootPrettyJSON)
			ctx = context.WithValue(ctx, contextKeyQuiet{}, a.rootQuiet)
			ctx = context.WithValue(ctx, contextKeyNoColor{}, a.rootNoColor || strings.TrimSpace(a.env["NO_COLOR"]) != "")
			cmd.SetContext(ctx)
			return nil
		},
	}
	root.Version = formattedVersion()
	root.PersistentFlags().StringVar(&a.rootOutput, "output", "", "output mode for command results: pretty or json")
	root.PersistentFlags().BoolVar(&a.rootJSON, "json", false, "shortcut for --output json")
	root.PersistentFlags().BoolVar(&a.rootPrettyJSON, "pretty", false, "pretty-print JSON output when used with --json")
	root.PersistentFlags().BoolVar(&a.rootQuiet, "quiet", false, "suppress success output (both pretty and JSON envelopes); rely on exit code. Errors still print on stderr.")
	root.PersistentFlags().BoolVar(&a.rootNoColor, "no-color", false, "disable ANSI color in pretty output")
	root.PersistentFlags().BoolVarP(&a.rootVerbose, "verbose", "v", false, "echo structured logs to stderr (equivalent to AGORA_VERBOSE=1); does not change exit codes or JSON envelopes")
	root.PersistentFlags().BoolVarP(&a.rootYes, "yes", "y", false, "accept default answers and suppress interactive prompts (equivalent to AGORA_NO_INPUT=1)")
	root.PersistentFlags().Bool("all", false, "show the full command tree in help output")
	root.PersistentFlags().BoolVar(&a.rootUpgradeCheck, "upgrade-check", false, "print non-interactive upgrade guidance and exit")
	root.AddCommand(a.buildLoginCommand("login"))
	root.AddCommand(a.buildLogoutCommand("logout"))
	root.AddCommand(a.buildWhoAmICommand())
	root.AddCommand(a.buildAuthCommand())
	root.AddCommand(a.buildConfigCommand())
	root.AddCommand(a.buildProjectCommand())
	root.AddCommand(a.buildQuickstartCommand())
	root.AddCommand(a.buildInitCommand())
	root.AddCommand(a.buildVersionCommand())
	root.AddCommand(a.buildIntrospectCommand())
	root.AddCommand(a.buildTelemetryCommand())
	root.AddCommand(a.buildUpgradeCommand())
	root.AddCommand(a.buildOpenCommand())
	root.AddCommand(a.buildMCPCommand())
	// Keep "add" unregistered until it has a real implementation. Calls to
	// `agora add` should behave as unknown command for now.
	defaultHelp := root.HelpFunc()
	root.SetHelpFunc(func(cmd *cobra.Command, args []string) {
		if showAllHelp(cmd) && a.resolveOutputMode(cmd) == outputJSON {
			// Unified discovery: --help --all --json emits the same envelope as
			// `agora introspect --json` so agents only handle one schema.
			_ = emitEnvelope(cmd.OutOrStdout(), "introspect", buildIntrospectionData(cmd.Root()), jsonPrettyFromContext(cmd))
			return
		}
		defaultHelp(cmd, args)
		if !showAllHelp(cmd) {
			return
		}
		fmt.Fprintln(cmd.OutOrStdout())
		fmt.Fprintln(cmd.OutOrStdout(), "Full Command Tree")
		for _, info := range buildCommandTree(cmd.Root()) {
			fmt.Fprintf(cmd.OutOrStdout(), "\n  %s\n", info.Path)
			if info.Short != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "    %s\n", info.Short)
			}
			for _, f := range info.Flags {
				if f.Default != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "    --%s %s  %s (default: %s)\n", f.Name, f.Type, f.Usage, f.Default)
				} else {
					fmt.Fprintf(cmd.OutOrStdout(), "    --%s %s  %s\n", f.Name, f.Type, f.Usage)
				}
			}
		}
	})
	return root
}

// example trims leading/trailing newlines off a multi-line raw-string
// example so it slots cleanly into Cobra's Example field.
func example(value string) string {
	return strings.Trim(value, "\n")
}

func (a *App) buildVersionCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Show Agora CLI build information",
		Example: example(`
  agora version
  agora version --json
`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return renderResult(cmd, "version", versionInfo())
		},
	}
}

func (a *App) buildTelemetryCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "telemetry",
		Short: "Inspect or update telemetry preferences",
		Long:  "Telemetry is limited to operational diagnostics and never includes tokens or app certificates. DO_NOT_TRACK=1 disables telemetry at runtime.",
		Example: example(`
  agora telemetry status
  agora telemetry disable
  agora telemetry enable --json
`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.renderTelemetry(cmd)
		},
	}
	cmd.AddCommand(&cobra.Command{
		Use:   "status",
		Short: "Show telemetry status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.renderTelemetry(cmd)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "enable",
		Short: "Enable telemetry",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.setTelemetry(cmd, true)
		},
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "disable",
		Short: "Disable telemetry",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return a.setTelemetry(cmd, false)
		},
	})
	return cmd
}

func (a *App) renderTelemetry(cmd *cobra.Command) error {
	path, err := resolveConfigFilePath(a.env)
	if err != nil {
		return err
	}
	return renderResult(cmd, "telemetry", map[string]any{
		"action":     "status",
		"configPath": path,
		"doNotTrack": strings.TrimSpace(a.env["DO_NOT_TRACK"]) != "",
		"enabled":    a.cfg.TelemetryEnabled && strings.TrimSpace(a.env["DO_NOT_TRACK"]) == "",
	})
}

func (a *App) setTelemetry(cmd *cobra.Command, enabled bool) error {
	next := a.cfg
	next.TelemetryEnabled = enabled
	path, err := resolveConfigFilePath(a.env)
	if err != nil {
		return err
	}
	if err := writeSecureJSON(path, next); err != nil {
		return err
	}
	a.cfg = next
	a.applyConfigToEnv()
	return renderResult(cmd, "telemetry", map[string]any{
		"action":     map[bool]string{true: "enable", false: "disable"}[enabled],
		"configPath": path,
		"enabled":    enabled && strings.TrimSpace(a.env["DO_NOT_TRACK"]) == "",
		"doNotTrack": strings.TrimSpace(a.env["DO_NOT_TRACK"]) != "",
	})
}

func (a *App) buildUpgradeCommand() *cobra.Command {
	var check bool
	cmd := &cobra.Command{
		Use:     "upgrade",
		Aliases: []string{"update"},
		Short:   "Upgrade Agora CLI in place when installer-managed; otherwise print upgrade guidance",
		Long: `Upgrade Agora CLI to the latest release.

When the binary was installed with install.sh / install.ps1, this command performs an in-place self-update: download the new archive, verify its SHA-256 against the published checksums.txt, and atomically replace the running binary. The same primitives the installer uses.

For Homebrew, npm, and other package-manager-managed installs, the command prints the recommended upgrade command and exits successfully (status: "manual"). It will not shadow a managed install.

Use --check to resolve the latest version and report what would happen without writing anything.`,
		Example: example(`
  agora upgrade
  agora upgrade --check --json
  agora update --json
`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := a.performSelfUpdate(check)
			if err != nil {
				return err
			}
			return renderResult(cmd, "upgrade", data)
		},
	}
	cmd.Flags().BoolVar(&check, "check", false, "resolve the latest release and report what would happen without writing anything")
	return cmd
}

func (a *App) buildOpenCommand() *cobra.Command {
	var target string
	var noBrowser bool
	cmd := &cobra.Command{
		Use:   "open",
		Short: "Open Agora Console or CLI docs",
		Example: example(`
  agora open --target console
  agora open --target docs
  agora open --target product-docs
`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			url := "https://console.agora.io"
			switch target {
			case "docs":
				url = "https://agoraio.github.io/cli/"
			case "product-docs":
				url = "https://docs.agora.io"
			case "console":
			default:
				return fmt.Errorf("unknown open target %q. Use console, docs, or product-docs.", target)
			}
			status := "printed"
			if !noBrowser && a.resolveOutputMode(cmd) != outputJSON && openBrowser(url) {
				status = "opened"
			}
			return renderResult(cmd, "open", map[string]any{"action": "open", "status": status, "target": target, "url": url})
		},
	}
	cmd.Flags().StringVar(&target, "target", "console", "target to open: console, docs, or product-docs")
	cmd.Flags().BoolVar(&noBrowser, "no-browser", false, "print the URL without opening a browser")
	return cmd
}

func (a *App) buildLoginCommand(use string) *cobra.Command {
	var noBrowser bool
	var region string
	cmd := &cobra.Command{
		Use:   use,
		Short: "Authenticate with Agora Console",
		Long:  "Open an OAuth login flow in the browser and store the local Agora session for future CLI commands.",
		Example: example(`
  agora login
  agora login --no-browser
  agora login --region cn
`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := a.login(noBrowser, region, jsonProgressFor(a, cmd, use))
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
		Example: example(`
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
	var plain bool
	cmd := &cobra.Command{
		Use:   "whoami",
		Short: "Show the current auth status",
		Long:  "Display whether the CLI is authenticated and which scope and session expiry are currently active. For automation, prefer `agora auth status --json`.",
		Example: example(`
  agora whoami
  agora whoami --plain
  agora whoami --json
`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := a.authStatus()
			if err != nil {
				return err
			}
			if plain {
				fmt.Fprintln(cmd.OutOrStdout(), asString(data["status"]))
				if auth, _ := data["authenticated"].(bool); !auth {
					return &exitError{code: 3}
				}
				return nil
			}
			return a.renderAuthStatusResult(cmd, data)
		},
	}
	cmd.Flags().BoolVar(&plain, "plain", false, "print only authenticated or unauthenticated for shell scripts")
	return cmd
}

func (a *App) buildAuthCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Manage Agora authentication",
		Long:  "Authentication helpers for logging in, logging out, and inspecting the current local session.",
		Example: example(`
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
		Example: example(`
  agora auth status
  agora auth status --json
`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			data, err := a.authStatus()
			if err != nil {
				return err
			}
			return a.renderAuthStatusResult(cmd, data)
		},
	})
	return cmd
}

func (a *App) renderAuthStatusResult(cmd *cobra.Command, data map[string]any) error {
	if auth, ok := data["authenticated"].(bool); ok && !auth {
		if mode, _ := cmd.Context().Value(contextKeyOutputMode{}).(outputMode); mode == outputJSON {
			logPath := resolveLogFilePathForDisplay(a.env)
			err := &cliError{
				Message: noLocalSessionErrorMessage,
				Code:    "AUTH_UNAUTHENTICATED",
			}
			if emitErr := emitErrorEnvelope(cmd.OutOrStdout(), "auth status", err, 3, logPath); emitErr != nil {
				return emitErr
			}
			return &exitError{code: 3}
		}
		cmd.SetContext(context.WithValue(cmd.Context(), exitCodeKey{}, 3))
	}
	if err := renderResult(cmd, "auth status", data); err != nil {
		return err
	}
	return exitIfNeeded(cmd)
}

func (a *App) buildConfigCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Manage persisted Agora CLI defaults",
		Long:  "Read and update local CLI defaults such as API endpoints, output mode, log level, and browser behavior.",
		Example: example(`
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
	var telemetryEnabled, browserAutoOpen, verbose bool
	update := &cobra.Command{
		Use:   "update",
		Short: "Update persisted CLI defaults",
		Long:  "Write new default values to the local Agora CLI config file. Environment variables still take precedence at runtime.",
		Example: example(`
  agora config update --output json
  agora config update --browser-auto-open=false
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
				next.TelemetryEnabled = telemetryEnabled
			}
			if cmd.Flags().Changed("browser-auto-open") {
				next.BrowserAutoOpen = browserAutoOpen
			}
			if cmd.Flags().Changed("log-level") {
				next.LogLevel = cfg.LogLevel
			}
			if cmd.Flags().Changed("verbose") {
				next.Verbose = verbose
			}
			if cmd.Flags().Changed("output") {
				next.Output = cfg.Output
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
	update.Flags().BoolVar(&telemetryEnabled, "telemetry-enabled", false, "persist telemetry preference; use --telemetry-enabled=false to disable")
	update.Flags().BoolVar(&browserAutoOpen, "browser-auto-open", false, "persist browser auto-open preference; use --browser-auto-open=false to disable")
	update.Flags().StringVar(&cfg.LogLevel, "log-level", cfg.LogLevel, "persist default log level")
	update.Flags().BoolVar(&verbose, "verbose", false, "persist verbose logging preference; use --verbose=false to disable")
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
		Example: example(`
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

func (a *App) buildProjectCreate() *cobra.Command {
	var region, rtmDataCenter, template string
	var features []string
	var dryRun bool
	var idempotencyKey string
	cmd := &cobra.Command{
		Use:   "create [name]",
		Short: "Create a new remote Agora project",
		Long:  "Create a new Agora project resource in the selected control-plane region and optionally enable features after creation.",
		Example: example(`
  agora project create my-app
  agora project create my-agent-demo --region global --feature rtc --feature convoai
  agora project create my-rtm-demo --rtm-data-center EU
  agora project create my-voice-agent --template voice-agent
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 || strings.TrimSpace(args[0]) == "" {
				return errors.New("project name is required")
			}
			normalizedRTMDataCenter, err := normalizeRTMDataCenter(rtmDataCenter)
			if err != nil {
				return err
			}
			if dryRun {
				plannedFeatures := projectCreateFeatures(template, features)
				plannedRTMDataCenter, err := rtmDataCenterForFeatures(plannedFeatures, normalizedRTMDataCenter)
				if err != nil {
					return err
				}
				result := map[string]any{
					"action":          "create",
					"dryRun":          true,
					"enabledFeatures": plannedFeatures,
					"idempotencyKey":  idempotencyKey,
					"projectName":     args[0],
					"region":          region,
					"status":          "planned",
					"template":        template,
				}
				if plannedRTMDataCenter != "" {
					result["rtmDataCenter"] = plannedRTMDataCenter
				}
				return renderResult(cmd, "project create", result)
			}
			data, err := a.projectCreate(args[0], region, template, features, normalizedRTMDataCenter, idempotencyKey)
			if err != nil {
				return err
			}
			return renderResult(cmd, "project create", data)
		},
	}
	cmd.Flags().StringVar(&region, "region", "", "control plane region for the project context (global or cn)")
	cmd.Flags().StringVar(&rtmDataCenter, "rtm-data-center", "", "RTM data center to configure when rtm is enabled (CN, NA, EU, or AP); defaults to NA")
	cmd.Flags().StringVar(&template, "template", "", "apply a higher-level project preset such as voice-agent")
	cmd.Flags().StringArrayVar(&features, "feature", nil, "enable one or more features after creation; defaults to rtc, rtm, and convoai; convoai also enables rtm")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "return the planned project create result without creating remote resources")
	cmd.Flags().StringVar(&idempotencyKey, "idempotency-key", "", "caller-provided key for safe retries when supported by the API")
	return cmd
}

func (a *App) buildProjectList() *cobra.Command {
	var page, pageSize int
	var keyword string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List projects available to the current account",
		Long:  "List remote Agora projects visible to the authenticated account, with optional filtering and pagination.",
		Example: example(`
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
		Example: example(`
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
		ValidArgsFunction: a.completeProjectNames,
	}
}

func (a *App) buildProjectShow() *cobra.Command {
	return &cobra.Command{
		Use:   "show [project]",
		Short: "Show one project",
		Long:  "Display details for the current project or for a project provided explicitly by name or ID.",
		Example: example(`
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
		ValidArgsFunction: a.completeProjectNames,
	}
}

func (a *App) buildProjectEnv() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "env",
		Short: "Export project environment variables",
		Long: `Render environment variables for a project in dotenv, shell, or JSON form.

Use "project env write" when you want to persist the values into a managed dotenv file.`,
		Example: example(`
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
			if format == envJSON {
				target, err := a.resolveProjectTarget(a.projectEnvProject)
				if err != nil {
					return err
				}
				return renderResult(cmd, "project env", map[string]any{
					"action":      "env",
					"format":      "json",
					"projectId":   target.project.ProjectID,
					"projectName": target.project.Name,
					"region":      target.region,
					"values":      values,
				})
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), renderProjectEnv(values, format))
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
		Long: `Write Agora App ID and App Certificate values to a dotenv file.

If no path is provided, the CLI chooses the default target using the existing env files in the working directory.

Next.js apps are detected from package.json (a next dependency), a next.config file, env.local.example, or repo .agora project.json fields template (quickstart) or projectType; those workspaces receive NEXT_PUBLIC_AGORA_APP_ID and NEXT_AGORA_APP_CERTIFICATE. Use --template nextjs or --template standard to override detection.

When .agora/project.json exists, this command updates it for the selected project and records projectType/envPath when missing. If no repo-local binding exists yet, it creates .agora/project.json in the current working directory.`,
		Example: example(`
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
			target, err := a.resolveProjectTarget(a.projectEnvProject)
			if err != nil {
				return err
			}
			cwd, err := os.Getwd()
			if err != nil {
				return err
			}
			pathFromUser := strings.TrimSpace(path)
			absPath, err := resolveProjectEnvWriteAbsolutePath(cwd, pathFromUser)
			if err != nil {
				return err
			}
			workspaceDir := filepath.Dir(absPath)
			projectType, err := detectProjectType(workspaceDir, a.projectEnvWriteTemplate)
			if err != nil {
				return err
			}
			layout, err := detectProjectEnvCredentialLayout(workspaceDir, a.projectEnvWriteTemplate)
			if err != nil {
				return err
			}
			values, err := projectCredentialEnvValuesForLayout(target.project, layout)
			if err != nil {
				return err
			}
			conflicting := conflictingKeysForProjectEnvLayout(layout)
			file, err := writeProjectEnvFile(absPath, values, appendFlag, overwriteFlag, conflicting, pathFromUser == "")
			if err != nil {
				return err
			}
			metaUpdated, metadataPath, err := syncLocalProjectBindingAfterEnvWrite(workspaceDir, cwd, absPath, target, projectType)
			if err != nil {
				return err
			}
			payload := map[string]any{
				"action":           "env-write",
				"credentialLayout": credentialLayoutLabel(layout),
				"keysWritten":      projectEnvKeys(values),
				"projectType":      projectType,
				"path":             file.Path,
				"projectId":        target.project.ProjectID,
				"projectName":      target.project.Name,
				"status":           file.Status,
			}
			if metaUpdated {
				payload["metadataUpdated"] = true
				payload["metadataPath"] = metadataPath
			}
			return renderResult(cmd, "project env write", payload)
		},
	}
	write.Flags().Bool("overwrite", false, "replace the target file with only Agora App ID and App Certificate values")
	write.Flags().Bool("append", false, "append Agora App ID and App Certificate values when no existing values are present")
	write.Flags().StringVar(&a.projectEnvWriteTemplate, "template", "", "credential key layout: nextjs or standard; if omitted, detect Next.js from the workspace")
	cmd.AddCommand(write)
	return cmd
}

func (a *App) buildProjectFeature() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "feature",
		Short: "Manage project feature state",
		Long:  "Inspect and enable product features such as rtc, rtm, and convoai for a remote Agora project.",
		Example: example(`
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
		Example: example(`
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
		ValidArgsFunction: a.completeProjectNames,
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "status <feature> [project]",
		Short: "Show one feature status",
		Example: example(`
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
		ValidArgsFunction: a.completeFeatureThenProject,
	})
	cmd.AddCommand(&cobra.Command{
		Use:   "enable <feature> [project]",
		Short: "Enable one feature for a project",
		Example: example(`
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
		ValidArgsFunction: a.completeFeatureThenProject,
	})
	return cmd
}

func (a *App) buildProjectDoctor() *cobra.Command {
	var deep bool
	var feature string
	cmd := &cobra.Command{
		Use:   "doctor [project]",
		Short: "Diagnose whether a project is ready for selected feature development",
		Long: `Run a readiness check for a project, including auth state, project context, and required feature configuration.

Exit codes:
  0  healthy
  1  blocking project issues
  2  warnings
  3  auth or session issues`,
		Example: example(`
  agora project doctor
  agora project doctor --feature rtm
  agora project doctor --deep
  agora project doctor my-agent-demo --json
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := validateDoctorFeature(feature); err != nil {
				return err
			}
			project := ""
			if len(args) > 0 {
				project = args[0]
			}
			result := a.projectDoctor(project, feature, deep)
			code := 0
			switch result.Status {
			case "auth_error":
				code = 3
			case "not_ready":
				code = 1
			case "warning":
				code = 2
			}
			if a.resolveOutputMode(cmd) == outputJSON && code != 0 {
				err := doctorEnvelopeError(result)
				logPath := resolveLogFilePathForDisplay(a.env)
				if emitErr := emitFailureEnvelopeWithData(cmd.OutOrStdout(), "project doctor", result, err, code, logPath, jsonPrettyFromContext(cmd)); emitErr != nil {
					return emitErr
				}
				return &exitError{code: code}
			}
			if err := renderResult(cmd, "project doctor", result); err != nil {
				return err
			}
			if code != 0 {
				return &exitError{code: code}
			}
			return nil
		},
	}
	cmd.Flags().BoolVar(&deep, "deep", false, "run deeper repo-local checks for .agora metadata and quickstart env consistency")
	cmd.Flags().StringVar(&feature, "feature", "convoai", "target feature readiness to evaluate: rtc, rtm, or convoai")
	return cmd
}

func doctorEnvelopeError(result projectDoctorResult) error {
	code := "PROJECT_NOT_READY"
	if result.Status == "auth_error" {
		code = "AUTH_UNAUTHENTICATED"
	}
	if len(result.BlockingIssues) > 0 && result.BlockingIssues[0].Code != "" {
		code = result.BlockingIssues[0].Code
	}
	return &cliError{Message: result.Summary, Code: code}
}
