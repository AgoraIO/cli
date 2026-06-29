# Project Webhook CLI Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add `agora project webhook` commands for RTC, RTM, and ConvoAI webhook event discovery, create, list, show, update, delete, and MCP parity.

**Architecture:** Keep webhook behavior in a focused `internal/cli/webhooks.go` file: backend DTOs, normalization, validation, API helpers, and command-level business methods live together. Register the Cobra surface under `project`, render pretty output through `render.go`, extend MCP with thin dispatch wrappers, and test end-to-end through the existing fake CLI BFF.

**Tech Stack:** Go, Cobra, standard `crypto/rand`, standard `encoding/base64`, standard `regexp`, existing Agora CLI JSON envelope, existing MCP stdio server.

---

## File Structure

- Create `internal/cli/webhooks.go`: webhook DTOs, normalized structs, validation helpers, event resolution, secret generation, BFF API methods, and command business methods.
- Create `internal/cli/webhooks_test.go`: unit tests for validation, event key generation/resolution, secret generation, region defaulting, response extraction, and redaction.
- Modify `internal/cli/commands.go`: add `buildProjectWebhook()` and register it under `buildProjectCommand()`.
- Modify `internal/cli/render.go`: add pretty render cases for `project webhook events`, `list`, `show`, `create`, `update`, and `delete`.
- Modify `internal/cli/mcp.go`: add six webhook tools and dispatch cases, including destructive delete confirmation.
- Modify `internal/cli/mcp_test.go`: update the expected MCP tool surface.
- Modify `internal/cli/integration_test.go`: extend the fake BFF with NCS event/config endpoints and add webhook command integration tests.
- Modify `docs/automation.md`: document JSON payload shapes, examples, secret redaction, delete confirmation, and MCP tools.
- Modify `README.md`: add webhook to the command overview and examples.
- Regenerate `docs/commands.md` using the repo's doc generator after commands are registered.
- Modify `docs/llms.txt`: manually add webhook MCP tool names to the compact agent-facing MCP summary.

## Task 0: Backend Confirmation Check

**Files:**
- Modify: `docs/superpowers/specs/2026-06-07-project-webhook-design.md`

- [ ] **Step 1: Confirm event ID mapping before production behavior is wired**

Ask the backend owner this exact question:

```text
For /api/cli/v1/projects/{projectId}/ncs-configs/{feature}, does the request field eventIds contain values from /api/cli/v1/ncs-events/{feature}.items[].eventId, not eventType?
```

Accepted confirmation:

```text
eventIds uses ncs-events.items[].eventId.
```

- [ ] **Step 2: Record the confirmation in the design spec**

Edit `docs/superpowers/specs/2026-06-07-project-webhook-design.md` and replace the final sentence in the event API paragraph with:

```markdown
Backend owner confirmation: config `eventIds` are populated from event `eventId`, not `eventType`.
```

- [ ] **Step 3: Commit the confirmation**

Run:

```bash
git add docs/superpowers/specs/2026-06-07-project-webhook-design.md
git commit -m "docs: confirm webhook event id mapping"
```

Expected: one documentation commit.

## Task 1: Webhook Unit Helpers

**Files:**
- Create: `internal/cli/webhooks.go`
- Create: `internal/cli/webhooks_test.go`

- [ ] **Step 1: Write failing helper tests**

Add `internal/cli/webhooks_test.go`:

```go
package cli

import (
	"regexp"
	"testing"
)

func TestWebhookEventKeyFromDisplayName(t *testing.T) {
	tests := map[string]string{
		"Channel Created":       "channel-created",
		" User.Joined / Left ":  "user-joined-left",
		"RTC_Recording.Started": "rtc-recording-started",
	}
	for input, want := range tests {
		if got := webhookEventKey(input); got != want {
			t.Fatalf("webhookEventKey(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestResolveWebhookEventInputs(t *testing.T) {
	events := []webhookEvent{
		{ID: 1001, Key: "channel-created", DisplayName: "Channel Created"},
		{ID: 1002, Key: "channel-destroyed", DisplayName: "Channel Destroyed"},
	}
	got, err := resolveWebhookEventIDs(events, []string{"channel-created", "1002", "Channel Created"}, "rtc")
	if err != nil {
		t.Fatal(err)
	}
	want := []int{1001, 1002}
	if !webhookIntSlicesEqual(got, want) {
		t.Fatalf("event IDs = %#v, want %#v", got, want)
	}
}

func TestResolveWebhookEventInputsRejectsUnknownAndAmbiguous(t *testing.T) {
	events := []webhookEvent{
		{ID: 1001, Key: "same-name", DisplayName: "Same Name"},
		{ID: 1002, Key: "same-name", DisplayName: "Same--Name"},
	}
	if _, err := resolveWebhookEventIDs(events, []string{"missing"}, "rtc"); !hasCLIErrorCode(err, "WEBHOOK_EVENT_UNKNOWN") {
		t.Fatalf("expected WEBHOOK_EVENT_UNKNOWN, got %v", err)
	}
	if _, err := resolveWebhookEventIDs(events, []string{"same-name"}, "rtc"); !hasCLIErrorCode(err, "WEBHOOK_EVENT_AMBIGUOUS") {
		t.Fatalf("expected WEBHOOK_EVENT_AMBIGUOUS, got %v", err)
	}
}

func TestGenerateWebhookSecretMatchesBackendPattern(t *testing.T) {
	secret, err := generateWebhookSecret()
	if err != nil {
		t.Fatal(err)
	}
	if len(secret) != 32 {
		t.Fatalf("secret length = %d, want 32", len(secret))
	}
	if !regexp.MustCompile(`^[A-Za-z0-9_-]{7,32}$`).MatchString(secret) {
		t.Fatalf("secret does not match backend pattern: %q", secret)
	}
}

func TestWebhookDeliveryRegionDefault(t *testing.T) {
	tests := []struct {
		controlPlane string
		want         string
	}{
		{controlPlane: "global", want: "na"},
		{controlPlane: "cn", want: "cn"},
		{controlPlane: "", want: "na"},
	}
	for _, tt := range tests {
		if got := defaultWebhookDeliveryRegion(tt.controlPlane); got != tt.want {
			t.Fatalf("defaultWebhookDeliveryRegion(%q) = %q, want %q", tt.controlPlane, got, tt.want)
		}
	}
}
```

- [ ] **Step 2: Run helper tests and verify they fail**

Run:

```bash
go test ./internal/cli -run 'TestWebhook(EventKey|Resolve|Generate|Delivery)' -count=1
```

Expected: compile failure for undefined webhook helper symbols.

- [ ] **Step 3: Add helper implementation**

Create `internal/cli/webhooks.go` with these helper foundations:

```go
package cli

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

const redactedWebhookSecret = "********"

var webhookSecretPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{7,32}$`)
var webhookEventKeyInvalidChars = regexp.MustCompile(`[^a-z0-9]+`)

