package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

const (
	defaultRecipesBaseURL  = "https://recipes.agora.io/api/v1"
	recipeAPISchemaVersion = 1
	maxRecipeResponseBytes = 2 << 20
)

type recipeSummary struct {
	Slug         string   `json:"slug"`
	Title        string   `json:"title"`
	Tagline      string   `json:"tagline"`
	Description  string   `json:"description"`
	Platforms    []string `json:"platforms"`
	UseCases     []string `json:"useCases"`
	Capabilities []string `json:"capabilities"`
	MainRepoURL  string   `json:"mainRepoUrl"`
	RecipeURL    string   `json:"recipeUrl"`
	Author       string   `json:"author"`
	Updated      string   `json:"updated"`
	Difficulty   string   `json:"difficulty"`
	Type         string   `json:"type"`
	Official     bool     `json:"official"`
}

type recipeDetail struct {
	recipeSummary
	RecipeRawURL  string           `json:"recipeRawUrl"`
	PrimaryPrompt string           `json:"primaryPrompt"`
	CLI           *recipeCLIConfig `json:"cli"`
}

type recipeCLIConfig struct {
	ProjectType    string       `json:"projectType"`
	Env            recipeCLIEnv `json:"env"`
	InstallCommand string       `json:"installCommand"`
	RunCommand     string       `json:"runCommand"`
}

type recipeCLIEnv struct {
	ExamplePath       string `json:"examplePath"`
	TargetPath        string `json:"targetPath"`
	AppIDKey          string `json:"appIdKey"`
	AppCertificateKey string `json:"appCertificateKey"`
}

type recipeListResponse struct {
	SchemaVersion int             `json:"schemaVersion"`
	Items         []recipeSummary `json:"items"`
	Total         int             `json:"total"`
	Type          string          `json:"type"`
}

type recipeDetailResponse struct {
	SchemaVersion int          `json:"schemaVersion"`
	Recipe        recipeDetail `json:"recipe"`
}

func (a *App) buildRecipesCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recipes",
		Short: "Discover official Agora recipes",
		Long:  "Browse the versioned Agora recipes catalog used by recipe-backed init.",
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return fmt.Errorf("unknown command %q for %q", args[0], cmd.CommandPath())
			}
			return cmd.Help()
		},
	}
	cmd.AddCommand(a.buildRecipesListCommand())
	cmd.AddCommand(a.buildRecipesShowCommand())
	return cmd
}

func (a *App) buildRecipesListCommand() *cobra.Command {
	var recipeType string
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List recipes from the Agora catalog",
		Example: example(`
  agora recipes list
  agora recipes list --type ai
  agora recipes list --type rtc --json
`),
		RunE: func(cmd *cobra.Command, _ []string) error {
			result, err := a.listRecipes(recipeType)
			if err != nil {
				return err
			}
			return renderResult(cmd, "recipes list", map[string]any{
				"action": "list",
				"items":  result.Items,
				"total":  result.Total,
				"type":   result.Type,
			})
		},
	}
	cmd.Flags().StringVar(&recipeType, "type", "all", "recipe type: all, ai, or rtc")
	return cmd
}

func (a *App) buildRecipesShowCommand() *cobra.Command {
	return &cobra.Command{
		Use:   "show <slug>",
		Short: "Show one Agora recipe",
		Args:  cobra.ExactArgs(1),
		Example: example(`
  agora recipes show python-quickstart
  agora recipes show tool-calling --json
`),
		RunE: func(cmd *cobra.Command, args []string) error {
			recipe, err := a.getRecipe(args[0])
			if err != nil {
				return err
			}
			return renderResult(cmd, "recipes show", map[string]any{
				"action": "show",
				"recipe": recipe,
			})
		},
	}
}

func normalizeRecipeType(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" {
		normalized = "all"
	}
	switch normalized {
	case "all", "ai", "rtc":
		return normalized, nil
	default:
		return "", &cliError{Message: fmt.Sprintf("unknown recipe type %q. Use all, ai, or rtc.", value), Code: "RECIPE_TYPE_INVALID"}
	}
}

