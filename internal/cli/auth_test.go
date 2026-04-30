package cli

import (
	"bytes"
	"reflect"
	"strings"
	"testing"
)

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
