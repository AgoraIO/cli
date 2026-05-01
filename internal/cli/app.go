// Package cli implements the agora-cli-go binary. The package is structured
// into focused files so each concern can be reasoned about independently:
//
//   - app.go       — App struct, Execute() entry point, output-mode resolver,
//     cobra context keys, env snapshot, TTY detection.
//   - config.go    — appConfig type, defaults, merge, env injection.
//   - paths.go     — config / session / context file path resolution and
//     secure I/O helpers (writeSecureJSON).
//   - envelope.go  — JSON envelope shape, error envelope, exit-code plumbing,
//     JSONRequested / JSONPrettyRequested helpers.
//   - render.go    — pretty output dispatch (renderResult), printBlock,
//     printDoctor, asString, redactSensitive.
//   - version.go   — build-time injected version vars (version / commit / date)
//     and versionInfo / formattedVersion helpers.
//
// Each command lives in its own *.go file and registers itself via the
// build* methods in commands.go.
package cli

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/spf13/cobra"
)

// outputMode is the type backing both --output values and AGORA_OUTPUT.
// The two valid values are exposed as constants below.
type outputMode string

const (
	outputJSON   outputMode = "json"
	outputPretty outputMode = "pretty"
)

type session struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
	TokenType    string `json:"tokenType"`
	Scope        string `json:"scope"`
	ExpiresAt    string `json:"expiresAt"`
	ObtainedAt   string `json:"obtainedAt"`
}

type projectContext struct {
	CurrentProjectID   *string `json:"currentProjectId"`
	CurrentProjectName *string `json:"currentProjectName"`
	CurrentRegion      string  `json:"currentRegion"`
	PreferredRegion    string  `json:"preferredRegion"`
}

type projectSummary struct {
	AllowStaticWithDynamic bool    `json:"allowStaticWithDynamic"`
	AppID                  string  `json:"appId"`
	CreatedAt              string  `json:"createdAt"`
	Name                   string  `json:"name"`
	ProjectID              string  `json:"projectId"`
	ProjectType            string  `json:"projectType"`
	Region                 *string `json:"region,omitempty"`
	SignKey                *string `json:"signKey"`
	Stage                  int     `json:"stage"`
	Status                 string  `json:"status"`
	UpdatedAt              string  `json:"updatedAt"`
	Vid                    int     `json:"vid"`
}

type projectDetail struct {
	AllowStaticWithDynamic bool    `json:"allowStaticWithDynamic"`
	AppID                  string  `json:"appId"`
	CertificateEnabled     bool    `json:"certificateEnabled"`
	CreatedAt              string  `json:"createdAt"`
	Name                   string  `json:"name"`
	ProjectID              string  `json:"projectId"`
	ProjectType            string  `json:"projectType"`
	Region                 *string `json:"region,omitempty"`
	SignKey                *string `json:"signKey"`
	Stage                  int     `json:"stage"`
	Status                 string  `json:"status"`
	TokenEnabled           bool    `json:"tokenEnabled"`
	UpdatedAt              string  `json:"updatedAt"`
	Usage7d                int     `json:"usage7d"`
	UseCaseID              *string `json:"useCaseId,omitempty"`
	Vid                    int     `json:"vid"`
}

type projectListResponse struct {
	Items    []projectSummary `json:"items"`
	Page     int              `json:"page"`
	PageSize int              `json:"pageSize"`
	Total    int              `json:"total"`
}

type featureItem struct {
	Feature string `json:"feature"`
	Message string `json:"message"`
	Status  string `json:"status"`
}

type doctorIssue struct {
	Code             string `json:"code"`
	Message          string `json:"message"`
	SuggestedCommand string `json:"suggestedCommand,omitempty"`
}

type doctorCheckItem struct {
	Message          string `json:"message"`
	Name             string `json:"name"`
	Status           string `json:"status"`
	SuggestedCommand string `json:"suggestedCommand,omitempty"`
}

type doctorCheckCategory struct {
	Category string            `json:"category"`
	Items    []doctorCheckItem `json:"items"`
	Status   string            `json:"status"`
}

type projectDoctorResult struct {
	Action         string                `json:"action"`
	BlockingIssues []doctorIssue         `json:"blockingIssues"`
	Checks         []doctorCheckCategory `json:"checks"`
	Feature        string                `json:"feature"`
	Healthy        bool                  `json:"healthy"`
	Mode           string                `json:"mode"`
	Project        any                   `json:"project"`
	Status         string                `json:"status"`
	Summary        string                `json:"summary"`
	Warnings       []doctorIssue         `json:"warnings"`
	Workspace      any                   `json:"workspace,omitempty"`
}

