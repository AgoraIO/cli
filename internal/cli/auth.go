package cli

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"
)

func (a *App) login(noBrowser bool, region string) (map[string]any, error) {
	config := a.oauthConfig()
	pair, err := generatePKCE()
	if err != nil {
		return nil, err
	}
	state, err := randomToken(24)
	if err != nil {
		return nil, err
	}
	timeout := 120 * time.Second
	if raw := strings.TrimSpace(a.env["AGORA_LOGIN_TIMEOUT_MS"]); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed <= 0 {
			return nil, errors.New("AGORA_LOGIN_TIMEOUT_MS must be a positive number.")
		}
		timeout = time.Duration(parsed) * time.Millisecond
	}
	callback, err := waitForOAuthCallback(state, timeout)
	if err != nil {
		return nil, err
	}
	defer callback.Close()
	u, _ := url.Parse(config.AuthorizeURL)
	q := u.Query()
	q.Set("response_type", "code")
	q.Set("client_id", config.ClientID)
	q.Set("redirect_uri", callback.RedirectURI)
	q.Set("scope", config.Scope)
	q.Set("state", state)
	q.Set("code_challenge", pair.CodeChallenge)
	q.Set("code_challenge_method", "S256")
	u.RawQuery = q.Encode()
	fmt.Fprintf(os.Stderr, "Open this URL to continue login:\n%s\n", u.String())
	if !noBrowser && a.env["AGORA_BROWSER_AUTO_OPEN"] != "0" {
		if !openBrowser(u.String()) {
			fmt.Fprintln(os.Stderr, "Browser did not open automatically. Copy the URL above or re-run with --no-browser.")
		}
	}
	payload, err := callback.Wait()
	if err != nil {
		return nil, err
	}
	token, err := a.exchangeAuthorizationCode(config.TokenURL, config.ClientID, payload.Code, pair.CodeVerifier, callback.RedirectURI)
	if err != nil {
		return nil, err
	}
	if err := saveSession(a.env, token); err != nil {
		return nil, err
	}
	ctx, err := loadContext(a.env)
	if err != nil {
		return nil, err
	}
	nextRegion := ctx.PreferredRegion
	if nextRegion == "" {
		nextRegion = "global"
	}
	if region == "global" || region == "cn" {
		nextRegion = region
	}
	ctx.CurrentRegion = nextRegion
	ctx.PreferredRegion = nextRegion
	if err := saveContext(a.env, ctx); err != nil {
		return nil, err
	}
	return map[string]any{"action": "login", "expiresAt": token.ExpiresAt, "scope": token.Scope, "status": "authenticated"}, nil
}

func (a *App) logout() (map[string]any, error) {
	cleared, err := clearSession(a.env)
	if err != nil {
		return nil, err
	}
	if err := clearContext(a.env); err != nil {
		return nil, err
	}
	return map[string]any{"action": "logout", "clearedSession": cleared, "status": "logged-out"}, nil
}

func (a *App) authStatus() (map[string]any, error) {
	s, err := loadSession(a.env)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return map[string]any{"action": "status", "authenticated": false, "expiresAt": nil, "scope": nil, "status": "unauthenticated"}, nil
	}
	return map[string]any{"action": "status", "authenticated": true, "expiresAt": s.ExpiresAt, "scope": s.Scope, "status": "authenticated"}, nil
}

type oauthConfig struct {
	AuthorizeURL string
	TokenURL     string
	ClientID     string
	Scope        string
}

func (a *App) oauthConfig() oauthConfig {
	base := strings.TrimRight(a.env["AGORA_OAUTH_BASE_URL"], "/")
	return oauthConfig{
		AuthorizeURL: base + "/api/v0/oauth/authorize",
		TokenURL:     base + "/api/v0/oauth/token",
		ClientID:     a.env["AGORA_OAUTH_CLIENT_ID"],
		Scope:        a.env["AGORA_OAUTH_SCOPE"],
	}
}

type pkcePair struct {
	CodeVerifier  string
	CodeChallenge string
}

