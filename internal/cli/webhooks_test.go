package cli

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestWebhookEventKeyFromDisplayName(t *testing.T) {
	tests := []struct {
		name        string
		displayName string
		want        string
	}{
		{name: "lowercases and hyphenates words", displayName: "User Joined", want: "user-joined"},
		{name: "collapses invalid runs", displayName: " RTC: User_JOINED!!! ", want: "rtc-user-joined"},
		{name: "trims generated separators", displayName: "---Recording Started---", want: "recording-started"},
		{name: "drops non ascii separators", displayName: "中文 Event", want: "event"},
		{name: "all invalid becomes empty", displayName: "!!!", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := webhookEventKey(tt.displayName)
			if got != tt.want {
				t.Fatalf("webhookEventKey(%q) = %q, want %q", tt.displayName, got, tt.want)
			}
		})
	}
}

func TestNormalizeWebhookEventsIgnoresChineseDisplayName(t *testing.T) {
	resp := ncsEventListResponse{
		Items: []ncsEvent{
			{EventID: 1001, DisplayName: "Channel Created", DisplayNameCn: "频道创建", EventType: 7, Payload: `{"x":1}`},
		},
	}

	got := normalizeWebhookEvents(resp)
	want := []webhookEvent{
		{ID: 1001, Key: "channel-created", DisplayName: "Channel Created", EventType: 7, Payload: `{"x":1}`},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("normalizeWebhookEvents() = %#v, want %#v", got, want)
	}
}

func TestNormalizeWebhookConfigMapsKnownEventsOnly(t *testing.T) {
	retry := true
	events := []webhookEvent{
		{ID: 1001, Key: "channel-created", DisplayName: "Channel Created"},
		{ID: 1002, Key: "channel-deleted", DisplayName: "Channel Deleted"},
	}
	item := ncsConfig{
		ConfigID:       7,
		URL:            "https://example.com/hook",
		URLRegion:      "na",
		Enabled:        true,
		EventIDs:       []int{1002, 9999, 1001},
		Retry:          &retry,
		UseIPWhitelist: true,
		Secret:         "secret_123",
	}

	got := normalizeWebhookConfig(item, events)
	if got.ConfigID != item.ConfigID || got.URL != item.URL || got.URLRegion != item.URLRegion || !got.Enabled || got.Retry != &retry || !got.UseIPWhitelist || got.Secret != item.Secret {
		t.Fatalf("normalizeWebhookConfig() did not copy stable fields: %#v", got)
	}
	if !reflect.DeepEqual(got.EventIDs, item.EventIDs) {
		t.Fatalf("EventIDs = %v, want %v", got.EventIDs, item.EventIDs)
	}
	wantEvents := []webhookEvent{events[1], events[0]}
	if !reflect.DeepEqual(got.Events, wantEvents) {
		t.Fatalf("Events = %#v, want %#v", got.Events, wantEvents)
	}
}

func TestSelectWebhookConfigFromCreateResponsePrefersSecret(t *testing.T) {
	resp := ncsConfigListResponse{
		Items: []ncsConfig{
			{ConfigID: 17, URL: "https://example.com/hook", URLRegion: "na", EventIDs: []int{1001}, Secret: "other_secret"},
			{ConfigID: 42, URL: "https://example.com/hook", URLRegion: "na", EventIDs: []int{1001}, Secret: "secret_123"},
		},
	}

	got, err := selectCreatedWebhookConfig(resp, "https://example.com/hook", "na", []int{1001}, "secret_123")
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigID != 42 {
		t.Fatalf("selected configId = %d, want 42", got.ConfigID)
	}
}

