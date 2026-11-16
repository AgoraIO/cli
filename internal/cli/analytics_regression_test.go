package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

func TestAnalyticsLiveMCPPreference(t *testing.T) {
	server, read := analyticsRecorder(t)
	a := analyticsTestApp(t, server.URL)
	state, err := ensureAppConfigState(a.env)
	if err != nil {
		t.Fatal(err)
	}
	a.cfg, a.cfgState = state.Config, state
	a.applyConfigToEnv()
	if _, err := a.callMCPTool("agora.init", map[string]any{}, nil); err == nil {
		t.Fatal("missing name must fail")
	}
	before := len(waitAnalyticsEvents(t, read, 3))
	if before == 0 {
		t.Fatal("expected initial capture")
	}
	cfg := state.Config
	cfg.TelemetryEnabled = false
	if err := writeSecureJSON(state.ConfigFilePath, cfg); err != nil {
		t.Fatal(err)
	}
	if _, err := a.callMCPTool("agora.init", map[string]any{}, nil); err == nil {
		t.Fatal("business validation must still fail")
	}
	if len(read()) != before {
		t.Fatal("existing MCP continued sending after persisted opt-out")
	}
	status, err := a.callMCPTool("agora.telemetry.status", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if status.(map[string]any)["captureActive"] != false {
		t.Fatal("MCP status uses stale preference")
	}
	cfg.TelemetryEnabled = true
	if err := writeSecureJSON(state.ConfigFilePath, cfg); err != nil {
		t.Fatal(err)
	}
	op := a.beginAnalytics("recipes list", "mcp")
	if op == nil {
		t.Fatal("persisted re-enable did not resume capture")
	}
	op.finish(nil)
}

func TestAnalyticsOptOutBeforeNextBatch(t *testing.T) {
	first := make(chan struct{})
	release := make(chan struct{})
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		if calls.Add(1) == 1 {
			close(first)
			<-release
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	a := analyticsTestApp(t, server.URL)
	state, err := ensureAppConfigState(a.env)
	if err != nil {
		t.Fatal(err)
	}
	op := a.beginAnalytics("init", "mcp")
	select {
	case <-first:
	case <-time.After(time.Second):
		t.Fatal("initial batch not sent")
	}
	cfg := state.Config
	cfg.TelemetryEnabled = false
	if err := writeSecureJSON(state.ConfigFilePath, cfg); err != nil {
		t.Fatal(err)
	}
	op.beginStage("clone")
	close(release)
	op.finish(nil)
	if calls.Load() != 1 {
		t.Fatalf("queued batches sent after opt-out: %d", calls.Load())
	}
}

func TestAnalyticsEOFIsFailure(t *testing.T) {
	for _, err := range []error{io.EOF, io.ErrUnexpectedEOF, fmt.Errorf("API decode: %w", io.EOF), &url.Error{Op: "Get", URL: "https://example.invalid", Err: io.EOF}} {
		outcome, _ := analyticsOutcome(err)
		if outcome != "failure" {
			t.Errorf("backend disconnect %T classified as %s", err, outcome)
		}
	}
	for _, err := range []error{context.Canceled, fmt.Errorf("interrupted: %w", context.Canceled), &cliError{Code: "INIT_ABORTED"}} {
		outcome, _ := analyticsOutcome(err)
		if outcome != "cancelled" {
			t.Errorf("explicit cancellation classified as %s", outcome)
		}
	}
}

func TestAnalyticsBackendEOFRecordedAsFailure(t *testing.T) {
	collector, read := analyticsRecorder(t)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(1000))
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schemaVersion":`))
	}))
	defer backend.Close()
	a := analyticsTestApp(t, collector.URL)
	a.env["AGORA_RECIPES_BASE_URL"] = backend.URL
	_, err := a.callMCPTool("agora.recipes.list", map[string]any{}, nil)
	if err == nil {
		t.Fatal("truncated backend response must fail")
	}
	events := waitAnalyticsEvents(t, read, 1)
	if len(events) != 1 {
		b, _ := json.Marshal(events)
		t.Fatalf("unexpected events: %s", b)
	}
	if events[0].Properties["outcome"] != "failure" {
		t.Fatalf("backend disconnect counted as %v", events[0].Properties["outcome"])
	}
}

func TestAnalyticsLiveConfigFailsClosedAndHonorsEnvironment(t *testing.T) {
	for _, explicit := range []bool{false, true} {
		t.Run(strconv.FormatBool(explicit), func(t *testing.T) {
			server, _ := analyticsRecorder(t)
			a := analyticsTestApp(t, server.URL)
			state, err := ensureAppConfigState(a.env)
			if err != nil {
				t.Fatal(err)
			}
			a.cfgState = state
			a.cfg.TelemetryEnabled = false
			a.applyConfigToEnv() // Simulate starting MCP with disabled config.
			if explicit {
				a.osEnv["AGORA_SENTRY_ENABLED"] = "0"
			}
			cfg := state.Config
			cfg.TelemetryEnabled = true
			if err := writeSecureJSON(state.ConfigFilePath, cfg); err != nil {
				t.Fatal(err)
			}
			if a.analyticsEnabled() != !explicit {
				t.Fatal("persisted enable or explicit opt-out ignored")
			}
			if err := os.WriteFile(state.ConfigFilePath, []byte("{invalid"), 0o600); err != nil {
				t.Fatal(err)
			}
			if a.analyticsEnabled() {
				t.Fatal("unreadable preference must fail closed")
			}
		})
	}
}