func generatePKCE() (pkcePair, error) {
	raw, err := randomToken(64)
	if err != nil {
		return pkcePair{}, err
	}
	sum := sha256.Sum256([]byte(raw))
	return pkcePair{
		CodeVerifier:  raw,
		CodeChallenge: base64.RawURLEncoding.EncodeToString(sum[:]),
	}, nil
}

func randomToken(n int) (string, error) {
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func openBrowser(target string) bool {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", target)
	case "windows":
		cmd = exec.Command("cmd", "/c", "start", "", target)
	default:
		cmd = exec.Command("xdg-open", target)
	}
	return cmd.Start() == nil
}

type callbackServer struct {
	RedirectURI string
	wait        chan callbackPayload
	errs        chan error
	server      *http.Server
	listener    net.Listener
}

type callbackPayload struct {
	Code  string
	State string
}

func waitForOAuthCallback(expectedState string, timeout time.Duration) (*callbackServer, error) {
	wait := make(chan callbackPayload, 1)
	errs := make(chan error, 1)
	mux := http.NewServeMux()
	srv := &http.Server{Handler: mux}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	cs := &callbackServer{
		RedirectURI: fmt.Sprintf("http://localhost:%d/oauth/callback", ln.Addr().(*net.TCPAddr).Port),
		wait:        wait,
		errs:        errs,
		server:      srv,
		listener:    ln,
	}
	mux.HandleFunc("/oauth/callback", func(w http.ResponseWriter, r *http.Request) {
		code := r.URL.Query().Get("code")
		state := r.URL.Query().Get("state")
		oauthErr := r.URL.Query().Get("error")
		switch {
		case oauthErr != "":
			http.Error(w, "Agora CLI login failed. Return to the terminal for details.", http.StatusBadRequest)
			errs <- fmt.Errorf("OAuth authorization failed: %s", oauthErr)
		case code == "" || state == "":
			http.Error(w, "Agora CLI login callback was missing required fields.", http.StatusBadRequest)
		case state != expectedState:
			http.Error(w, "Agora CLI login state mismatch.", http.StatusBadRequest)
			errs <- errors.New("OAuth state mismatch.")
		default:
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "Agora CLI login complete. You can close this browser window.")
			wait <- callbackPayload{Code: code, State: state}
		}
	})
	go func() {
		_ = srv.Serve(ln)
	}()
	go func() {
		<-time.After(timeout)
		errs <- errors.New("Timed out waiting for the OAuth callback. Re-run with --no-browser to copy the URL manually, or check that your browser completed the login flow.")
	}()
	return cs, nil
}

func (c *callbackServer) Wait() (callbackPayload, error) {
	select {
	case payload := <-c.wait:
		return payload, nil
	case err := <-c.errs:
		return callbackPayload{}, err
	}
}

func (c *callbackServer) Close() error {
	return c.server.Close()
}

type tokenResponse struct {
	AccessToken  string      `json:"access_token"`
	ExpiresIn    int         `json:"expires_in"`
	RefreshToken string      `json:"refresh_token"`
	Scope        interface{} `json:"scope"`
	TokenType    string      `json:"token_type"`
}

