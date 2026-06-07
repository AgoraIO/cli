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
	key := strings.ToLower(strings.TrimSpace(displayName))
	key = webhookEventKeyInvalidChars.ReplaceAllString(key, "-")
	return strings.Trim(key, "-")
}

func validateWebhookFeature(feature string) error {
	if strings.TrimSpace(feature) == "" {
		return &cliError{Message: "webhook feature is required", Code: "WEBHOOK_FEATURE_REQUIRED"}
	}
	return validateFeatureID(feature)
}

func normalizeWebhookDeliveryRegion(value string) (string, error) {
	region := strings.ToLower(strings.TrimSpace(value))
	switch region {
	case "cn", "sea", "na", "eu":
		return region, nil
	default:
		return "", &cliError{
			Message: fmt.Sprintf("invalid webhook delivery region %q. Choose one of: cn, sea, na, eu.", value),
			Code:    "WEBHOOK_DELIVERY_REGION_INVALID",
		}
	}
}

func defaultWebhookDeliveryRegion(controlPlaneRegion string) string {
	if strings.ToLower(strings.TrimSpace(controlPlaneRegion)) == "cn" {
		return "cn"
	}
	return "na"
}

func generateWebhookSecret() (string, error) {
	raw := make([]byte, 24)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(raw), nil
}

func validateWebhookSecret(secret string) error {
	if webhookSecretPattern.MatchString(secret) {
		return nil
	}
	return &cliError{
		Message: "webhook secret must be 7-32 URL-safe characters.",
		Code:    "WEBHOOK_SECRET_INVALID",
	}
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
	byID := make(map[int]webhookEvent, len(events))
	byKey := make(map[string][]webhookEvent, len(events))
	byDisplayName := make(map[string][]webhookEvent, len(events))
	for _, event := range events {
		byID[event.ID] = event
		if event.Key != "" {
			byKey[event.Key] = append(byKey[event.Key], event)
		}
		byDisplayName[event.DisplayName] = append(byDisplayName[event.DisplayName], event)
	}

	selected := make(map[int]struct{}, len(inputs))
	for _, input := range inputs {
		value := strings.TrimSpace(input)
		if value == "" {
			continue
		}
		if id, err := strconv.Atoi(value); err == nil {
			if _, ok := byID[id]; !ok {
				return nil, unknownWebhookEventError(input, feature)
			}
			selected[id] = struct{}{}
			continue
		}
		if matches := byKey[value]; len(matches) > 0 {
			ids := uniqueWebhookEventIDs(matches)
			if len(ids) > 1 {
				return nil, &cliError{
					Message: fmt.Sprintf("webhook event %q is ambiguous. Use the numeric event ID instead.", input),
					Code:    "WEBHOOK_EVENT_AMBIGUOUS",
				}
			}
			selected[ids[0]] = struct{}{}
			continue
		}
		if matches := byDisplayName[value]; len(matches) > 0 {
			ids := uniqueWebhookEventIDs(matches)
			if len(ids) > 1 {
				return nil, &cliError{
					Message: fmt.Sprintf("webhook event %q is ambiguous. Use the numeric event ID instead.", input),
					Code:    "WEBHOOK_EVENT_AMBIGUOUS",
				}
			}
			selected[ids[0]] = struct{}{}
			continue
		}
		return nil, unknownWebhookEventError(input, feature)
	}

	out := make([]int, 0, len(selected))
	for id := range selected {
		out = append(out, id)
	}
	sort.Ints(out)
	return out, nil
}

func uniqueWebhookEventIDs(events []webhookEvent) []int {
	seen := make(map[int]struct{}, len(events))
	ids := make([]int, 0, len(events))
	for _, event := range events {
		if _, ok := seen[event.ID]; ok {
			continue
		}
		seen[event.ID] = struct{}{}
		ids = append(ids, event.ID)
	}
	return ids
}

func unknownWebhookEventError(input string, feature string) error {
	return &cliError{
		Message: fmt.Sprintf("unknown webhook event %q. Run `agora project webhook events --feature %s` to see available events.", input, strings.TrimSpace(feature)),
		Code:    "WEBHOOK_EVENT_UNKNOWN",
	}
}