func TestSelectWebhookConfigDoesNotSelectSecretWithWrongShape(t *testing.T) {
	resp := ncsConfigListResponse{
		Items: []ncsConfig{
			{ConfigID: 99, URL: "https://old.example.com/hook", URLRegion: "eu", EventIDs: []int{1002}, Secret: "secret_123", UpdatedAt: "2026-01-04T00:00:00Z"},
			{ConfigID: 42, URL: "https://example.com/hook", URLRegion: "na", EventIDs: []int{1001}, Secret: "secret_123", UpdatedAt: "2026-01-03T00:00:00Z"},
		},
	}

	got, err := selectCreatedWebhookConfig(resp, "https://example.com/hook", "na", []int{1001}, "secret_123")
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigID != 42 {
		t.Fatalf("selected configId = %d, want 42", got.ConfigID)
	}
}

func TestSelectWebhookConfigFallbackMatchesEventIDsAsSet(t *testing.T) {
	resp := ncsConfigListResponse{
		Items: []ncsConfig{
			{ConfigID: 42, URL: "https://example.com/hook", URLRegion: "na", EventIDs: []int{1002, 1001}},
		},
	}

	got, err := selectCreatedWebhookConfig(resp, "https://example.com/hook", "na", []int{1001, 1002}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigID != 42 {
		t.Fatalf("selected configId = %d, want 42", got.ConfigID)
	}
}

func TestSelectWebhookConfigFallbackPicksNewestThenHighestID(t *testing.T) {
	resp := ncsConfigListResponse{
		Items: []ncsConfig{
			{ConfigID: 17, URL: "https://example.com/hook", URLRegion: "na", EventIDs: []int{1001}, UpdatedAt: "2026-01-02T00:00:00Z"},
			{ConfigID: 42, URL: "https://example.com/hook", URLRegion: "na", EventIDs: []int{1001}, UpdatedAt: "2026-01-03T00:00:00Z"},
			{ConfigID: 50, URL: "https://example.com/hook", URLRegion: "na", EventIDs: []int{1001}, UpdatedAt: "2026-01-03T00:00:00Z"},
			{ConfigID: 99, URL: "https://example.com/hook", URLRegion: "na", EventIDs: []int{1001, 1002}, UpdatedAt: "2026-01-04T00:00:00Z"},
		},
	}

	got, err := selectCreatedWebhookConfig(resp, "https://example.com/hook", "na", []int{1001}, "")
	if err != nil {
		t.Fatal(err)
	}
	if got.ConfigID != 50 {
		t.Fatalf("selected configId = %d, want 50", got.ConfigID)
	}
}

func TestSelectWebhookConfigReturnsNotFound(t *testing.T) {
	_, err := selectCreatedWebhookConfig(ncsConfigListResponse{
		Items: []ncsConfig{
			{ConfigID: 17, URL: "https://example.com/hook", URLRegion: "eu", EventIDs: []int{1001}},
		},
	}, "https://example.com/hook", "na", []int{1001}, "")
	if !hasCLIErrorCode(err, "WEBHOOK_CONFIG_NOT_FOUND") {
		t.Fatalf("expected WEBHOOK_CONFIG_NOT_FOUND, got %T %v", err, err)
	}
}

func TestRedactWebhookConfigSecret(t *testing.T) {
	cfg := webhookConfig{ConfigID: 42, Secret: "secret_123"}

	redacted := redactWebhookConfigSecret(cfg, false)
	if redacted.Secret != redactedWebhookSecret {
		t.Fatalf("redacted secret = %q, want %q", redacted.Secret, redactedWebhookSecret)
	}

	revealed := redactWebhookConfigSecret(cfg, true)
	if revealed.Secret != cfg.Secret {
		t.Fatalf("revealed secret = %q, want %q", revealed.Secret, cfg.Secret)
	}
}

func TestResolveWebhookEventInputs(t *testing.T) {
	events := []webhookEvent{
		{ID: 30, Key: "channel-user-left", DisplayName: "Channel User Left"},
		{ID: 10, Key: "channel-user-joined", DisplayName: "Channel User Joined"},
		{ID: 20, Key: "recording_started", DisplayName: "Recording Started"},
	}

	got, err := resolveWebhookEventIDs(events, []string{
		"channel-user-joined",
		"30",
		"Recording Started",
		"10",
	}, "rtc")
	if err != nil {
		t.Fatal(err)
	}
	want := []int{10, 20, 30}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolved event IDs = %v, want %v", got, want)
	}
}

