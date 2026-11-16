package cli

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

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

func TestAnalyticsUnfinishedTLSHandshakeHonorsCancellation(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = listener.Close() }()
	stop := make(chan struct{})
	defer close(stop)
	go func() {
		conn, acceptErr := listener.Accept()
		if acceptErr != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		<-stop // Deliberately never complete the TLS handshake.
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	req, err := http.NewRequest(http.MethodPost, "https://"+listener.Addr().String(), strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	start := time.Now()
	writeAnalyticsRequest(ctx, &http.Client{}, req)
	if time.Since(start) > time.Second {
		t.Fatal("failed sending ignored cancellation")
	}
}
