package cli

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// Empty until an approved collector is deployed. Release builds may set this
// with -ldflags; local verification uses AGORA_TELEMETRY_URL.
var analyticsEndpoint = ""

const analyticsSendTimeout = 10 * time.Second

var analyticsIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
var analyticsSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var analyticsVersionPattern = regexp.MustCompile(`^[A-Za-z0-9.+_-]{0,64}$`)
var analyticsCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

// Init stage names. These and the agent value sets below are part of the
// collector allowlist: emit sites and validAnalyticsEvent share them so the
// two can never drift.
const (
	analyticsStageValidate        = "validate"
	analyticsStageSourceResolve   = "source_resolve"
	analyticsStageLocalValidate   = "local_validate"
	analyticsStageProjectResolve  = "project_resolve"
	analyticsStageClone           = "clone"
	analyticsStageEnvWrite        = "env_write"
	analyticsStageBindingWrite    = "binding_write"
	analyticsStageContextWrite    = "context_write"
	analyticsStageAgentRulesWrite = "agent_rules_write"
)

var analyticsStages = []string{analyticsStageValidate, analyticsStageSourceResolve, analyticsStageLocalValidate, analyticsStageProjectResolve, analyticsStageClone, analyticsStageEnvWrite, analyticsStageBindingWrite, analyticsStageContextWrite, analyticsStageAgentRulesWrite}

const (
	analyticsAgentCustom  = "custom"
	analyticsValueUnknown = "unknown"

	analyticsSurfaceDesktop = "desktop"
	analyticsSurfaceCLI     = "cli"

	analyticsDetectionExplicit    = "explicit"
	analyticsDetectionEnvironment = "environment"
	analyticsDetectionNone        = "none"
)

// analyticsKnownAgentFamilies are agent labels reported as-is; any other
// detected label is reported as analyticsAgentCustom.
var analyticsKnownAgentFamilies = []string{"codex", "claude-code", "kiro", "cursor", "cline", "windsurf", "aider"}
var analyticsAgentFamilies = append(slices.Clone(analyticsKnownAgentFamilies), analyticsAgentCustom, analyticsValueUnknown)
var analyticsAgentSurfaces = []string{analyticsSurfaceDesktop, analyticsSurfaceCLI, analyticsValueUnknown}
var analyticsDetectionSources = []string{analyticsDetectionExplicit, analyticsDetectionEnvironment, analyticsDetectionNone}

// analyticsRegions are the coarse control-plane regions an event may report.
var analyticsRegions = []string{regionGlobal, regionCN}

type analyticsEvent struct {
	Event       string         `json:"event"`
	EventID     string         `json:"event_id"`
	OperationID string         `json:"operation_id"`
	DistinctID  string         `json:"distinct_id"`
	Timestamp   time.Time      `json:"timestamp"`
	Properties  map[string]any `json:"properties"`
}

type analyticsOperation struct {
	command      string
	channel      string
	id           string
	distinctID   string
	properties   map[string]any
	started      time.Time
	initStarted  *analyticsEvent
	stage        string
	stageStarted time.Time
	events       []json.RawMessage
	batchBytes   int
	endpoint     string
	// projectCreateReported is set once the createProject API boundary has
	// emitted cli_project_create_completed for this operation.
	projectCreateReported bool
}

type analyticsContextKey struct{}

func analyticsUUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		return ""
	}
	b[6] = (b[6] & 15) | 64
	b[8] = (b[8] & 63) | 128
	h := hex.EncodeToString(b[:])
	return h[:8] + "-" + h[8:12] + "-" + h[12:16] + "-" + h[16:20] + "-" + h[20:]
}

// analyticsEnabled applies the shared telemetry gate to the startup
// preference. Shell commands use this snapshot for their whole run.
func (a *App) analyticsEnabled() bool {
	return a.telemetryAllowed(a.cfg.TelemetryEnabled)
}

func (a *App) currentTelemetryPreference() bool {
	cfg, valid := a.currentTelemetryConfig()
	return valid && cfg.TelemetryEnabled
}

func (a *App) currentTelemetryConfig() (appConfig, bool) {
	path, err := resolveConfigFilePath(a.env)
	if err != nil {
		return appConfig{}, false
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) && a.cfgState.ConfigFilePath == "" {
		return a.cfg, true // Apps constructed without a config store.
	}
	if err != nil {
		return appConfig{}, false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil || fields == nil {
		return appConfig{}, false
	}
	if enabled, ok := fields["telemetryEnabled"]; ok && string(enabled) != "true" && string(enabled) != "false" {
		return appConfig{}, false
	}
	config := defaultConfig()
	if json.Unmarshal(data, &config) != nil {
		return appConfig{}, false
	}
	return config, true
}

func (a *App) analyticsURL() string {
	endpoint := strings.TrimSpace(a.env["AGORA_TELEMETRY_URL"])
	if endpoint == "" {
		endpoint = analyticsEndpoint
	}
	u, err := url.Parse(endpoint)
	if err != nil || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return ""
	}
	if u.Scheme != "https" && !(u.Scheme == "http" && (u.Hostname() == "127.0.0.1" || u.Hostname() == "localhost" || u.Hostname() == "::1")) {
		return ""
	}
	return endpoint
}

