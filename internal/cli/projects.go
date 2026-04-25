package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func (a *App) listProjects(keyword string, page, pageSize int) (projectListResponse, error) {
	var out projectListResponse
	err := a.apiRequest("GET", "/api/cli/v1/projects", map[string]string{"keyword": keyword, "page": fmt.Sprint(page), "pageSize": fmt.Sprint(pageSize)}, nil, &out)
	return out, err
}

func (a *App) createProject(name string) (projectDetail, error) {
	var out projectDetail
	body := map[string]any{"name": name, "projectType": "paas"}
	err := a.apiRequest("POST", "/api/cli/v1/projects", nil, body, &out)
	return out, err
}

func (a *App) getProject(projectID string) (projectDetail, error) {
	var out projectDetail
	err := a.apiRequest("GET", "/api/cli/v1/projects/"+projectID, nil, nil, &out)
	return out, err
}

func (a *App) resolveProjectByNameOrID(value string) (*projectSummary, error) {
	list, err := a.listProjects(value, 1, 20)
	if err != nil {
		return nil, err
	}
	for _, item := range list.Items {
		if item.ProjectID == value || item.Name == value {
			copy := item
			return &copy, nil
		}
	}
	return nil, nil
}

type projectTarget struct {
	project projectDetail
	region  string
}

func (a *App) resolveProjectTarget(explicit string) (projectTarget, error) {
	ctx, err := loadContext(a.env)
	if err != nil {
		return projectTarget{}, err
	}
	if explicit != "" {
		resolved, err := a.resolveProjectByNameOrID(explicit)
		if err != nil {
			return projectTarget{}, err
		}
		if resolved == nil {
			return projectTarget{}, fmt.Errorf("Project %q was not found.", explicit)
		}
		project, err := a.getProject(resolved.ProjectID)
		if err != nil {
			return projectTarget{}, err
		}
		region := ctx.CurrentRegion
		if region == "" {
			region = "global"
		}
		if project.Region != nil && *project.Region != "" {
			region = *project.Region
		} else if resolved.Region != nil && *resolved.Region != "" {
			region = *resolved.Region
		}
		return projectTarget{project: project, region: region}, nil
	}
	if binding, ok, _, err := detectLocalProjectBinding(); err != nil {
		return projectTarget{}, err
	} else if ok && binding.ProjectID != "" {
		project, err := a.getProject(binding.ProjectID)
		if err != nil {
			return projectTarget{}, err
		}
		region := binding.Region
		if region == "" {
			region = ctx.CurrentRegion
		}
		if region == "" {
			region = "global"
		}
		if project.Region != nil && *project.Region != "" {
			region = *project.Region
		}
		return projectTarget{project: project, region: region}, nil
	}
	if ctx.CurrentProjectID == nil || *ctx.CurrentProjectID == "" {
		return projectTarget{}, errors.New("No project selected. Run `agora project use <project>`, work inside a repo with `.agora/project.json`, or pass a project explicitly.")
	}
	project, err := a.getProject(*ctx.CurrentProjectID)
	if err != nil {
		return projectTarget{}, err
	}
	region := ctx.CurrentRegion
	if project.Region != nil && *project.Region != "" {
		region = *project.Region
	}
	return projectTarget{project: project, region: region}, nil
}

func (a *App) getRTM2Config(projectID string) (map[string]any, error) {
	out := map[string]any{}
	err := a.apiRequest("GET", "/api/cli/v1/projects/"+projectID+"/rtm2-config", nil, nil, &out)
	return out, err
}

func (a *App) setRTM2Config(projectID, region string) error {
	body := map[string]any{
		"channelSubscribeEnabled": false,
		"debounce":                "2",
		"interval":                "30",
		"lockEnabled":             false,
		"occupancy":               "50",
		"region":                  region,
		"storageEnabled":          false,
		"streamChannelEnabled":    false,
		"userSubscribeEnabled":    false,
	}
	out := map[string]any{}
	return a.apiRequest("PUT", "/api/cli/v1/projects/"+projectID+"/rtm2-config", nil, body, &out)
}

func (a *App) getUAPConfig(projectID, productKey string) (map[string]any, error) {
	out := map[string]any{}
	err := a.apiRequest("GET", "/api/cli/v1/projects/"+projectID+"/uap-configs/"+productKey, nil, nil, &out)
	return out, err
}

func (a *App) setUAPConfig(projectID, productKey, region string) error {
	out := map[string]any{}
	return a.apiRequest("PUT", "/api/cli/v1/projects/"+projectID+"/uap-configs/"+productKey, nil, map[string]any{"enabled": true, "region": region}, &out)
}

func convoAIProduct(region string) string {
	if region == "cn" {
		return "convoai"
	}
	return "convoai-global"
}

