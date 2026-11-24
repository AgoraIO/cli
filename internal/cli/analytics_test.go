package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
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

	a := &App{env: map[string]string{"AGORA_HOME": t.TempDir(), "AGORA_TELEMETRY_URL": endpoint}, osEnv: map[string]string{}, cfg: defaultConfig(), httpClient: &http.Client{Timeout: time.Second}}
	if err := a.persistTelemetryChoice(true); err != nil {
		t.Fatal(err)
	}
	return a
}

// Sender completion no longer acknowledges server receipt. Wait on the test
// collector explicitly when an assertion needs to inspect received payloads.
func waitAnalyticsEvents(t *testing.T, read func() []analyticsEvent, count int) []analyticsEvent {
	t.Helper()
	return waitAnalytics(t, read, func(events []analyticsEvent) bool { return len(events) >= count })
}

func waitAnalytics(t *testing.T, read func() []analyticsEvent, ready func([]analyticsEvent) bool) []analyticsEvent {
	t.Helper()
	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()
	tick := time.NewTicker(time.Millisecond)
	defer tick.Stop()
	for {
		events := read()
		if ready(events) {
			return events
		}
		select {
		case <-tick.C:
		case <-deadline.C:
			t.Fatalf("collector did not receive the expected events: %+v", events)
		}
	}
}

func TestAnalyticsInstallationPrivacyAndStages(t *testing.T) {
	server, read := analyticsRecorder(t)
	a := analyticsTestApp(t, server.URL)
	a.osEnv = map[string]string{"CLAUDECODE": "1", "CLAUDE_CODE_ENTRYPOINT": "cli", "ANTHROPIC_MODEL": "private-model", "SECRET_TOKEN": "private-token"}
	op := a.beginAnalytics("init", "shell")
	op.source("recipe", "hotel-reception")
	op.beginStage("clone")
	op.finish(&cliError{Code: "RECIPE_NOT_FOUND", Message: "private-token /private/home/project"})
	events := waitAnalyticsEvents(t, read, 4)
	if len(events) != 4 {
		t.Fatalf("want start, two stages, final; got %+v", events)
	}
	for _, e := range events {
		if !strings.HasPrefix(e.DistinctID, "cli:") || e.OperationID != op.id || e.Properties["source_session_id"] != nil {
			t.Fatalf("bad installation identity: %+v", e)
		}
	}
	var final analyticsEvent
	for _, event := range events {
		if event.Event == "cli_init_completed" {
			final = event
		}
	}
	if final.Event != "cli_init_completed" || final.Properties["failed_stage"] != "clone" || final.Properties["outcome"] != "failure" || final.Properties["agent_family"] != "claude-code" {
		t.Fatalf("bad final: %+v", final)
	}
	b, _ := json.Marshal(events)
	if strings.Contains(string(b), "private-") || strings.Contains(string(b), "/private/home") {
		t.Fatalf("sensitive data leaked: %s", b)
	}
	next := a.beginAnalytics("recipes list", "shell")
	next.finish(nil)
	all := waitAnalyticsEvents(t, read, 5)
	last := all[len(all)-1]
	if last.DistinctID != events[0].DistinctID || last.Properties["source_session_id"] != nil || last.OperationID == op.id {
		t.Fatalf("installation identity or operation isolation changed: %+v", last)
	}
	info, err := os.Stat(filepath.Join(a.env["AGORA_HOME"], "analytics-id"))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatal("anonymous ID is not private")
	}
}

