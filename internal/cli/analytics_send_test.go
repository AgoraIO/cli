package cli

import (
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestAnalyticsSenderPreservesNetworkSettingsOnly(t *testing.T) {
	t.Setenv("HTTPS_PROXY", "http://proxy.example:8080")
	t.Setenv("NO_PROXY", "localhost,.internal")
	wantSettings := []string{"HTTPS_PROXY=http://proxy.example:8080", "NO_PROXY=localhost,.internal", "SSL_CERT_FILE=/network/ca.pem"}
	if runtime.GOOS != "windows" {
		t.Setenv("https_proxy", "http://lowercase.example:8080")
		wantSettings = append(wantSettings, "https_proxy=http://lowercase.example:8080")
	}
	t.Setenv("SSL_CERT_FILE", "/network/ca.pem")
	t.Setenv("AGORA_ACCESS_TOKEN", "private-business-token")
	t.Setenv("UNRELATED_SECRET", "private")
	env := strings.Join(analyticsSenderEnv(), "\n")
	for _, want := range wantSettings {
		if !strings.Contains(env, want) {
			t.Fatalf("missing network setting: %s", want)
		}
	}
	if strings.Contains(env, "AGORA_ACCESS_TOKEN=") || strings.Contains(env, "UNRELATED_SECRET=") {
		t.Fatal("business environment inherited by sender")
	}
}

func TestAnalyticsDetachedSenderUsesHTTPSProxy(t *testing.T) {
	received := make(chan string, 1)
	proxy := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		received <- r.Method + " " + r.Host
		w.WriteHeader(http.StatusBadGateway)
	}))
	defer proxy.Close()
	t.Setenv("https_proxy", "")
	t.Setenv("HTTPS_PROXY", proxy.URL)
	t.Setenv("NO_PROXY", "")
	t.Setenv("no_proxy", "")
	a := analyticsTestApp(t, "https://collector.example.invalid/api/telemetry/v1/events")
	op := a.beginAnalytics("recipes list", "shell")
	op.finish(nil)
	select {
	case got := <-received:
		if got != "CONNECT collector.example.invalid:443" {
			t.Fatalf("unexpected proxy request: %s", got)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("detached sender ignored HTTPS_PROXY")
	}
}

