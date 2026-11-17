package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

const testTelemetryAppID = "0123456789abcdef0123456789abcdef"

func TestAnalyticsCreationSurvivesLaterFailure(t *testing.T) {
	for _, channel := range []string{"shell", "mcp"} {
		for _, source := range []string{"project create", "quickstart", "recipe"} {
			for _, failure := range []string{"features", "query"} {
				t.Run(channel+"/"+source+"/"+failure, func(t *testing.T) {
					collector, read := analyticsRecorder(t)
					a := analyticsTestApp(t, collector.URL)
					var creates atomic.Int32
					backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if r.Method == "POST" && r.URL.Path == "/api/cli/v1/projects" {
							creates.Add(1)
							time.Sleep(25 * time.Millisecond)
							_ = json.NewEncoder(w).Encode(projectDetail{AppID: testTelemetryAppID, ProjectID: "prj_created", Name: "private-project"})
							return
						}
						if failure == "features" {
							time.Sleep(120 * time.Millisecond)
						}
						w.WriteHeader(http.StatusServiceUnavailable)
						_, _ = w.Write([]byte(`{"message":"private failure"}`))
					}))
					defer backend.Close()
					a.env["AGORA_API_BASE_URL"] = backend.URL
					a.osEnv["AGORA_API_BASE_URL"] = backend.URL
					if err := saveSession(a.env, session{AccessToken: "secret", TokenType: "Bearer", ExpiresAt: time.Now().Add(time.Hour).Format(time.RFC3339)}); err != nil {
						t.Fatal(err)
					}
					command := "init"
					if source == "project create" {
						command = source
					}
					op := a.beginAnalytics(command, channel)
					features := []string{"rtc"}
					if failure == "features" {
						features = []string{"rtm"}
					}
					var err error
					if command == "project create" {
						_, err = a.projectCreate("private-project", "", features, "", "", op)
					} else {
						target := filepath.Join(t.TempDir(), "demo")
						if source == "quickstart" {
							template, selectErr := selectQuickstartDefinition("nextjs", "video-call")
							if selectErr != nil {
								t.Fatal(selectErr)
							}
							_, err = a.initProject("private-project", target, template, "", features, "", true, false, io.Discard, strings.NewReader(""), nil, op)
						} else {
							recipe := recipeDetail{recipeSummary: recipeSummary{Slug: "test-source"}, CLI: &recipeCLIConfig{ProjectType: "python", Env: recipeCLIEnv{ExamplePath: ".env.example", TargetPath: ".env", AppIDKey: "AGORA_APP_ID", AppCertificateKey: "AGORA_APP_CERTIFICATE"}}}
							_, err = a.initRecipeProject("private-project", target, recipe, "", features, "", true, false, io.Discard, strings.NewReader(""), nil, op)
						}
					}
					if command == "init" && err == nil {
						t.Fatal("fixture must fail after creation")
					}
					op.finish(err)
					events := waitAnalyticsEvents(t, read, 1)
					var creation *analyticsEvent
					for i := range events {
						e := &events[i]
						if e.Event == "cli_project_create_completed" {
							if creation != nil {
								t.Fatal("duplicate creation")
							}
							creation = e
						}
						if e.Event == "cli_init_started" && e.Properties["app_id"] != nil {
							t.Fatal("backfilled early event")
						}
						if e.Event == "cli_init_completed" && (e.Properties["outcome"] != "failure" || e.Properties["app_id"] != testTelemetryAppID) {
							t.Fatalf("lost outer failure/App ID: %+v", e)
						}
					}
					if creates.Load() != 1 || creation == nil || creation.Properties["outcome"] != "success" || creation.Properties["exit_code"] != float64(0) || creation.Properties["app_id"] != testTelemetryAppID {
						t.Fatalf("wrong creation %+v", events)
					}
					duration := creation.Properties["duration_ms"].(float64)
					if duration < 20 || duration > 115 {
						t.Fatalf("creation timing includes later feature work: %v", duration)
					}
				})
			}
		}
	}
}

func TestAnalyticsCreationAuthFailureAndInvalidAppID(t *testing.T) {
	collector, read := analyticsRecorder(t)
	a := analyticsTestApp(t, collector.URL)
	a.env["AGORA_OUTPUT"] = "json"
	op := a.beginAnalytics("project create", "mcp")
	if _, err := a.createProject("private", "", op); err == nil {
		t.Fatal("expected auth failure")
	}
	op.finish(nil)
	events := waitAnalyticsEvents(t, read, 1)
	if len(events) != 1 || events[0].Properties["outcome"] != "failure" || events[0].Properties["app_id"] != nil {
		t.Fatal(events)
	}
	for _, id := range []string{"private name", "app_0001", "", "secret-token"} {
		op.setAppID(id)
	}
	if op.properties["app_id"] != nil {
		t.Fatal("accepted free text app ID")
	}
}

