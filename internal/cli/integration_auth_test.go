package cli

// Integration tests for `agora login` / `agora whoami` / `agora auth status`.
// Shared helpers live in integration_test.go.

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type loginBrowserResult struct {
	status int
	body   string
	err    error
}

func runLoginAndCaptureBrowser(t *testing.T, configHome string, oauth *fakeOAuthServer) (cliResult, loginBrowserResult) {
	t.Helper()
	browser := loginBrowserResult{}
	followed := false
	result := runCLI(t, []string{"login", "--region", "global"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":         configHome,
		"AGORA_OAUTH_BASE_URL":    oauth.baseURL,
		"AGORA_OAUTH_CLIENT_ID":   "test-public-client",
		"AGORA_OAUTH_SCOPE":       "basic_info,console",
		"AGORA_BROWSER_AUTO_OPEN": "0",
		"AGORA_LOGIN_TIMEOUT_MS":  "2000",
		"AGORA_LOG_LEVEL":         "error",
		"AGORA_DEBUG":             "0",
	}, onStderr: func(stderr string) bool {
		if followed {
			return false
		}
		u := parseAuthURL(stderr)
		if u == "" {
			return false
		}
		followed = true
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			browser.err = err
			return false
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		browser.status = resp.StatusCode
		browser.body = string(body)
		browser.err = err
		return true
	}})
	return result, browser
}

func TestCLILoginAndWhoAmI(t *testing.T) {
	configHome := t.TempDir()
	oauth := newFakeOAuthServer()
	defer oauth.server.Close()
	browserBody := ""
	browserStatus := 0
	followed := false

	result := runCLI(t, []string{"login", "--region", "cn"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":         configHome,
		"AGORA_OAUTH_BASE_URL":    oauth.baseURL,
		"AGORA_OAUTH_CLIENT_ID":   "test-public-client",
		"AGORA_OAUTH_SCOPE":       "basic_info,console",
		"AGORA_BROWSER_AUTO_OPEN": "0",
		"AGORA_LOGIN_TIMEOUT_MS":  "2000",
		"AGORA_LOG_LEVEL":         "error",
		"AGORA_DEBUG":             "0",
	}, onStderr: func(stderr string) bool {
		if followed {
			return false
		}
		u := parseAuthURL(stderr)
		if u == "" {
			return false
		}
		followed = true
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			body, readErr := io.ReadAll(resp.Body)
			if readErr != nil {
				return false
			}
			browserBody = string(body)
			browserStatus = resp.StatusCode
			resp.Body.Close()
		}
		return err == nil
	}})
	if result.exitCode != 0 {
		t.Fatalf("expected exit 0, got %d stderr=%s", result.exitCode, result.stderr)
	}
	if browserStatus != http.StatusOK || !strings.Contains(browserBody, "认证成功") || !strings.Contains(browserBody, `aria-label="声网"`) {
		t.Fatalf("expected localized CN success page, got status=%d body=%s", browserStatus, browserBody)
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
	if data["region"] != "cn" {
		t.Fatalf("expected persisted auth region cn, got %v", data["region"])
	}
}

func TestCLIAuthStatusExitCode(t *testing.T) {
	result := runCLI(t, []string{"auth", "status", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME": t.TempDir(),
		"AGORA_LOG_LEVEL": "error",
		"AGORA_DEBUG":     "0",
	}})
	if result.exitCode != 3 || !strings.Contains(result.stdout, `"ok":false`) || !strings.Contains(result.stdout, `"code":"AUTH_UNAUTHENTICATED"`) || !strings.Contains(result.stdout, `"exitCode":3`) || result.stderr != "" {
		t.Fatalf("expected structured unauthenticated status error, got exit=%d stdout=%s stderr=%s", result.exitCode, result.stdout, result.stderr)
	}
}

func TestCLILoginTokenExchangeFailureRendersErrorPage(t *testing.T) {
	configHome := t.TempDir()
	oauth := newFakeOAuthServer()
	oauth.tokenStatus = http.StatusBadRequest
	oauth.tokenBody = `{"error":"invalid_grant","secret":"must-not-reach-browser"}`
	defer oauth.server.Close()

	result, browser := runLoginAndCaptureBrowser(t, configHome, oauth)
	if result.exitCode == 0 {
		t.Fatalf("expected login failure, got stdout=%s stderr=%s", result.stdout, result.stderr)
	}
	if browser.err != nil {
		t.Fatal(browser.err)
	}
	if browser.status != http.StatusInternalServerError {
		t.Fatalf("expected browser status 500, got %d", browser.status)
	}
	for _, forbidden := range []string{"Authentication successful", "must-not-reach-browser", "invalid_grant"} {
		if strings.Contains(browser.body, forbidden) {
			t.Fatalf("failure page contained %q", forbidden)
		}
	}
	saved, err := loadSession(map[string]string{"XDG_CONFIG_HOME": configHome})
	if err != nil {
		t.Fatal(err)
	}
	if saved != nil {
		t.Fatal("token-exchange failure should not save a session")
	}
}

func TestCLILoginSessionWriteFailureRendersErrorPage(t *testing.T) {
	configHome := t.TempDir()
	sessionPath := filepath.Join(configHome, "agora-cli", "session.json")
	if err := os.MkdirAll(sessionPath, 0o700); err != nil {
		t.Fatal(err)
	}
	oauth := newFakeOAuthServer()
	defer oauth.server.Close()

	result, browser := runLoginAndCaptureBrowser(t, configHome, oauth)
	if result.exitCode == 0 {
		t.Fatalf("expected login failure, got stdout=%s stderr=%s", result.stdout, result.stderr)
	}
	if browser.err != nil {
		t.Fatal(browser.err)
	}
	if browser.status != http.StatusInternalServerError {
		t.Fatalf("expected browser status 500, got %d", browser.status)
	}
	if strings.Contains(browser.body, "Authentication successful") || strings.Contains(browser.body, "access-token-value") {
		t.Fatal("session-write failure rendered success or exposed credentials")
	}
}
