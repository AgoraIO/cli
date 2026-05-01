package cli

// Integration tests for `agora login` / `agora whoami` / `agora auth status`.
// Shared helpers live in integration_test.go.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestCLILoginAndWhoAmIParity(t *testing.T) {
	configHome := t.TempDir()
	oauth := newFakeOAuthServer()
	defer oauth.server.Close()

	result := runCLI(t, []string{"login"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":         configHome,
		"AGORA_OAUTH_BASE_URL":    oauth.baseURL,
		"AGORA_OAUTH_CLIENT_ID":   "test-public-client",
		"AGORA_OAUTH_SCOPE":       "basic_info,console",
		"AGORA_BROWSER_AUTO_OPEN": "0",
		"AGORA_LOGIN_TIMEOUT_MS":  "2000",
		"AGORA_LOG_LEVEL":         "error",
		"AGORA_DEBUG":             "0",
	}, onStderr: func(stderr string) bool {
		u := parseAuthURL(stderr)
		if u == "" {
			return false
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			resp.Body.Close()
		}
		return err == nil
	}})
	if result.exitCode != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s", result.exitCode, result.stderr)
	}

	status := runCLI(t, []string{"whoami", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME": configHome,
		"AGORA_LOG_LEVEL": "error",
		"AGORA_DEBUG":     "0",
	}})
	if status.exitCode != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s", status.exitCode, status.stderr)
	}
	if len(oauth.authorizeRedirectURIs) != 1 || !strings.Contains(oauth.authorizeRedirectURIs[0], "http://localhost:") {
		t.Fatalf("expected localhost redirect URI, got %+v", oauth.authorizeRedirectURIs)
	}
	if len(oauth.authorizeRawQueries) != 1 || !strings.Contains(oauth.authorizeRawQueries[0], "code_challenge=") || !strings.Contains(oauth.authorizeRawQueries[0], "code_challenge_method=S256") {
		t.Fatalf("expected authorize URL to include PKCE challenge, got %+v", oauth.authorizeRawQueries)
	}
	if len(oauth.tokenRequests) != 1 || !strings.Contains(oauth.tokenRequests[0], "code_verifier=") {
		t.Fatalf("expected token request to include PKCE verifier, got %+v", oauth.tokenRequests)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(status.stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	data := envelope["data"].(map[string]any)
	if data["authenticated"] != true {
		t.Fatalf("expected authenticated response, got %v", status.stdout)
	}
}

func TestCLIAuthStatusExitCodeParity(t *testing.T) {
	result := runCLI(t, []string{"auth", "status", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME": t.TempDir(),
		"AGORA_LOG_LEVEL": "error",
		"AGORA_DEBUG":     "0",
	}})
	if result.exitCode != 3 || !strings.Contains(result.stdout, `"ok":false`) || !strings.Contains(result.stdout, `"code":"AUTH_UNAUTHENTICATED"`) || !strings.Contains(result.stdout, `"exitCode":3`) || result.stderr != "" {
		t.Fatalf("expected structured unauthenticated status error, got exit=%d stdout=%s stderr=%s", result.exitCode, result.stdout, result.stderr)
	}
}
