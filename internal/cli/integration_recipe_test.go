package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIInitFromOfficialRecipeUsesCatalogEnvContract(t *testing.T) {
	configHome := t.TempDir()
	rootDir := t.TempDir()
	api := newFakeCLIBFF()
	defer api.server.Close()
	persistSessionForIntegration(t, configHome)

	recipeRepo := createLocalGitRepo(t, map[string]string{
		"README.md":           "# Tool Calling\n",
		"server/.env.example": "AGORA_APP_ID=\nAGORA_APP_CERTIFICATE=\nCUSTOM_LLM_URL=\n",
	})
	recipes := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/recipes/tool-calling" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"schemaVersion": 1,
			"recipe": map[string]any{
				"slug":          "tool-calling",
				"title":         "Tool Calling",
				"mainRepoUrl":   recipeRepo,
				"recipeUrl":     "https://recipes.agora.io/recipes/tool-calling",
				"recipeRawUrl":  "https://example.com/RECIPE.md",
				"primaryPrompt": "Follow the official recipe.",
				"type":          "ai",
				"official":      true,
				"cli": map[string]any{
					"projectType": "python",
					"env": map[string]any{
						"examplePath":       "server/.env.example",
						"targetPath":        "server/.env.local",
						"appIdKey":          "AGORA_APP_ID",
						"appCertificateKey": "AGORA_APP_CERTIFICATE",
					},
					"installCommand": "bun run setup",
					"runCommand":     "bun run dev",
				},
			},
		})
	}))
	defer recipes.Close()

	targetDir := filepath.Join(rootDir, "tool-demo")
	result := runCLI(t, []string{"init", "tool-demo", "--recipe", "tool-calling", "--new-project", "--dir", targetDir, "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":        configHome,
			"AGORA_API_BASE_URL":     api.baseURL,
			"AGORA_RECIPES_BASE_URL": recipes.URL + "/api/v1",
			"AGORA_LOG_LEVEL":        "error",
		},
		workdir: rootDir,
	})
	if result.exitCode != 0 || !strings.Contains(result.stdout, `"sourceType":"recipe"`) || !strings.Contains(result.stdout, `"sourceId":"tool-calling"`) || !strings.Contains(result.stdout, `"envPath":"server/.env.local"`) {
		t.Fatalf("unexpected recipe init result: %+v", result)
	}

	envFile, err := os.ReadFile(filepath.Join(targetDir, "server", ".env.local"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(envFile), "AGORA_APP_ID=app_0001") || !strings.Contains(string(envFile), "CUSTOM_LLM_URL=") {
		t.Fatalf("unexpected recipe env: %s", envFile)
	}
	binding, err := loadLocalProjectBinding(targetDir)
	if err != nil {
		t.Fatal(err)
	}
	if binding.Recipe != "tool-calling" || binding.Template != "" || binding.ProjectType != "python" || binding.EnvPath != "server/.env.local" {
		t.Fatalf("unexpected recipe binding: %+v", binding)
	}
	if _, err := os.Stat(filepath.Join(targetDir, ".git")); !os.IsNotExist(err) {
		t.Fatalf("expected cloned recipe git metadata removed, got %v", err)
	}
}

func TestCLIRecipeResolutionFailsBeforeProjectCreation(t *testing.T) {
	api := newFakeCLIBFF()
	defer api.server.Close()
	recipes := httptest.NewServer(http.NotFoundHandler())
	defer recipes.Close()

	result := runCLI(t, []string{"init", "missing-demo", "--recipe", "missing", "--new-project", "--json"}, cliRunOptions{
		env: map[string]string{
			"AGORA_HOME":             t.TempDir(),
			"AGORA_API_BASE_URL":     api.baseURL,
			"AGORA_RECIPES_BASE_URL": recipes.URL,
			"AGORA_LOG_LEVEL":        "error",
		},
	})
	if result.exitCode != 1 || !strings.Contains(result.stdout, `"code":"RECIPE_NOT_FOUND"`) {
		t.Fatalf("expected RECIPE_NOT_FOUND, got %+v", result)
	}
	api.mu.Lock()
	defer api.mu.Unlock()
	if len(api.projects) != 0 {
		t.Fatalf("recipe resolution created a project: %+v", api.projects)
	}
}

func TestCLIInitRejectsRecipeAndTemplateTogether(t *testing.T) {
	result := runCLI(t, []string{"init", "demo", "--template", "python", "--recipe", "tool-calling", "--json"}, cliRunOptions{
		env: map[string]string{"AGORA_HOME": t.TempDir(), "AGORA_LOG_LEVEL": "error"},
	})
	if result.exitCode != 1 || !strings.Contains(result.stdout, `"code":"INIT_SOURCE_CONFLICT"`) {
		t.Fatalf("expected INIT_SOURCE_CONFLICT, got %+v", result)
	}
}
