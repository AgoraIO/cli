package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func newTestApp(t *testing.T) *App {
	t.Helper()
	t.Setenv("AGORA_HOME", t.TempDir())
	a, err := NewApp()
	if err != nil {
		t.Fatalf("NewApp: %v", err)
	}
	return a
}

// TestServeMCPHandlesLargeFramesAboveDefaultBuffer guards the regression
// where bufio.Scanner's 64 KiB default cap would silently terminate the
// MCP loop on large `tools/call` payloads. We feed a frame whose tool
// arguments exceed 256 KiB and assert the server still emits a single
// JSON-RPC response with `id` echoed back.
func TestServeMCPHandlesLargeFramesAboveDefaultBuffer(t *testing.T) {
	a := newTestApp(t)
	bigKeyword := strings.Repeat("a", 256*1024)
	req := map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "agora.version",
			"arguments": map[string]any{"keyword": bigKeyword},
		},
	}
	frame, _ := json.Marshal(req)
	in := bytes.NewReader(append(frame, '\n'))
	var out bytes.Buffer
	if err := a.serveMCP(in, &out); err != nil {
		t.Fatalf("serveMCP: %v", err)
	}
	if out.Len() == 0 {
		t.Fatalf("expected a response, got nothing")
	}
	var resp mcpResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v\nbody: %q", err, out.String())
	}
	if resp.Error != nil {
		t.Fatalf("expected success, got error: %+v", resp.Error)
	}
	idFloat, ok := resp.ID.(float64)
	if !ok || int(idFloat) != 1 {
		t.Fatalf("expected id=1, got %v (%T)", resp.ID, resp.ID)
	}
}

// TestServeMCPNotificationsReturnNoResponse covers the JSON-RPC 2.0
// rule: any frame without an id is a notification and MUST NOT receive
// a response.
func TestServeMCPNotificationsReturnNoResponse(t *testing.T) {
	a := newTestApp(t)
	frame := []byte(`{"jsonrpc":"2.0","method":"notifications/initialized","params":{}}` + "\n")
	var out bytes.Buffer
	if err := a.serveMCP(bytes.NewReader(frame), &out); err != nil {
		t.Fatalf("serveMCP: %v", err)
	}
	if out.Len() != 0 {
		t.Fatalf("expected no response for notification, got: %q", out.String())
	}
}

// TestServeMCPInitializeAdvertisesProtocolVersion confirms the
// initialize handshake emits the documented protocol version and a
// stable serverInfo object.
func TestServeMCPInitializeAdvertisesProtocolVersion(t *testing.T) {
	a := newTestApp(t)
	frame := []byte(`{"jsonrpc":"2.0","id":42,"method":"initialize","params":{}}` + "\n")
	var out bytes.Buffer
	if err := a.serveMCP(bytes.NewReader(frame), &out); err != nil {
		t.Fatalf("serveMCP: %v", err)
	}
	var resp mcpResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v\nbody: %q", err, out.String())
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	result, ok := resp.Result.(map[string]any)
	if !ok {
		t.Fatalf("expected map result, got %T", resp.Result)
	}
	if result["protocolVersion"] != mcpProtocolVersion {
		t.Fatalf("protocolVersion = %v, want %v", result["protocolVersion"], mcpProtocolVersion)
	}
	if info, ok := result["serverInfo"].(map[string]any); !ok || info["name"] != "agora-cli" {
		t.Fatalf("expected serverInfo.name=agora-cli, got %+v", result["serverInfo"])
	}
}

// TestMCPToolsListCoversFullSurface guards the contract that the MCP
// surface enumerates every supported tool (so the schema matches the
// CLI command tree). When new commands are added to mcpTools(), update
// this expected list.
func TestMCPToolsListCoversFullSurface(t *testing.T) {
	expected := []string{
		"agora.auth.logout",
		"agora.auth.status",
		"agora.config.get",
		"agora.config.path",
		"agora.init",
		"agora.introspect",
		"agora.project.create",
		"agora.project.doctor",
		"agora.project.env",
		"agora.project.env_write",
		"agora.project.feature.enable",
		"agora.project.feature.list",
		"agora.project.feature.status",
		"agora.project.list",
		"agora.project.show",
		"agora.project.use",
		"agora.project.webhook.create",
		"agora.project.webhook.delete",
		"agora.project.webhook.events",
		"agora.project.webhook.list",
		"agora.project.webhook.show",
		"agora.project.webhook.update",
		"agora.quickstart.create",
		"agora.quickstart.env_write",
		"agora.quickstart.list",
		"agora.telemetry.status",
		"agora.upgrade.check",
		"agora.version",
	}
	got := map[string]bool{}
	for _, tool := range mcpTools() {
		name, _ := tool["name"].(string)
		got[name] = true
	}
	for _, name := range expected {
		if !got[name] {
			t.Errorf("missing MCP tool: %q", name)
		}
	}
	if len(got) != len(expected) {
		extra := []string{}
		for name := range got {
			found := false
			for _, e := range expected {
				if name == e {
					found = true
					break
				}
			}
			if !found {
				extra = append(extra, name)
			}
		}
		if len(extra) > 0 {
			t.Errorf("unexpected MCP tools (update test or remove): %v", extra)
		}
	}
}