func (a *App) getFeatureItem(feature string, project projectDetail, region string) (featureItem, error) {
	switch feature {
	case "rtc":
		return featureItem{Feature: "rtc", Message: "rtc included with the project", Status: "included"}, nil
	case "rtm":
		cfg, err := a.getRTM2Config(project.ProjectID)
		if err != nil {
			return featureItem{}, err
		}
		enabled, _ := cfg["enabled"].(bool)
		if enabled {
			return featureItem{Feature: "rtm", Message: "rtm enabled", Status: "enabled"}, nil
		}
		return featureItem{Feature: "rtm", Message: "rtm disabled", Status: "disabled"}, nil
	case "convoai":
		cfg, err := a.getUAPConfig(project.ProjectID, convoAIProduct(region))
		if err != nil {
			return featureItem{}, err
		}
		enabled, _ := cfg["enabled"].(bool)
		if enabled {
			return featureItem{Feature: "convoai", Message: "convoai enabled", Status: "enabled"}, nil
		}
		return featureItem{Feature: "convoai", Message: "convoai not enabled", Status: "disabled"}, nil
	default:
		return featureItem{}, fmt.Errorf("%q must be one of: rtc, rtm, convoai.", feature)
	}
}

func (a *App) listProjectFeatures(project projectDetail, region string) ([]featureItem, error) {
	items := make([]featureItem, 0, 3)
	for _, feature := range []string{"rtc", "rtm", "convoai"} {
		item, err := a.getFeatureItem(feature, project, region)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (a *App) enableProjectFeature(feature string, project projectDetail, region string) (map[string]any, error) {
	switch feature {
	case "rtc":
		return map[string]any{"action": "feature-enable", "feature": "rtc", "message": "rtc is included with the project", "projectId": project.ProjectID, "projectName": project.Name, "status": "included"}, nil
	case "rtm":
		rtmRegion := "NA"
		if region == "cn" {
			rtmRegion = "CN"
		}
		if err := a.setRTM2Config(project.ProjectID, rtmRegion); err != nil {
			return nil, err
		}
		return map[string]any{"action": "feature-enable", "feature": "rtm", "message": "rtm enabled", "projectId": project.ProjectID, "projectName": project.Name, "status": "enabled"}, nil
	case "convoai":
		uapRegion := "global"
		if region == "cn" {
			uapRegion = "cn"
		}
		if err := a.setUAPConfig(project.ProjectID, convoAIProduct(region), uapRegion); err != nil {
			return nil, err
		}
		return map[string]any{"action": "feature-enable", "feature": "convoai", "message": "convoai enabled", "projectId": project.ProjectID, "projectName": project.Name, "status": "enabled"}, nil
	default:
		return nil, fmt.Errorf("%q must be one of: rtc, rtm, convoai.", feature)
	}
}

func (a *App) projectCreate(name, region, template string, features []string) (map[string]any, error) {
	ctx, err := loadContext(a.env)
	if err != nil {
		return nil, err
	}
	if region == "" {
		region = ctx.PreferredRegion
		if region == "" {
			region = "global"
		}
	}
	project, err := a.createProject(name)
	if err != nil {
		return nil, err
	}
	if template == "voice-agent" {
		features = append(features, "rtc", "rtm", "convoai")
	}
	seen := map[string]bool{}
	enabled := []string{}
	for _, feature := range features {
		if seen[feature] {
			continue
		}
		seen[feature] = true
		if _, err := a.enableProjectFeature(feature, project, region); err != nil {
			return nil, err
		}
		enabled = append(enabled, feature)
	}
	ctx.CurrentProjectID = &project.ProjectID
	ctx.CurrentProjectName = &project.Name
	ctx.CurrentRegion = region
	ctx.PreferredRegion = region
	if err := saveContext(a.env, ctx); err != nil {
		return nil, err
	}
	return map[string]any{"action": "create", "appId": project.AppID, "enabledFeatures": enabled, "projectId": project.ProjectID, "projectName": project.Name, "region": region}, nil
}

func (a *App) projectUse(projectArg string) (map[string]any, error) {
	current, err := loadContext(a.env)
	if err != nil {
		return nil, err
	}
	resolved, err := a.resolveProjectByNameOrID(projectArg)
	if err != nil {
		return nil, err
	}
	if resolved == nil {
		return nil, fmt.Errorf("Project %q was not found.", projectArg)
	}
	region := current.CurrentRegion
	if region == "" {
		region = current.PreferredRegion
	}
	if resolved.Region != nil && *resolved.Region != "" {
		region = *resolved.Region
	}
	current.CurrentProjectID = &resolved.ProjectID
	current.CurrentProjectName = &resolved.Name
	current.CurrentRegion = region
	if current.PreferredRegion == "" {
		current.PreferredRegion = region
	}
	if err := saveContext(a.env, current); err != nil {
		return nil, err
	}
	return map[string]any{"action": "use", "projectId": resolved.ProjectID, "projectName": resolved.Name, "region": region, "status": "selected"}, nil
}

func (a *App) projectShow(projectArg string) (map[string]any, error) {
	target, err := a.resolveProjectTarget(projectArg)
	if err != nil {
		return nil, err
	}
	return map[string]any{"action": "show", "appId": target.project.AppID, "appCertificate": target.project.SignKey, "projectId": target.project.ProjectID, "projectName": target.project.Name, "region": target.region, "signKey": target.project.SignKey, "tokenEnabled": target.project.TokenEnabled}, nil
}

type envFormat string

const (
	envDotenv envFormat = "dotenv"
	envShell  envFormat = "shell"
	envJSON   envFormat = "json"
)

func resolveProjectEnvOutputFormat(format string, shell bool, mode outputMode) (envFormat, error) {
	if format != "" && shell {
		return "", errors.New("`--format` and `--shell` cannot be used together.")
	}
	if format != "" && mode == outputJSON {
		return "", errors.New("`--format` and `--json` cannot be used together.")
	}
	if shell && mode == outputJSON {
		return "", errors.New("`--shell` and `--json` cannot be used together.")
	}
	if mode == outputJSON {
		return envJSON, nil
	}
	if shell {
		return envShell, nil
	}
	if format == "" {
		return envDotenv, nil
	}
	return envFormat(format), nil
}

func (a *App) projectEnvValues(projectArg string, withSecrets bool) (map[string]any, error) {
	target, err := a.resolveProjectTarget(projectArg)
	if err != nil {
		return nil, err
	}
	features, err := a.listProjectFeatures(target.project, target.region)
	if err != nil {
		return nil, err
	}
	enabled := map[string]bool{}
	for _, item := range features {
		enabled[item.Feature] = item.Status == "enabled" || item.Status == "included"
	}
	values := map[string]any{
		"AGORA_PROJECT_ID":       target.project.ProjectID,
		"AGORA_PROJECT_NAME":     target.project.Name,
		"AGORA_REGION":           target.region,
		"AGORA_APP_ID":           target.project.AppID,
		"AGORA_ENABLED_FEATURES": strings.Join(enabledFeatures(enabled), ","),
		"AGORA_FEATURE_RTC":      enabled["rtc"],
		"AGORA_FEATURE_RTM":      enabled["rtm"],
		"AGORA_FEATURE_CONVOAI":  enabled["convoai"],
	}
	if withSecrets {
		if target.project.SignKey == nil || *target.project.SignKey == "" {
			return nil, fmt.Errorf("Project %q does not have an app certificate.", target.project.Name)
		}
		values["AGORA_APP_CERTIFICATE"] = *target.project.SignKey
	}
	return values, nil
}

func enabledFeatures(features map[string]bool) []string {
	out := []string{}
	for _, name := range []string{"rtc", "rtm", "convoai"} {
		if features[name] {
			out = append(out, name)
		}
	}
	return out
}

func projectEnvKeys(values map[string]any) []string {
	keys := []string{
		"AGORA_PROJECT_ID",
		"AGORA_PROJECT_NAME",
		"AGORA_REGION",
		"AGORA_APP_ID",
		"AGORA_ENABLED_FEATURES",
		"AGORA_FEATURE_RTC",
		"AGORA_FEATURE_RTM",
		"AGORA_FEATURE_CONVOAI",
		"AGORA_APP_CERTIFICATE",
	}
	out := []string{}
	for _, key := range keys {
		if _, ok := values[key]; ok {
			out = append(out, key)
		}
	}
	return out
}

func renderProjectEnv(values map[string]any, format envFormat) string {
	if format == envJSON {
		raw, _ := json.MarshalIndent(values, "", "  ")
		return string(raw) + "\n"
	}
	lines := make([]string, 0, len(values))
	for _, key := range projectEnvKeys(values) {
		value := values[key]
		switch format {
		case envShell:
			lines = append(lines, "export "+key+"="+renderShellScalar(value))
		default:
			lines = append(lines, key+"="+renderDotenvScalar(value))
		}
	}
	return strings.Join(lines, "\n") + "\n"
}

func renderDotenvScalar(v any) string {
	s := fmt.Sprint(v)
	if _, ok := v.(bool); ok {
		return s
	}
	if s == "" {
		return `""`
	}
	if safeEnvText(s) {
		return s
	}
	raw, _ := json.Marshal(s)
	return string(raw)
}

func renderShellScalar(v any) string {
	s := fmt.Sprint(v)
	if s == "" {
		return "''"
	}
	if safeEnvText(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func safeEnvText(value string) bool {
	for _, r := range value {
		if !(r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune("_./,:-", r)) {
			return false
		}
	}
	return true
}

type envWriteResult struct {
	Path   string
	Status string
}

func writeProjectEnvFile(path string, values map[string]any, appendMode, overwrite bool) (envWriteResult, error) {
	usedDefaultTarget := path == ""
	if path == "" {
		resolved, err := resolveDefaultTargetPath(".")
		if err != nil {
			return envWriteResult{}, err
		}
		path = resolved
	}
	filePath, err := filepath.Abs(path)
	if err != nil {
		return envWriteResult{}, err
	}
	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return envWriteResult{}, err
	}
	existing, err := os.ReadFile(filePath)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return envWriteResult{}, err
	}
	block := renderManagedBlock(values, detectEOL(string(existing)))
	status := ""
	switch {
	case errors.Is(err, os.ErrNotExist):
		existing = []byte(block + "\n")
		status = "created"
	case overwrite:
		existing = []byte(block + "\n")
		status = "overwritten"
	case strings.Contains(string(existing), "# BEGIN AGORA CLI"):
		existing = []byte(replaceManagedBlock(string(existing), block))
		status = "updated"
	case appendMode || usedDefaultTarget:
		trimmed := strings.TrimRight(string(existing), "\r\n")
		sep := "\n\n"
		if trimmed == "" {
			sep = ""
		}
		existing = []byte(trimmed + sep + block + "\n")
		status = "appended"
	default:
		return envWriteResult{}, fmt.Errorf("%s already exists. Use --append to append it or --overwrite to replace it.", path)
	}
	if err := os.WriteFile(filePath, existing, 0o644); err != nil {
		return envWriteResult{}, err
	}
	return envWriteResult{Path: filePath, Status: status}, nil
}

func detectEOL(v string) string {
	if strings.Contains(v, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

func renderManagedBlock(values map[string]any, eol string) string {
	body := strings.TrimRight(renderProjectEnv(values, envDotenv), "\n")
	return strings.Join([]string{"# BEGIN AGORA CLI", "# Generated by `agora project env`", body, "# END AGORA CLI"}, eol)
}

func replaceManagedBlock(existing, block string) string {
	start := strings.Index(existing, "# BEGIN AGORA CLI")
	end := strings.Index(existing, "# END AGORA CLI")
	if start == -1 || end == -1 {
		return existing
	}
	end += len("# END AGORA CLI")
	suffix := existing[end:]
	suffix = strings.TrimLeft(suffix, "\r\n")
	if suffix != "" {
		suffix = "\n" + suffix
	}
	return existing[:start] + block + suffix
}

func resolveDefaultTargetPath(cwd string) (string, error) {
	entries, err := os.ReadDir(cwd)
	if err != nil {
		return "", err
	}
	candidates := []string{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == ".env" || name == ".env.local" || (strings.HasPrefix(name, ".env.") && !strings.HasSuffix(name, ".example") && !strings.HasSuffix(name, ".sample") && !strings.HasSuffix(name, ".template")) {
			candidates = append(candidates, name)
		}
	}
	sort.Slice(candidates, func(i, j int) bool {
		weight := func(v string) int {
			switch v {
			case ".env.local":
				return 0
			case ".env":
				return 1
			default:
				return 2
			}
		}
		if weight(candidates[i]) != weight(candidates[j]) {
			return weight(candidates[i]) < weight(candidates[j])
		}
		return candidates[i] < candidates[j]
	})
	for _, candidate := range candidates {
		raw, err := os.ReadFile(filepath.Join(cwd, candidate))
		if err == nil && strings.Contains(string(raw), "# BEGIN AGORA CLI") {
			return candidate, nil
		}
	}
	for _, preferred := range []string{".env.local", ".env"} {
		for _, candidate := range candidates {
			if candidate == preferred {
				return candidate, nil
			}
		}
	}
	if len(candidates) > 0 {
		return candidates[0], nil
	}
	return ".env.local", nil
}

func (a *App) projectFeatureStatus(feature, projectArg string) (map[string]any, error) {
	target, err := a.resolveProjectTarget(projectArg)
	if err != nil {
		return nil, err
	}
	item, err := a.getFeatureItem(feature, target.project, target.region)
	if err != nil {
		return nil, err
	}
	return map[string]any{"action": "feature-status", "feature": feature, "message": item.Message, "projectId": target.project.ProjectID, "projectName": target.project.Name, "status": item.Status}, nil
}

func (a *App) projectFeatureEnable(feature, projectArg string) (map[string]any, error) {
	target, err := a.resolveProjectTarget(projectArg)
	if err != nil {
		return nil, err
	}
	return a.enableProjectFeature(feature, target.project, target.region)
}