func (a *App) listRecipes(recipeType string) (recipeListResponse, error) {
	normalized, err := normalizeRecipeType(recipeType)
	if err != nil {
		return recipeListResponse{}, err
	}
	var response recipeListResponse
	if err := a.recipeAPIRequest("/recipes", map[string]string{"type": normalized}, &response); err != nil {
		return recipeListResponse{}, err
	}
	if err := validateRecipeSchema(response.SchemaVersion); err != nil {
		return recipeListResponse{}, err
	}
	if response.Type != normalized {
		return recipeListResponse{}, invalidRecipeResponse(fmt.Sprintf("type is %q; expected %q", response.Type, normalized))
	}
	if response.Total != len(response.Items) {
		return recipeListResponse{}, invalidRecipeResponse("total does not match the number of items")
	}
	for _, recipe := range response.Items {
		if err := validateRecipeSummary(recipe); err != nil {
			return recipeListResponse{}, err
		}
		if normalized != "all" && recipe.Type != normalized {
			return recipeListResponse{}, invalidRecipeResponse(fmt.Sprintf("recipe %q does not match the %q filter", recipe.Slug, normalized))
		}
	}
	sort.SliceStable(response.Items, func(i, j int) bool {
		left := strings.ToLower(response.Items[i].Title)
		right := strings.ToLower(response.Items[j].Title)
		if left == right {
			return response.Items[i].Slug < response.Items[j].Slug
		}
		return left < right
	})
	return response, nil
}

func (a *App) getRecipe(slug string) (recipeDetail, error) {
	slug = strings.TrimSpace(slug)
	if slug == "" {
		return recipeDetail{}, &cliError{Message: "recipe slug is required.", Code: "RECIPE_NOT_FOUND"}
	}
	var response recipeDetailResponse
	if err := a.recipeAPIRequest("/recipes/"+url.PathEscape(slug), nil, &response); err != nil {
		return recipeDetail{}, err
	}
	if err := validateRecipeSchema(response.SchemaVersion); err != nil {
		return recipeDetail{}, err
	}
	if err := validateRecipeSummary(response.Recipe.recipeSummary); err != nil {
		return recipeDetail{}, err
	}
	if response.Recipe.Slug != slug {
		return recipeDetail{}, invalidRecipeResponse(fmt.Sprintf("slug is %q; expected %q", response.Recipe.Slug, slug))
	}
	if strings.TrimSpace(response.Recipe.RecipeRawURL) == "" {
		return recipeDetail{}, invalidRecipeResponse("recipeRawUrl is required")
	}
	return response.Recipe, nil
}