type webhookEvent struct {
	ID          int    `json:"id"`
	Key         string `json:"key"`
	DisplayName string `json:"displayName"`
	EventType   int    `json:"eventType"`
	Payload     string `json:"payload,omitempty"`
}

type webhookConfig struct {
	ConfigID       int            `json:"configId"`
	URL            string         `json:"url"`
	URLRegion      string         `json:"urlRegion"`
	Enabled        bool           `json:"enabled"`
	EventIDs       []int          `json:"eventIds"`
	Events         []webhookEvent `json:"events,omitempty"`
	Retry          *bool          `json:"retry,omitempty"`
	UseIPWhitelist bool           `json:"useIpWhitelist"`
	Secret         string         `json:"secret,omitempty"`
}

func webhookEventKey(displayName string) string {
	value := strings.ToLower(strings.TrimSpace(displayName))
	value = webhookEventKeyInvalidChars.ReplaceAllString(value, "-")
	return strings.Trim(value, "-")
}

func validateWebhookFeature(feature string) error {
	if strings.TrimSpace(feature) == "" {
		return &cliError{Message: "feature is required", Code: "WEBHOOK_FEATURE_REQUIRED"}
	}
	if err := validateFeatureID(feature); err != nil {
		return err
	}
	return nil
}

func normalizeWebhookDeliveryRegion(value string) (string, error) {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "cn", "sea", "na", "eu":
		return value, nil
	default:
		return "", &cliError{Message: "--delivery-region must be one of: cn, sea, na, eu", Code: "WEBHOOK_DELIVERY_REGION_INVALID"}
	}
}

func defaultWebhookDeliveryRegion(controlPlaneRegion string) string {
	if strings.TrimSpace(controlPlaneRegion) == "cn" {
		return "cn"
	}
	return "na"
}

func generateWebhookSecret() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func validateWebhookSecret(secret string) error {
	if !webhookSecretPattern.MatchString(secret) {
		return &cliError{Message: "webhook secret must match ^[A-Za-z0-9_-]{7,32}$", Code: "WEBHOOK_SECRET_INVALID"}
	}
	return nil
}

