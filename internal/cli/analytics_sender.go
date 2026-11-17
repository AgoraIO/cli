package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"sync/atomic"
	"time"
)

const analyticsSenderCommand = "__telemetry-send"
const analyticsQueueSize = 20
const analyticsEventLimit = 4096
const analyticsHandoffTimeout = 100 * time.Millisecond

// Process-wide, including multiple App instances in an embedding host.
var analyticsChildren atomic.Int32

type analyticsPipe struct {
	events chan []byte
	done   chan struct{}
	pipe   io.WriteCloser
}

func (op *analyticsOperation) submit(event analyticsEvent) {
	body, err := json.Marshal(event)
	if err != nil || len(body) > analyticsEventLimit || !validAnalyticsEvent(event) {
		return
	}
	if !op.attempted {
		op.attempted = true
		op.sender = startAnalyticsPipe(op.endpoint)
	}
	if op.sender == nil {
		return
	}
	select {
	case op.sender.events <- body:
	default:
	}
}

func startAnalyticsPipe(endpoint string) *analyticsPipe {
	for {
		n := analyticsChildren.Load()
		if n >= 2 {
			return nil
		}
		if analyticsChildren.CompareAndSwap(n, n+1) {
			break
		}
	}
	release := func() { analyticsChildren.Add(-1) }
	executable, err := os.Executable()
	if err != nil {
		release()
		return nil
	}
	cmd := exec.Command(executable, analyticsSenderCommand, endpoint)
	detachAnalyticsProcess(cmd)
	// An explicit empty environment avoids passing business authentication to the child.
	cmd.Env = []string{}
	pipe, err := cmd.StdinPipe()
	if err != nil {
		release()
		return nil
	}
	// nil stdout/stderr are connected to the null device by os/exec.
	if err := cmd.Start(); err != nil {
		_ = pipe.Close()
		release()
		return nil
	}
	go func() { _ = cmd.Wait(); release() }()
	sender := &analyticsPipe{events: make(chan []byte, analyticsQueueSize), done: make(chan struct{}), pipe: pipe}
	go func() {
		defer close(sender.done)
		defer func() { _ = pipe.Close() }()
		for body := range sender.events {
			if _, err := pipe.Write(append(body, '\n')); err != nil {
				return
			}
		}
	}()
	return sender
}

func (s *analyticsPipe) close() {
	close(s.events)
	timer := time.NewTimer(analyticsHandoffTimeout)
	defer timer.Stop()
	select {
	case <-s.done:
	case <-timer.C:
		// Closing an os.File pipe interrupts a blocked writer on supported platforms.
		_ = s.pipe.Close()
	}
}

// RunAnalyticsSender recognizes the private entry before config/session loading.
// Its only input is allowlisted NDJSON; it cannot execute business commands.
func RunAnalyticsSender(args []string, input io.Reader) bool {
	if len(args) == 0 || args[0] != analyticsSenderCommand {
		return false
	}
	if len(args) == 2 {
		a := &App{env: map[string]string{"AGORA_TELEMETRY_URL": args[1]}}
		if endpoint := a.analyticsURL(); endpoint != "" {
			sendAnalyticsStream(input, endpoint)
		}
	}
	return true
}

func sendAnalyticsStream(input io.Reader, endpoint string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	events := make(chan []byte, analyticsQueueSize)
	go func() {
		// ReadSlice lets us discard an oversized line and continue with later events.
		reader := bufio.NewReaderSize(input, analyticsEventLimit+1)
		for {
			line, err := reader.ReadSlice('\n')
			oversized := errors.Is(err, bufio.ErrBufferFull)
			if oversized {
				for errors.Is(err, bufio.ErrBufferFull) {
					_, err = reader.ReadSlice('\n')
				}
			} else if err == nil && len(line)-1 <= analyticsEventLimit {
				var event analyticsEvent
				decoder := json.NewDecoder(bytes.NewReader(line))
				decoder.DisallowUnknownFields()
				if decoder.Decode(&event) == nil && validAnalyticsEvent(event) {
					canonical, marshalErr := json.Marshal(event)
					if marshalErr != nil {
						continue
					}
					select {
					case events <- canonical:
					default:
					}
				}
			}
			if err != nil {
				// Start the tail budget on EOF, including any HTTP already in flight.
				time.AfterFunc(analyticsSendTimeout, cancel)
				close(events)
				return
			}
		}
	}()
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.DisableKeepAlives = true
	transport.Proxy = nil
	transport.Protocols = new(http.Protocols)
	transport.Protocols.SetHTTP1(true)
	defer transport.CloseIdleConnections()
	client := &http.Client{Transport: transport, Timeout: analyticsSendTimeout, CheckRedirect: func(_ *http.Request, _ []*http.Request) error { return http.ErrUseLastResponse }}
	for {
		select {
		case <-ctx.Done():
			return
		case event, ok := <-events:
			if !ok {
				return
			}
			body := append([]byte(`{"events":[`), event...)
			body = append(body, ']', '}')
			req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
			if err != nil {
				continue
			}
			req.Header.Set("Content-Type", "application/json")
			req.GetBody = nil
			response, _ := client.Do(req)
			if response != nil {
				_ = response.Body.Close()
			}
		}
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
