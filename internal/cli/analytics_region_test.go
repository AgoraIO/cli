package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// shellCaptureMode is the effective mode a shell operation starting now uses.
func shellCaptureMode(a *App) string {
	_, mode, _ := a.analyticsCapture(a.cfg.TelemetryEnabled)
	return mode
}

// hitCounter is a collector that only counts requests.
func hitCounter(t *testing.T) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusAccepted)
	}))
	t.Cleanup(server.Close)
	return server, &hits
}

func writeAnalyticsTestContext(t *testing.T, a *App, raw string) {
	t.Helper()
	path, err := resolveContextFilePath(a.env)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(raw), 0o600); err != nil {
		t.Fatal(err)
	}
}

func setAnalyticsTestRegion(t *testing.T, a *App, region string) {
	t.Helper()
	writeAnalyticsTestContext(t, a, `{"currentRegion":"`+region+`"}`)
}

// runShellAnalytics runs an instrumented no-op shell command and returns its
// stderr (where the first-use notice goes).
func runShellAnalytics(t *testing.T, a *App) string {
	t.Helper()
	var stderr bytes.Buffer
	cmd := &cobra.Command{Use: "list", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.SetErr(&stderr)
	cmd.SetContext(context.Background())
	a.instrumentAnalytics(cmd, "recipes list")
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	return stderr.String()
}

func runMCPAnalytics(t *testing.T, a *App) {
	t.Helper()
	_, _ = a.callMCPTool("agora.recipes.list", map[string]any{}, nil)
}

// regionTestApp has a global collector and a failing recipes backend so
// recipes list completes quickly with one event per operation.
func regionTestApp(t *testing.T, globalURL string) *App {
	t.Helper()
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(backend.Close)
	a := analyticsTestApp(t, globalURL)
	a.env["AGORA_RECIPES_BASE_URL"] = backend.URL
	return a
}

func analyticsIDPath(a *App) string { return filepath.Join(a.env["AGORA_HOME"], "analytics-id") }

func assertNoFile(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("%s exists: %v", filepath.Base(path), err)
	}
}

// settleAnalytics drains in-process MCP sends. Shell sends use a detached
// subprocess, which is never spawned when the mode is off.
func settleAnalytics(t *testing.T) {
	t.Helper()
	if !drainAnalyticsSends(5 * analyticsMCPDrainTimeout) {
		t.Fatal("in-flight sends did not drain")
	}
}

