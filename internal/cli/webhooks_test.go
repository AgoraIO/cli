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
