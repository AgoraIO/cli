package cli

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptrace"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// Empty until an approved collector is deployed. Release builds may set this
// with -ldflags; local verification uses AGORA_TELEMETRY_URL.
var analyticsEndpoint = ""

// This bounds connection setup and writing, not waiting for a response.
const analyticsSendTimeout = 5 * time.Second

var analyticsIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
var analyticsSlugPattern = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)
var analyticsVersionPattern = regexp.MustCompile(`^[A-Za-z0-9.+_-]{0,64}$`)
var analyticsCodePattern = regexp.MustCompile(`^[A-Z][A-Z0-9_]{0,63}$`)

type analyticsEvent struct {
	Event       string         `json:"event"`
	EventID     string         `json:"event_id"`
	OperationID string         `json:"operation_id"`
	DistinctID  string         `json:"distinct_id"`
	Timestamp   time.Time      `json:"timestamp"`
	Properties  map[string]any `json:"properties"`
}

type analyticsOperation struct {
	app          *App
	command      string
	id           string
	distinctID   string
	properties   map[string]any
	started      time.Time
	stage        string
	stageStarted time.Time
	events       chan analyticsEvent
	done         chan struct{}
	cancel       context.CancelFunc
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

func (a *App) analyticsEnabled() bool {
	if a.analyticsOptedOut() {
		return false
	}
	cfg, valid := a.currentTelemetryConfig()
	return valid && cfg.TelemetryEnabled && cfg.TelemetryConsent == telemetryConsentGranted
}

func (a *App) analyticsOptedOut() bool {
	if a == nil || strings.TrimSpace(a.env["DO_NOT_TRACK"]) != "" || a.env["AGORA_TELEMETRY_ENABLED"] == "0" {
		return true
	}
	sentryOverride := a.env["AGORA_SENTRY_ENABLED"]
	if a.cfgState.ConfigFilePath != "" {
		// applyConfigToEnv also writes this key. Only an explicit OS override
		// should mask a preference changed since this MCP process started.
		sentryOverride = a.osEnv["AGORA_SENTRY_ENABLED"]
	}
	return sentryOverride == "0"
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
	config := defaultConfig()
	if json.Unmarshal(data, &config) != nil {
		return appConfig{}, false
	}
	config.TelemetryConsent = normalizeTelemetryConsent(config.TelemetryConsent)
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
	if !a.analyticsEnabled() || endpoint == "" {
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
	id := a.analyticsIdentity()
	if id == "" {
		return nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	op := &analyticsOperation{app: a, command: command, id: analyticsUUID(), distinctID: id, properties: props, started: time.Now(), events: make(chan analyticsEvent, 64), done: make(chan struct{}), cancel: cancel}
	go op.deliver(ctx, endpoint)
	if command == "init" {
		op.emit("cli_init_started", nil)
		op.beginStage("validate")
	}
	return op
}

func analyticsAgent(env map[string]string) (family, surface, source string) {
	surface, source = "unknown", "none"
	label := agentLabelFromOSEnv(env)
	family = "unknown"
	if label == "" {
		return
	}
	source = "environment"
	if strings.TrimSpace(env["AGORA_AGENT"]) != "" {
		source = "explicit"
	}
	switch label {
	case "codex", "claude-code", "kiro", "cursor", "cline", "windsurf", "aider":
		family = label
	default:
		family = "custom"
	}
	if family == "claude-code" {
		switch env["CLAUDE_CODE_ENTRYPOINT"] {
		case "cli":
			surface = "cli"
		case "claude-desktop":
			surface = "desktop"
		}
	}
	if family == "codex" && env["CODEX_INTERNAL_ORIGINATOR_OVERRIDE"] == "Codex Desktop" {
		surface = "desktop"
	}
	if family == "kiro" {
		if env["AWS_SDK_UA_APP_ID"] == "kiro-ide" {
			surface = "desktop"
		} else if env["KIRO_CLI_ACP_CLIENT_NAME"] == "kiro-tui" {
			surface = "cli"
		}
	}
	return
}

func (op *analyticsOperation) emit(name string, fields map[string]any) {
	if op == nil {
		return
	}
	props := make(map[string]any, len(op.properties)+len(fields))
	for k, v := range op.properties {
		props[k] = v
	}
	for k, v := range fields {
		props[k] = v
	}
	e := analyticsEvent{Event: name, EventID: analyticsUUID(), OperationID: op.id, DistinctID: op.distinctID, Timestamp: time.Now().UTC(), Properties: props}
	select {
	case op.events <- e:
	default:
	}
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
	op.emit(event, fields)
	close(op.events)
	timer := time.NewTimer(analyticsSendTimeout)
	defer timer.Stop()
	select {
	case <-op.done:
	case <-timer.C:
	}
	op.cancel()
}

func (op *analyticsOperation) deliver(ctx context.Context, endpoint string) {
	defer close(op.done)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	// Each batch gets its own HTTP/1 connection. Closing it after writing must
	// not reset an HTTP/2 stream or replay a request on a reused connection.
	transport.DisableKeepAlives = true
	transport.Protocols = new(http.Protocols)
	transport.Protocols.SetHTTP1(true)
	client := &http.Client{Transport: transport, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	defer transport.CloseIdleConnections()
	for {
		var first analyticsEvent
		select {
		case <-ctx.Done():
			return
		case e, ok := <-op.events:
			if !ok {
				return
			}
			first = e
		}
		events := []analyticsEvent{first}
	drain:
		for len(events) < 20 {
			select {
			case e, ok := <-op.events:
				if !ok {
					break drain
				}
				events = append(events, e)
			default:
				break drain
			}
		}
		if !op.app.analyticsEnabled() {
			return // Discard this operation's remaining events after opt-out.
		}
		body, err := json.Marshal(map[string]any{"events": events})
		if err != nil {
			continue
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			continue
		}
		req.Header.Set("Content-Type", "application/json")
		writeAnalyticsRequest(ctx, client, req)
	}
}

// Wait only until net/http has written the complete request, or sending fails.
// A completed write is not an acknowledgement or an ingestion confirmation.
func writeAnalyticsRequest(ctx context.Context, client *http.Client, req *http.Request) {
	ctx, cancel := context.WithTimeout(ctx, analyticsSendTimeout)
	defer cancel()
	written := make(chan struct{}, 1)
	trace := &httptrace.ClientTrace{WroteRequest: func(info httptrace.WroteRequestInfo) {
		if info.Err == nil {
			select {
			case written <- struct{}{}:
			default:
			}
		}
	}}
	req = req.WithContext(httptrace.WithClientTrace(ctx, trace))
	req.GetBody = nil // No redirects or automatic body replay.
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		response, _ := client.Do(req)
		if response != nil {
			_ = response.Body.Close()
		}
	}()
	select {
	case <-written:
	case <-finished:
	case <-ctx.Done():
	}
	cancel()
	<-finished // Join local cleanup; do not wait for the server's response.
}

func analyticsFor(cmd *cobra.Command) *analyticsOperation {
	op, _ := cmd.Context().Value(analyticsContextKey{}).(*analyticsOperation)
	return op
}

func (a *App) instrumentAnalytics(cmd *cobra.Command, command string) {
	run := cmd.RunE
	cmd.RunE = func(cmd *cobra.Command, args []string) (err error) {
		a.maybePromptTelemetryConsent(cmd)
		op := a.beginAnalytics(command, "shell")
		ctx := cmd.Context()
		cmd.SetContext(context.WithValue(ctx, analyticsContextKey{}, op))
		defer func() { op.finish(err); cmd.SetContext(ctx) }()
		return run(cmd, args)
	}
}

func firstAnalytics(ops []*analyticsOperation) *analyticsOperation {
	if len(ops) > 0 {
		return ops[0]
	}
	return nil
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