// App is the top-level CLI runtime. One *App is built per process by NewApp,
// owns the shared HTTP client, the loaded config, the env snapshots, and the
// fully-built cobra root command.
type App struct {
	root                    *cobra.Command
	env                     map[string]string
	osEnv                   map[string]string // raw OS env snapshot before applyConfigToEnv; used for CI detection & user-set env precedence
	cfg                     appConfig
	cfgState                configState
	rootOutput              string
	rootJSON                bool
	rootPrettyJSON          bool
	rootQuiet               bool
	rootNoColor             bool
	rootUpgradeCheck        bool
	rootDebug               bool
	rootYes                 bool
	httpClient              *http.Client
	telemetry               telemetryClient
	projectEnvProject       string
	projectEnvFormat        string
	projectEnvShell         bool
	projectEnvSecrets       bool
	projectEnvWriteTemplate string
}

// NewApp boots the App: snapshot env (before any mutation), load or migrate
// the config, then layer config defaults onto env. Returns an error if the
// config directory or file cannot be created.
func NewApp() (*App, error) {
	env := snapshotEnv()
	osEnv := make(map[string]string, len(env))
	for k, v := range env {
		osEnv[k] = v
	}
	state, err := ensureAppConfigState(env)
	if err != nil {
		return nil, err
	}
	a := &App{
		env:        env,
		osEnv:      osEnv,
		cfg:        state.Config,
		cfgState:   state,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	a.applyConfigToEnv()
	a.root = a.buildRoot()
	// Best-effort initialize the telemetry sink. Returns a no-op when
	// telemetry is disabled, when DO_NOT_TRACK is set, or when the
	// Sentry DSN is empty (build without telemetry compiled in). Telemetry
	// must never block the CLI from starting.
	a.telemetry = initTelemetry(a.cfg.TelemetryEnabled, a.env, versionInfo())
	return a, nil
}

// Execute is the process entry point. It resolves the output mode, decides
// whether to print the first-run config banner (suppressed in CI), runs the
// cobra root, and routes any error through the JSON envelope or pretty
// stderr path depending on the mode. Returns *exitError or *renderedError
// for the cmd/main.go shim to translate into the process exit code.
func (a *App) Execute() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	// Best-effort telemetry flush at exit. Bounded by a short timeout so
	// telemetry can never delay the CLI returning control to the shell.
	defer func() {
		if a.telemetry != nil {
			a.telemetry.Flush(2 * time.Second)
		}
	}()
	a.root.SetContext(ctx)
	// Best-effort cache hygiene on every startup. Anything older than
	// projectListCacheMaxAge (24h) is removed so we never accumulate
	// unbounded data under <AGORA_HOME>/cache and so a stale cache
	// from a prior auth session can never silently shape today's CLI
	// output. Errors are intentionally ignored: cache cleanup must
	// never block a user's command.
	_ = pruneStaleCaches(a.env)
	rawOutput := readRawFlagValue(os.Args[1:], "--output")
	if rawOutput != "json" && rawOutput != "pretty" {
		rawOutput = ""
	}
	mode := a.resolveOutputModeFromEnv(rawOutput)
	if hasFlag(os.Args[1:], "--json") {
		mode = outputJSON
	}
	if shouldPrintConfigBannerWithEnv(mode, isTTY(os.Stderr), a.cfgState.Status, a.osEnv) {
		if banner := formatConfigBanner(a.cfgState); banner != "" {
			fmt.Fprintln(os.Stderr, banner)
		}
	}
	if err := a.root.ExecuteContext(ctx); err != nil {
		if _, ok := ExitCode(err); ok {
			return err
		}
		exitCode := exitCodeForError(err)
		logPath := resolveLogFilePathForDisplay(a.env)
		_ = appendAppLog("error", "command.failed", a.env, map[string]any{
			"error":       err.Error(),
			"logFilePath": logPath,
		})
		// Forward the failure to telemetry. The sink is responsible for
		// honoring opt-out, redacting sensitive fields, and never blocking.
		if a.telemetry != nil {
			a.telemetry.CaptureException(err, map[string]any{
				"command":  a.guessCommandLabel(os.Args[1:]),
				"exitCode": exitCode,
			})
		}
		if mode == outputJSON {
			_ = emitErrorEnvelope(os.Stdout, a.guessCommandLabel(os.Args[1:]), err, exitCode, logPath)
			if exitCode != 1 {
				return &exitError{code: exitCode}
			}
			return &renderedError{err: err}
		}
		fmt.Fprintln(os.Stderr, err.Error())
		fmt.Fprintf(os.Stderr, "Detailed log: %s\n", logPath)
		if exitCode != 1 {
			return &exitError{code: exitCode}
		}
		return &renderedError{err: err}
	}
	return nil
}