func TestResolveWebhookEventInputsRejectsUnknownAndAmbiguous(t *testing.T) {
	events := []webhookEvent{
		{ID: 10, Key: "channel-user-joined", DisplayName: "Channel User Joined"},
		{ID: 20, Key: "recording-started", DisplayName: "Recording Started"},
		{ID: 30, Key: "recording-started", DisplayName: "Recording Started Duplicate"},
	}

	_, err := resolveWebhookEventIDs(events, []string{"not-real"}, "rtc")
	if !hasCLIErrorCode(err, "WEBHOOK_EVENT_UNKNOWN") {
		t.Fatalf("expected WEBHOOK_EVENT_UNKNOWN, got %T %v", err, err)
	}
	if !strings.Contains(err.Error(), "agora project webhook events --feature rtc") {
		t.Fatalf("expected list suggestion in error, got %q", err.Error())
	}

	_, err = resolveWebhookEventIDs(events, []string{"recording-started"}, "rtc")
	if !hasCLIErrorCode(err, "WEBHOOK_EVENT_AMBIGUOUS") {
		t.Fatalf("expected WEBHOOK_EVENT_AMBIGUOUS, got %T %v", err, err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "numeric event id") {
		t.Fatalf("expected numeric event ID guidance, got %q", err.Error())
	}
}

func TestWebhookResolveEventInputsIgnoresEmptyValues(t *testing.T) {
	events := []webhookEvent{
		{ID: 10, Key: "channel-user-joined", DisplayName: "Channel User Joined"},
	}

	got, err := resolveWebhookEventIDs(events, []string{"", "  ", "channel-user-joined"}, "rtc")
	if err != nil {
		t.Fatal(err)
	}
	want := []int{10}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolved event IDs = %v, want %v", got, want)
	}
}

func TestNonEmptyWebhookEventInputsSplitsCommaSeparatedValues(t *testing.T) {
	got := nonEmptyWebhookEventInputs([]string{"1001, 1002,, channel-created ", "  "})
	want := []string{"1001", "1002", "channel-created"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("nonEmptyWebhookEventInputs() = %v, want %v", got, want)
	}
}

func TestWebhookResolveEventInputsDoesNotUseGeneratedDisplayNameKey(t *testing.T) {
	events := []webhookEvent{
		{ID: 10, Key: "backend-event-key", DisplayName: "Display Name"},
	}

	_, err := resolveWebhookEventIDs(events, []string{"display-name"}, "rtc")
	if !hasCLIErrorCode(err, "WEBHOOK_EVENT_UNKNOWN") {
		t.Fatalf("expected WEBHOOK_EVENT_UNKNOWN, got %T %v", err, err)
	}

	got, err := resolveWebhookEventIDs(events, []string{"Display Name"}, "rtc")
	if err != nil {
		t.Fatal(err)
	}
	want := []int{10}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("resolved event IDs = %v, want %v", got, want)
	}
}

