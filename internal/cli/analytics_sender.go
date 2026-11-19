package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

const analyticsSenderCommand = "__telemetry-send"
const analyticsRequestLimit = 64 * 1024
const analyticsHandoffTimeout = 100 * time.Millisecond

type analyticsBatch struct {
	Events []analyticsEvent `json:"events"`
}

func (op *analyticsOperation) submit(event analyticsEvent) {
	if op.batchBytes > analyticsRequestLimit || !validAnalyticsEvent(event) {
		return
	}
	body, err := json.Marshal(event)
	if err != nil {
		return
	}
	size := op.batchBytes
	if size == 0 {
		size = len(`{"events":[]}`)
	}
	if len(op.events) > 0 {
		size++ // comma between events
	}
	size += len(body)
	op.batchBytes = size
	if size > analyticsRequestLimit {
		// Drop an oversized operation as a whole rather than send a partial lifecycle.
		op.events = nil
		return
	}
	op.events = append(op.events, json.RawMessage(body))
}

func (op *analyticsOperation) flush() {
	if len(op.events) == 0 {
		return
	}
	body, err := json.Marshal(struct {
		Events []json.RawMessage `json:"events"`
	}{Events: op.events})
	op.events = nil
	if err != nil || len(body) > analyticsRequestLimit {
		return
	}
	executable, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(executable, analyticsSenderCommand, op.endpoint)
	detachAnalyticsProcess(cmd)
	cmd.Env = analyticsSenderEnv()
	pipe, err := cmd.StdinPipe()
	if err != nil {
		return
	}
	// nil stdout/stderr are connected to the null device by os/exec.
	if err := cmd.Start(); err != nil {
		_ = pipe.Close()
		return
	}
	go func() { _ = cmd.Wait() }()
	handoffAnalyticsBatch(pipe, body)
}

func analyticsSenderEnv() []string {
	// Preserve network settings without inheriting business credentials.
	env := []string{}
	for _, key := range []string{"HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY", "http_proxy", "https_proxy", "no_proxy", "SSL_CERT_FILE", "SSL_CERT_DIR"} {
		if value, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+value)
		}
	}
	return env
}

func handoffAnalyticsBatch(pipe io.WriteCloser, body []byte) {
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer func() { _ = pipe.Close() }()
		_, _ = pipe.Write(body)
	}()
	timer := time.NewTimer(analyticsHandoffTimeout)
	defer timer.Stop()
	select {
	case <-done:
	case <-timer.C:
		_ = pipe.Close()
	}
}

// RunAnalyticsSender recognizes the private entry before config/session loading.
// Its only input is an allowlisted JSON batch; it cannot execute business commands.
func RunAnalyticsSender(args []string, input io.Reader) bool {
	if len(args) == 0 || args[0] != analyticsSenderCommand {
		return false
	}
	if len(args) == 2 {
		a := &App{env: map[string]string{"AGORA_TELEMETRY_URL": args[1]}}
		if endpoint := a.analyticsURL(); endpoint != "" {
			sendAnalyticsBatch(input, endpoint)
		}
	}
	return true
}

func sendAnalyticsBatch(input io.Reader, endpoint string) {
	data, err := io.ReadAll(io.LimitReader(input, analyticsRequestLimit+1))
	if err != nil || len(data) > analyticsRequestLimit {
		return
	}
	var batch analyticsBatch
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&batch) != nil || len(batch.Events) == 0 {
		return
	}
	if decoder.Decode(new(any)) != io.EOF {
		return
	}
	for _, event := range batch.Events {
		if !validAnalyticsEvent(event) {
			return
		}
	}
	body, err := json.Marshal(batch)
	if err != nil || len(body) > analyticsRequestLimit {
		return
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: analyticsSendTimeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	req, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(body))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.GetBody = nil
	response, _ := client.Do(req)
	if response != nil {
		_ = response.Body.Close()
	}
}

// Defense in depth: never let arbitrary properties, names or raw errors enter
// the pipe, even if a future instrumentation call accidentally adds one.
func validAnalyticsEvent(e analyticsEvent) bool {
	if !analyticsIDPattern.MatchString(e.EventID) || !analyticsIDPattern.MatchString(e.OperationID) || !strings.HasPrefix(e.DistinctID, "cli:") || !analyticsIDPattern.MatchString(strings.TrimPrefix(e.DistinctID, "cli:")) {
		return false
	}
	p := e.Properties
	command, _ := p["command"].(string)
	switch command {
	case "init":
		if e.Event != "cli_init_started" && e.Event != "cli_init_stage_completed" && e.Event != "cli_init_completed" && e.Event != "cli_project_create_completed" {
			return false
		}
	case "recipes list", "recipes show", "project create":
		if e.Event != "cli_"+strings.ReplaceAll(command, " ", "_")+"_completed" {
			return false
		}
	default:
		return false
	}
	for key, value := range p {
		str, isString := value.(string)
		if key != "duration_ms" && key != "exit_code" && key != "result_count" && !isString {
			return false
		}
		switch key {
		case "command":
		case "os":
			if !analyticsPlatformPattern.MatchString(str) {
				return false
			}
		case "arch":
			if len(str) > 20 || !analyticsPlatformPattern.MatchString(str) {
				return false
			}
		case "cli_version":
			if !analyticsVersionPattern.MatchString(str) {
				return false
			}
		case "invocation_channel":
			if str != "shell" && str != "mcp" {
				return false
			}
		case "agent_family":
			if !oneOf(str, "codex", "claude-code", "kiro", "cursor", "cline", "windsurf", "aider", "custom", "unknown") {
				return false
			}
		case "agent_surface":
			if !oneOf(str, "desktop", "cli", "unknown") {
				return false
			}
		case "agent_detection_source":
			if !oneOf(str, "explicit", "environment", "none") {
				return false
			}
		case "source_type":
			if !oneOf(str, "recipe", "quickstart") {
				return false
			}
		case "source_id", "recipe_id":
			if len(str) > 100 || !analyticsSlugPattern.MatchString(str) {
				return false
			}
		case "app_id":
			if !analyticsAppIDPattern.MatchString(str) || strings.HasPrefix(command, "recipes ") {
				return false
			}
		case "project_action":
			if !oneOf(str, "created", "reused", "unknown") {
				return false
			}
		case "stage", "failed_stage":
			if !oneOf(str, "validate", "source_resolve", "project_resolve", "clone", "env_write", "binding_write", "context_write", "agent_rules_write") {
				return false
			}
		case "outcome":
			if !oneOf(str, "success", "failure", "cancelled") {
				return false
			}
		case "error_code":
			if str != "" && !analyticsCodePattern.MatchString(str) {
				return false
			}
		case "duration_ms", "exit_code", "result_count":
			var n float64
			switch v := value.(type) {
			case int:
				n = float64(v)
			case int64:
				n = float64(v)
			case float64:
				n = v
			default:
				return false
			}
			maxValue := float64(604800000)
			if key == "exit_code" {
				maxValue = 255
			}
			if key == "result_count" {
				maxValue = 1000000
			}
			if n < 0 || n > maxValue || n != float64(int64(n)) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func oneOf(value string, values ...string) bool {
	for _, v := range values {
		if value == v {
			return true
		}
	}
	return false
}