func TestAnalyticsRecipeInitAttributionAndLocalValidation(t *testing.T) {
	target := t.TempDir()
	for _, channel := range []string{"shell", "mcp"} {
		for _, lookupFailed := range []bool{false, true} {
			scenario := "existing target"
			if lookupFailed {
				scenario = "recipe API failure"
			}
			t.Run(channel+"/"+scenario, func(t *testing.T) {
				collector, read := analyticsRecorder(t)
				api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
					if lookupFailed {
						w.WriteHeader(http.StatusServiceUnavailable)
						return
					}
					_, _ = io.WriteString(w, `{"schemaVersion":1,"recipe":{"slug":"hotel","title":"Hotel","mainRepoUrl":"https://github.com/AgoraIO/hotel","recipeRawUrl":"https://example.com/RECIPE.md","type":"ai","official":true,"cli":{"projectType":"python","env":{"examplePath":".env.example","targetPath":".env","appIdKey":"AGORA_APP_ID","appCertificateKey":"AGORA_APP_CERTIFICATE"}}}}`)
				}))
				defer api.Close()
				a := analyticsTestApp(t, collector.URL)
				a.env["AGORA_RECIPES_BASE_URL"] = api.URL

				var err error
				if channel == "shell" {
					a.root = a.buildRoot()
					a.root.SetOut(io.Discard)
					a.root.SetErr(io.Discard)
					a.root.SetArgs([]string{"init", "demo", "--recipe", "hotel", "--dir", target, "--json"})
					err = a.root.Execute()
				} else {
					_, err = a.callMCPTool("agora.init", map[string]any{"name": "demo", "recipe": "hotel", "dir": target}, nil)
				}
				wantStage, wantCode, wantID := "local_validate", "QUICKSTART_TARGET_EXISTS", any("hotel")
				if lookupFailed {
					wantStage, wantCode, wantID = "source_resolve", "RECIPE_API_FAILED", nil
				}
				var businessErr *cliError
				if !errors.As(err, &businessErr) || businessErr.Code != wantCode {
					t.Fatalf("expected %s, got %v", wantCode, err)
				}

				events := waitAnalytics(t, read, func(events []analyticsEvent) bool {
					return len(events) > 0 && events[len(events)-1].Event == "cli_init_completed"
				})
				first, last := events[0], events[len(events)-1]
				if first.Event != "cli_init_started" || first.OperationID != last.OperationID {
					t.Fatalf("unexpected init lifecycle: %+v", events)
				}
				for _, event := range []analyticsEvent{first, last} {
					p := event.Properties
					if p["source_type"] != "recipe" || p["source_id"] != wantID || p["recipe_id"] != wantID {
						t.Errorf("unexpected recipe attribution for %s: %v", event.Event, p)
					}
				}
				if last.Properties["failed_stage"] != wantStage || last.Properties["error_code"] != wantCode {
					t.Errorf("unexpected failure: %v", last.Properties)
				}
				if !lookupFailed && !slices.ContainsFunc(events, func(e analyticsEvent) bool {
					return e.Properties["stage"] == "source_resolve" && e.Properties["outcome"] == "success"
				}) {
					t.Error("expected source resolution to succeed before local validation")
				}
			})
		}
	}
}

