package cli

import (
	"context"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

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

func TestAnalyticsRegionDerivation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		context string // empty means no context file
		want    string
		ok      bool
	}{
		{"no context defaults to global", "", regionGlobal, true},
		{"cn login", `{"currentRegion":"cn"}`, regionCN, true},
		{"cn login is case-insensitive", `{"currentRegion":" CN "}`, regionCN, true},
		{"global login", `{"currentRegion":"global"}`, regionGlobal, true},
		{"empty region uses CLI default", `{}`, regionGlobal, true},
		{"unknown region uses CLI default host", `{"currentRegion":"private-eu-1"}`, regionGlobal, true},
		{"unreadable context is undeterminable", `{invalid`, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := analyticsTestApp(t, "http://127.0.0.1:1")
			if tc.context != "" {
				writeAnalyticsTestContext(t, a, tc.context)
			}
			got, ok := a.analyticsRegion()
			if got != tc.want || ok != tc.ok {
				t.Fatalf("analyticsRegion() = %q, %v; want %q, %v", got, ok, tc.want, tc.ok)
			}
			op := a.beginAnalytics("recipes list", "shell")
			if op == nil {
				t.Fatal("analytics did not start")
			}
			value, present := op.properties["region"]
			if present != tc.ok || (present && value != tc.want) {
				t.Fatalf("region property = %v (present %v); want %q (present %v)", value, present, tc.want, tc.ok)
			}
		})
	}
}

func TestAnalyticsRegionOnShellEvents(t *testing.T) {
	server, read := analyticsRecorder(t)
	a := analyticsTestApp(t, server.URL)
	writeAnalyticsTestContext(t, a, `{"currentRegion":"cn"}`)
	cmd := &cobra.Command{Use: "list", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.SetErr(io.Discard)
	cmd.SetContext(context.Background())
	a.instrumentAnalytics(cmd, "recipes list")
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatal(err)
	}
	events := waitAnalyticsEvents(t, read, 1)
	for _, e := range events {
		if e.Properties["region"] != regionCN || e.Properties["invocation_channel"] != "shell" {
			t.Fatalf("shell event missing cn region: %+v", e)
		}
	}
}

func TestAnalyticsRegionOnMCPEventsIsReadPerOperation(t *testing.T) {
	collector, read := analyticsRecorder(t)
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer backend.Close()
	a := analyticsTestApp(t, collector.URL)
	a.env["AGORA_RECIPES_BASE_URL"] = backend.URL

	// The same long-lived MCP App reports whichever region is persisted when
	// each tool call starts.
	for i, region := range []string{regionGlobal, regionCN, regionGlobal} {
		writeAnalyticsTestContext(t, a, `{"currentRegion":"`+region+`"}`)
		_, _ = a.callMCPTool("agora.recipes.list", map[string]any{}, nil)
		events := waitAnalyticsEvents(t, read, i+1)
		last := events[len(events)-1]
		if last.Properties["region"] != region || last.Properties["invocation_channel"] != "mcp" {
			t.Fatalf("call %d: want region %q, got %+v", i, region, last.Properties)
		}
	}

	writeAnalyticsTestContext(t, a, `{invalid`)
	_, _ = a.callMCPTool("agora.recipes.list", map[string]any{}, nil)
	events := waitAnalyticsEvents(t, read, 4)
	if _, present := events[3].Properties["region"]; present {
		t.Fatalf("undeterminable region must be omitted: %+v", events[3].Properties)
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
	for _, region := range []any{"", "CN", "Global", " cn", "eu", "us-east-1", "https://cli-cn.agora.io", 1, true, nil} {
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
