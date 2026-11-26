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

// CN usage capture is configured separately and never uses the global
// collector. Release builds may set these with -ldflags once a CN collector
// is approved; local verification uses AGORA_TELEMETRY_CN_URL and
// AGORA_TELEMETRY_CN_MODE.
var analyticsEndpointCN = ""
var analyticsModeCN = analyticsModeOff

// Capture modes. full is the global behavior. aggregate sends the same
// events without a persistent installation ID or App IDs. off sends nothing.
const (
	analyticsModeOff       = "off"
	analyticsModeAggregate = "aggregate"
	analyticsModeFull      = "full"
)

var analyticsModes = []string{analyticsModeOff, analyticsModeAggregate, analyticsModeFull}

// analyticsAggregateProperties are the only properties an aggregate-mode
// event may carry. Notably app_id is absent.
var analyticsAggregateProperties = []string{"command", "outcome", "error_code", "exit_code", "stage", "failed_stage", "duration_ms", "cli_version", "os", "arch", "invocation_channel", "agent_family", "agent_surface", "agent_detection_source", "source_type", "source_id", "recipe_id", "project_action", "result_count", "region"}

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
	mode         string // analyticsModeAggregate or analyticsModeFull
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

// analyticsURL is the global collector. AGORA_TELEMETRY_URL never applies to
// CN operations; see analyticsCNURL.
func (a *App) analyticsURL() string {
	return validAnalyticsURL(valueOrDefault(a.env["AGORA_TELEMETRY_URL"], analyticsEndpoint))
}

// analyticsCNURL is the CN collector, or empty when none is configured.
func (a *App) analyticsCNURL() string {
	return validAnalyticsURL(valueOrDefault(a.env["AGORA_TELEMETRY_CN_URL"], analyticsEndpointCN))
}

// analyticsCNMode is the configured CN capture mode. Anything other than the
// three known values means off.
func (a *App) analyticsCNMode() string {
	mode := valueOrDefault(a.env["AGORA_TELEMETRY_CN_MODE"], analyticsModeCN)
	if !slices.Contains(analyticsModes, mode) {
		return analyticsModeOff
	}
	return mode
}

// analyticsCapture resolves where and how an operation starting now may
// report. region is global, cn or unknown. mode is the effective mode: off
// when the region is unknown (fail closed), the region's mode is off, it has
// no valid collector, or the shared telemetry gate is closed. endpoint is
// the region's collector, reported even when mode is off so status can show
// transportConfigured. CN never falls back to the global collector.
func (a *App) analyticsCapture(preference bool) (region, mode, endpoint string) {
	region, ok := a.analyticsRegion()
	switch {
	case !ok:
		return analyticsValueUnknown, analyticsModeOff, ""
	case region == regionCN:
		mode, endpoint = a.analyticsCNMode(), a.analyticsCNURL()
	default:
		mode, endpoint = analyticsModeFull, a.analyticsURL()
	}
	if endpoint == "" || !a.telemetryAllowed(preference) {
		mode = analyticsModeOff
	}
	return region, mode, endpoint
}

func validAnalyticsURL(endpoint string) string {
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

// analyticsPreference is the saved preference an operation is gated on: the
// startup snapshot for shell commands, re-read per call for long-lived MCP.
func (a *App) analyticsPreference(channel string) bool {
	if channel == "mcp" {
		return a.currentTelemetryPreference()
	}
	return a.cfg.TelemetryEnabled
}

// beginAnalytics starts an operation, or returns nil when its effective mode
// is off. Region and mode are resolved here, at each operation start, so a
// long-lived MCP server follows a login into another region.
func (a *App) beginAnalytics(command, channel string) *analyticsOperation {
	region, mode, endpoint := a.analyticsCapture(a.analyticsPreference(channel))
	if mode == analyticsModeOff {
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
	// Only the CN collector receives region; global events stay unchanged.
	if region == regionCN {
		props["region"] = regionCN
	}
	var id string
	if mode == analyticsModeAggregate {
		// No persistent identity: a fresh ID per operation links nothing.
		if u := analyticsUUID(); u != "" {
			id = "cli:" + u
		}
	} else {
		id = a.analyticsIdentity()
	}
	if id == "" {
		return nil
	}
	op := &analyticsOperation{command: command, channel: channel, mode: mode, id: analyticsUUID(), distinctID: id, properties: props, started: time.Now(), endpoint: endpoint}
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
// targets. An explicit AGORA_API_BASE_URL on the built-in CN API host is cn.
// Otherwise it uses the persisted context region that authRegion uses to
// choose between apiBaseURL and apiBaseURLCN; a missing context is the CLI
// default (global), so a user who has never logged in is treated as global.
// Unlike authRegion, an unreadable context is undeterminable (ok=false)
// rather than global, so callers can fail closed. Only analyticsRegions
// values are returned, never the raw persisted string.
func (a *App) analyticsRegion() (string, bool) {
	if override := strings.TrimSpace(a.explicitEnvValue("AGORA_API_BASE_URL")); override != "" {
		u, err := url.Parse(override)
		cn, cnErr := url.Parse(apiBaseURLCN)
		if err == nil && cnErr == nil && u.Hostname() != "" && strings.EqualFold(u.Hostname(), cn.Hostname()) {
			return regionCN, true
		}
	}
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
	if op.mode == analyticsModeAggregate {
		maps.DeleteFunc(props, func(key string, _ any) bool { return !slices.Contains(analyticsAggregateProperties, key) })
	}
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

// captureMode is the operation's mode; a nil operation captures nothing.
func (op *analyticsOperation) captureMode() string {
	if op == nil {
		return analyticsModeOff
	}
	return op.mode
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
		op := a.beginAnalytics(command, "shell")
		a.showTelemetryNotice(cmd.ErrOrStderr(), op.captureMode())
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
	if op == nil || op.mode == analyticsModeAggregate || !analyticsAppIDPattern.MatchString(id) || op.properties["app_id"] != nil {
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