func TestAnalyticsOptOutAndEndpointSafety(t *testing.T) {
	for _, key := range []string{"DO_NOT_TRACK", "AGORA_SENTRY_ENABLED", "AGORA_TELEMETRY_ENABLED", "config", "no-endpoint", "unsafe-url"} {
		t.Run(key, func(t *testing.T) {
			server, read := analyticsRecorder(t)
			a := analyticsTestApp(t, server.URL)
			switch key {
			case "config":
				if err := a.persistTelemetryChoice(false); err != nil {
					t.Fatal(err)
				}
			case "no-endpoint":
				delete(a.env, "AGORA_TELEMETRY_URL")
			case "unsafe-url":
				a.env["AGORA_TELEMETRY_URL"] = "http://external.example/events"
			case "DO_NOT_TRACK":
				a.env[key] = "any-value"
			default:
				a.env[key] = "0"
			}
			op := a.beginAnalytics("init", "shell")
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

func TestAnalyticsMCPIgnoresLegacyWebIdentity(t *testing.T) {
	server, read := analyticsRecorder(t)
	a := analyticsTestApp(t, server.URL)
	for _, args := range []map[string]any{{"sourceDistinctId": "browser", "sourceSessionId": "session"}, {"sourceDistinctId": "browser"}} {
		if _, err := a.callMCPTool("agora.init", args, nil); err == nil {
			t.Fatal("missing name should still fail")
		}
	}
	events := waitAnalyticsEvents(t, read, 6)
	var finals []analyticsEvent
	for _, e := range events {
		if e.Event == "cli_init_completed" {
			finals = append(finals, e)
		}
	}
	slices.SortFunc(finals, func(a, b analyticsEvent) int { return a.Timestamp.Compare(b.Timestamp) })
	if len(finals) != 2 || finals[0].OperationID == finals[1].OperationID || !strings.HasPrefix(finals[0].DistinctID, "cli:") || finals[0].DistinctID != finals[1].DistinctID {
		t.Fatalf("bad MCP isolation: %+v", finals)
	}
	for _, e := range finals {
		if e.Properties["invocation_channel"] != "mcp" || e.Properties["source_session_id"] != nil {
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
	events := waitAnalyticsEvents(t, read, 1)
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
	op := a.beginAnalytics("init", "shell")
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

func TestAnalyticsWebIdentityInputsRemoved(t *testing.T) {
	for _, command := range [][]string{{"init"}, {"recipes", "list"}, {"recipes", "show"}} {
		for _, flag := range []string{"source-distinct-id", "source-session-id"} {
			a := analyticsTestApp(t, "")
			root := a.buildRoot()
			cmd, _, err := root.Find(command)
			if err != nil {
				t.Fatal(err)
			}
			if cmd.Flags().Lookup(flag) != nil {
				t.Fatalf("%v still accepts %s", command, flag)
			}
			root.SetOut(&bytes.Buffer{})
			root.SetErr(&bytes.Buffer{})
			root.SetArgs(append(append([]string{}, command...), "--"+flag, "legacy-web-id"))
			if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "unknown flag") {
				t.Fatalf("legacy web flag was not rejected: %v", err)
			}
		}
	}
	for _, tool := range mcpTools() {
		schema, _ := tool["inputSchema"].(map[string]any)
		properties, _ := schema["properties"].(map[string]any)
		for _, key := range []string{"sourceDistinctId", "sourceSessionId"} {
			if _, exists := properties[key]; exists {
				t.Fatalf("MCP tool %v still advertises %s", tool["name"], key)
			}
		}
	}
}

func TestAnalyticsProjectCreateFailureBeforeAPIIsReported(t *testing.T) {
	for _, tc := range []struct {
		name    string
		channel string
		args    []string
		mcpArgs map[string]any
		setup   func(t *testing.T, a *App)
	}{
		{name: "invalid feature", channel: "shell", args: []string{"project", "create", "demo", "--feature", "bogus", "--json"}},
		{name: "invalid rtm data center", channel: "shell", args: []string{"project", "create", "demo", "--rtm-data-center", "XX", "--json"}},
		{name: "unreadable context", channel: "shell", args: []string{"project", "create", "demo", "--json"}, setup: func(t *testing.T, a *App) {
			path, err := resolveContextFilePath(a.env)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte("{invalid"), 0o600); err != nil {
				t.Fatal(err)
			}
		}},
		{name: "missing name", channel: "mcp", mcpArgs: map[string]any{}},
	} {
		t.Run(tc.channel+"/"+tc.name, func(t *testing.T) {
			server, read := analyticsRecorder(t)
			a := analyticsTestApp(t, server.URL)
			if tc.setup != nil {
				tc.setup(t, a)
			}
			var err error
			if tc.channel == "shell" {
				a.root = a.buildRoot()
				a.root.SetOut(io.Discard)
				a.root.SetErr(io.Discard)
				a.root.SetArgs(tc.args)
				err = a.root.Execute()
			} else {
				_, err = a.callMCPTool("agora.project.create", tc.mcpArgs, nil)
			}
			if err == nil {
				t.Fatal("fixture must fail before the create API call")
			}
			waitAnalyticsEvents(t, read, 1)
			// Give a duplicate a chance to arrive before asserting exactly one.
			time.Sleep(50 * time.Millisecond)
			events := read()
			if len(events) != 1 {
				t.Fatalf("want exactly one event, got %+v", events)
			}
			p := events[0].Properties
			if events[0].Event != "cli_project_create_completed" || p["outcome"] != "failure" || p["error_code"] == "" || p["exit_code"] == float64(0) || p["invocation_channel"] != tc.channel || p["app_id"] != nil {
				t.Fatalf("unexpected pre-API failure event: %+v", events[0])
			}
		})
	}
}

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

func TestAnalyticsPolicySnapshotForRunningOperation(t *testing.T) {
	server, read := analyticsRecorder(t)
	a := analyticsTestApp(t, server.URL)
	state, err := ensureAppConfigState(a.env)
	if err != nil {
		t.Fatal(err)
	}
	op := a.beginAnalytics("init", "mcp")
	cfg := state.Config
	cfg.TelemetryEnabled = false
	if err := writeSecureJSON(state.ConfigFilePath, cfg); err != nil {
		t.Fatal(err)
	}
	op.beginStage("clone")
	op.finish(nil)
	events := waitAnalyticsEvents(t, read, 4)
	if len(events) != 4 {
		t.Fatalf("running operation changed snapshot: %+v", events)
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
			if a.telemetryAllowed(a.currentTelemetryPreference()) != !explicit {
				t.Fatal("persisted enable or explicit opt-out ignored")
			}
			if err := os.WriteFile(state.ConfigFilePath, []byte("{invalid"), 0o600); err != nil {
				t.Fatal(err)
			}
			if a.beginAnalytics("recipes list", "mcp") != nil {
				t.Fatal("unreadable preference must fail closed")
			}
		})
	}
}

// The collector allowlists these exact values; changing them is a wire
// format change that must be coordinated with the collector.
func TestAnalyticsAllowlistValuesArePinned(t *testing.T) {
	for _, tc := range []struct {
		name      string
		got, want []string
	}{
		{"stages", analyticsStages, []string{"validate", "source_resolve", "local_validate", "project_resolve", "clone", "env_write", "binding_write", "context_write", "agent_rules_write"}},
		{"agent families", analyticsAgentFamilies, []string{"codex", "claude-code", "kiro", "cursor", "cline", "windsurf", "aider", "custom", "unknown"}},
		{"agent surfaces", analyticsAgentSurfaces, []string{"desktop", "cli", "unknown"}},
		{"detection sources", analyticsDetectionSources, []string{"explicit", "environment", "none"}},
	} {
		if !slices.Equal(tc.got, tc.want) {
			t.Errorf("%s changed: got %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}