func TestAnalyticsStreamingBeyondThreeSecondsAndLimits(t *testing.T) {
	server, read := analyticsRecorder(t)
	a := analyticsTestApp(t, server.URL)
	op := a.beginAnalytics("init", "mcp")
	deadline := time.Now().Add(time.Second)
	for len(read()) < 1 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	if len(read()) != 1 {
		t.Fatal("start not received before completion")
	}
	time.Sleep(3100 * time.Millisecond)
	op.beginStage("clone")
	op.finish(nil)
	events := waitAnalyticsEvents(t, read, 4)
	if len(events) != 4 {
		t.Fatalf("sender expired while operation open: %d", len(events))
	}

	// Both active child slots remain in use until EOF/network completion.
	first := a.beginAnalytics("init", "mcp")
	second := a.beginAnalytics("init", "mcp")
	third := a.beginAnalytics("init", "mcp")
	if first.sender == nil || second.sender == nil || third.sender != nil || !third.attempted {
		t.Fatal("child limit not enforced")
	}
	first.finish(nil)
	second.finish(nil)
	deadline = time.Now().Add(time.Second)
	for analyticsChildren.Load() != 0 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	third.beginStage("clone")
	if third.sender != nil {
		t.Fatal("full operation retried child launch")
	}
	third.finish(nil)
}

func TestAnalyticsSlowPipeAndOverflow(t *testing.T) {
	reader, writer := io.Pipe()
	defer func() { _ = reader.Close() }()
	sender := &analyticsPipe{events: make(chan []byte, analyticsQueueSize), done: make(chan struct{}), pipe: writer}
	go func() {
		defer close(sender.done)
		for body := range sender.events {
			if _, err := writer.Write(body); err != nil {
				return
			}
		}
	}()
	op := &analyticsOperation{attempted: true, sender: sender, properties: map[string]any{"command": "recipes list"}, id: analyticsUUID(), distinctID: "cli:" + analyticsUUID()}
	started := time.Now()
	for range 1000 {
		op.emit("cli_recipes_list_completed", nil)
	}
	if len(sender.events) > 20 {
		t.Fatal("queue exceeded limit")
	}
	sender.close()
	if time.Since(started) > 500*time.Millisecond {
		t.Fatal("blocked on non-reading pipe")
	}
	select {
	case <-sender.done:
	case <-time.After(time.Second):
		t.Fatal("writer not released")
	}
}

func TestAnalyticsStreamRejectsOversizeAndSecretsIndividually(t *testing.T) {
	server, read := analyticsRecorder(t)
	a := analyticsTestApp(t, server.URL)
	op := a.beginAnalytics("recipes list", "shell")
	// Build valid, maximum-length allowlisted fields, then an oversized record.
	event := analyticsEvent{Event: "cli_recipes_list_completed", EventID: analyticsUUID(), OperationID: op.id, DistinctID: op.distinctID, Timestamp: time.Now().UTC(), Properties: op.properties}
	event.Properties["source_type"] = "recipe"
	event.Properties["source_id"] = strings.Repeat("a", 100)
	event.Properties["recipe_id"] = strings.Repeat("a", 100)
	event.Properties["cli_version"] = strings.Repeat("v", 64)
	event.Properties["error_code"] = strings.Repeat("A", 64)
	body, _ := json.Marshal(event)
	if len(body) > 4096 {
		t.Fatal("legal event exceeds contract")
	}
	input := append([]byte(strings.Repeat("x", 5000)+"\n"), body...)
	input = append(input, '\n')
	private := bytes.Replace(body, []byte(`"properties":{`), []byte(`"properties":{"token":"private",`), 1)
	input = append(input, private...)
	input = append(input, '\n')
	sendAnalyticsStream(bytes.NewReader(input), server.URL)
	if len(read()) != 1 {
		t.Fatalf("invalid event affected legal sibling: %+v", read())
	}
	op.finish(nil)
	waitAnalyticsEvents(t, read, 2)
}