func webhookIntSlicesEqual(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func resolveWebhookEventIDs(events []webhookEvent, inputs []string, feature string) ([]int, error) {
	byID := map[int]webhookEvent{}
	byKey := map[string][]webhookEvent{}
	byDisplayName := map[string]webhookEvent{}
	for _, event := range events {
		byID[event.ID] = event
		byKey[event.Key] = append(byKey[event.Key], event)
		byDisplayName[event.DisplayName] = event
	}
	seen := map[int]bool{}
	out := []int{}
	for _, input := range inputs {
		input = strings.TrimSpace(input)
		if input == "" {
			continue
		}
		if id, err := strconv.Atoi(input); err == nil {
			if _, ok := byID[id]; !ok {
				return nil, &cliError{Message: fmt.Sprintf("Unknown webhook event ID %d. Run `agora project webhook events --feature %s`.", id, feature), Code: "WEBHOOK_EVENT_UNKNOWN"}
			}
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
			continue
		}
		if matches := byKey[input]; len(matches) > 1 {
			return nil, &cliError{Message: fmt.Sprintf("Webhook event key %q is ambiguous. Pass the numeric event ID.", input), Code: "WEBHOOK_EVENT_AMBIGUOUS"}
		} else if len(matches) == 1 {
			id := matches[0].ID
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
			continue
		}
		if event, ok := byDisplayName[input]; ok {
			if !seen[event.ID] {
				seen[event.ID] = true
				out = append(out, event.ID)
			}
			continue
		}
		return nil, &cliError{Message: fmt.Sprintf("Unknown webhook event key %q. Run `agora project webhook events --feature %s`.", input, feature), Code: "WEBHOOK_EVENT_UNKNOWN"}
	}
	sort.Ints(out)
	return out, nil
}
```

- [ ] **Step 4: Add test-only CLI error helper**

Append to `internal/cli/webhooks_test.go`:

```go
func hasCLIErrorCode(err error, code string) bool {
	if err == nil {
		return false
	}
	structured, ok := err.(*cliError)
	return ok && structured.Code == code
}
```

- [ ] **Step 5: Run helper tests and verify they pass**

Run:

```bash
go test ./internal/cli -run 'TestWebhook(EventKey|Resolve|Generate|Delivery)' -count=1
```

Expected: `ok`.

- [ ] **Step 6: Commit helper layer**

Run:

```bash
git add internal/cli/webhooks.go internal/cli/webhooks_test.go
git commit -m "feat: add webhook validation helpers"
```

Expected: one feature commit.

## Task 2: Webhook API Adapter

**Files:**
- Modify: `internal/cli/webhooks.go`
- Modify: `internal/cli/webhooks_test.go`

- [ ] **Step 1: Write failing adapter tests**

Append these tests:

```go
func TestNormalizeWebhookEventsIgnoresChineseDisplayName(t *testing.T) {
	resp := ncsEventListResponse{Items: []ncsEvent{{EventID: 1001, DisplayName: "Channel Created", DisplayNameCn: "频道创建", EventType: 7, Payload: `{"x":1}`}}}
	got := normalizeWebhookEvents(resp)
	if len(got) != 1 || got[0].Key != "channel-created" || got[0].ID != 1001 || got[0].EventType != 7 || got[0].Payload != `{"x":1}` {
		t.Fatalf("unexpected normalized events: %#v", got)
	}
}

func TestSelectWebhookConfigFromCreateResponsePrefersSecret(t *testing.T) {
	resp := ncsConfigListResponse{Items: []ncsConfig{
		{ConfigID: 41, URL: "https://example.com/webhook", URLRegion: "na", EventIDs: []int{1001}, Secret: "other", UpdatedAt: "2026-06-07T00:00:01Z"},
		{ConfigID: 42, URL: "https://example.com/webhook", URLRegion: "na", EventIDs: []int{1001}, Secret: "secret_123", UpdatedAt: "2026-06-07T00:00:02Z"},
	}}
	got, err := selectCreatedWebhookConfig(resp, "https://example.com/webhook", "na", []int{1001}, "secret_123")
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigID != 42 {
		t.Fatalf("configId = %d, want 42", got.ConfigID)
	}
}

func TestRedactWebhookConfigSecret(t *testing.T) {
	cfg := webhookConfig{ConfigID: 42, Secret: "secret_123"}
	got := redactWebhookConfigSecret(cfg, false)
	if got.Secret != redactedWebhookSecret {
		t.Fatalf("secret = %q, want redacted", got.Secret)
	}
	got = redactWebhookConfigSecret(cfg, true)
	if got.Secret != "secret_123" {
		t.Fatalf("secret = %q, want original", got.Secret)
	}
}
```

- [ ] **Step 2: Run adapter tests and verify they fail**

Run:

```bash
go test ./internal/cli -run 'Test(NormalizeWebhook|SelectWebhook|RedactWebhook)' -count=1
```

Expected: compile failure for missing adapter types/functions.

- [ ] **Step 3: Add backend DTOs and normalization**

Append to `internal/cli/webhooks.go`:

```go
type ncsEventListResponse struct {
	Items []ncsEvent `json:"items"`
}

type ncsEvent struct {
	EventID       int    `json:"eventId"`
	DisplayName   string `json:"displayName"`
	DisplayNameCn string `json:"displayNameCn"`
	EventType     int    `json:"eventType"`
	Payload       string `json:"payload"`
}

type ncsConfigListResponse struct {
	Items []ncsConfig `json:"items"`
}

type ncsConfig struct {
	ConfigID       int    `json:"configId"`
	URL            string `json:"url"`
	URLRegion      string `json:"urlRegion"`
	Enabled        bool   `json:"enabled"`
	EventIDs       []int  `json:"eventIds"`
	Retry          *bool  `json:"retry"`
	UseIPWhitelist bool   `json:"useIpWhitelist"`
	Secret         string `json:"secret"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}

func normalizeWebhookEvents(resp ncsEventListResponse) []webhookEvent {
	out := make([]webhookEvent, 0, len(resp.Items))
	for _, item := range resp.Items {
		out = append(out, webhookEvent{ID: item.EventID, Key: webhookEventKey(item.DisplayName), DisplayName: item.DisplayName, EventType: item.EventType, Payload: item.Payload})
	}
	return out
}

func normalizeWebhookConfig(item ncsConfig, events []webhookEvent) webhookConfig {
	byID := map[int]webhookEvent{}
	for _, event := range events {
		byID[event.ID] = event
	}
	cfgEvents := []webhookEvent{}
	for _, id := range item.EventIDs {
		if event, ok := byID[id]; ok {
			cfgEvents = append(cfgEvents, event)
		}
	}
	return webhookConfig{ConfigID: item.ConfigID, URL: item.URL, URLRegion: item.URLRegion, Enabled: item.Enabled, EventIDs: append([]int{}, item.EventIDs...), Events: cfgEvents, Retry: item.Retry, UseIPWhitelist: item.UseIPWhitelist, Secret: item.Secret}
}

func redactWebhookConfigSecret(cfg webhookConfig, reveal bool) webhookConfig {
	if reveal {
		return cfg
	}
	if cfg.Secret != "" {
		cfg.Secret = redactedWebhookSecret
	}
	return cfg
}

func selectCreatedWebhookConfig(resp ncsConfigListResponse, url, urlRegion string, eventIDs []int, secret string) (ncsConfig, error) {
	matches := []ncsConfig{}
	for _, item := range resp.Items {
		if secret != "" && item.Secret == secret {
			return item, nil
		}
		if item.URL == url && item.URLRegion == urlRegion && webhookIntSlicesEqual(item.EventIDs, eventIDs) {
			matches = append(matches, item)
		}
	}
	if len(matches) == 0 {
		return ncsConfig{}, &cliError{Message: "created webhook config was not found in backend response", Code: "WEBHOOK_CONFIG_NOT_FOUND"}
	}
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].UpdatedAt != matches[j].UpdatedAt {
			return matches[i].UpdatedAt > matches[j].UpdatedAt
		}
		return matches[i].ConfigID > matches[j].ConfigID
	})
	return matches[0], nil
}
```

- [ ] **Step 4: Add API helper methods**

Append to `internal/cli/webhooks.go`:

```go
func (a *App) listWebhookEvents(feature string) ([]webhookEvent, error) {
	if err := validateWebhookFeature(feature); err != nil {
		return nil, err
	}
	var out ncsEventListResponse
	if err := a.apiRequest("GET", "/api/cli/v1/ncs-events/"+feature, nil, nil, &out); err != nil {
		return nil, err
	}
	return normalizeWebhookEvents(out), nil
}

func (a *App) listWebhookConfigs(projectID, feature string) (ncsConfigListResponse, error) {
	var out ncsConfigListResponse
	err := a.apiRequest("GET", "/api/cli/v1/projects/"+projectID+"/ncs-configs/"+feature, nil, nil, &out)
	return out, err
}

func (a *App) createWebhookConfig(projectID, feature string, body map[string]any) (ncsConfigListResponse, error) {
	var out ncsConfigListResponse
	err := a.apiRequest("POST", "/api/cli/v1/projects/"+projectID+"/ncs-configs/"+feature, nil, body, &out)
	return out, err
}

func (a *App) updateWebhookConfig(projectID, feature string, configID int, body map[string]any) (ncsConfigListResponse, error) {
	var out ncsConfigListResponse
	err := a.apiRequest("PUT", fmt.Sprintf("/api/cli/v1/projects/%s/ncs-configs/%s/%d", projectID, feature, configID), nil, body, &out)
	return out, err
}

func (a *App) deleteWebhookConfig(projectID, feature string, configID int) error {
	out := map[string]any{}
	return a.apiRequest("DELETE", fmt.Sprintf("/api/cli/v1/projects/%s/ncs-configs/%s/%d", projectID, feature, configID), nil, nil, &out)
}
```

- [ ] **Step 5: Run adapter tests**

Run:

```bash
go test ./internal/cli -run 'Test(NormalizeWebhook|SelectWebhook|RedactWebhook|WebhookEventKey|ResolveWebhook|GenerateWebhook|WebhookDelivery)' -count=1
```

Expected: `ok`.

- [ ] **Step 6: Commit adapter layer**

Run:

```bash
git add internal/cli/webhooks.go internal/cli/webhooks_test.go
git commit -m "feat: add webhook api adapter"
```

Expected: one feature commit.

## Task 3: Fake BFF and Integration Tests

**Files:**
- Modify: `internal/cli/integration_test.go`

- [ ] **Step 1: Extend fake BFF state**

Add these types near `fakeProject`:

```go
type fakeNCSConfig struct {
	ConfigID       int    `json:"configId"`
	URL            string `json:"url"`
	URLRegion      string `json:"urlRegion"`
	Enabled        bool   `json:"enabled"`
	EventIDs       []int  `json:"eventIds"`
	Retry          *bool  `json:"retry,omitempty"`
	UseIPWhitelist bool   `json:"useIpWhitelist"`
	Secret         string `json:"secret"`
	CreatedAt      string `json:"createdAt"`
	UpdatedAt      string `json:"updatedAt"`
}
```

Extend `fakeCLIBFF`:

```go
ncsConfigs map[string][]fakeNCSConfig
ncsBodies  []map[string]any
```

Initialize it in `newFakeCLIBFF()`:

```go
api := &fakeCLIBFF{projects: map[string]*fakeProject{}, ncsConfigs: map[string][]fakeNCSConfig{}}
```

- [ ] **Step 2: Add fake NCS routes**

Add cases before the generic project `GET` route:

```go
case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/cli/v1/ncs-events/"):
	feature := strings.TrimPrefix(r.URL.Path, "/api/cli/v1/ncs-events/")
	_ = feature
	_ = json.NewEncoder(w).Encode(map[string]any{"items": []map[string]any{
		{"eventId": 1001, "displayName": "Channel Created", "displayNameCn": "频道创建", "eventType": 1, "payload": `{"event":"created"}`},
		{"eventId": 1002, "displayName": "Channel Destroyed", "displayNameCn": "频道销毁", "eventType": 2, "payload": `{"event":"destroyed"}`},
	}})
case strings.Contains(r.URL.Path, "/ncs-configs/"):
	api.handleFakeNCSConfigs(w, r)
```

Add helper method after `newFakeCLIBFF()`:

```go
func (api *fakeCLIBFF) handleFakeNCSConfigs(w http.ResponseWriter, r *http.Request) {
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	projectID := parts[4]
	feature := parts[6]
	key := projectID + "/" + feature
	switch r.Method {
	case http.MethodGet:
		_ = json.NewEncoder(w).Encode(map[string]any{"items": api.ncsConfigs[key]})
	case http.MethodPost:
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		api.mu.Lock()
		api.ncsBodies = append(api.ncsBodies, body)
		api.mu.Unlock()
		config := fakeNCSConfig{
			ConfigID:       42 + len(api.ncsConfigs[key]),
			URL:            body["url"].(string),
			URLRegion:      body["urlRegion"].(string),
			Enabled:        body["enabled"].(bool),
			EventIDs:       floatSliceToInts(body["eventIds"].([]any)),
			Retry:          boolPtr(true),
			UseIPWhitelist: body["useIpWhitelist"].(bool),
			Secret:         body["secret"].(string),
			CreatedAt:      "2026-06-07T00:00:01Z",
			UpdatedAt:      "2026-06-07T00:00:01Z",
		}
		api.ncsConfigs[key] = append(api.ncsConfigs[key], config)
		_ = json.NewEncoder(w).Encode(map[string]any{"items": api.ncsConfigs[key]})
	case http.MethodPut:
		configID, _ := strconv.Atoi(parts[7])
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		api.mu.Lock()
		api.ncsBodies = append(api.ncsBodies, body)
		api.mu.Unlock()
		for i := range api.ncsConfigs[key] {
			if api.ncsConfigs[key][i].ConfigID == configID {
				api.ncsConfigs[key][i].URL = body["url"].(string)
				api.ncsConfigs[key][i].URLRegion = body["urlRegion"].(string)
				api.ncsConfigs[key][i].Enabled = body["enabled"].(bool)
				api.ncsConfigs[key][i].EventIDs = floatSliceToInts(body["eventIds"].([]any))
				api.ncsConfigs[key][i].UseIPWhitelist = body["useIpWhitelist"].(bool)
				api.ncsConfigs[key][i].UpdatedAt = "2026-06-07T00:00:02Z"
			}
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"items": api.ncsConfigs[key]})
	case http.MethodDelete:
		configID, _ := strconv.Atoi(parts[7])
		next := []fakeNCSConfig{}
		for _, item := range api.ncsConfigs[key] {
			if item.ConfigID != configID {
				next = append(next, item)
			}
		}
		api.ncsConfigs[key] = next
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}
}
```

Add helpers:

```go
func boolPtr(value bool) *bool { return &value }

func floatSliceToInts(values []any) []int {
	out := make([]int, 0, len(values))
	for _, value := range values {
		out = append(out, int(value.(float64)))
	}
	return out
}
```

- [ ] **Step 3: Write failing integration tests**

Add tests that shell out through the built binary:

```go
func TestProjectWebhookEventsJSON(t *testing.T) {
	bin := buildTestBinary(t)
	api := newFakeCLIBFF()
	defer api.server.Close()
	configHome := t.TempDir()
	persistSessionForIntegration(t, configHome)
	result := runCLI(t, bin, map[string]string{"XDG_CONFIG_HOME": configHome, "AGORA_CLI_BASE_URL": api.baseURL}, "project", "webhook", "events", "--feature", "rtc", "--json")
	if result.exitCode != 0 || !strings.Contains(result.stdout, `"command":"project webhook events"`) || !strings.Contains(result.stdout, `"key":"channel-created"`) || strings.Contains(result.stdout, "displayNameCn") {
		t.Fatalf("unexpected result: exit=%d stdout=%s stderr=%s", result.exitCode, result.stdout, result.stderr)
	}
}

func TestProjectWebhookCreateJSON(t *testing.T) {
	bin := buildTestBinary(t)
	api := newFakeCLIBFF()
	defer api.server.Close()
	project := buildFakeProject("demo", "prj_0001", "app_0001", "global")
	api.projects[project.ProjectID] = &project
	configHome := t.TempDir()
	persistSessionForIntegration(t, configHome)
	result := runCLI(t, bin, map[string]string{"XDG_CONFIG_HOME": configHome, "AGORA_CLI_BASE_URL": api.baseURL}, "project", "webhook", "create", "--project", "demo", "--feature", "rtc", "--url", "https://example.com/webhook", "--event", "channel-created", "--json")
	if result.exitCode != 0 || !strings.Contains(result.stdout, `"urlRegion":"na"`) || !strings.Contains(result.stdout, `"enabled":true`) || !strings.Contains(result.stdout, `"secret":"`) {
		t.Fatalf("unexpected result: exit=%d stdout=%s stderr=%s", result.exitCode, result.stdout, result.stderr)
	}
	if len(api.ncsBodies) != 1 || api.ncsBodies[0]["enabled"] != true || api.ncsBodies[0]["useIpWhitelist"] != false {
		t.Fatalf("unexpected create body: %#v", api.ncsBodies)
	}
}