func (a *App) initRecipeProject(name, targetDir string, recipe recipeDetail, existingProject string, features []string, rtmDataCenter string, newProject bool, promptForReuse bool, promptOut io.Writer, promptIn io.Reader, progress progressEmitter) (map[string]any, error) {
	if err := validateRecipeCLIConfig(recipe.CLI); err != nil {
		return nil, err
	}
	absTarget, err := resolveScaffoldTarget(targetDir)
	if err != nil {
		return nil, err
	}
	resolution, err := a.resolveInitProjectForScaffold(name, existingProject, features, rtmDataCenter, newProject, promptForReuse, promptOut, promptIn, progress)
	if err != nil {
		return nil, err
	}
	if err := cloneScaffoldRepo(recipe.MainRepoURL, absTarget, "", progress); err != nil {
		return nil, err
	}

	target := resolution.target
	binding := localProjectBinding{
		ProjectID:   target.project.ProjectID,
		ProjectName: target.project.Name,
		Region:      target.region,
		ProjectType: recipe.CLI.ProjectType,
		Recipe:      recipe.Slug,
	}
	envPath := filepath.ToSlash(filepath.Clean(recipe.CLI.Env.TargetPath))
	writtenEnv, err := writeCredentialEnv(credentialEnvTarget{
		Path:              filepath.Join(absTarget, filepath.FromSlash(recipe.CLI.Env.TargetPath)),
		ExamplePath:       filepath.Join(absTarget, filepath.FromSlash(recipe.CLI.Env.ExamplePath)),
		AppIDKey:          recipe.CLI.Env.AppIDKey,
		AppCertificateKey: recipe.CLI.Env.AppCertificateKey,
	}, target.project, credentialEnvWriteOptions{AllowAppend: true})
	if err != nil {
		return nil, cleanupFailedScaffold(absTarget, "configure recipe env", err)
	}
	binding.EnvPath = envPath
	written := []string{envPath, filepath.ToSlash(filepath.Join(localAgoraDirName, localProjectFileName))}
	nextSteps := []string{"cd " + filepath.Base(absTarget)}
	if recipe.CLI.InstallCommand != "" {
		nextSteps = append(nextSteps, recipe.CLI.InstallCommand)
	}
	if recipe.CLI.RunCommand != "" {
		nextSteps = append(nextSteps, recipe.CLI.RunCommand)
	}
	if err := writeLocalProjectBinding(absTarget, binding); err != nil {
		return nil, cleanupFailedScaffold(absTarget, "write .agora project metadata", err)
	}
	sort.Strings(written)
	if err := a.persistInitProjectContext(target); err != nil {
		return nil, err
	}

	result := map[string]any{
		"action":                 "init",
		"cloneUrl":               recipe.MainRepoURL,
		"enabledFeatures":        resolution.enabledFeatures,
		"envPath":                envPath,
		"envStatus":              writtenEnv.Status,
		"metadataPath":           filepath.ToSlash(filepath.Join(localAgoraDirName, localProjectFileName)),
		"nextSteps":              nextSteps,
		"path":                   absTarget,
		"primaryPrompt":          recipe.PrimaryPrompt,
		"projectAction":          resolution.projectAction,
		"projectId":              target.project.ProjectID,
		"projectName":            target.project.Name,
		"projectSelectionReason": resolution.projectSelectionReason,
		"recipe":                 recipe.Slug,
		"recipeRawUrl":           recipe.RecipeRawURL,
		"recipeUrl":              recipe.RecipeURL,
		"region":                 target.region,
		"reusedExistingProject":  resolution.projectAction == "existing",
		"sourceId":               recipe.Slug,
		"sourceType":             "recipe",
		"status":                 "ready",
		"title":                  recipe.Title,
		"written":                written,
	}
	if resolution.createdRTMDataCenter != "" {
		result["rtmDataCenter"] = resolution.createdRTMDataCenter
	}
	return result, nil
}

func cleanupFailedScaffold(targetDir, action string, cause error) error {
	if cleanupErr := os.RemoveAll(targetDir); cleanupErr != nil {
		return fmt.Errorf("failed to %s: %v; cleanup also failed for %s: %v", action, cause, targetDir, cleanupErr)
	}
	return fmt.Errorf("failed to %s: %v; removed %s", action, cause, targetDir)
}

func validateRecipeCLIConfig(config *recipeCLIConfig) error {
	if config == nil {
		return &cliError{
			Message: "this official recipe does not provide CLI initialization metadata yet. Choose another recipe from `agora recipes list`.",
			Code:    "RECIPE_INIT_UNSUPPORTED",
		}
	}
	if strings.TrimSpace(config.ProjectType) == "" {
		return invalidRecipeResponse("cli.projectType is required")
	}
	for label, value := range map[string]string{
		"cli.env.examplePath":       config.Env.ExamplePath,
		"cli.env.targetPath":        config.Env.TargetPath,
		"cli.env.appIdKey":          config.Env.AppIDKey,
		"cli.env.appCertificateKey": config.Env.AppCertificateKey,
	} {
		if strings.TrimSpace(value) == "" {
			return invalidRecipeResponse(label + " is required")
		}
	}
	for label, value := range map[string]string{
		"cli.env.examplePath": config.Env.ExamplePath,
		"cli.env.targetPath":  config.Env.TargetPath,
	} {
		cleaned := filepath.Clean(filepath.FromSlash(value))
		if filepath.IsAbs(cleaned) || cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
			return invalidRecipeResponse(label + " must be a relative path inside the recipe")
		}
	}
	return nil
}