func (a *App) analyticsIdentity() string {
	dir, err := resolveAgoraDirectory(a.env)
	if err != nil {
		return ""
	}
	path := filepath.Join(dir, "analytics-id")
	if b, err := os.ReadFile(path); err == nil {
		id := strings.TrimSpace(string(b))
		if len(id) == 36 && analyticsIDPattern.MatchString(id) {
			return "cli:" + id
		}
		return ""
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return ""
	}
	id := analyticsUUID()
	if id == "" {
		return ""
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		if b, readErr := os.ReadFile(path); readErr == nil {
			other := strings.TrimSpace(string(b))
			if len(other) == 36 && analyticsIDPattern.MatchString(other) {
				return "cli:" + other
			}
		}
		return ""
	}
	_, writeErr := f.WriteString(id)
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		return ""
	}
	return "cli:" + id
}

func (a *App) beginAnalytics(command, channel string) *analyticsOperation {
	endpoint := a.analyticsURL()
	enabled := a.analyticsEnabled()
	if channel == "mcp" {
		enabled = a.telemetryAllowed(a.currentTelemetryPreference())
	}
	if !enabled || endpoint == "" {
		return nil
	}
	props := map[string]any{"command": command, "os": runtime.GOOS, "arch": runtime.GOARCH, "invocation_channel": channel}
	version, _ := versionInfo()["version"].(string)
	if !analyticsVersionPattern.MatchString(version) {
		version = "custom"
	}
	props["cli_version"] = version
	family, surface, source := analyticsAgent(a.osEnv)
	props["agent_family"], props["agent_surface"], props["agent_detection_source"] = family, surface, source
	// Read per operation: a long-lived MCP server may outlive a login into
	// another region.
	if region, ok := a.analyticsRegion(); ok {
		props["region"] = region
	}
	id := a.analyticsIdentity()
	if id == "" {
		return nil
	}
	op := &analyticsOperation{command: command, channel: channel, id: analyticsUUID(), distinctID: id, properties: props, started: time.Now(), endpoint: endpoint}
	op.batchBytes = len(analyticsBatchEnvelope)
	if command == "init" {
		event := op.newEvent("cli_init_started", nil)
		start, err := json.Marshal(event)
		if err != nil {
			return nil
		}
		op.initStarted = &event
		// flush prepends this event after copying source attribution onto it,
		// so reserve its worst-case size (plus the separating comma) up front.
		op.batchBytes += len(start) + analyticsStartAttributionReserve + 1
		op.beginStage(analyticsStageValidate)
	}
	return op
}

// analyticsRegion reports the coarse control-plane region the CLI currently
// targets, using the same persisted context region that authRegion uses to
// choose between apiBaseURL and apiBaseURLCN. A missing context is the CLI
// default (global). Unlike authRegion, an unreadable context is reported as
// undeterminable rather than defaulting, so the property is omitted. Only
// analyticsRegions values are ever returned, never the raw persisted string.
func (a *App) analyticsRegion() (string, bool) {
	ctx, err := loadContext(a.env)
	if err != nil {
		return "", false
	}
	region := currentRegionFromContext(ctx)
	if !slices.Contains(analyticsRegions, region) {
		return "", false
	}
	return region, true
}

func analyticsAgent(env map[string]string) (family, surface, source string) {
	family, surface, source = analyticsValueUnknown, analyticsValueUnknown, analyticsDetectionNone
	label := agentLabelFromOSEnv(env)
	if label == "" {
		return
	}
	source = analyticsDetectionEnvironment
	if strings.TrimSpace(env["AGORA_AGENT"]) != "" {
		source = analyticsDetectionExplicit
	}
	family = analyticsAgentCustom
	if slices.Contains(analyticsKnownAgentFamilies, label) {
		family = label
	}
	if family == "claude-code" {
		switch env["CLAUDE_CODE_ENTRYPOINT"] {
		case "cli":
			surface = analyticsSurfaceCLI
		case "claude-desktop":
			surface = analyticsSurfaceDesktop
		}
	}
	if family == "codex" && env["CODEX_INTERNAL_ORIGINATOR_OVERRIDE"] == "Codex Desktop" {
		surface = analyticsSurfaceDesktop
	}
	if family == "kiro" {
		if env["AWS_SDK_UA_APP_ID"] == "kiro-ide" {
			surface = analyticsSurfaceDesktop
		} else if env["KIRO_CLI_ACP_CLIENT_NAME"] == "kiro-tui" {
			surface = analyticsSurfaceCLI
		}
	}
	return
}

func (op *analyticsOperation) emit(name string, fields map[string]any) {
	if op == nil {
		return
	}
	op.submit(op.newEvent(name, fields))
}