func TestAnalyticsSenderSurvivesParentExit(t *testing.T) {
	received := make(chan analyticsEvent, 4)
	allowResponse := make(chan struct{}, 1)
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Events []analyticsEvent `json:"events"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
			return
		}
		for _, e := range body.Events {
			received <- e
		}
		if calls.Add(1) == 1 {
			<-allowResponse
		}
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	defer func() {
		select {
		case allowResponse <- struct{}{}:
		default:
		}
	}()
	home := t.TempDir()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(executable)
	cmd.Env = append(os.Environ(), "GO_WANT_CLI_HELPER_PROCESS=1", `GO_CLI_HELPER_ARGS_JSON=["init","local-only","--template","invalid","--json"]`, "AGORA_HOME="+home, "AGORA_TELEMETRY_URL="+server.URL, "AGORA_SENTRY_ENABLED=1", "AGORA_TELEMETRY_ENABLED=1", "DO_NOT_TRACK=")
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() { _, err := cmd.CombinedOutput(); done <- err }()
	select {
	case <-done:
	case <-ctx.Done():
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		t.Fatal("child retained output handles")
	}
	// The parent has exited while its sender is still blocked on the first
	// response. Only now allow the child to post its queued stage/final events.
	allowResponse <- struct{}{}
	events := make([]analyticsEvent, 0, 4)
	for range 4 {
		select {
		case e := <-received:
			events = append(events, e)
		case <-time.After(2 * time.Second):
			t.Fatal("child did not finish after parent exit")
		}
	}
	if events[0].Event != "cli_init_started" || events[3].Event != "cli_init_completed" {
		t.Fatalf("missing detached lifecycle: %+v", events)
	}
	if _, err := os.Stat(filepath.Join(home, "telemetry-notice-shown")); err != nil {
		t.Fatal(err)
	}
}

func TestAnalyticsCreationTimingIncludesRefreshAndRetry(t *testing.T) {
	collector, read := analyticsRecorder(t)
	a := analyticsTestApp(t, collector.URL)
	var requests, refreshes atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(30 * time.Millisecond)
		if r.URL.Path == "/api/v0/oauth/token" {
			refreshes.Add(1)
			_, _ = w.Write([]byte(`{"access_token":"local","token_type":"Bearer","expires_in":3600,"refresh_token":"local","scope":"basic_info,console"}`))
			return
		}
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(projectDetail{AppID: testTelemetryAppID})
	}))
	defer backend.Close()
	a.osEnv["AGORA_API_BASE_URL"] = backend.URL
	a.osEnv["AGORA_OAUTH_BASE_URL"] = backend.URL
	if err := saveSession(a.env, session{AccessToken: "local", RefreshToken: "local", TokenType: "Bearer", ExpiresAt: time.Now().Add(-time.Hour).Format(time.RFC3339)}); err != nil {
		t.Fatal(err)
	}
	op := a.beginAnalytics("project create", "shell")
	_, err := a.createProject("local", "", op)
	op.finish(err)
	if err != nil {
		t.Fatal(err)
	}
	events := waitAnalyticsEvents(t, read, 1)
	if len(events) != 1 || requests.Load() != 2 || refreshes.Load() != 2 || events[0].Properties["duration_ms"].(float64) < 110 {
		t.Fatalf("refresh/retry not in shared boundary: %+v requests=%d refreshes=%d", events, requests.Load(), refreshes.Load())
	}
}

func TestAnalyticsCLIStartupAndMCPReadFailure(t *testing.T) {
	for _, invalid := range []string{"{invalid", "null", `{"telemetryEnabled":null}`, `{"telemetryEnabled":"true"}`} {
		t.Run(invalid, func(t *testing.T) {
			server, read := analyticsRecorder(t)
			a := analyticsTestApp(t, server.URL)
			state, err := ensureAppConfigState(a.env)
			if err != nil {
				t.Fatal(err)
			}
			a.cfgState = state
			if err := os.WriteFile(state.ConfigFilePath, []byte(invalid), 0o600); err != nil {
				t.Fatal(err)
			}
			if op := a.beginAnalytics("recipes list", "mcp"); op != nil {
				op.finish(nil)
				t.Fatal("MCP used unreadable preference")
			}
			// Normal CLI deliberately retains the startup policy despite later edits.
			op := a.beginAnalytics("recipes list", "shell")
			if op == nil {
				t.Fatal("CLI re-read startup preference")
			}
			op.finish(nil)
			waitAnalyticsEvents(t, read, 1)
		})
	}
}

func TestAnalyticsRequestAndEOFBudgets(t *testing.T) {
	received := make(chan time.Time, 3)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(io.Discard, r.Body)
		received <- time.Now()
		<-release
	}))
	defer server.Close()
	defer close(release)
	event := analyticsEvent{Event: "cli_recipes_list_completed", EventID: analyticsUUID(), OperationID: analyticsUUID(), DistinctID: "cli:" + analyticsUUID(), Timestamp: time.Now().UTC(), Properties: map[string]any{"command": "recipes list"}}
	body, _ := json.Marshal(event)
	input, writer := io.Pipe()
	done := make(chan struct{})
	go func() { sendAnalyticsStream(input, server.URL); _ = input.Close(); close(done) }()
	for range 2 {
		if _, err := writer.Write(append(bytes.Clone(body), '\n')); err != nil {
			t.Fatal(err)
		}
	}
	var first, second time.Time
	select {
	case first = <-received:
	case <-time.After(time.Second):
		t.Fatal("first request missing")
	}
	select {
	case second = <-received:
	case <-time.After(4 * time.Second):
		t.Fatal("request timeout or continued reading failed")
	}
	if second.Sub(first) < 2500*time.Millisecond {
		t.Fatal("request fixture did not stall")
	}
	// An open input does not impose a lifetime deadline; after EOF the second
	// stalled request must finish within the independent tail budget.
	_ = writer.Close()
	started := time.Now()
	select {
	case <-done:
	case <-time.After(3500 * time.Millisecond):
		t.Fatal("EOF drain exceeded budget")
	}
	if time.Since(started) > 3400*time.Millisecond {
		t.Fatal("EOF tail was unbounded")
	}
}