func TestWebhookResolveEventInputsRejectsAmbiguousDisplayName(t *testing.T) {
	events := []webhookEvent{
		{ID: 10, Key: "first-key", DisplayName: "Recording Finished"},
		{ID: 20, Key: "second-key", DisplayName: "Recording Finished"},
	}

	_, err := resolveWebhookEventIDs(events, []string{"Recording Finished"}, "rtc")
	if !hasCLIErrorCode(err, "WEBHOOK_EVENT_AMBIGUOUS") {
		t.Fatalf("expected WEBHOOK_EVENT_AMBIGUOUS, got %T %v", err, err)
	}
	if !strings.Contains(strings.ToLower(err.Error()), "numeric event id") {
		t.Fatalf("expected numeric event ID guidance, got %q", err.Error())
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
	if !webhookSecretPattern.MatchString(secret) {
		t.Fatalf("secret %q does not match backend pattern", secret)
	}
	if err := validateWebhookSecret(secret); err != nil {
		t.Fatalf("generated secret did not validate: %v", err)
	}
}

func TestWebhookDeliveryRegionDefault(t *testing.T) {
	tests := []struct {
		controlPlaneRegion string
		want               string
	}{
		{controlPlaneRegion: "cn", want: "cn"},
		{controlPlaneRegion: "global", want: "na"},
		{controlPlaneRegion: "", want: "na"},
		{controlPlaneRegion: "eu", want: "na"},
	}

	for _, tt := range tests {
		t.Run(tt.controlPlaneRegion, func(t *testing.T) {
			got := defaultWebhookDeliveryRegion(tt.controlPlaneRegion)
			if got != tt.want {
				t.Fatalf("defaultWebhookDeliveryRegion(%q) = %q, want %q", tt.controlPlaneRegion, got, tt.want)
			}
		})
	}
}

func TestValidateWebhookFeature(t *testing.T) {
	if err := validateWebhookFeature("rtc"); err != nil {
		t.Fatalf("expected known feature to validate, got %v", err)
	}
	if err := validateWebhookFeature(""); !hasCLIErrorCode(err, "WEBHOOK_FEATURE_REQUIRED") {
		t.Fatalf("expected WEBHOOK_FEATURE_REQUIRED, got %T %v", err, err)
	}
	if err := validateWebhookFeature("unknown"); err == nil || hasCLIErrorCode(err, "WEBHOOK_FEATURE_REQUIRED") {
		t.Fatalf("expected validateFeatureID error for unknown feature, got %T %v", err, err)
	}
}

func TestValidateWebhookSecret(t *testing.T) {
	if err := validateWebhookSecret("abc_DEF-123"); err != nil {
		t.Fatalf("expected valid secret, got %v", err)
	}
	for _, secret := range []string{"", "short", strings.Repeat("a", 33), "has spaces"} {
		err := validateWebhookSecret(secret)
		if !hasCLIErrorCode(err, "WEBHOOK_SECRET_INVALID") {
			t.Fatalf("validateWebhookSecret(%q) expected WEBHOOK_SECRET_INVALID, got %T %v", secret, err, err)
		}
	}
}

func TestValidateWebhookDeliveryRegion(t *testing.T) {
	for _, input := range []string{"cn", " SEA ", "na", "EU"} {
		got, err := normalizeWebhookDeliveryRegion(input)
		if err != nil {
			t.Fatalf("normalizeWebhookDeliveryRegion(%q): %v", input, err)
		}
		if got != strings.ToLower(strings.TrimSpace(input)) {
			t.Fatalf("normalizeWebhookDeliveryRegion(%q) = %q", input, got)
		}
	}
	if _, err := normalizeWebhookDeliveryRegion("global"); !hasCLIErrorCode(err, "WEBHOOK_DELIVERY_REGION_INVALID") {
		t.Fatalf("expected WEBHOOK_DELIVERY_REGION_INVALID, got %T %v", err, err)
	}
}

func TestWebhookIntSlicesEqual(t *testing.T) {
	if !webhookIntSlicesEqual([]int{1, 2}, []int{1, 2}) {
		t.Fatal("expected equal slices")
	}
	if webhookIntSlicesEqual([]int{1, 2}, []int{2, 1}) {
		t.Fatal("expected order-sensitive mismatch")
	}
	if webhookIntSlicesEqual([]int{1}, []int{1, 2}) {
		t.Fatal("expected length mismatch")
	}
}

func hasCLIErrorCode(err error, code string) bool {
	var structured *cliError
	return errors.As(err, &structured) && structured.Code == code
}