func (op *analyticsOperation) newEvent(name string, fields map[string]any) analyticsEvent {
	props := maps.Clone(op.properties)
	maps.Copy(props, fields)
	return analyticsEvent{Event: name, EventID: analyticsUUID(), OperationID: op.id, DistinctID: op.distinctID, Timestamp: time.Now().UTC(), Properties: props}
}

func (op *analyticsOperation) beginStage(stage string) {
	if op == nil || op.command != "init" {
		return
	}
	if op.stage != "" {
		op.completeStage(nil)
	}
	op.stage, op.stageStarted = stage, time.Now()
}

func analyticsOutcome(err error) (string, string) {
	if err == nil {
		return "success", ""
	}
	var ce *cliError
	code := "CLI_ERROR"
	if errors.As(err, &ce) && analyticsCodePattern.MatchString(ce.Code) {
		code = ce.Code
	}
	if errors.Is(err, context.Canceled) || code == "INIT_ABORTED" {
		return "cancelled", code
	}
	return "failure", code
}

func (op *analyticsOperation) completeStage(err error) {
	if op == nil || op.stage == "" {
		return
	}
	outcome, code := analyticsOutcome(err)
	op.emit("cli_init_stage_completed", map[string]any{"stage": op.stage, "outcome": outcome, "error_code": code, "duration_ms": time.Since(op.stageStarted).Milliseconds()})
	op.stage = ""
}

func (op *analyticsOperation) finish(err error) {
	if op == nil {
		return
	}
	failedStage := ""
	if err != nil {
		failedStage = op.stage
	}
	op.completeStage(err)
	outcome, code := analyticsOutcome(err)
	fields := map[string]any{"outcome": outcome, "error_code": code, "duration_ms": time.Since(op.started).Milliseconds(), "exit_code": 0}
	if err != nil {
		fields["exit_code"] = exitCodeForError(err)
	}
	event := "cli_" + strings.ReplaceAll(op.command, " ", "_") + "_completed"
	if op.command == "init" && failedStage != "" {
		fields["failed_stage"] = failedStage
	}
	// `project create` normally reports at the API boundary (projectCreated)
	// so its timing excludes later feature work. A failure before that call
	// (not logged in, unreadable context, invalid flags) reports here instead.
	if op.command != "project create" || !op.projectCreateReported {
		op.emit(event, fields)
	}
	op.flush()
}

func analyticsFor(cmd *cobra.Command) *analyticsOperation {
	op, _ := cmd.Context().Value(analyticsContextKey{}).(*analyticsOperation)
	return op
}

func (a *App) instrumentAnalytics(cmd *cobra.Command, command string) {
	run := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) (err error) {
		if dry, _ := cmd.Flags().GetBool("dry-run"); dry {
			return run(cmd, args)
		}
		a.maybeShowTelemetryNotice(cmd)
		op := a.beginAnalytics(command, "shell")
		if command == "init" {
			recipe, _ := cmd.Flags().GetString("recipe")
			template, _ := cmd.Flags().GetString("template")
			op.setInitSourceType(recipe, template)
		}
		ctx := cmd.Context()
		cmd.SetContext(context.WithValue(ctx, analyticsContextKey{}, op))
		defer func() { op.finish(err); cmd.SetContext(ctx) }()
		return run(cmd, args)
	}
}

func (op *analyticsOperation) setInitSourceType(recipe, template string) {
	if op == nil {
		return
	}
	recipe, template = strings.TrimSpace(recipe), strings.TrimSpace(template)
	switch {
	case recipe != "" && template == "":
		op.properties["source_type"] = "recipe"
	case template != "" && recipe == "":
		op.properties["source_type"] = "quickstart"
	}
}

func (op *analyticsOperation) source(kind, id string) {
	if op == nil || len(id) > 100 || !analyticsSlugPattern.MatchString(id) {
		return
	}
	op.properties["source_type"], op.properties["source_id"] = kind, id
	if kind == "recipe" {
		op.properties["recipe_id"] = id
	}
}

var analyticsAppIDPattern = regexp.MustCompile(`^[0-9a-fA-F]{32}$`)
var analyticsPlatformPattern = regexp.MustCompile(`^[a-z0-9]{1,20}$`)

func (op *analyticsOperation) setAppID(id string) {
	if op == nil || !analyticsAppIDPattern.MatchString(id) || op.properties["app_id"] != nil {
		return
	}
	op.properties["app_id"] = id
}

// setProjectAction records whether init created a project or reused one.
func (op *analyticsOperation) setProjectAction(action string) {
	if op == nil {
		return
	}
	op.properties["project_action"] = "reused"
	if action == "created" {
		op.properties["project_action"] = "created"
	}
}

func (op *analyticsOperation) projectCreated(id string, started time.Time, err error) {
	if op == nil {
		return
	}
	if err == nil {
		op.setAppID(id)
	}
	outcome, code := analyticsOutcome(err)
	exit := 0
	if err != nil {
		exit = exitCodeForError(err)
	}
	op.emit("cli_project_create_completed", map[string]any{"outcome": outcome, "error_code": code, "duration_ms": time.Since(started).Milliseconds(), "exit_code": exit})
	op.projectCreateReported = true
}