func normalizeScope(v interface{}) string {
	switch x := v.(type) {
	case string:
		return x
	case []interface{}:
		out := make([]string, 0, len(x))
		for _, item := range x {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return strings.Join(out, ",")
	default:
		return ""
	}
}

func (a *App) exchangeAuthorizationCode(tokenURL, clientID, code, codeVerifier, redirectURI string) (session, error) {
	values := url.Values{
		"client_id":     []string{clientID},
		"code":          []string{code},
		"code_verifier": []string{codeVerifier},
		"grant_type":    []string{"authorization_code"},
		"redirect_uri":  []string{redirectURI},
	}
	return a.exchangeToken(tokenURL, values)
}

func (a *App) refreshAccessToken(refreshToken string) (session, error) {
	cfg := a.oauthConfig()
	values := url.Values{
		"client_id":     []string{cfg.ClientID},
		"grant_type":    []string{"refresh_token"},
		"refresh_token": []string{refreshToken},
	}
	return a.exchangeToken(cfg.TokenURL, values)
}

func (a *App) exchangeToken(tokenURL string, values url.Values) (session, error) {
	req, err := http.NewRequest(http.MethodPost, tokenURL, strings.NewReader(values.Encode()))
	if err != nil {
		return session{}, err
	}
	req.Header.Set("content-type", "application/x-www-form-urlencoded")
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return session{}, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return session{}, fmt.Errorf("OAuth token exchange failed with status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var token tokenResponse
	if err := json.Unmarshal(body, &token); err != nil {
		return session{}, err
	}
	if token.AccessToken == "" || token.RefreshToken == "" || token.TokenType == "" || token.ExpiresIn <= 0 || normalizeScope(token.Scope) == "" {
		var raw map[string]any
		_ = json.Unmarshal(body, &raw)
		_ = appendAppLog("debug", "oauth.token.response.invalid_shape", a.env, map[string]any{
			"response":     raw,
			"responseKeys": sortedMapKeys(raw),
		})
		return session{}, errors.New("OAuth token response was missing required fields.")
	}
	now := time.Now().UTC()
	return session{
		AccessToken:  token.AccessToken,
		RefreshToken: token.RefreshToken,
		TokenType:    token.TokenType,
		Scope:        normalizeScope(token.Scope),
		ObtainedAt:   now.Format(time.RFC3339),
		ExpiresAt:    now.Add(time.Duration(token.ExpiresIn) * time.Second).Format(time.RFC3339),
	}, nil
}

func isAuthRequired(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "Run `agora login` first") || strings.Contains(msg, "No refresh token available")
}

func (a *App) ensureValidAccessToken() (*session, error) {
	s, err := loadSession(a.env)
	if err != nil {
		return nil, err
	}
	if s == nil {
		return nil, errors.New("No local Agora session found. Run `agora login` first.")
	}
	expiry, err := time.Parse(time.RFC3339, s.ExpiresAt)
	if err != nil {
		return nil, err
	}
	if expiry.After(time.Now().Add(1 * time.Minute)) {
		return s, nil
	}
	refreshed, err := a.refreshAccessToken(s.RefreshToken)
	if err != nil {
		return nil, err
	}
	if err := saveSession(a.env, refreshed); err != nil {
		return nil, err
	}
	return &refreshed, nil
}

func (a *App) apiRequest(method, pathname string, query map[string]string, body any, out any) error {
	s, err := a.ensureValidAccessToken()
	if err != nil {
		return err
	}
	makeReq := func(token *session) (*http.Request, error) {
		base := strings.TrimRight(a.env["AGORA_API_BASE_URL"], "/")
		u, err := url.Parse(base + pathname)
		if err != nil {
			return nil, err
		}
		q := u.Query()
		for k, v := range query {
			if v != "" {
				q.Set(k, v)
			}
		}
		u.RawQuery = q.Encode()
		var reader io.Reader
		if body != nil {
			raw, err := json.Marshal(body)
			if err != nil {
				return nil, err
			}
			reader = bytes.NewReader(raw)
		}
		req, err := http.NewRequest(method, u.String(), reader)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Authorization", token.TokenType+" "+token.AccessToken)
		if body != nil {
			req.Header.Set("content-type", "application/json")
		}
		return req, nil
	}
	for attempt := 0; attempt < 2; attempt++ {
		req, err := makeReq(s)
		if err != nil {
			return err
		}
		resp, err := a.httpClient.Do(req)
		if err != nil {
			return err
		}
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusUnauthorized && attempt == 0 {
			refreshed, err := a.refreshAccessToken(s.RefreshToken)
			if err != nil {
				return err
			}
			if err := saveSession(a.env, refreshed); err != nil {
				return err
			}
			s = &refreshed
			continue
		}
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			if resp.StatusCode == http.StatusUnauthorized {
				return fmt.Errorf("session expired or invalid. Run `agora login` to re-authenticate.")
			}
			return fmt.Errorf("API error (HTTP %d): %s", resp.StatusCode, strings.TrimSpace(string(raw)))
		}
		return json.Unmarshal(raw, out)
	}
	return fmt.Errorf("%s %s failed after retry", method, pathname)
}
