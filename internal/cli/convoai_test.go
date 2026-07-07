package cli

import (
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/spf13/cobra"
)

func TestValidateChannelName(t *testing.T) {
	if err := validateChannelName(""); err == nil {
		t.Fatal("empty channel must be rejected")
	}
	if err := validateChannelName(strings.Repeat("a", 65)); err == nil {
		t.Fatal("channel over 64 bytes must be rejected")
	}
	if err := validateChannelName("bad space"); err == nil {
		t.Fatal("space must be rejected")
	}
	if err := validateChannelName("my-dev_room.1"); err != nil {
		t.Fatalf("valid channel rejected: %v", err)
	}
}

func TestResolveUIDGeneratesNonZero(t *testing.T) {
	if got := resolveUID(0); got == 0 {
		t.Fatal("generated uid must be non-zero")
	}
	if got := resolveUID(42); got != 42 {
		t.Fatalf("explicit uid must pass through, got %d", got)
	}
}

func TestResolveDisjointAgentUID(t *testing.T) {
	got := resolveAgentUID(0)
	if got < 10000000 || got > 99999999 {
		t.Fatalf("generated agent uid out of reserved range: %d", got)
	}
}

func TestConvoaiPlaygroundIsRegistered(t *testing.T) {
	// Use newTestApp (defined in mcp_test.go) to boot a real App and obtain
	// its fully-wired root cobra command, matching the pattern used by sibling
	// test files in this package.
	a := newTestApp(t)
	root := a.buildRoot()

	cmd, _, err := root.Find([]string{"convoai", "playground"})
	if err != nil {
		t.Fatalf("convoai playground not found: %v", err)
	}
	if cmd.Name() != "playground" {
		t.Fatalf("expected playground, got %q", cmd.Name())
	}
	for _, name := range []string{"channel", "port", "uid", "agent-uid", "ttl", "no-open"} {
		if cmd.Flag(name) == nil {
			t.Fatalf("missing --%s flag", name)
		}
	}
	// --channel must be marked required.
	if ann := cmd.Flag("channel").Annotations[cobra.BashCompOneRequiredFlag]; len(ann) == 0 {
		t.Fatalf("--channel must be required")
	}
}

