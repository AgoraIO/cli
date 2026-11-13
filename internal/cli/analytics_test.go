package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func analyticsRecorder(t *testing.T) (*httptest.Server, func() []analyticsEvent) {
	t.Helper()
	var mu sync.Mutex
	var events []analyticsEvent
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" || r.URL.RawQuery != "" {
			t.Error("analytics leaked authentication or query parameters")
		}
		var body struct {
			Events []analyticsEvent `json:"events"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		mu.Lock()
		events = append(events, body.Events...)
		mu.Unlock()
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(server.Close)
	return server, func() []analyticsEvent { mu.Lock(); defer mu.Unlock(); return append([]analyticsEvent(nil), events...) }
}

func analyticsTestApp(t *testing.T, endpoint string) *App {
	t.Helper()
	return &App{env: map[string]string{"AGORA_HOME": t.TempDir(), "AGORA_TELEMETRY_URL": endpoint}, osEnv: map[string]string{}, cfg: defaultConfig(), httpClient: &http.Client{Timeout: time.Second}}
}

func TestAnalyticsAttributionPrivacyAndStages(t *testing.T) {
	server, read := analyticsRecorder(t)
	a := analyticsTestApp(t, server.URL)
	a.osEnv = map[string]string{"CLAUDECODE": "1", "CLAUDE_CODE_ENTRYPOINT": "cli", "ANTHROPIC_MODEL": "private-model", "SECRET_TOKEN": "private-token"}
	op := a.beginAnalytics("init", "shell", "browser-123", "session-456")
	op.source("recipe", "hotel-reception")
	op.beginStage("clone")
	op.finish(&cliError{Code: "RECIPE_NOT_FOUND", Message: "private-token /private/home/project"})
	events := read()
	if len(events) != 4 {
		t.Fatalf("want start, two stages, final; got %+v", events)
	}
	for _, e := range events {
		if e.DistinctID != "browser-123" || e.OperationID != op.id || e.Properties["source_session_id"] != "session-456" {
			t.Fatalf("bad attribution: %+v", e)
		}
	}
	final := events[len(events)-1]
	if final.Event != "cli_init_completed" || final.Properties["failed_stage"] != "clone" || final.Properties["outcome"] != "failure" || final.Properties["agent_family"] != "claude-code" {
		t.Fatalf("bad final: %+v", final)
	}
	b, _ := json.Marshal(events)
	if strings.Contains(string(b), "private-") || strings.Contains(string(b), "/private/home") {
		t.Fatalf("sensitive data leaked: %s", b)
	}
	next := a.beginAnalytics("recipes list", "shell", "", "")
	next.finish(nil)
	all := read()
	last := all[len(all)-1]
	if !strings.HasPrefix(last.DistinctID, "cli:") || last.Properties["source_session_id"] != nil || last.OperationID == op.id {
		t.Fatalf("web identity persisted: %+v", last)
	}
	info, err := os.Stat(filepath.Join(a.env["AGORA_HOME"], "analytics-id"))
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatal("anonymous ID is not private")
	}
}

func TestAnalyticsOptOutAndEndpointSafety(t *testing.T) {
	for _, key := range []string{"DO_NOT_TRACK", "AGORA_SENTRY_ENABLED", "AGORA_TELEMETRY_ENABLED", "config", "no-endpoint", "unsafe-url"} {
		t.Run(key, func(t *testing.T) {
			server, read := analyticsRecorder(t)
			a := analyticsTestApp(t, server.URL)
			switch key {
			case "config":
				a.cfg.TelemetryEnabled = false
			case "no-endpoint":
				delete(a.env, "AGORA_TELEMETRY_URL")
			case "unsafe-url":
				a.env["AGORA_TELEMETRY_URL"] = "http://external.example/events"
			case "DO_NOT_TRACK":
				a.env[key] = "any-value"
			default:
				a.env[key] = "0"
			}
			op := a.beginAnalytics("init", "shell", "browser", "session")
			if op != nil {
				op.finish(nil)
				t.Fatal("disabled transport created an operation")
			}
			if len(read()) != 0 {
				t.Fatal("opt out emitted events")
			}
			if _, err := os.Stat(filepath.Join(a.env["AGORA_HOME"], "analytics-id")); !os.IsNotExist(err) {
				t.Fatal("opt out created an identity")
			}
		})
	}
}

func TestAnalyticsMCPIsolationAndInvalidSource(t *testing.T) {
	server, read := analyticsRecorder(t)
	a := analyticsTestApp(t, server.URL)
	for _, args := range []map[string]any{{"sourceDistinctId": "browser", "sourceSessionId": "session"}, {"sourceDistinctId": "browser"}} {
		if _, err := a.callMCPTool("agora.init", args, nil); err == nil {
			t.Fatal("missing name should still fail")
		}
	}
	events := read()
	var finals []analyticsEvent
	for _, e := range events {
		if e.Event == "cli_init_completed" {
			finals = append(finals, e)
		}
	}
	if len(finals) != 2 || finals[0].OperationID == finals[1].OperationID || finals[0].DistinctID != "browser" || !strings.HasPrefix(finals[1].DistinctID, "cli:") {
		t.Fatalf("bad MCP isolation: %+v", finals)
	}
	for _, e := range finals {
		if e.Properties["invocation_channel"] != "mcp" {
			t.Fatal("wrong MCP channel")
		}
	}
}

func TestAnalyticsCommandWithoutWebContext(t *testing.T) {
	server, read := analyticsRecorder(t)
	a := analyticsTestApp(t, server.URL)
	a.root = a.buildRoot()
	var out, stderr bytes.Buffer
	a.root.SetOut(&out)
	a.root.SetErr(&stderr)
	a.root.SetArgs([]string{"recipes", "list", "--type", "invalid", "--json"})
	if err := a.root.Execute(); err == nil {
		t.Fatal("invalid type should fail")
	}
	events := read()
	if len(events) != 1 || events[0].Event != "cli_recipes_list_completed" || events[0].Properties["error_code"] != "RECIPE_TYPE_INVALID" || !strings.HasPrefix(events[0].DistinctID, "cli:") {
		t.Fatalf("bad standalone events: %+v", events)
	}
}

func TestAnalyticsNetworkFailureIsBounded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-time.After(1200 * time.Millisecond):
		}
	}))
	defer server.Close()
	a := analyticsTestApp(t, server.URL)
	start := time.Now()
	op := a.beginAnalytics("init", "shell", "", "")
	op.finish(errors.New("private error"))
	if time.Since(start) > 1500*time.Millisecond {
		t.Fatal("analytics delayed command beyond bounded flush")
	}
}

func TestAnalyticsAgentClassification(t *testing.T) {
	for _, tc := range []struct {
		env                     map[string]string
		family, surface, source string
	}{
		{map[string]string{"CLAUDECODE": "1", "CLAUDE_CODE_ENTRYPOINT": "claude-desktop"}, "claude-code", "desktop", "environment"},
		{map[string]string{"KIRO_SESSION_ID": "private", "KIRO_CLI_ACP_CLIENT_NAME": "kiro-tui"}, "kiro", "cli", "environment"},
		{map[string]string{"CODEX_THREAD_ID": "private"}, "codex", "unknown", "environment"},
		{map[string]string{"CODEX_THREAD_ID": "private", "AGORA_AGENT_DISABLE_INFER": "1"}, "unknown", "unknown", "none"},
		{map[string]string{"AGORA_AGENT": "private custom value"}, "custom", "unknown", "explicit"},
	} {
		family, surface, source := analyticsAgent(tc.env)
		if family != tc.family || surface != tc.surface || source != tc.source {
			t.Fatalf("classification: %s %s %s", family, surface, source)
		}
	}
}