func TestAnalyticsFinishDoesNotWaitForResponse(t *testing.T) {
	for _, flushHeaders := range []bool{false, true} {
		t.Run(map[bool]string{false: "no response headers", true: "unfinished response body"}[flushHeaders], func(t *testing.T) {
			release := make(chan struct{})
			received := make(chan analyticsEvent, 1)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Events []analyticsEvent `json:"events"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
					return
				}
				if len(body.Events) != 1 {
					t.Errorf("unexpected events: %d", len(body.Events))
					return
				}
				received <- body.Events[0]
				if flushHeaders {
					w.WriteHeader(http.StatusAccepted)
					w.(http.Flusher).Flush()
				}
				<-release
			}))
			defer server.Close()
			defer close(release)
			a := analyticsTestApp(t, server.URL)
			op := a.beginAnalytics("recipes list", "shell")
			finished := make(chan struct{})
			go func() { op.finish(nil); close(finished) }()
			select {
			case <-finished:
			case <-time.After(500 * time.Millisecond):
				t.Fatal("command waited for response after writing its request")
			}
			select {
			case event := <-received:
				if event.Event != "cli_recipes_list_completed" {
					t.Fatalf("missing complete final event: %+v", event)
				}
			case <-time.After(time.Second):
				t.Fatal("request body was truncated or not sent")
			}
		})
	}
}

func TestAnalyticsDoesNotRetryOrFollowRedirects(t *testing.T) {
	for _, result := range []string{"closed connection", "429", "503", "redirect"} {
		t.Run(result, func(t *testing.T) {
			var calls, redirected atomic.Int32
			target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { redirected.Add(1) }))
			defer target.Close()
			received := make(chan struct{}, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = io.Copy(io.Discard, r.Body)
				calls.Add(1)
				received <- struct{}{}
				switch result {
				case "closed connection":
					conn, _, err := w.(http.Hijacker).Hijack()
					if err != nil {
						t.Error(err)
						return
					}
					_ = conn.Close()
				case "429":
					w.Header().Set("Retry-After", "60")
					w.WriteHeader(http.StatusTooManyRequests)
				case "503":
					w.WriteHeader(http.StatusServiceUnavailable)
				case "redirect":
					w.Header().Set("Location", target.URL)
					w.WriteHeader(http.StatusTemporaryRedirect)
				}
			}))
			defer server.Close()
			a := analyticsTestApp(t, server.URL)
			for range 2 {
				op := a.beginAnalytics("recipes list", "mcp")
				if op == nil {
					t.Fatal("a previous response suppressed a later operation")
				}
				op.finish(nil)
				select {
				case <-received:
				case <-time.After(time.Second):
					t.Fatal("batch not received")
				}
			}
			if calls.Load() != 2 || redirected.Load() != 0 {
				t.Fatalf("unexpected retry/redirect: calls=%d redirects=%d", calls.Load(), redirected.Load())
			}
		})
	}
}

func TestAnalyticsStalledTLSBoundsShellAndMCPCompletion(t *testing.T) {
	for _, channel := range []string{"shell", "mcp"} {
		t.Run(channel, func(t *testing.T) {
			listener, err := net.Listen("tcp", "127.0.0.1:0")
			if err != nil {
				t.Fatal(err)
			}
			defer func() { _ = listener.Close() }()
			stop, accepted := make(chan struct{}), make(chan struct{})
			defer close(stop)
			go func() {
				conn, acceptErr := listener.Accept()
				if acceptErr != nil {
					return
				}
				defer func() { _ = conn.Close() }()
				close(accepted)
				<-stop // Keep TLS setup stalled beyond the command's drain budget.
			}()
			a := analyticsTestApp(t, "https://"+listener.Addr().String())
			start := time.Now()
			if channel == "shell" {
				a.root = a.buildRoot()
				a.root.SetArgs([]string{"recipes", "list", "--type", "invalid", "--json"})
				err = a.root.Execute()
			} else {
				_, err = a.callMCPTool("agora.recipes.list", map[string]any{"type": "invalid"}, nil)
			}
			elapsed := time.Since(start)
			var businessErr *cliError
			if !errors.As(err, &businessErr) || businessErr.Code != "RECIPE_TYPE_INVALID" {
				t.Fatalf("telemetry changed business error: %v", err)
			}
			// Allow scheduling overhead around the one-second timer, but reject
			// a multi-second drain such as the previous five-second budget.
			if elapsed > 1500*time.Millisecond {
				t.Fatalf("telemetry delayed %s completion: %s", channel, elapsed)
			}
			select {
			case <-accepted:
			case <-time.After(time.Second):
				t.Fatal("test did not exercise a stalled TLS connection")
			}
		})
	}
}

func TestAnalyticsBatchAccountingIncludesInitStart(t *testing.T) {
	received := make(chan int, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var batch analyticsBatch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			t.Error(err)
		}
		received <- len(batch.Events)
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	a := analyticsTestApp(t, server.URL)
	op := a.beginAnalytics("init", "mcp")
	// Attribution is copied onto cli_init_started only at flush; use the
	// longest allowed values so the start grows as much as possible.
	op.source("recipe", strings.Repeat("a", 100))
	op.stage = ""
	accepted := 0
	for {
		event := op.newEvent("cli_init_stage_completed", map[string]any{"stage": analyticsStageClone, "outcome": "success", "error_code": "", "duration_ms": 1})
		body, err := json.Marshal(event)
		if err != nil {
			t.Fatal(err)
		}
		if op.batchBytes+1+len(body) > analyticsRequestLimit {
			break
		}
		op.submit(event)
		accepted++
	}
	if len(op.events) != accepted || op.batchBytes > analyticsRequestLimit {
		t.Fatalf("accounting rejected an event below the limit: accepted=%d kept=%d bytes=%d", accepted, len(op.events), op.batchBytes)
	}
	op.flush()
	select {
	case n := <-received:
		if n != accepted+1 {
			t.Fatalf("want start plus %d events, got %d", accepted, n)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("batch accepted by accounting was dropped at flush")
	}
	if !drainAnalyticsSends(2 * time.Second) {
		t.Fatal("send did not finish")
	}
}

func TestAnalyticsMCPSendsInProcessWithoutBlocking(t *testing.T) {
	received := make(chan analyticsBatch, 1)
	release := make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("MCP sender attached business authorization")
		}
		var batch analyticsBatch
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			t.Error(err)
		}
		received <- batch
		<-release
		w.WriteHeader(http.StatusAccepted)
	}))
	defer server.Close()
	a := analyticsTestApp(t, server.URL)
	started := time.Now()
	if _, err := a.callMCPTool("agora.recipes.list", map[string]any{"type": "invalid"}, nil); err == nil {
		t.Fatal("invalid type must fail")
	}
	if time.Since(started) > 500*time.Millisecond {
		t.Fatal("MCP tool call waited for telemetry delivery")
	}
	select {
	case batch := <-received:
		if len(batch.Events) != 1 || batch.Events[0].Properties["invocation_channel"] != "mcp" {
			t.Fatalf("unexpected MCP batch: %+v", batch)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("MCP batch not delivered")
	}
	// The request is still open in this process, so the drain must time out.
	if drainAnalyticsSends(50 * time.Millisecond) {
		t.Fatal("MCP delivery did not run in process")
	}
	close(release)
	if !drainAnalyticsSends(2 * time.Second) {
		t.Fatal("in-process MCP delivery did not finish")
	}
}