func TestProjectWebhookUpdateReadMergePut(t *testing.T) {
	bin := buildTestBinary(t)
	api := newFakeCLIBFF()
	defer api.server.Close()
	project := buildFakeProject("demo", "prj_0001", "app_0001", "global")
	api.projects[project.ProjectID] = &project
	api.ncsConfigs["prj_0001/rtc"] = []fakeNCSConfig{{ConfigID: 42, URL: "https://old.example/webhook", URLRegion: "eu", Enabled: true, EventIDs: []int{1001}, UseIPWhitelist: false, Secret: "secret_123"}}
	configHome := t.TempDir()
	persistSessionForIntegration(t, configHome)
	result := runCLI(t, bin, map[string]string{"XDG_CONFIG_HOME": configHome, "AGORA_CLI_BASE_URL": api.baseURL}, "project", "webhook", "update", "42", "--project", "demo", "--feature", "rtc", "--url", "https://new.example/webhook", "--json")
	if result.exitCode != 0 || strings.Contains(result.stdout, "secret_123") || !strings.Contains(result.stdout, `"secret":"********"`) {
		t.Fatalf("unexpected result: exit=%d stdout=%s stderr=%s", result.exitCode, result.stdout, result.stderr)
	}
	last := api.ncsBodies[len(api.ncsBodies)-1]
	if last["url"] != "https://new.example/webhook" || last["urlRegion"] != "eu" || last["enabled"] != true {
		t.Fatalf("PUT body did not preserve fields: %#v", last)
	}
}