func TestMCPProjectWebhookDeleteRequiresConfirm(t *testing.T) {
	a := newTestApp(t)
	_, err := a.callMCPTool("agora.project.webhook.delete", map[string]any{
		"configId": float64(42),
		"feature":  "rtc",
		"project":  "demo",
		"confirm":  false,
	}, nil)
	if err == nil {
		t.Fatal("expected confirmation error")
	}
	var cliErr *cliError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected *cliError, got %T: %v", err, err)
	}
	if cliErr.Code != "CONFIRMATION_REQUIRED" {
		t.Fatalf("code = %q, want CONFIRMATION_REQUIRED", cliErr.Code)
	}
}

// TestMCPVersionToolReturnsBuildInfo runs end-to-end through serveMCP
// for a no-arg, no-network tool to verify the request/response loop
// works, including the content[0].text envelope.
func TestMCPVersionToolReturnsBuildInfo(t *testing.T) {
	a := newTestApp(t)
	frame := []byte(`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"agora.version","arguments":{}}}` + "\n")
	var out bytes.Buffer
	if err := a.serveMCP(bytes.NewReader(frame), &out); err != nil {
		t.Fatalf("serveMCP: %v", err)
	}
	var resp mcpResponse
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v\nbody: %q", err, out.String())
	}
	if resp.Error != nil {
		t.Fatalf("unexpected error: %+v", resp.Error)
	}
	result := resp.Result.(map[string]any)
	contentArr := result["content"].([]any)
	first := contentArr[0].(map[string]any)
	if !strings.Contains(first["text"].(string), `"version"`) {
		t.Fatalf("expected version payload, got: %v", first["text"])
	}
}

func TestMCPQuickstartCreateEmitsProgressNotifications(t *testing.T) {
	repo := createLocalGitRepo(t, map[string]string{
		"env.local.example": "NEXT_PUBLIC_AGORA_APP_ID=\nNEXT_AGORA_APP_CERTIFICATE=\n",
		"README.md":         "demo\n",
	})
	t.Setenv("AGORA_QUICKSTART_NEXTJS_REPO_URL", repo)
	a := newTestApp(t)
	target := filepath.Join(t.TempDir(), "demo")
	frame := []byte(`{"jsonrpc":"2.0","id":9,"method":"tools/call","params":{"name":"agora.quickstart.create","arguments":{"template":"nextjs","dir":` + strconv.Quote(target) + `},"_meta":{"progressToken":"clone-1"}}}` + "\n")
	var out bytes.Buffer
	if err := a.serveMCP(bytes.NewReader(frame), &out); err != nil {
		t.Fatalf("serveMCP: %v", err)
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) < 2 {
		t.Fatalf("expected progress notification and final response, got: %q", out.String())
	}
	if !strings.Contains(lines[0], `"method":"notifications/progress"`) || !strings.Contains(lines[0], `"progressToken":"clone-1"`) || !strings.Contains(out.String(), `"stage":"clone:start"`) {
		t.Fatalf("expected MCP progress notification, got: %q", out.String())
	}
	var resp mcpResponse
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &resp); err != nil {
		t.Fatalf("unmarshal final response: %v\nbody: %q", err, lines[len(lines)-1])
	}
	if resp.Error != nil {
		t.Fatalf("unexpected final error: %+v\nall output: %q", resp.Error, out.String())
	}
}

// TestStringSliceArgShapes verifies the MCP slice coercion handles the
// three real-world payload shapes: native JSON array, comma-string,
// and missing key.
func TestStringSliceArgShapes(t *testing.T) {
	tests := []struct {
		name string
		args map[string]any
		want []string
	}{
		{name: "json array", args: map[string]any{"features": []any{"rtc", "rtm"}}, want: []string{"rtc", "rtm"}},
		{name: "comma string", args: map[string]any{"features": "rtc, rtm , convoai"}, want: []string{"rtc", "rtm", "convoai"}},
		{name: "missing", args: map[string]any{}, want: nil},
		{name: "empty string", args: map[string]any{"features": ""}, want: nil},
		{name: "nil value", args: map[string]any{"features": nil}, want: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stringSliceArg(tt.args, "features")
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Fatalf("[%d] got %q, want %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}