func TestAnalyticsRegionDerivation(t *testing.T) {
	for _, tc := range []struct {
		name, context, apiBase, want string
		ok                           bool
	}{
		{name: "no context defaults to global", want: regionGlobal, ok: true},
		{name: "cn login", context: `{"currentRegion":"cn"}`, want: regionCN, ok: true},
		{name: "cn is case-insensitive", context: `{"currentRegion":" CN "}`, want: regionCN, ok: true},
		{name: "global login", context: `{"currentRegion":"global"}`, want: regionGlobal, ok: true},
		{name: "unknown stored region uses CLI default", context: `{"currentRegion":"private-eu-1"}`, want: regionGlobal, ok: true},
		{name: "corrupt context is undeterminable", context: `{invalid`, ok: false},
		{name: "CN API override", apiBase: apiBaseURLCN, want: regionCN, ok: true},
		{name: "CN API override with path wins over corrupt context", context: `{invalid`, apiBase: strings.ToUpper(apiBaseURLCN) + "/api", want: regionCN, ok: true},
		{name: "other API override keeps context region", context: `{"currentRegion":"cn"}`, apiBase: "http://127.0.0.1:9", want: regionCN, ok: true},
		{name: "global API override is global", apiBase: apiBaseURL, want: regionGlobal, ok: true},
		{name: "lookalike host is not CN", apiBase: "https://cli-cn.agora.io.example.com", want: regionGlobal, ok: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := analyticsTestApp(t, "http://127.0.0.1:1")
			if tc.context != "" {
				writeAnalyticsTestContext(t, a, tc.context)
			}
			if tc.apiBase != "" {
				a.osEnv["AGORA_API_BASE_URL"] = tc.apiBase
			}
			got, ok := a.analyticsRegion()
			if got != tc.want || ok != tc.ok {
				t.Fatalf("analyticsRegion() = %q, %v; want %q, %v", got, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestAnalyticsCNModeAndURLValidation(t *testing.T) {
	a := analyticsTestApp(t, "http://127.0.0.1:1")
	for raw, want := range map[string]string{"": analyticsModeOff, "off": analyticsModeOff, "aggregate": analyticsModeAggregate, " full ": analyticsModeFull, "FULL": analyticsModeOff, "on": analyticsModeOff} {
		a.env["AGORA_TELEMETRY_CN_MODE"] = raw
		if got := a.analyticsCNMode(); got != want {
			t.Errorf("mode %q = %q, want %q", raw, got, want)
		}
	}
	for raw, valid := range map[string]bool{"": false, "https://cn.example/events": true, "http://127.0.0.1:9/e": true, "http://cn.example/e": false, "https://u:p@cn.example/e": false, "https://cn.example/e?x=1": false} {
		a.env["AGORA_TELEMETRY_CN_URL"] = raw
		if got := a.analyticsCNURL() != ""; got != valid {
			t.Errorf("CN URL %q valid=%v, want %v", raw, got, valid)
		}
	}
}

// For a CN operation the global collector is never contacted, in any mode,
// whether or not a CN collector is configured, on shell and MCP.
func TestAnalyticsCNNeverContactsGlobalCollector(t *testing.T) {
	for _, mode := range []string{"", "off", "aggregate", "full", "bogus"} {
		for _, withCN := range []bool{false, true} {
			for _, channel := range []string{"shell", "mcp"} {
				name := "mode=" + mode + "/cnURL=" + map[bool]string{true: "yes", false: "no"}[withCN] + "/" + channel
				t.Run(name, func(t *testing.T) {
					global, globalHits := hitCounter(t)
					cn, cnRead := analyticsRecorder(t)
					a := regionTestApp(t, global.URL)
					setAnalyticsTestRegion(t, a, regionCN)
					if mode != "" {
						a.env["AGORA_TELEMETRY_CN_MODE"] = mode
					}
					if withCN {
						a.env["AGORA_TELEMETRY_CN_URL"] = cn.URL
					}
					if channel == "shell" {
						runShellAnalytics(t, a)
					} else {
						runMCPAnalytics(t, a)
					}
					capturing := withCN && (mode == "aggregate" || mode == "full")
					if capturing {
						waitAnalyticsEvents(t, cnRead, 1)
					}
					settleAnalytics(t)
					if globalHits.Load() != 0 {
						t.Fatalf("global collector contacted %d times", globalHits.Load())
					}
					if !capturing && len(cnRead()) != 0 {
						t.Fatal("off mode sent events")
					}
				})
			}
		}
	}
}

func TestAnalyticsCNDefaultIsOff(t *testing.T) {
	for _, channel := range []string{"shell", "mcp"} {
		t.Run(channel, func(t *testing.T) {
			global, globalHits := hitCounter(t)
			cn, cnHits := hitCounter(t)
			a := regionTestApp(t, global.URL)
			a.env["AGORA_TELEMETRY_CN_URL"] = cn.URL // URL alone does not enable CN.
			setAnalyticsTestRegion(t, a, regionCN)
			stderr := ""
			if channel == "shell" {
				stderr = runShellAnalytics(t, a)
			} else {
				runMCPAnalytics(t, a)
			}
			settleAnalytics(t)
			if globalHits.Load()+cnHits.Load() != 0 || stderr != "" {
				t.Fatalf("CN default sent requests or a notice: global=%d cn=%d stderr=%q", globalHits.Load(), cnHits.Load(), stderr)
			}
			assertNoFile(t, analyticsIDPath(a))
			assertNoFile(t, filepath.Join(a.env["AGORA_HOME"], "telemetry-notice-shown"))
			assertNoFile(t, filepath.Join(a.env["AGORA_HOME"], "telemetry-notice-aggregate-shown"))
		})
	}
}

func TestAnalyticsCorruptContextIsOff(t *testing.T) {
	for _, channel := range []string{"shell", "mcp"} {
		t.Run(channel, func(t *testing.T) {
			global, globalHits := hitCounter(t)
			a := regionTestApp(t, global.URL)
			writeAnalyticsTestContext(t, a, `{invalid`)
			stderr := ""
			if channel == "shell" {
				stderr = runShellAnalytics(t, a)
			} else {
				runMCPAnalytics(t, a)
			}
			settleAnalytics(t)
			if globalHits.Load() != 0 || stderr != "" {
				t.Fatalf("corrupt context captured: hits=%d stderr=%q", globalHits.Load(), stderr)
			}
			assertNoFile(t, analyticsIDPath(a))
		})
	}
}

func TestAnalyticsCNAggregate(t *testing.T) {
	global, globalHits := hitCounter(t)
	cn, read := analyticsRecorder(t)
	a := regionTestApp(t, global.URL)
	// A stored installation ID must not be read or reused in aggregate mode.
	stored := "11111111-1111-4111-8111-111111111111"
	if err := os.WriteFile(analyticsIDPath(a), []byte(stored), 0o600); err != nil {
		t.Fatal(err)
	}
	setAnalyticsTestRegion(t, a, regionCN)
	a.env["AGORA_TELEMETRY_CN_MODE"] = "aggregate"
	a.env["AGORA_TELEMETRY_CN_URL"] = cn.URL

	stderr := runShellAnalytics(t, a)
	if !strings.Contains(stderr, "No installation ID or Agora App ID is sent") || strings.Contains(stderr, "installation profile") {
		t.Fatalf("wrong aggregate notice: %q", stderr)
	}
	if again := runShellAnalytics(t, a); again != "" {
		t.Fatalf("aggregate notice repeated: %q", again)
	}
	assertNoFile(t, filepath.Join(a.env["AGORA_HOME"], "telemetry-notice-shown"))
	runMCPAnalytics(t, a)

	// An operation that would set app_id drops it in aggregate mode.
	op := a.beginAnalytics("project create", "mcp")
	op.setAppID(testTelemetryAppID)
	op.emit("cli_project_create_completed", map[string]any{"outcome": "success", "error_code": "", "duration_ms": 1, "exit_code": 0, "app_id": testTelemetryAppID})
	op.projectCreateReported = true
	op.finish(nil)

	events := waitAnalyticsEvents(t, read, 4)
	settleAnalytics(t)
	for _, e := range events {
		if e.Properties["region"] != regionCN {
			t.Fatalf("CN event missing region: %+v", e.Properties)
		}
		if _, ok := e.Properties["app_id"]; ok {
			t.Fatalf("aggregate event carries app_id: %+v", e.Properties)
		}
		if e.DistinctID == "cli:"+stored || e.DistinctID == "cli:"+e.OperationID || !strings.HasPrefix(e.DistinctID, "cli:") {
			t.Fatalf("aggregate distinct_id links to identity or operation: %+v", e)
		}
	}
	distinct := map[string]bool{}
	for _, e := range events {
		distinct[e.DistinctID] = true
	}
	if len(distinct) != len(events) {
		t.Fatalf("aggregate distinct_id reused across operations: %v", distinct)
	}
	if got, _ := os.ReadFile(analyticsIDPath(a)); string(got) != stored {
		t.Fatal("stored analytics-id changed")
	}
	if globalHits.Load() != 0 {
		t.Fatalf("global collector contacted %d times", globalHits.Load())
	}
}

func TestAnalyticsCNAggregateCreatesNoIdentity(t *testing.T) {
	cn, read := analyticsRecorder(t)
	a := regionTestApp(t, "http://127.0.0.1:1")
	setAnalyticsTestRegion(t, a, regionCN)
	a.env["AGORA_TELEMETRY_CN_MODE"] = "aggregate"
	a.env["AGORA_TELEMETRY_CN_URL"] = cn.URL
	runMCPAnalytics(t, a)
	waitAnalyticsEvents(t, read, 1)
	settleAnalytics(t)
	assertNoFile(t, analyticsIDPath(a))
}

func TestAnalyticsCNFullAndAPIOverride(t *testing.T) {
	global, globalHits := hitCounter(t)
	cn, read := analyticsRecorder(t)
	a := regionTestApp(t, global.URL)
	// No saved region: the CN API host alone selects CN.
	a.osEnv["AGORA_API_BASE_URL"] = apiBaseURLCN
	a.env["AGORA_TELEMETRY_CN_MODE"] = "full"
	a.env["AGORA_TELEMETRY_CN_URL"] = cn.URL
	runMCPAnalytics(t, a)
	events := waitAnalyticsEvents(t, read, 1)
	settleAnalytics(t)
	stored, err := os.ReadFile(analyticsIDPath(a))
	if err != nil {
		t.Fatal("full mode did not use the installation ID")
	}
	if events[0].Properties["region"] != regionCN || events[0].DistinctID != "cli:"+string(stored) || globalHits.Load() != 0 {
		t.Fatalf("wrong CN full routing: %+v global=%d", events[0], globalHits.Load())
	}
}

func TestAnalyticsGlobalFullUnchanged(t *testing.T) {
	for _, channel := range []string{"shell", "mcp"} {
		t.Run(channel, func(t *testing.T) {
			global, read := analyticsRecorder(t)
			cn, cnHits := hitCounter(t)
			a := regionTestApp(t, global.URL)
			a.env["AGORA_TELEMETRY_CN_MODE"] = "full"
			a.env["AGORA_TELEMETRY_CN_URL"] = cn.URL
			setAnalyticsTestRegion(t, a, regionGlobal)
			if channel == "shell" {
				if stderr := runShellAnalytics(t, a); stderr != telemetryDisclosure+"\n" {
					t.Fatalf("global notice changed: %q", stderr)
				}
			} else {
				runMCPAnalytics(t, a)
			}
			events := waitAnalyticsEvents(t, read, 1)
			settleAnalytics(t)
			stored, _ := os.ReadFile(analyticsIDPath(a))
			for _, e := range events {
				if _, ok := e.Properties["region"]; ok {
					t.Fatalf("global event carries region: %+v", e.Properties)
				}
				if e.DistinctID != "cli:"+string(stored) {
					t.Fatalf("global event lost installation ID: %+v", e)
				}
			}
			if cnHits.Load() != 0 {
				t.Fatal("global operation contacted the CN collector")
			}
		})
	}
}

func TestAnalyticsMCPRegionSwitchesPerCall(t *testing.T) {
	global, globalRead := analyticsRecorder(t)
	cn, cnRead := analyticsRecorder(t)
	a := regionTestApp(t, global.URL)
	a.env["AGORA_TELEMETRY_CN_URL"] = cn.URL

	setAnalyticsTestRegion(t, a, regionGlobal)
	runMCPAnalytics(t, a)
	waitAnalyticsEvents(t, globalRead, 1)

	// CN default (off): nothing is sent anywhere.
	setAnalyticsTestRegion(t, a, regionCN)
	runMCPAnalytics(t, a)
	settleAnalytics(t)
	if len(cnRead()) != 0 || len(globalRead()) != 1 {
		t.Fatal("CN off call was captured")
	}

	// CN aggregate in the same session routes only to CN.
	a.env["AGORA_TELEMETRY_CN_MODE"] = "aggregate"
	runMCPAnalytics(t, a)
	cnEvents := waitAnalyticsEvents(t, cnRead, 1)
	if cnEvents[0].Properties["region"] != regionCN {
		t.Fatalf("CN call missing region: %+v", cnEvents[0])
	}

	setAnalyticsTestRegion(t, a, regionGlobal)
	runMCPAnalytics(t, a)
	globalEvents := waitAnalyticsEvents(t, globalRead, 2)
	settleAnalytics(t)
	if len(globalEvents) != 2 || len(cnRead()) != 1 {
		t.Fatalf("calls misrouted: global=%d cn=%d", len(globalEvents), len(cnRead()))
	}
	for _, e := range globalEvents {
		if _, ok := e.Properties["region"]; ok {
			t.Fatalf("global event carries region: %+v", e.Properties)
		}
	}
}

func mcpInitializeInstructions(t *testing.T, a *App) (string, bool) {
	t.Helper()
	result, err := a.handleMCPRequest(mcpRequest{Method: "initialize"}, json.NewEncoder(&bytes.Buffer{}))
	if err != nil {
		t.Fatal(err)
	}
	text, ok := result.(map[string]any)["instructions"].(string)
	return text, ok
}

func TestMCPInitializeDisclosureFollowsRegionMode(t *testing.T) {
	cn, _ := analyticsRecorder(t)
	for _, tc := range []struct {
		name, context, cnMode string
		cnURL, optOut         bool
		want                  string
	}{
		{name: "global full", context: `{"currentRegion":"global"}`, want: telemetryDisclosure},
		{name: "global opted out", context: `{"currentRegion":"global"}`, optOut: true},
		{name: "cn default off", context: `{"currentRegion":"cn"}`, cnURL: true},
		{name: "cn aggregate without URL", context: `{"currentRegion":"cn"}`, cnMode: "aggregate"},
		{name: "cn aggregate", context: `{"currentRegion":"cn"}`, cnMode: "aggregate", cnURL: true, want: telemetryAggregateDisclosure},
		{name: "cn full", context: `{"currentRegion":"cn"}`, cnMode: "full", cnURL: true, want: telemetryDisclosure},
		{name: "unknown region uses global logic", context: `{invalid`, want: telemetryDisclosure},
		{name: "unknown region opted out", context: `{invalid`, optOut: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := analyticsTestApp(t, "https://collector.example/events")
			writeAnalyticsTestContext(t, a, tc.context)
			a.env["AGORA_TELEMETRY_CN_MODE"] = tc.cnMode
			if tc.cnURL {
				a.env["AGORA_TELEMETRY_CN_URL"] = cn.URL
			}
			if tc.optOut {
				a.env["DO_NOT_TRACK"] = "1"
			}
			text, ok := mcpInitializeInstructions(t, a)
			if text != tc.want || ok != (tc.want != "") {
				t.Fatalf("instructions = %q (present %v), want %q", text, ok, tc.want)
			}
		})
	}
}

func TestAnalyticsOptOutsDisableBothRegions(t *testing.T) {
	optOuts := map[string]func(a *App){
		"DO_NOT_TRACK":            func(a *App) { a.env["DO_NOT_TRACK"] = "1" },
		"AGORA_TELEMETRY_ENABLED": func(a *App) { a.env["AGORA_TELEMETRY_ENABLED"] = "0" },
		"AGORA_SENTRY_ENABLED":    func(a *App) { a.env["AGORA_SENTRY_ENABLED"] = "0"; a.osEnv["AGORA_SENTRY_ENABLED"] = "0" },
		"saved preference": func(a *App) {
			if err := a.persistTelemetryChoice(false); err != nil {
				panic(err)
			}
		},
	}
	for name, optOut := range optOuts {
		for _, region := range []string{regionGlobal, regionCN} {
			for _, channel := range []string{"shell", "mcp"} {
				t.Run(name+"/"+region+"/"+channel, func(t *testing.T) {
					global, globalHits := hitCounter(t)
					cn, cnHits := hitCounter(t)
					a := regionTestApp(t, global.URL)
					a.env["AGORA_TELEMETRY_CN_MODE"] = "full"
					a.env["AGORA_TELEMETRY_CN_URL"] = cn.URL
					setAnalyticsTestRegion(t, a, region)
					optOut(a)
					stderr := ""
					if channel == "shell" {
						stderr = runShellAnalytics(t, a)
					} else {
						runMCPAnalytics(t, a)
					}
					settleAnalytics(t)
					if globalHits.Load()+cnHits.Load() != 0 || stderr != "" {
						t.Fatalf("opt-out ignored: global=%d cn=%d stderr=%q", globalHits.Load(), cnHits.Load(), stderr)
					}
					status := a.telemetryStatus("status", "", a.analyticsPreference(channel))
					if status["captureActive"] != false || status["captureMode"] != analyticsModeOff || status["region"] != region {
						t.Fatalf("status while opted out: %v", status)
					}
				})
			}
		}
	}
}

func TestAnalyticsValidatorRegionValues(t *testing.T) {
	a := analyticsTestApp(t, "http://127.0.0.1:1")
	op := a.beginAnalytics("recipes list", "shell")
	if op == nil {
		t.Fatal("analytics did not start")
	}
	event := func(region any) analyticsEvent {
		props := maps.Clone(op.properties)
		props["region"] = region
		return analyticsEvent{Event: "cli_recipes_list_completed", EventID: analyticsUUID(), OperationID: op.id, DistinctID: op.distinctID, Timestamp: time.Now().UTC(), Properties: props}
	}
	for _, region := range []string{"global", "cn"} {
		if !validAnalyticsEvent(event(region)) {
			t.Errorf("region %q rejected", region)
		}
	}
	for _, region := range []any{"", "CN", "Global", " cn", "eu", "https://cli-cn.agora.io", 1, true, nil} {
		if validAnalyticsEvent(event(region)) {
			t.Errorf("region %#v accepted", region)
		}
	}
	noRegion := event("global")
	delete(noRegion.Properties, "region")
	if !validAnalyticsEvent(noRegion) {
		t.Error("event without region rejected; region must stay optional")
	}
}

func TestTelemetryStatusRegionAndCaptureMode(t *testing.T) {
	cn, _ := analyticsRecorder(t)
	for _, tc := range []struct {
		name, context, apiBase, cnMode string
		cnURL                          bool
		region, mode                   string
		transport                      bool
	}{
		{name: "global full", region: regionGlobal, mode: analyticsModeFull, transport: true},
		{name: "cn default", context: `{"currentRegion":"cn"}`, region: regionCN, mode: analyticsModeOff},
		{name: "cn URL but mode off", context: `{"currentRegion":"cn"}`, cnURL: true, region: regionCN, mode: analyticsModeOff, transport: true},
		{name: "cn aggregate without URL", context: `{"currentRegion":"cn"}`, cnMode: "aggregate", region: regionCN, mode: analyticsModeOff},
		{name: "cn aggregate", context: `{"currentRegion":"cn"}`, cnMode: "aggregate", cnURL: true, region: regionCN, mode: analyticsModeAggregate, transport: true},
		{name: "cn full", context: `{"currentRegion":"cn"}`, cnMode: "full", cnURL: true, region: regionCN, mode: analyticsModeFull, transport: true},
		{name: "cn via API override", apiBase: apiBaseURLCN, cnMode: "aggregate", cnURL: true, region: regionCN, mode: analyticsModeAggregate, transport: true},
		{name: "corrupt context", context: `{invalid`, region: analyticsValueUnknown, mode: analyticsModeOff},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := analyticsTestApp(t, "https://collector.example/events")
			if tc.context != "" {
				writeAnalyticsTestContext(t, a, tc.context)
			}
			if tc.apiBase != "" {
				a.osEnv["AGORA_API_BASE_URL"] = tc.apiBase
			}
			a.env["AGORA_TELEMETRY_CN_MODE"] = tc.cnMode
			if tc.cnURL {
				a.env["AGORA_TELEMETRY_CN_URL"] = cn.URL
			}
			shell := a.telemetryStatus("status", "", a.cfg.TelemetryEnabled)
			result, err := a.callMCPTool("agora.telemetry.status", nil, nil)
			if err != nil {
				t.Fatal(err)
			}
			for label, status := range map[string]map[string]any{"shell": shell, "mcp": result.(map[string]any)} {
				if status["region"] != tc.region || status["captureMode"] != tc.mode || status["transportConfigured"] != tc.transport || status["captureActive"] != (tc.mode != analyticsModeOff) || status["enabled"] != true {
					t.Errorf("%s status = %v; want region=%s mode=%s transport=%v", label, status, tc.region, tc.mode, tc.transport)
				}
			}
		})
	}
}
