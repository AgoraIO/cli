package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

const version = "0.1.3"

type outputMode string

const (
	outputJSON   outputMode = "json"
	outputPretty outputMode = "pretty"
)

type appConfig struct {
	Version          int        `json:"version"`
	APIBaseURL       string     `json:"apiBaseUrl"`
	BrowserAutoOpen  bool       `json:"browserAutoOpen"`
	LogLevel         string     `json:"logLevel"`
	OAuthBaseURL     string     `json:"oauthBaseUrl"`
	OAuthClientID    string     `json:"oauthClientId"`
	OAuthScope       string     `json:"oauthScope"`
	Output           outputMode `json:"output"`
	TelemetryEnabled bool       `json:"telemetryEnabled"`
	Verbose          bool       `json:"verbose"`
}

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
}

type jsonEnvelope struct {
	OK      bool           `json:"ok"`
	Command string         `json:"command"`
	Data    any            `json:"data"`
	Error   *envelopeError `json:"error,omitempty"`
	Meta    map[string]any `json:"meta"`
}

type envelopeError struct {
	Message     string `json:"message"`
	LogFilePath string `json:"logFilePath,omitempty"`
}

type App struct {
	root              *cobra.Command
	env               map[string]string
	cfg               appConfig
	cfgState          configState
	rootOutput        string
	rootJSON          bool
	httpClient        *http.Client
	projectEnvProject string
	projectEnvFormat  string
	projectEnvShell   bool
	projectEnvSecrets bool
}

func NewApp() (*App, error) {
	env := snapshotEnv()
	state, err := ensureAppConfigState(env)
	if err != nil {
		return nil, err
	}
	a := &App{
		env:        env,
		cfg:        state.Config,
		cfgState:   state,
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
	a.applyConfigToEnv()
	a.root = a.buildRoot()
	return a, nil
}

func (a *App) Execute() error {
	mode := a.cfg.Output
	if output := readRawFlagValue(os.Args[1:], "--output"); output == "json" || output == "pretty" {
		mode = outputMode(output)
	}
	if hasFlag(os.Args[1:], "--json") {
		mode = outputJSON
	}
	if shouldPrintConfigBanner(mode, isTTY(os.Stderr), a.cfgState.Status) {
		if banner := formatConfigBanner(a.cfgState); banner != "" {
			fmt.Fprintln(os.Stderr, banner)
		}
	}
	if err := a.root.Execute(); err != nil {
		if _, ok := ExitCode(err); ok {
			return err
		}
		logPath := resolveLogFilePathForDisplay(a.env)
		_ = appendAppLog("error", "command.failed", a.env, map[string]any{
			"error":       err.Error(),
			"logFilePath": logPath,
		})
		if mode == outputJSON {
			_ = emitErrorEnvelope(os.Stdout, a.guessCommandLabel(os.Args[1:]), err, 1, logPath)
			return &renderedError{err: err}
		}
		fmt.Fprintln(os.Stderr, err.Error())
		fmt.Fprintf(os.Stderr, "Detailed log: %s\n", logPath)
		return &renderedError{err: err}
	}
	return nil
}

func JSONRequested(args []string) bool {
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--json" {
			return true
		}
		if arg == "--output" && index+1 < len(args) && args[index+1] == "json" {
			return true
		}
		if strings.HasPrefix(arg, "--output=") && strings.TrimPrefix(arg, "--output=") == "json" {
			return true
		}
	}
	return false
}

func EmitJSONError(command string, err error, exitCode int, logFilePath string) error {
	return emitErrorEnvelope(os.Stdout, command, err, exitCode, logFilePath)
}

func readRawFlagValue(args []string, flag string) string {
	for index := 0; index < len(args)-1; index++ {
		if args[index] == flag {
			return args[index+1]
		}
	}
	return ""
}

func hasFlag(args []string, flag string) bool {
	for _, arg := range args {
		if arg == flag {
			return true
		}
	}
	return false
}

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
		for _, arg := range remaining {
			if strings.HasPrefix(arg, "-") {
				break
			}
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
	for _, arg := range args {
		if strings.HasPrefix(arg, "-") {
			break
		}
		return arg
	}
	return "agora"
}

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

func defaultConfig() appConfig {
	return appConfig{
		Version:          2,
		APIBaseURL:       "https://agora-cli.agora.io",
		BrowserAutoOpen:  true,
		LogLevel:         "info",
		OAuthBaseURL:     "https://sso2.agora.io",
		OAuthClientID:    "agora_web_cli",
		OAuthScope:       "basic_info,console",
		Output:           outputPretty,
		TelemetryEnabled: true,
		Verbose:          false,
	}
}

func ensureAppConfig() (appConfig, error) {
	state, err := ensureAppConfigState(snapshotEnv())
	if err != nil {
		return appConfig{}, err
	}
	return state.Config, nil
}

