package cli

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

func TestOAuthConfigForRegion(t *testing.T) {
	app := &App{
		cfg: defaultConfig(),
		env: map[string]string{
			"AGORA_OAUTH_CLIENT_ID": "test-client",
			"AGORA_OAUTH_SCOPE":     "basic_info,console",
		},
	}

	t.Run("cn region uses shengwang sso by default", func(t *testing.T) {
		cfg := app.oauthConfigForRegion("cn")
		if cfg.AuthorizeURL != cnOAuthBaseURL+"/api/v0/oauth/authorize" {
			t.Fatalf("unexpected authorize url: %s", cfg.AuthorizeURL)
		}
		if cfg.TokenURL != cnOAuthBaseURL+"/api/v0/oauth/token" {
			t.Fatalf("unexpected token url: %s", cfg.TokenURL)
		}
	})

	t.Run("global uses default agora sso", func(t *testing.T) {
		cfg := app.oauthConfigForRegion("global")
		if cfg.AuthorizeURL != globalOAuthBaseURL+"/api/v0/oauth/authorize" {
			t.Fatalf("unexpected authorize url: %s", cfg.AuthorizeURL)
		}
		if cfg.TokenURL != globalOAuthBaseURL+"/api/v0/oauth/token" {
			t.Fatalf("unexpected token url: %s", cfg.TokenURL)
		}
	})

	t.Run("env override wins over region default", func(t *testing.T) {
		app.env["AGORA_OAUTH_BASE_URL"] = "https://auth.example.com"
		cfg := app.oauthConfigForRegion("cn")
		if cfg.AuthorizeURL != "https://auth.example.com/api/v0/oauth/authorize" {
			t.Fatalf("unexpected authorize url: %s", cfg.AuthorizeURL)
		}
		if cfg.TokenURL != "https://auth.example.com/api/v0/oauth/token" {
			t.Fatalf("unexpected token url: %s", cfg.TokenURL)
		}
	})

	t.Run("client and scope default without env", func(t *testing.T) {
		app.env = map[string]string{}
		cfg := app.oauthConfigForRegion("global")
		if cfg.ClientID != defaultOAuthClientID || cfg.Scope != defaultOAuthScope {
			t.Fatalf("unexpected oauth defaults: %+v", cfg)
		}
	})
}

func TestAPIBaseURLForRegion(t *testing.T) {
	app := &App{
		cfg: defaultConfig(),
	}

	t.Run("cn region uses cn cli api by default", func(t *testing.T) {
		if got := app.apiBaseURLForRegion("cn"); got != cnAPIBaseURL {
			t.Fatalf("unexpected api base url: %s", got)
		}
	})

	t.Run("global uses default cli api", func(t *testing.T) {
		if got := app.apiBaseURLForRegion("global"); got != globalAPIBaseURL {
			t.Fatalf("unexpected api base url: %s", got)
		}
	})

	t.Run("env override wins over region default", func(t *testing.T) {
		app.osEnv = map[string]string{"AGORA_API_BASE_URL": "https://api.example.com"}
		if got := app.apiBaseURLForRegion("cn"); got != "https://api.example.com" {
			t.Fatalf("unexpected api base url: %s", got)
		}
		app.osEnv = nil
	})

	t.Run("config does not override region default", func(t *testing.T) {
		if got := app.apiBaseURLForRegion("cn"); got != cnAPIBaseURL {
			t.Fatalf("unexpected api base url: %s", got)
		}
	})
}

func TestReadConfirmYesDefaultAcceptsEnterAndRepromptsInvalidInput(t *testing.T) {
	t.Run("enter defaults to yes", func(t *testing.T) {
		var out bytes.Buffer
		ok, err := readConfirmYesDefault(strings.NewReader("\n"), &out, "Sign in now? [Y/n]: ")
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatal("expected empty answer to default to yes")
		}
		if got := out.String(); got != "Sign in now? [Y/n]: " {
			t.Fatalf("unexpected prompt output: %q", got)
		}
	})

	t.Run("invalid answer asks again", func(t *testing.T) {
		var out bytes.Buffer
		ok, err := readConfirmYesDefault(strings.NewReader("maybe\ny\n"), &out, "Sign in now? [Y/n]: ")
		if err != nil {
			t.Fatal(err)
		}
		if !ok {
			t.Fatal("expected second answer to confirm login")
		}
		got := out.String()
		if strings.Count(got, "Sign in now? [Y/n]: ") != 2 {
			t.Fatalf("expected prompt twice, got %q", got)
		}
		if !strings.Contains(got, "Please answer y or n.\n") {
			t.Fatalf("expected retry guidance, got %q", got)
		}
	})
}

func TestEnsureValidAccessTokenSkipsPromptInJSONMode(t *testing.T) {
	dir := t.TempDir()
	app := &App{
		env: map[string]string{
			"XDG_CONFIG_HOME": dir,
			"AGORA_OUTPUT":    "json",
		},
	}
	s, err := app.ensureValidAccessToken()
	if s != nil {
		t.Fatalf("expected nil session, got %+v", s)
	}
	if err == nil || err.Error() != noLocalSessionErrorMessage {
		t.Fatalf("expected missing session error, got %v", err)
	}
}

func TestBrowserOpenCommandWindowsPreservesOAuthQuery(t *testing.T) {
	target := "https://sso.example/authorize?response_type=code&code_challenge=abc&code_challenge_method=S256&state=xyz"
	name, args := browserOpenCommand("windows", target)
	if name != "rundll32" {
		t.Fatalf("expected rundll32 opener, got %s", name)
	}
	expected := []string{"url.dll,FileProtocolHandler", target}
	if !reflect.DeepEqual(args, expected) {
		t.Fatalf("unexpected args: %#v", args)
	}
	for _, arg := range args {
		if arg == "cmd" || arg == "/c" || arg == "start" {
			t.Fatalf("windows opener must not shell through cmd.exe, got %#v", args)
		}
	}
}