func TestProjectWebhookDeleteRequiresYesInJSON(t *testing.T) {
	bin := buildTestBinary(t)
	api := newFakeCLIBFF()
	defer api.server.Close()
	project := buildFakeProject("demo", "prj_0001", "app_0001", "global")
	api.projects[project.ProjectID] = &project
	configHome := t.TempDir()
	persistSessionForIntegration(t, configHome)
	result := runCLI(t, bin, map[string]string{"XDG_CONFIG_HOME": configHome, "AGORA_CLI_BASE_URL": api.baseURL}, "project", "webhook", "delete", "42", "--project", "demo", "--feature", "rtc", "--json")
	if result.exitCode == 0 || !strings.Contains(result.stdout, `"code":"CONFIRMATION_REQUIRED"`) {
		t.Fatalf("expected confirmation error, got exit=%d stdout=%s stderr=%s", result.exitCode, result.stdout, result.stderr)
	}
}
```

Add these additional integration tests in the same section:

```go
func TestProjectWebhookCreateExplicitSecretAndRejectInvalidSecret(t *testing.T) {
	bin := buildTestBinary(t)
	api := newFakeCLIBFF()
	defer api.server.Close()
	project := buildFakeProject("demo", "prj_0001", "app_0001", "global")
	api.projects[project.ProjectID] = &project
	configHome := t.TempDir()
	persistSessionForIntegration(t, configHome)

	ok := runCLI(t, bin, map[string]string{"XDG_CONFIG_HOME": configHome, "AGORA_CLI_BASE_URL": api.baseURL}, "project", "webhook", "create", "--project", "demo", "--feature", "rtc", "--url", "https://example.com/webhook", "--event", "1001", "--secret", "secret_123", "--json")
	if ok.exitCode != 0 || !strings.Contains(ok.stdout, `"secret":"secret_123"`) {
		t.Fatalf("expected explicit secret success, got exit=%d stdout=%s stderr=%s", ok.exitCode, ok.stdout, ok.stderr)
	}

	bad := runCLI(t, bin, map[string]string{"XDG_CONFIG_HOME": configHome, "AGORA_CLI_BASE_URL": api.baseURL}, "project", "webhook", "create", "--project", "demo", "--feature", "rtc", "--url", "https://example.com/webhook", "--event", "1001", "--secret", "this-secret-is-too-long-for-the-backend-pattern", "--json")
	if bad.exitCode == 0 || !strings.Contains(bad.stdout, `"code":"WEBHOOK_SECRET_INVALID"`) {
		t.Fatalf("expected invalid secret error, got exit=%d stdout=%s stderr=%s", bad.exitCode, bad.stdout, bad.stderr)
	}
}

func TestProjectWebhookCreateDefaultsCNDeliveryRegion(t *testing.T) {
	bin := buildTestBinary(t)
	api := newFakeCLIBFF()
	defer api.server.Close()
	project := buildFakeProject("demo-cn", "prj_cn", "app_cn", "cn")
	api.projects[project.ProjectID] = &project
	configHome := t.TempDir()
	persistSessionForIntegration(t, configHome)
	result := runCLI(t, bin, map[string]string{"XDG_CONFIG_HOME": configHome, "AGORA_CLI_BASE_URL": api.baseURL}, "project", "webhook", "create", "--project", "demo-cn", "--feature", "rtc", "--url", "https://example.cn/webhook", "--event", "channel-created", "--json")
	if result.exitCode != 0 || !strings.Contains(result.stdout, `"urlRegion":"cn"`) {
		t.Fatalf("expected cn default region, got exit=%d stdout=%s stderr=%s", result.exitCode, result.stdout, result.stderr)
	}
}

func TestProjectWebhookListRedactsAndShowWithSecretReveals(t *testing.T) {
	bin := buildTestBinary(t)
	api := newFakeCLIBFF()
	defer api.server.Close()
	project := buildFakeProject("demo", "prj_0001", "app_0001", "global")
	api.projects[project.ProjectID] = &project
	api.ncsConfigs["prj_0001/rtc"] = []fakeNCSConfig{{ConfigID: 42, URL: "https://example.com/webhook", URLRegion: "na", Enabled: true, EventIDs: []int{1001}, UseIPWhitelist: false, Secret: "secret_123"}}
	configHome := t.TempDir()
	persistSessionForIntegration(t, configHome)

	list := runCLI(t, bin, map[string]string{"XDG_CONFIG_HOME": configHome, "AGORA_CLI_BASE_URL": api.baseURL}, "project", "webhook", "list", "--project", "demo", "--feature", "rtc", "--json")
	if list.exitCode != 0 || strings.Contains(list.stdout, "secret_123") || !strings.Contains(list.stdout, `"secret":"********"`) {
		t.Fatalf("expected list redaction, got exit=%d stdout=%s stderr=%s", list.exitCode, list.stdout, list.stderr)
	}

	show := runCLI(t, bin, map[string]string{"XDG_CONFIG_HOME": configHome, "AGORA_CLI_BASE_URL": api.baseURL}, "project", "webhook", "show", "42", "--project", "demo", "--feature", "rtc", "--with-secret", "--json")
	if show.exitCode != 0 || !strings.Contains(show.stdout, `"secret":"secret_123"`) {
		t.Fatalf("expected show --with-secret reveal, got exit=%d stdout=%s stderr=%s", show.exitCode, show.stdout, show.stderr)
	}
}

func TestProjectWebhookUpdateSecretFlagRejected(t *testing.T) {
	bin := buildTestBinary(t)
	api := newFakeCLIBFF()
	defer api.server.Close()
	configHome := t.TempDir()
	persistSessionForIntegration(t, configHome)
	result := runCLI(t, bin, map[string]string{"XDG_CONFIG_HOME": configHome, "AGORA_CLI_BASE_URL": api.baseURL}, "project", "webhook", "update", "42", "--feature", "rtc", "--secret", "secret_123", "--json")
	if result.exitCode == 0 || !strings.Contains(result.stderr, "unknown flag: --secret") {
		t.Fatalf("expected unknown --secret flag, got exit=%d stdout=%s stderr=%s", result.exitCode, result.stdout, result.stderr)
	}
}
```

- [ ] **Step 4: Run integration tests and verify they fail**

Run:

```bash
go test ./internal/cli -run 'TestProjectWebhook' -count=1
```

Expected: command-not-found or compile failures until CLI commands are added.

- [ ] **Step 5: Commit fake BFF and failing tests only when compile is restored**

After Task 4 makes these tests compile and pass, include these changes in the Task 4 commit instead of creating a red commit.

Expected: no commit in this task.

## Task 4: CLI Commands and Business Methods

**Files:**
- Modify: `internal/cli/webhooks.go`
- Modify: `internal/cli/commands.go`
- Modify: `internal/cli/integration_test.go`

- [ ] **Step 1: Add command result builders**

Append to `internal/cli/webhooks.go`:

```go
type webhookCreateOptions struct {
	Feature        string
	Project        string
	URL            string
	EventInputs    []string
	Secret         string
	DeliveryRegion string
}