func mergeConfig(cfg *appConfig, raw map[string]any) {
	if v, ok := raw["apiBaseUrl"].(string); ok && v != "" {
		cfg.APIBaseURL = v
	}
	if v, ok := raw["browserAutoOpen"].(bool); ok {
		cfg.BrowserAutoOpen = v
	}
	if v, ok := raw["logLevel"].(string); ok && v != "" {
		cfg.LogLevel = v
	}
	if v, ok := raw["oauthBaseUrl"].(string); ok && v != "" {
		cfg.OAuthBaseURL = v
	}
	if v, ok := raw["oauthClientId"].(string); ok && v != "" {
		cfg.OAuthClientID = v
	}
	if v, ok := raw["oauthScope"].(string); ok && v != "" {
		cfg.OAuthScope = v
	}
	if v, ok := raw["output"].(string); ok && (v == "json" || v == "pretty") {
		cfg.Output = outputMode(v)
	}
	if v, ok := raw["telemetryEnabled"].(bool); ok {
		cfg.TelemetryEnabled = v
	}
	if v, ok := raw["verbose"].(bool); ok {
		cfg.Verbose = v
	}
}

func (a *App) applyConfigToEnv() {
	a.setEnvIfMissing("AGORA_API_BASE_URL", a.cfg.APIBaseURL)
	a.setEnvIfMissing("AGORA_OAUTH_BASE_URL", a.cfg.OAuthBaseURL)
	a.setEnvIfMissing("AGORA_OAUTH_CLIENT_ID", a.cfg.OAuthClientID)
	a.setEnvIfMissing("AGORA_OAUTH_SCOPE", a.cfg.OAuthScope)
	a.setEnvIfMissing("AGORA_OUTPUT", string(a.cfg.Output))
	a.setEnvIfMissing("AGORA_SENTRY_ENABLED", boolString(a.cfg.TelemetryEnabled))
	a.setEnvIfMissing("AGORA_BROWSER_AUTO_OPEN", boolString(a.cfg.BrowserAutoOpen))
	a.setEnvIfMissing("AGORA_LOG_LEVEL", a.cfg.LogLevel)
	a.setEnvIfMissing("AGORA_VERBOSE", boolString(a.cfg.Verbose))
}

func (a *App) setEnvIfMissing(key, value string) {
	if _, ok := a.env[key]; !ok && value != "" {
		a.env[key] = value
	}
}

func boolString(v bool) string {
	if v {
		return "1"
	}
	return "0"
}

func isTTY(file *os.File) bool {
	info, err := file.Stat()
	if err != nil {
		return false
	}
	return (info.Mode() & os.ModeCharDevice) != 0
}

func resolveConfigRoot(env map[string]string) (string, error) {
	if v := strings.TrimSpace(env["AGORA_HOME"]); v != "" {
		return v, nil
	}
	if v := strings.TrimSpace(env["XDG_CONFIG_HOME"]); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, ".agora-cli"), nil
	}
	if v := strings.TrimSpace(env["APPDATA"]); v != "" {
		return v, nil
	}
	return filepath.Join(home, ".config"), nil
}

func resolveAgoraDirectory(env map[string]string) (string, error) {
	root, err := resolveConfigRoot(env)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(env["AGORA_HOME"]) != "" {
		return root, nil
	}
	hasExplicitRoot := strings.TrimSpace(env["XDG_CONFIG_HOME"]) != "" || strings.TrimSpace(env["APPDATA"]) != ""
	if runtime.GOOS == "darwin" && !hasExplicitRoot {
		return root, nil
	}
	return filepath.Join(root, "agora-cli"), nil
}

func resolveConfigFilePath(env map[string]string) (string, error) {
	dir, err := resolveAgoraDirectory(env)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func resolveSessionFilePath(env map[string]string) (string, error) {
	dir, err := resolveAgoraDirectory(env)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "session.json"), nil
}

func resolveContextFilePath(env map[string]string) (string, error) {
	dir, err := resolveAgoraDirectory(env)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "context.json"), nil
}

func writeSecureJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func loadContext(env map[string]string) (projectContext, error) {
	path, err := resolveContextFilePath(env)
	if err != nil {
		return projectContext{}, err
	}
	ctx := projectContext{CurrentRegion: "global", PreferredRegion: "global"}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ctx, nil
	}
	if err != nil {
		return projectContext{}, err
	}
	if err := json.Unmarshal(data, &ctx); err != nil {
		return projectContext{}, err
	}
	if ctx.CurrentRegion == "" {
		ctx.CurrentRegion = "global"
	}
	if ctx.PreferredRegion == "" {
		ctx.PreferredRegion = "global"
	}
	return ctx, nil
}

func saveContext(env map[string]string, ctx projectContext) error {
	path, err := resolveContextFilePath(env)
	if err != nil {
		return err
	}
	return writeSecureJSON(path, ctx)
}