// resolveOutputMode is the per-command output-mode resolver, called by
// PersistentPreRunE and any command needing to branch on JSON-vs-pretty
// behavior. It honors both --json and --output flags and falls through to
// the env / config / CI-auto-detect chain via resolveOutputModeFromEnv.
func (a *App) resolveOutputMode(cmd *cobra.Command) outputMode {
	jsonFlag, _ := cmd.Flags().GetBool("json")
	if jsonFlag {
		return outputJSON
	}
	output, _ := cmd.Flags().GetString("output")
	return a.resolveOutputModeFromEnv(output)
}

// resolveOutputModeFromEnv applies the documented precedence:
//
//  1. Explicit --output flag wins.
//  2. User-set AGORA_OUTPUT (in the original OS env) wins.
//  3. Configured cfg.Output wins if user explicitly set it to JSON.
//  4. CI environment auto-detect → JSON.
//  5. Otherwise pretty.
//
// Step 2 reads from a.osEnv (the env snapshotted *before* applyConfigToEnv
// injected the config defaults), so the config-default of "pretty" does not
// shadow CI auto-detection. This makes the JSON-by-default-in-CI behavior
// reliable while still honoring an explicit user override.
func (a *App) resolveOutputModeFromEnv(raw string) outputMode {
	if raw == "json" {
		return outputJSON
	}
	if raw == "pretty" {
		return outputPretty
	}
	if a.osEnv != nil {
		switch a.osEnv["AGORA_OUTPUT"] {
		case "json":
			return outputJSON
		case "pretty":
			return outputPretty
		}
	}
	if a.cfg.Output == outputJSON {
		return outputJSON
	}
	if isCIEnvironment(a.osEnv) {
		return outputJSON
	}
	return outputPretty
}

// guessCommandLabel inspects argv to recover the stable command label for
// envelope output when an early failure (unknown command, missing required
// flag) prevents the normal renderResult path from running.
func (a *App) guessCommandLabel(args []string) string {
	cmd, remaining, err := a.root.Find(args)
	base := "agora"
	if cmd != nil {
		base = strings.TrimSpace(strings.TrimPrefix(cmd.CommandPath(), "agora"))
		if base == "" {
			base = "agora"
		}
	}
	if cmd != nil && cmd.HasAvailableSubCommands() {
		if arg := firstNonFlag(remaining); arg != "" {
			if base == "agora" {
				return arg
			}
			return strings.TrimSpace(base + " " + arg)
		}
	}
	if err == nil {
		return base
	}
	if label := guessUnknownCommandLabel(err.Error()); label != "" {
		return label
	}
	if arg := firstNonFlag(args); arg != "" {
		return arg
	}
	return "agora"
}

func firstNonFlag(args []string) string {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return ""
	}
	return args[0]
}

// guessUnknownCommandLabel parses Cobra's `unknown command "X" for "agora Y"`
// error string back into a "Y X" label so the JSON envelope reports the
// command the user attempted, not the literal string "agora".
func guessUnknownCommandLabel(message string) string {
	const prefix = `unknown command "`
	start := strings.Index(message, prefix)
	if start == -1 {
		return ""
	}
	start += len(prefix)
	end := strings.Index(message[start:], `"`)
	if end == -1 {
		return ""
	}
	unknown := message[start : start+end]
	forPrefix := ` for "`
	forIndex := strings.Index(message, forPrefix)
	if forIndex == -1 {
		return unknown
	}
	forIndex += len(forPrefix)
	forEnd := strings.Index(message[forIndex:], `"`)
	if forEnd == -1 {
		return unknown
	}
	base := strings.TrimSpace(strings.TrimPrefix(message[forIndex:forIndex+forEnd], "agora"))
	if base == "" {
		return unknown
	}
	return strings.TrimSpace(base + " " + unknown)
}

// snapshotEnv copies the current process environment into a map so the App
// can carry an env-like view that we are free to mutate (via applyConfigToEnv)
// without leaking back into the OS-level environ.
func snapshotEnv() map[string]string {
	env := map[string]string{}
	for _, pair := range os.Environ() {
		parts := strings.SplitN(pair, "=", 2)
		if len(parts) == 2 {
			env[parts[0]] = parts[1]
		}
	}
	return env
}

func (a *App) noInput() bool {
	if a.rootYes {
		return true
	}
	value := strings.ToLower(strings.TrimSpace(a.env["AGORA_NO_INPUT"]))
	return value == "1" || value == "true" || value == "yes" || value == "y"
}

// isTTY reports whether the given file is connected to a terminal. Used for
// the first-run banner, interactive prompts, and color decisions.
func isTTY(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

// Cobra context keys: persistent root flags are propagated to subcommands
// through the cmd Context() so handlers can read them without re-walking
// the flag set.
type contextKeyOutputMode struct{}
type contextKeyJSONPretty struct{}
type contextKeyQuiet struct{}
type contextKeyNoColor struct{}