func TestGetConfigMintsToken(t *testing.T) {
	sess := &playgroundSession{
		appID:    "970CA35de60c44645bbae8a215061b33",
		appCert:  "5CFd2fd1755d40ecb72977518be15d3b",
		channel:  "my-dev-room",
		uid:      12345,
		agentUID: 87654321,
		ttl:      3600,
	}
	h := newPlaygroundHandler(sess, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/get_config", nil)
	req.Host = "127.0.0.1:8787"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var env struct {
		Code int `json:"code"`
		Data struct {
			AppID       string `json:"app_id"`
			Token       string `json:"token"`
			UID         string `json:"uid"`
			ChannelName string `json:"channel_name"`
			AgentUID    string `json:"agent_uid"`
		} `json:"data"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	if env.Code != 0 || len(env.Data.Token) < 3 || env.Data.Token[:3] != "007" {
		t.Fatalf("bad envelope: %+v", env)
	}
	if env.Data.UID != "12345" || env.Data.ChannelName != "my-dev-room" || env.Data.AgentUID != "87654321" {
		t.Fatalf("field mismatch: %+v", env.Data)
	}
	if env.Data.AppID != sess.appID {
		t.Fatalf("app_id mismatch: %s", env.Data.AppID)
	}
}

func TestGetConfigRejectsNonLoopbackHost(t *testing.T) {
	sess := &playgroundSession{appID: "a", appCert: "b", channel: "c", uid: 1, ttl: 3600}
	h := newPlaygroundHandler(sess, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/get_config", nil)
	req.Host = "evil.example.com"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-loopback Host, got %d", rec.Code)
	}
}

func TestGetConfigUIDOverride(t *testing.T) {
	sess := &playgroundSession{appID: "a", appCert: "b", channel: "c", uid: 12345, ttl: 3600}
	h := newPlaygroundHandler(sess, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/get_config?uid=777", nil)
	req.Host = "localhost:8787"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var env struct {
		Data struct {
			UID string `json:"uid"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Data.UID != "777" {
		t.Fatalf("expected uid override 777, got %s", env.Data.UID)
	}
}

func TestGetConfigUIDZeroFallsThrough(t *testing.T) {
	sess := &playgroundSession{appID: "a", appCert: "b", channel: "c", uid: 12345, ttl: 3600}
	h := newPlaygroundHandler(sess, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/get_config?uid=0", nil)
	req.Host = "localhost:8787"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	var env struct {
		Data struct {
			UID string `json:"uid"`
		} `json:"data"`
	}
	_ = json.Unmarshal(rec.Body.Bytes(), &env)
	if env.Data.UID != "12345" {
		t.Fatalf("uid=0 must fall through to session uid, got %s", env.Data.UID)
	}
}

func TestGetConfigAcceptsMixedCaseHost(t *testing.T) {
	sess := &playgroundSession{appID: "a", appCert: "b", channel: "c", uid: 1, ttl: 3600}
	h := newPlaygroundHandler(sess, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/get_config", nil)
	req.Host = "LOCALHOST:8787"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("mixed-case localhost must be accepted, got %d", rec.Code)
	}
}

func TestListenFreePortAutoIncrements(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	busy := ln.Addr().(*net.TCPAddr).Port

	got, err := listenPlayground(busy, false /* not explicit */)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Close()
	if got.Addr().(*net.TCPAddr).Port == busy {
		t.Fatal("expected auto-increment away from the busy port")
	}
}

func TestListenExplicitBusyPortFails(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln.Close()
	busy := ln.Addr().(*net.TCPAddr).Port
	if _, err := listenPlayground(busy, true /* explicit */); err == nil {
		t.Fatal("explicit busy port must hard-fail")
	}
}

func TestPlaygroundStartupEnvelope(t *testing.T) {
	sess := &playgroundSession{appID: "APP123", channel: "my-dev-room", uid: 12345, agentUID: 87654321, port: 8787}
	data := playgroundStartupData(sess, "http://127.0.0.1:8787")
	if data["appId"] != "APP123" || data["channel"] != "my-dev-room" {
		t.Fatalf("bad envelope: %+v", data)
	}
	if data["url"] != "http://127.0.0.1:8787" || data["agentUid"] != "87654321" {
		t.Fatalf("bad envelope: %+v", data)
	}
	if data["uid"] != "12345" {
		t.Fatalf("bad uid: %+v", data)
	}
}

func TestStaticServesGzipWithEncoding(t *testing.T) {
	// Synthetic FS: index.html raw (small), assets/app.js only as .gz.
	gzPayload := []byte{0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0xff,
		0x4a, 0xcb, 0xcf, 0x57, 0x48, 0x54, 0xc8, 0x48, 0xcd, 0xc9, 0xc9, 0x57, 0x04, 0x00,
		0x00, 0x00, 0xff, 0xff} // partial, just non-empty gz bytes for testing
	testFS := fstest.MapFS{
		"index.html":       {Data: []byte("<html></html>")},
		"assets/app.js.gz": {Data: gzPayload},
	}

	sess := &playgroundSession{appID: "a", appCert: "b", channel: "c", uid: 1, ttl: 3600}
	h := newPlaygroundHandler(sess, testFS)

	// Request / with Accept-Encoding: gzip → should get gzip-encoded JS.
	req := httptest.NewRequest(http.MethodGet, "/assets/app.js", nil)
	req.Host = "127.0.0.1:8787"
	req.Header.Set("Accept-Encoding", "gzip")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if got := rec.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("expected Content-Encoding: gzip, got %q", got)
	}
	if rec.Body.Len() == 0 {
		t.Fatal("expected non-empty body for gzip response")
	}

	// Request index.html → served raw (exact file exists).
	req2 := httptest.NewRequest(http.MethodGet, "/", nil)
	req2.Host = "127.0.0.1:8787"
	rec2 := httptest.NewRecorder()
	h.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("expected 200 for index.html, got %d", rec2.Code)
	}
	if got := rec2.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("index.html should not have Content-Encoding, got %q", got)
	}

	// Request with nil assets → 404.
	hNil := newPlaygroundHandler(sess, nil)
	req3 := httptest.NewRequest(http.MethodGet, "/", nil)
	req3.Host = "127.0.0.1:8787"
	rec3 := httptest.NewRecorder()
	hNil.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusNotFound {
		t.Fatalf("nil assets: expected 404, got %d", rec3.Code)
	}
}
