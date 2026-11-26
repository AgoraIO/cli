package cli

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"slices"
	"strings"
	"sync"
	"time"
)

const analyticsSenderCommand = "__telemetry-send"
const analyticsRequestLimit = 64 * 1024
const analyticsHandoffTimeout = 100 * time.Millisecond

// analyticsMCPDrainTimeout bounds how long `mcp serve` waits for in-flight
// usage batches after the host closes stdin.
const analyticsMCPDrainTimeout = time.Second

const analyticsBatchEnvelope = `{"events":[]}`

// analyticsStartAttributionReserve is the largest JSON growth of
// cli_init_started when flush copies source attribution onto it: the longest
// source_type value plus source_id and recipe_id at their 100-byte maximum.
var analyticsStartAttributionReserve = len(`,"source_type":"quickstart","source_id":"` + strings.Repeat("x", 100) + `","recipe_id":"` + strings.Repeat("x", 100) + `"`)

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
	// batchBytes starts at the envelope plus any reserved cli_init_started
	// event, so an accepted event always fits once flush prepends the start.
	size := op.batchBytes
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

// flush hands the finished operation's batch to a sender. Shell commands
// exit right after finishing, so they hand off to a detached subprocess that
// outlives them. `mcp serve` is long-lived, so it sends from a goroutine in
// process instead of spawning one subprocess per tool call. Both paths use
// sendAnalyticsBatch, so validation, limits and the HTTP client are shared.
func (op *analyticsOperation) flush() {
	body := op.batchBody()
	if body == nil {
		return
	}
	if op.channel == "mcp" {
		sendAnalyticsInBackground(body, op.endpoint)
		return
	}
	spawnAnalyticsSender(body, op.endpoint)
}

// batchBody returns the serialized batch for the finished operation, or nil
// when there is nothing valid to send. It clears the buffered events.
func (op *analyticsOperation) batchBody() []byte {
	if len(op.events) == 0 {
		return nil
	}
	events := op.events
	if start := op.initStarted; start != nil {
		// The start keeps its original timestamp. Only source attribution can
		// be added later, after resolving the public catalog entry.
		for _, key := range []string{"source_type", "source_id", "recipe_id"} {
			if value, ok := op.properties[key]; ok {
				start.Properties[key] = value
			}
		}
		if !validAnalyticsEvent(*start) {
			op.events = nil
			return nil
		}
		first, err := json.Marshal(start)
		if err != nil {
			op.events = nil
			return nil
		}
		events = append([]json.RawMessage{first}, events...)
	}
	body, err := json.Marshal(struct {
		Events []json.RawMessage `json:"events"`
	}{Events: events})
	op.events = nil
	if err != nil || len(body) > analyticsRequestLimit {
		return nil
	}
	return body
}

// analyticsInFlight tracks in-process MCP sends so `mcp serve` can give them
// a short, bounded chance to finish when the host closes stdin.
var analyticsInFlight sync.WaitGroup

func sendAnalyticsInBackground(body []byte, endpoint string) {
	analyticsInFlight.Add(1)
	go func() {
		defer analyticsInFlight.Done()
		sendAnalyticsBatch(bytes.NewReader(body), endpoint)
	}()
}

// drainAnalyticsSends waits up to timeout for in-process sends to finish and
// reports whether they all did. Requests still running are abandoned when the
// process exits.
func drainAnalyticsSends(timeout time.Duration) bool {
	done := make(chan struct{})
	go func() { analyticsInFlight.Wait(); close(done) }()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-done:
		return true
	case <-timer.C:
		return false
	}
}

func spawnAnalyticsSender(body []byte, endpoint string) {
	executable, err := os.Executable()
	if err != nil {
		return
	}
	cmd := exec.Command(executable, analyticsSenderCommand, endpoint)
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
		// The parent picked the operation's regional collector (global or
		// CN); analyticsURL only re-validates it with the shared URL rules.
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
			if !slices.Contains(analyticsAgentFamilies, str) {
				return false
			}
		case "agent_surface":
			if !slices.Contains(analyticsAgentSurfaces, str) {
				return false
			}
		case "agent_detection_source":
			if !slices.Contains(analyticsDetectionSources, str) {
				return false
			}
		case "region":
			if !slices.Contains(analyticsRegions, str) {
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
			if !slices.Contains(analyticsStages, str) {
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
