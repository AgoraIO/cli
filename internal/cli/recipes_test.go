package cli

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestListRecipesValidatesTypeAndSorts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/recipes" || r.URL.Query().Get("type") != "ai" {
			t.Fatalf("unexpected request URL: %s", r.URL.String())
		}
		if r.Header.Get("User-Agent") == "" {
			t.Fatal("expected User-Agent header")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schemaVersion":1,"type":"ai","total":2,"items":[{"slug":"zulu","title":"zulu","mainRepoUrl":"https://github.com/AgoraIO/zulu","type":"ai","official":true},{"slug":"alpha","title":"Alpha","mainRepoUrl":"https://github.com/AgoraIO/alpha","type":"ai","official":true}]}`))
	}))
	defer server.Close()

	a := &App{
		env:        map[string]string{"AGORA_RECIPES_BASE_URL": server.URL + "/api/v1"},
		osEnv:      map[string]string{"AGORA_AGENT": "test"},
		httpClient: server.Client(),
	}
	response, err := a.listRecipes("AI")
	if err != nil {
		t.Fatal(err)
	}
	if len(response.Items) != 2 || response.Items[0].Slug != "alpha" || response.Items[1].Slug != "zulu" {
		t.Fatalf("recipes not sorted deterministically: %+v", response.Items)
	}

	_, err = a.listRecipes("video")
	var cliErr *cliError
	if !errors.As(err, &cliErr) || cliErr.Code != "RECIPE_TYPE_INVALID" {
		t.Fatalf("expected RECIPE_TYPE_INVALID, got %v", err)
	}
}

func TestGetRecipeAllowsOptionalCLIConfigAndRejectsUnsupportedSchema(t *testing.T) {
	response := `{"schemaVersion":1,"recipe":{"slug":"tool-calling","title":"Tool Calling","mainRepoUrl":"https://github.com/AgoraIO/tool-calling","recipeRawUrl":"https://example.com/RECIPE.md","type":"ai","official":true}}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	a := &App{env: map[string]string{"AGORA_RECIPES_BASE_URL": server.URL}, osEnv: map[string]string{}, httpClient: server.Client()}
	recipe, err := a.getRecipe("tool-calling")
	if err != nil || recipe.CLI != nil {
		t.Fatalf("expected optional cli metadata to be accepted, got recipe=%+v err=%v", recipe, err)
	}

	response = `{"schemaVersion":2,"recipe":{}}`
	_, err = a.getRecipe("tool-calling")
	var cliErr *cliError
	if !errors.As(err, &cliErr) || cliErr.Code != "RECIPE_SCHEMA_UNSUPPORTED" {
		t.Fatalf("expected RECIPE_SCHEMA_UNSUPPORTED, got %v", err)
	}
}

func TestGetRecipeEscapesSlugAsOnePathSegment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.EscapedPath() != "/recipes/nested%2Fslug" {
			t.Fatalf("escaped path = %q, want /recipes/nested%%2Fslug", r.URL.EscapedPath())
		}
		_, _ = w.Write([]byte(`{"schemaVersion":1,"recipe":{"slug":"nested/slug","title":"Nested","mainRepoUrl":"https://github.com/AgoraIO/nested","recipeRawUrl":"https://example.com/RECIPE.md","type":"ai","official":true}}`))
	}))
	defer server.Close()

	a := &App{env: map[string]string{"AGORA_RECIPES_BASE_URL": server.URL}, osEnv: map[string]string{}, httpClient: server.Client()}
	recipe, err := a.getRecipe("nested/slug")
	if err != nil || recipe.Slug != "nested/slug" {
		t.Fatalf("unexpected recipe=%+v err=%v", recipe, err)
	}
}

func TestRecipeAPIRejectsMalformedAndNonOfficialResponses(t *testing.T) {
	response := `not-json`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	a := &App{env: map[string]string{"AGORA_RECIPES_BASE_URL": server.URL}, osEnv: map[string]string{}, httpClient: server.Client()}
	_, err := a.listRecipes("all")
	assertCLIErrorCode(t, err, "RECIPE_RESPONSE_INVALID")

	response = `{"schemaVersion":1,"type":"all","total":1,"items":[{"slug":"community","title":"Community","mainRepoUrl":"https://github.com/example/community","type":"ai","official":false}]}`
	_, err = a.listRecipes("all")
	assertCLIErrorCode(t, err, "RECIPE_RESPONSE_INVALID")
	if !strings.Contains(err.Error(), "non-official") {
		t.Fatalf("expected official-only validation message, got %v", err)
	}
}

func TestListRecipesValidatesServerFilteringContract(t *testing.T) {
	response := `{"schemaVersion":1,"type":"ai","total":1,"items":[{"slug":"rtc","title":"RTC","mainRepoUrl":"https://github.com/AgoraIO/rtc","type":"rtc","official":true}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	a := &App{env: map[string]string{"AGORA_RECIPES_BASE_URL": server.URL}, osEnv: map[string]string{}, httpClient: server.Client()}
	_, err := a.listRecipes("ai")
	assertCLIErrorCode(t, err, "RECIPE_RESPONSE_INVALID")

	response = `{"schemaVersion":1,"type":"all","total":1,"items":[{"slug":"ai","title":"AI","mainRepoUrl":"https://github.com/AgoraIO/ai","type":"ai","official":true}]}`
	_, err = a.listRecipes("ai")
	assertCLIErrorCode(t, err, "RECIPE_RESPONSE_INVALID")

	response = `{"schemaVersion":1,"type":"ai","total":2,"items":[{"slug":"ai","title":"AI","mainRepoUrl":"https://github.com/AgoraIO/ai","type":"ai","official":true}]}`
	_, err = a.listRecipes("ai")
	assertCLIErrorCode(t, err, "RECIPE_RESPONSE_INVALID")
}

func TestValidateRecipeCLIConfigRejectsEscapingPaths(t *testing.T) {
	err := validateRecipeCLIConfig(&recipeCLIConfig{
		ProjectType: "python",
		Env: recipeCLIEnv{
			ExamplePath:       "server/.env.example",
			TargetPath:        "../.env",
			AppIDKey:          "AGORA_APP_ID",
			AppCertificateKey: "AGORA_APP_CERTIFICATE",
		},
	})
	var cliErr *cliError
	if !errors.As(err, &cliErr) || cliErr.Code != "RECIPE_RESPONSE_INVALID" {
		t.Fatalf("expected RECIPE_RESPONSE_INVALID, got %v", err)
	}
}

func TestValidateRecipeCLIConfigReportsUnsupportedWhenAbsent(t *testing.T) {
	err := validateRecipeCLIConfig(nil)
	var cliErr *cliError
	if !errors.As(err, &cliErr) || cliErr.Code != "RECIPE_INIT_UNSUPPORTED" {
		t.Fatalf("expected RECIPE_INIT_UNSUPPORTED, got %v", err)
	}
}