type webhookUpdateOptions struct {
	ConfigID       int
	Feature        string
	Project        string
	URL            string
	EventInputs    []string
	DeliveryRegion string
	Enabled        *bool
}

func (a *App) projectWebhookEvents(feature string) (map[string]any, error) {
	events, err := a.listWebhookEvents(feature)
	if err != nil {
		return nil, err
	}
	return map[string]any{"action": "webhook-events", "feature": feature, "items": events}, nil
}
```

Then add `projectWebhookList`, `projectWebhookShow`, `projectWebhookCreate`, `projectWebhookUpdate`, and `projectWebhookDelete` in the same file using the API helpers from Task 2. The methods must return maps with `action`, `projectId`, `projectName`, `feature`, and either `items` or a single normalized config field set. Use these exact action values:

```go
"webhook-list"
"webhook-show"
"webhook-create"
"webhook-update"
"webhook-delete"
```

- [ ] **Step 2: Implement create behavior**

In `projectWebhookCreate`, perform these operations in order:

```go
target, err := a.resolveProjectTarget(opts.Project)
events, err := a.listWebhookEvents(opts.Feature)
eventIDs, err := resolveWebhookEventIDs(events, opts.EventInputs, opts.Feature)
secret := strings.TrimSpace(opts.Secret)
if secret == "" { secret, err = generateWebhookSecret() }
region := opts.DeliveryRegion
if region == "" { region = defaultWebhookDeliveryRegion(target.region) } else { region, err = normalizeWebhookDeliveryRegion(region) }
body := map[string]any{"enabled": true, "eventIds": eventIDs, "secret": secret, "url": opts.URL, "urlRegion": region, "useIpWhitelist": false}
resp, err := a.createWebhookConfig(target.project.ProjectID, opts.Feature, body)
selected, err := selectCreatedWebhookConfig(resp, opts.URL, region, eventIDs, secret)
cfg := normalizeWebhookConfig(selected, events)
```

Return the unredacted `cfg.Secret` for create.

- [ ] **Step 3: Implement update behavior**

In `projectWebhookUpdate`, perform GET-merge-PUT:

```go
target, err := a.resolveProjectTarget(opts.Project)
events, err := a.listWebhookEvents(opts.Feature)
list, err := a.listWebhookConfigs(target.project.ProjectID, opts.Feature)
existing, err := findNCSConfigByID(list.Items, opts.ConfigID)
nextURL := existing.URL
nextEventIDs := existing.EventIDs
nextRegion := existing.URLRegion
nextEnabled := existing.Enabled
if opts.URL != "" { nextURL = opts.URL }
if len(opts.EventInputs) > 0 { nextEventIDs, err = resolveWebhookEventIDs(events, opts.EventInputs, opts.Feature) }
if opts.DeliveryRegion != "" { nextRegion, err = normalizeWebhookDeliveryRegion(opts.DeliveryRegion) }
if opts.Enabled != nil { nextEnabled = *opts.Enabled }
body := map[string]any{"enabled": nextEnabled, "eventIds": nextEventIDs, "url": nextURL, "urlRegion": nextRegion, "useIpWhitelist": existing.UseIPWhitelist}
resp, err := a.updateWebhookConfig(target.project.ProjectID, opts.Feature, opts.ConfigID, body)
selected, err := findNCSConfigByID(resp.Items, opts.ConfigID)
cfg := redactWebhookConfigSecret(normalizeWebhookConfig(selected, events), false)
```

Add `findNCSConfigByID` returning `WEBHOOK_CONFIG_NOT_FOUND` when absent.

- [ ] **Step 4: Register `project webhook` under project**

In `buildProjectCommand()`, add:

```go
cmd.AddCommand(a.buildProjectWebhook())
```

Add `buildProjectWebhook()` in `internal/cli/commands.go` with subcommands matching the spec:

```go
func (a *App) buildProjectWebhook() *cobra.Command {
	cmd := &cobra.Command{Use: "webhook", Short: "Manage project webhooks"}
	cmd.AddCommand(/* events, list, show, create, update, delete */)
	return cmd
}
```

Each subcommand must call `renderResult` with the stable labels:

```go
"project webhook events"
"project webhook list"
"project webhook show"
"project webhook create"
"project webhook update"
"project webhook delete"
```

- [ ] **Step 5: Wire command flags**

Use these Cobra flags:

```go
events.Flags().StringVar(&feature, "feature", "", fmt.Sprintf("feature to inspect: %s", featureListString()))
list.Flags().StringVar(&feature, "feature", "", fmt.Sprintf("feature to inspect: %s", featureListString()))
list.Flags().StringVar(&project, "project", "", "project name or ID")
show.Flags().BoolVar(&withSecret, "with-secret", false, "reveal webhook secret when the backend returns it")
create.Flags().StringArrayVar(&eventsInput, "event", nil, "webhook event key, numeric ID, or display name; repeat for multiple events")
create.Flags().StringVar(&deliveryRegion, "delivery-region", "", "webhook delivery region: cn, sea, na, eu")
update.Flags().BoolVar(&enabled, "enabled", false, "enable the webhook")
update.Flags().BoolVar(&disabled, "disabled", false, "disable the webhook")
```

Use `Args` validators to require `config-id` on show/update/delete and reject both `--enabled` and `--disabled` on update with `WEBHOOK_ENABLED_FLAG_CONFLICT`.

- [ ] **Step 6: Implement delete confirmation**

In the delete command, fail in JSON/non-TTY unless `--yes` is set by the root flag:

```go
if !a.rootYes && a.resolveOutputMode(cmd) == outputJSON {
	return &cliError{Message: "Deletion requires --yes in JSON mode.", Code: "CONFIRMATION_REQUIRED"}
}
```

For pretty interactive mode, use the same stdin/TTY style as existing commands if a prompt helper exists. If there is no prompt helper in the repo, require `--yes` for all modes in v1 and document that behavior in the command long text.

- [ ] **Step 7: Run integration tests**

Run:

```bash
go test ./internal/cli -run 'TestProjectWebhook' -count=1
```

Expected: webhook integration tests pass.

- [ ] **Step 8: Commit CLI behavior**

Run:

```bash
git add internal/cli/webhooks.go internal/cli/commands.go internal/cli/integration_test.go
git commit -m "feat: add project webhook commands"
```

Expected: one feature commit.

## Task 5: Pretty Rendering

**Files:**
- Modify: `internal/cli/render.go`
- Modify: `internal/cli/integration_test.go`

- [ ] **Step 1: Write pretty output integration checks**

Add one pretty-mode test:

```go
func TestProjectWebhookEventsPrettyOmitsPayload(t *testing.T) {
	bin := buildTestBinary(t)
	api := newFakeCLIBFF()
	defer api.server.Close()
	configHome := t.TempDir()
	persistSessionForIntegration(t, configHome)
	result := runCLI(t, bin, map[string]string{"XDG_CONFIG_HOME": configHome, "AGORA_CLI_BASE_URL": api.baseURL}, "project", "webhook", "events", "--feature", "rtc")
	if result.exitCode != 0 || !strings.Contains(result.stdout, "channel-created") || strings.Contains(result.stdout, `{"event":"created"}`) {
		t.Fatalf("unexpected pretty output: exit=%d stdout=%s stderr=%s", result.exitCode, result.stdout, result.stderr)
	}
}
```

- [ ] **Step 2: Run pretty test and verify it fails**

Run:

```bash
go test ./internal/cli -run 'TestProjectWebhookEventsPrettyOmitsPayload' -count=1
```

Expected: output uses default JSON-ish pretty fallback until render cases are added.

- [ ] **Step 3: Add render cases**

In `renderResult`, add cases:

```go
case "project webhook events":
	m := data.(map[string]any)
	fmt.Fprintf(out, "Webhook Events: %s\n", asString(m["feature"]))
	if items, ok := m["items"].([]webhookEvent); ok {
		for _, item := range items {
			fmt.Fprintf(out, "- %s  ID %d  %s\n", item.Key, item.ID, item.DisplayName)
		}
	}