func validateRecipeSchema(version int) error {
	if version != recipeAPISchemaVersion {
		return &cliError{
			Message: fmt.Sprintf("recipes API schema version %d is not supported by this CLI; expected %d.", version, recipeAPISchemaVersion),
			Code:    "RECIPE_SCHEMA_UNSUPPORTED",
		}
	}
	return nil
}

func validateRecipeSummary(recipe recipeSummary) error {
	if strings.TrimSpace(recipe.Slug) == "" {
		return invalidRecipeResponse("slug is required")
	}
	if strings.TrimSpace(recipe.Title) == "" {
		return invalidRecipeResponse("title is required")
	}
	if strings.TrimSpace(recipe.MainRepoURL) == "" {
		return invalidRecipeResponse("mainRepoUrl is required")
	}
	if err := validateRepoOverrideURL(recipe.MainRepoURL); err != nil {
		return &cliError{Message: fmt.Sprintf("recipe %q has an invalid mainRepoUrl: %v.", recipe.Slug, err), Code: "RECIPE_REPO_URL_INVALID"}
	}
	if !recipe.Official {
		return invalidRecipeResponse("catalog included a non-official recipe")
	}
	if recipe.Type != "ai" && recipe.Type != "rtc" {
		return invalidRecipeResponse("type must be ai or rtc")
	}
	return nil
}

func invalidRecipeResponse(reason string) error {
	return &cliError{Message: "recipes API returned an invalid response: " + reason + ".", Code: "RECIPE_RESPONSE_INVALID"}
}

func (a *App) recipeAPIRequest(pathname string, query map[string]string, out any) error {
	base := strings.TrimRight(strings.TrimSpace(a.env["AGORA_RECIPES_BASE_URL"]), "/")
	if base == "" {
		base = defaultRecipesBaseURL
	}
	u, err := url.Parse(base + pathname)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return &cliError{Message: "recipes API base URL is invalid.", Code: "RECIPE_API_FAILED"}
	}
	values := u.Query()
	for key, value := range query {
		if value != "" {
			values.Set(key, value)
		}
	}
	u.RawQuery = values.Encode()

	req, err := http.NewRequest(http.MethodGet, u.String(), nil)
	if err != nil {
		return &cliError{Message: "could not create recipes API request: " + err.Error(), Code: "RECIPE_API_FAILED"}
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", agoraUserAgent(a.osEnv))
	resp, err := a.httpClient.Do(req)
	if err != nil {
		return &cliError{Message: "could not reach the recipes API: " + err.Error(), Code: "RECIPE_API_FAILED"}
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxRecipeResponseBytes+1))
	if err != nil {
		return &cliError{Message: "could not read the recipes API response: " + err.Error(), Code: "RECIPE_API_FAILED", HTTPStatus: resp.StatusCode}
	}
	if len(raw) > maxRecipeResponseBytes {
		return invalidRecipeResponse("response exceeds the 2 MiB limit")
	}
	if resp.StatusCode == http.StatusNotFound {
		return &cliError{Message: "recipe was not found in the Agora catalog.", Code: "RECIPE_NOT_FOUND", HTTPStatus: resp.StatusCode, RequestID: responseRequestID(resp.Header)}
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &cliError{Message: fmt.Sprintf("recipes API request failed with HTTP %d.", resp.StatusCode), Code: "RECIPE_API_FAILED", HTTPStatus: resp.StatusCode, RequestID: responseRequestID(resp.Header)}
	}
	if err := json.Unmarshal(raw, out); err != nil {
		return invalidRecipeResponse("response is not valid JSON")
	}
	return nil
}