func clearContext(env map[string]string) error {
	path, err := resolveContextFilePath(env)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func loadSession(env map[string]string) (*session, error) {
	path, err := resolveSessionFilePath(env)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func saveSession(env map[string]string, s session) error {
	path, err := resolveSessionFilePath(env)
	if err != nil {
		return err
	}
	return writeSecureJSON(path, s)
}

func clearSession(env map[string]string) (bool, error) {
	path, err := resolveSessionFilePath(env)
	if err != nil {
		return false, err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

func (a *App) resolveOutputMode(cmd *cobra.Command) outputMode {
	jsonFlag, _ := cmd.Flags().GetBool("json")
	if jsonFlag {
		return outputJSON
	}
	output, _ := cmd.Flags().GetString("output")
	return resolveConfiguredOutputMode(output, a.env)
}

func emitEnvelope(out io.Writer, command string, data any) error {
	return json.NewEncoder(out).Encode(jsonEnvelope{
		OK:      true,
		Command: command,
		Data:    data,
		Meta:    map[string]any{"outputMode": "json"},
	})
}

func emitErrorEnvelope(out io.Writer, command string, err error, exitCode int, logFilePath string) error {
	meta := map[string]any{
		"outputMode": "json",
		"exitCode":   exitCode,
	}
	return json.NewEncoder(out).Encode(jsonEnvelope{
		OK:      false,
		Command: command,
		Data:    nil,
		Error: &envelopeError{
			Message:     err.Error(),
			LogFilePath: logFilePath,
		},
		Meta: meta,
	})
}

func renderResult(cmd *cobra.Command, command string, data any) error {
	out := cmd.OutOrStdout()
	if aMode := cmd.Context().Value(contextKeyOutputMode{}); aMode != nil && aMode.(outputMode) == outputJSON {
		return emitEnvelope(out, command, data)
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
		printBlock(out, "Project", [][2]string{{"Name", asString(m["projectName"])}, {"Project ID", asString(m["projectId"])}, {"App ID", asString(m["appId"])}, {"App Certificate", asString(m["appCertificate"])}, {"Region", asString(m["region"])}, {"Token Enabled", asString(m["tokenEnabled"])}})
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
				fmt.Fprintf(out, "  Available: %s\n", asString(item["available"]))
				fmt.Fprintf(out, "  Runtime: %s\n", asString(item["runtime"]))
				fmt.Fprintf(out, "  Supports Init: %s\n", asString(item["supportsInit"]))
				fmt.Fprintf(out, "  Env: %s\n", asString(item["envDocs"]))
				fmt.Fprintf(out, "  Repo: %s\n", asString(item["repoUrl"]))
			}
		}
	case "quickstart create":
		m := data.(map[string]any)
		printBlock(out, "Quickstart", [][2]string{{"Template", asString(m["template"])}, {"Path", asString(m["path"])}, {"Project", asString(m["projectName"])}, {"Env", asString(m["envStatus"])}, {"Metadata", asString(m["metadataPath"])}, {"Status", asString(m["status"])}})
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
		printBlock(out, "Projects", [][2]string{{"Total", asString(m["total"])}})
		fmt.Fprintln(out)
		if items, ok := m["items"].([]projectSummary); ok {
			for _, item := range items {
				fmt.Fprintln(out, item.Name)
				printBlock(out, "", [][2]string{{"Project ID", item.ProjectID}, {"Type", item.ProjectType}, {"Status", item.Status}})
				fmt.Fprintln(out)
			}
		}
	case "project doctor":
		return printDoctor(out, data.(projectDoctorResult))
	default:
		encoded, _ := json.MarshalIndent(data, "", "  ")
		fmt.Fprintf(out, "%s\n%s\n", command, string(encoded))
	}
	return nil
}

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
		fmt.Fprintf(out, "%-*s : %s\n", width, row[0], row[1])
	}
}

func printDoctor(out io.Writer, result projectDoctorResult) error {
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
			marker := map[string]string{"pass": "✓", "warn": "!", "skipped": "-", "fail": "✗"}[item.Status]
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
	fmt.Fprintf(out, "  %s %s\n", marker, result.Summary)
	return nil
}

type contextKeyOutputMode struct{}
type exitCodeKey struct{}
type exitError struct{ code int }
type renderedError struct{ err error }

func (e *exitError) Error() string     { return "" }
func (e *renderedError) Error() string { return e.err.Error() }
func ExitCode(err error) (int, bool) {
	var exitErr *exitError
	if errors.As(err, &exitErr) {
		return exitErr.code, true
	}
	return 0, false
}

func ErrorRendered(err error) bool {
	var rendered *renderedError
	return errors.As(err, &rendered)
}

func parseBooleanString(value, option string) (bool, error) {
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	default:
		return false, fmt.Errorf("%s must be either \"true\" or \"false\".", option)
	}
}

func exitIfNeeded(cmd *cobra.Command) error {
	if code, ok := cmd.Context().Value(exitCodeKey{}).(int); ok && code != 0 {
		return &exitError{code: code}
	}
	return nil
}

type outputModeValue struct{ target *string }

func newOutputModeValue(target *string) *outputModeValue { return &outputModeValue{target: target} }
func (v *outputModeValue) String() string {
	if v.target == nil {
		return ""
	}
	return *v.target
}
func (v *outputModeValue) Set(value string) error {
	if value != "json" && value != "pretty" {
		return errors.New("--output must be one of: json, pretty")
	}
	*v.target = value
	return nil
}
func (v *outputModeValue) Type() string { return "output" }