case "project webhook list":
	m := data.(map[string]any)
	printBlock(out, "Webhooks", [][2]string{{"Project", asString(m["projectName"])}, {"Feature", asString(m["feature"])}})
	if items, ok := m["items"].([]webhookConfig); ok {
		for _, item := range items {
			fmt.Fprintf(out, "- %d  %s  %s  enabled=%v\n", item.ConfigID, item.URL, item.URLRegion, item.Enabled)
		}
	}
case "project webhook show", "project webhook create", "project webhook update":
	m := data.(map[string]any)
	printWebhookBlock(out, m)
	if command == "project webhook create" {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Store this secret now. It may not be shown again.")
	}
case "project webhook delete":
	m := data.(map[string]any)
	printBlock(out, "Webhook", [][2]string{{"Project", asString(m["projectName"])}, {"Feature", asString(m["feature"])}, {"Config ID", asString(m["configId"])}, {"Deleted", "true"}})
```

Add helper:

```go
func printWebhookBlock(out io.Writer, m map[string]any) {
	events := "-"
	if cfg, ok := m["config"].(webhookConfig); ok {
		keys := []string{}
		for _, event := range cfg.Events {
			keys = append(keys, event.Key)
		}
		if len(keys) > 0 {
			events = strings.Join(keys, ", ")
		}
		printBlock(out, "Webhook", [][2]string{{"Project", asString(m["projectName"])}, {"Feature", asString(m["feature"])}, {"Config ID", asString(cfg.ConfigID)}, {"URL", cfg.URL}, {"Events", events}, {"Delivery Region", renderWebhookDeliveryRegion(cfg.URLRegion)}, {"Enabled", asString(cfg.Enabled)}, {"Retry", asString(cfg.Retry)}, {"Secret", cfg.Secret}})
	}
}
```

Add:

```go
func renderWebhookDeliveryRegion(value string) string {
	switch value {
	case "cn":
		return "China (cn)"
	case "sea":
		return "Asia (sea)"
	case "na":
		return "North America (na)"
	case "eu":
		return "Europe (eu)"
	default:
		return value
	}
}
```

- [ ] **Step 4: Run pretty and JSON webhook tests**

Run:

```bash
go test ./internal/cli -run 'TestProjectWebhook' -count=1
```

Expected: all webhook integration tests pass.

- [ ] **Step 5: Commit rendering**

Run:

```bash
git add internal/cli/render.go internal/cli/integration_test.go
git commit -m "feat: render project webhook output"
```

Expected: one feature commit.

## Task 6: MCP Parity

**Files:**
- Modify: `internal/cli/mcp.go`
- Modify: `internal/cli/mcp_test.go`

- [ ] **Step 1: Update MCP surface test**

In `internal/cli/mcp_test.go`, add expected tool names:

```go
"agora.project.webhook.events",
"agora.project.webhook.list",
"agora.project.webhook.show",
"agora.project.webhook.create",
"agora.project.webhook.update",
"agora.project.webhook.delete",
```

- [ ] **Step 2: Run MCP test and verify it fails**

Run:

```bash
go test ./internal/cli -run 'TestMCPTools' -count=1
```

Expected: missing tool names.

- [ ] **Step 3: Add MCP descriptors**

In `mcpTools()`, add:

```go
mcpTool("agora.project.webhook.events", "List webhook events for a feature", map[string]string{"feature": "string"}),
mcpTool("agora.project.webhook.list", "List project webhook configs", map[string]string{"feature": "string", "project": "string"}),
mcpTool("agora.project.webhook.show", "Show one project webhook config", map[string]string{"configId": "number", "feature": "string", "project": "string", "withSecret": "boolean"}),
mcpTool("agora.project.webhook.create", "Create a project webhook config", map[string]string{"feature": "string", "project": "string", "url": "string", "events": "array", "secret": "string", "deliveryRegion": "string"}),
mcpTool("agora.project.webhook.update", "Update a project webhook config", map[string]string{"configId": "number", "feature": "string", "project": "string", "url": "string", "events": "array", "deliveryRegion": "string", "enabled": "boolean"}),
mcpTool("agora.project.webhook.delete", "Delete a project webhook config", map[string]string{"configId": "number", "feature": "string", "project": "string", "confirm": "boolean"}),
```

- [ ] **Step 4: Add MCP dispatch cases**

In `callMCPTool`, add cases that call the business methods:

```go
case "agora.project.webhook.events":
	return a.projectWebhookEvents(stringArg(args, "feature"))
case "agora.project.webhook.list":
	return a.projectWebhookList(stringArg(args, "feature"), stringArg(args, "project"), false)
case "agora.project.webhook.show":
	return a.projectWebhookShow(intArg(args, "configId", 0), stringArg(args, "feature"), stringArg(args, "project"), boolArg(args, "withSecret", false))
case "agora.project.webhook.create":
	return a.projectWebhookCreate(webhookCreateOptions{Feature: stringArg(args, "feature"), Project: stringArg(args, "project"), URL: stringArg(args, "url"), EventInputs: stringSliceArg(args, "events"), Secret: stringArg(args, "secret"), DeliveryRegion: stringArg(args, "deliveryRegion")})
case "agora.project.webhook.update":
	enabled := optionalBoolArg(args, "enabled")
	return a.projectWebhookUpdate(webhookUpdateOptions{ConfigID: intArg(args, "configId", 0), Feature: stringArg(args, "feature"), Project: stringArg(args, "project"), URL: stringArg(args, "url"), EventInputs: stringSliceArg(args, "events"), DeliveryRegion: stringArg(args, "deliveryRegion"), Enabled: enabled})
case "agora.project.webhook.delete":
	if !boolArg(args, "confirm", false) {
		return nil, &cliError{Message: "Webhook deletion requires confirm: true.", Code: "CONFIRMATION_REQUIRED"}
	}
	return a.projectWebhookDelete(intArg(args, "configId", 0), stringArg(args, "feature"), stringArg(args, "project"))
```

Add helper:

```go
func optionalBoolArg(args map[string]any, key string) *bool {
	if value, ok := args[key].(bool); ok {
		return &value
	}
	return nil
}
```

- [ ] **Step 5: Run MCP tests**

Run:

```bash
go test ./internal/cli -run 'TestMCP' -count=1
```

Expected: `ok`.

- [ ] **Step 6: Commit MCP parity**

Run:

```bash
git add internal/cli/mcp.go internal/cli/mcp_test.go
git commit -m "feat: expose project webhooks through mcp"
```

Expected: one feature commit.

## Task 7: Docs and Generated Command Reference

**Files:**
- Modify: `docs/automation.md`
- Modify: `README.md`
- Modify: `docs/commands.md`
- Modify: `docs/llms.txt`

- [ ] **Step 1: Update automation docs**

In `docs/automation.md`, add a `project webhook` section with these examples:

```bash
./agora project webhook events --feature rtc --json
./agora project webhook create --project my-project --feature rtc --url https://example.com/webhook --event channel-created --json
./agora project webhook show 42 --project my-project --feature rtc --with-secret --json
./agora project webhook update 42 --project my-project --feature rtc --url https://example.com/webhook2 --json
./agora project webhook delete 42 --project my-project --feature rtc --yes --json
```

Document that `enabled` is the canonical state field and that `status` is not emitted for webhooks.

- [ ] **Step 2: Update README command overview**

Add under the `project` command list:

```markdown
│   └── webhook
│       ├── events --feature <feature>       List available webhook event keys and IDs
│       ├── list --feature <feature>         List project webhook configs
│       ├── show <config-id> --feature ...   Show one webhook config
│       ├── create --feature ...             Create a webhook config
│       ├── update <config-id> --feature ... Update a webhook config
│       └── delete <config-id> --feature ... Delete a webhook config
```

- [ ] **Step 3: Regenerate command docs**

Run:

```bash
go run ./cmd/gendocs
```

Expected: `docs/commands.md` includes `agora project webhook` and all subcommands.

- [ ] **Step 4: Update `docs/llms.txt`**

`docs/llms.txt` is manually maintained in this repo. In the MCP notes section, add one bullet:

```markdown
- **Project webhooks**: MCP exposes `agora.project.webhook.{events,list,show,create,update,delete}` for feature-scoped webhook automation; delete requires `confirm: true`.
```

- [ ] **Step 5: Run doc example drift tests**

Run:

```bash
go test ./internal/cli -run 'TestAutomation|TestHelp|TestIntrospect' -count=1
```

Expected: docs/help/introspection tests pass.

- [ ] **Step 6: Commit docs**

Run:

```bash
git add docs/automation.md README.md docs/commands.md docs/llms.txt
git commit -m "docs: document project webhook commands"
```

Expected: one docs commit.

## Task 8: Full Verification and Cleanup

**Files:**
- Read: all modified files

- [ ] **Step 1: Run formatting**

Run:

```bash
gofmt -w internal/cli/webhooks.go internal/cli/webhooks_test.go internal/cli/commands.go internal/cli/render.go internal/cli/mcp.go internal/cli/mcp_test.go internal/cli/integration_test.go
```

Expected: no command output.

- [ ] **Step 2: Run full tests**

Run:

```bash
go test ./...
```

Expected: all packages pass.

- [ ] **Step 3: Inspect command tree**

Run:

```bash
go build -o agora .
./agora --help --all --json
```

Expected: JSON envelope includes the six `agora project webhook ...` command paths.

- [ ] **Step 4: Inspect worktree**

Run:

```bash
git status --short
```

Expected: no uncommitted files except intentional local binary `agora` if it is ignored.

- [ ] **Step 5: Commit final formatting or generated residue**

If Step 4 shows tracked formatting or generated-doc changes, commit them:

```bash
git add <tracked-files-from-step-4>
git commit -m "chore: finalize webhook cli implementation"
```

Expected: either one cleanup commit or no commit when the worktree is clean.

## Self-Review

Spec coverage:

- Command UX: Task 4 registers `events`, `list`, `show`, `create`, `update`, and `delete` with required `--feature`, optional `--project`, and stable command labels.
- Event discovery and keys: Tasks 1, 2, 3, and 4 cover event fetching, key generation, numeric IDs, exact display names, unknown and ambiguous errors, and no `displayNameCn` output.
- Delivery region: Tasks 1 and 4 cover validation plus `global -> na` and `cn -> cn` defaults.
- Secret handling: Tasks 1, 2, 3, and 4 cover 32-character generated secrets, explicit override, create reveal, list/show/update redaction, and no update `--secret`.
- Data model and adapter: Task 2 covers `items` arrays, normalization, create response selection, and config/event shapes.
- Update read-modify-write: Tasks 3 and 4 include an integration test that verifies omitted fields are preserved.
- JSON contract: Tasks 3, 4, and 7 cover envelope labels, `enabled` canonical state, and docs.
- Delete confirmation: Tasks 3, 4, and 6 cover CLI `--yes` and MCP `confirm: true`.
- MCP parity: Task 6 covers descriptors, dispatch, and tests.
- Docs and generated references: Task 7 covers automation docs, README, command docs, and `docs/llms.txt`.

Placeholder scan: run the forbidden-pattern search from the writing-plans skill and fix every match before executing.

Type consistency:

- `webhookEvent`, `webhookConfig`, `ncsEventListResponse`, `ncsConfigListResponse`, and `ncsConfig` are introduced before later tasks reference them.
- `projectWebhookCreate`, `projectWebhookUpdate`, and MCP dispatch use the same option type names.
- JSON keys use `configId`, `urlRegion`, `eventIds`, `useIpWhitelist`, and `enabled` consistently.
