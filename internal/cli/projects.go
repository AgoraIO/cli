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

func (a *App) createProject(name, idempotencyKey string) (projectDetail, error) {
	var out projectDetail
	body := map[string]any{"name": name, "projectType": "paas"}
	if strings.TrimSpace(idempotencyKey) != "" {
		body["idempotencyKey"] = strings.TrimSpace(idempotencyKey)
	}
	err := a.apiRequest("POST", "/api/cli/v1/projects", nil, body, &out)
	return out, err
}

func looksLikeProjectID(value string) bool {
	return strings.HasPrefix(strings.TrimSpace(value), "prj_")
}

func (a *App) getProject(projectID string) (projectDetail, error) {
	var out projectDetail
	err := a.apiRequest("GET", "/api/cli/v1/projects/"+projectID, nil, nil, &out)
	return out, err
}

func (a *App) resolveProjectByNameOrID(value string) (*projectSummary, error) {
	if looksLikeProjectID(value) {
		project, err := a.getProject(value)
		if err == nil {
			return &projectSummary{
				AppID:       project.AppID,
				CreatedAt:   project.CreatedAt,
				Name:        project.Name,
				ProjectID:   project.ProjectID,
				ProjectType: project.ProjectType,
				Region:      project.Region,
				SignKey:     project.SignKey,
				Stage:       project.Stage,
				Status:      project.Status,
				UpdatedAt:   project.UpdatedAt,
				Vid:         project.Vid,
			}, nil
		}
		var structured *cliError
		if !errors.As(err, &structured) || structured.HTTPStatus != 404 {
			return nil, err
		}
	}
	matches := []projectSummary{}
	page := 1
	for {
		list, err := a.listProjects(value, page, 100)
		if err != nil {
			return nil, err
		}
		for _, item := range list.Items {
			if item.ProjectID == value || item.Name == value {
				matches = append(matches, item)
			}
		}
		if page*list.PageSize >= list.Total || len(list.Items) == 0 {
			break
		}
		page++
	}
	if len(matches) == 0 {
		return nil, nil
	}
	if len(matches) > 1 {
		return nil, &cliError{Message: fmt.Sprintf("Project name %q matched multiple projects. Use the project ID instead.", value), Code: "PROJECT_AMBIGUOUS"}
	}
	copy := matches[0]
	return &copy, nil
}

type projectTarget struct {
	project projectDetail
	region  string
}

var errNoProjectSelected = &cliError{Message: "No project selected. Pass `--project`, work inside a repo with `.agora/project.json`, or run `agora project use <project>`.", Code: "PROJECT_NOT_SELECTED"}

func (a *App) resolveProjectTarget(explicit string) (projectTarget, error) {
	return a.resolveProjectTargetFrom(explicit, "")
}

func (a *App) resolveProjectTargetFrom(explicit, startPath string) (projectTarget, error) {
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
			return projectTarget{}, &cliError{Message: fmt.Sprintf("Project %q was not found. Run `agora project list` to see available projects.", explicit), Code: "PROJECT_NOT_FOUND"}
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
	if binding, ok, _, err := detectLocalProjectBindingFrom(startPath); err != nil {
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
		return projectTarget{}, errNoProjectSelected
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
		"storageEnabled":          false,
		"streamChannelEnabled":    false,
		"userSubscribeEnabled":    false,
	}
	if strings.TrimSpace(region) != "" {
		body["region"] = region
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
		return featureItem{}, validateFeatureID(feature)
	}
}

func (a *App) listProjectFeatures(project projectDetail, region string) ([]featureItem, error) {
	ids := featureIDs()
	items := make([]featureItem, 0, len(ids))
	for _, feature := range ids {
		item, err := a.getFeatureItem(feature, project, region)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (a *App) enableProjectFeature(feature string, project projectDetail, region, rtmDataCenter string) (map[string]any, error) {
	switch feature {
	case "rtc":
		return map[string]any{"action": "feature-enable", "feature": "rtc", "message": "rtc is included with the project", "projectId": project.ProjectID, "projectName": project.Name, "status": "included"}, nil
	case "rtm":
		var err error
		rtmDataCenter, err = normalizeRTMDataCenter(rtmDataCenter)
		if err != nil {
			return nil, err
		}
		if rtmDataCenter == "" {
			rtmDataCenter = "NA"
		}
		if err := a.setRTM2Config(project.ProjectID, rtmDataCenter); err != nil {
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
		return nil, validateFeatureID(feature)
	}
}

func (a *App) projectCreate(name, region, template string, features []string, rtmDataCenter string, idempotencyKey string) (map[string]any, error) {
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
	features = projectCreateFeatures(template, features)
	rtmDataCenter, err = rtmDataCenterForFeatures(features, rtmDataCenter)
	if err != nil {
		return nil, err
	}
	project, err := a.createProject(name, idempotencyKey)
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	enabled := []string{}
	for _, feature := range features {
		if seen[feature] {
			continue
		}
		seen[feature] = true
		if _, err := a.enableProjectFeature(feature, project, region, rtmDataCenter); err != nil {
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
	result := map[string]any{"action": "create", "appId": project.AppID, "enabledFeatures": enabled, "projectId": project.ProjectID, "projectName": project.Name, "region": region}
	if rtmDataCenter != "" {
		result["rtmDataCenter"] = rtmDataCenter
	}
	return result, nil
}

func normalizeRTMDataCenter(value string) (string, error) {
	normalized := strings.ToUpper(strings.TrimSpace(value))
	switch normalized {
	case "":
		return "", nil
	case "CN", "NA", "EU", "AP":
		return normalized, nil
	default:
		return "", fmt.Errorf("--rtm-data-center must be one of: CN, NA, EU, AP")
	}
}

func rtmDataCenterForFeatures(features []string, value string) (string, error) {
	normalized, err := normalizeRTMDataCenter(value)
	if err != nil {
		return "", err
	}
	if !featureListIncludes(features, "rtm") {
		if normalized != "" {
			return "", fmt.Errorf("--rtm-data-center can only be used when rtm is enabled")
		}
		return "", nil
	}
	if normalized == "" {
		return "NA", nil
	}
	return normalized, nil
}

func normalizeProjectCreateFeatures(features []string) []string {
	if len(features) == 0 {
		return defaultInitFeatures()
	}
	return features
}

func projectCreateFeatures(template string, features []string) []string {
	next := append([]string{}, features...)
	if template == "voice-agent" {
		next = append(next, featureIDs()...)
	}
	next = normalizeProjectCreateFeatures(next)
	if featureListIncludes(next, "convoai") && !featureListIncludes(next, "rtm") {
		next = append([]string{"rtm"}, next...)
	}
	return next
}

func featureListIncludes(features []string, target string) bool {
	for _, feature := range features {
		if feature == target {
			return true
		}
	}
	return false
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
		return nil, &cliError{Message: fmt.Sprintf("Project %q was not found. Run `agora project list` to see available projects.", projectArg), Code: "PROJECT_NOT_FOUND"}
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
	return map[string]any{"action": "show", "appId": target.project.AppID, "appCertificate": target.project.SignKey, "projectId": target.project.ProjectID, "projectName": target.project.Name, "region": target.region, "tokenEnabled": target.project.TokenEnabled}, nil
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
			return nil, &cliError{Message: fmt.Sprintf("project %q does not have an app certificate. Enable one in Agora Console or use a different project with `agora project use`.", target.project.Name), Code: "PROJECT_NO_CERTIFICATE"}
		}
		values["AGORA_APP_CERTIFICATE"] = *target.project.SignKey
	}
	return values, nil
}

type projectEnvCredentialLayout int

const (
	projectEnvLayoutStandard projectEnvCredentialLayout = iota
	projectEnvLayoutNextjs
)

func credentialLayoutLabel(layout projectEnvCredentialLayout) string {
	if layout == projectEnvLayoutNextjs {
		return "nextjs"
	}
	return "standard"
}

func credentialLayoutFromProjectType(projectType string) projectEnvCredentialLayout {
	switch strings.ToLower(strings.TrimSpace(projectType)) {
	case "nextjs":
		return projectEnvLayoutNextjs
	default:
		return projectEnvLayoutStandard
	}
}

func projectCredentialEnvValues(project projectDetail) (map[string]any, error) {
	return projectCredentialEnvValuesForLayout(project, projectEnvLayoutStandard)
}

func projectCredentialEnvValuesForLayout(project projectDetail, layout projectEnvCredentialLayout) (map[string]any, error) {
	if project.SignKey == nil || *project.SignKey == "" {
		return nil, &cliError{Message: fmt.Sprintf("project %q does not have an app certificate. Enable one in Agora Console or use a different project with `agora project use`.", project.Name), Code: "PROJECT_NO_CERTIFICATE"}
	}
	switch layout {
	case projectEnvLayoutNextjs:
		return map[string]any{
			"NEXT_PUBLIC_AGORA_APP_ID":   project.AppID,
			"NEXT_AGORA_APP_CERTIFICATE": *project.SignKey,
		}, nil
	default:
		return map[string]any{
			"AGORA_APP_ID":          project.AppID,
			"AGORA_APP_CERTIFICATE": *project.SignKey,
		}, nil
	}
}

func conflictingKeysForProjectEnvLayout(layout projectEnvCredentialLayout) []string {
	switch layout {
	case projectEnvLayoutNextjs:
		return []string{"AGORA_APP_ID", "AGORA_APP_CERTIFICATE", "APP_ID", "APP_CERTIFICATE"}
	default:
		return nil
	}
}

func detectProjectType(workspaceDir, explicitTemplate string) (string, error) {
	if t := strings.TrimSpace(strings.ToLower(explicitTemplate)); t != "" {
		switch t {
		case "nextjs":
			return "nextjs", nil
		case "standard", "default", "agora":
			return "standard", nil
		default:
			return "", &cliError{
				Message: fmt.Sprintf("unknown env template %q (use nextjs or standard)", strings.TrimSpace(explicitTemplate)),
				Code:    "PROJECT_ENV_TEMPLATE_UNKNOWN",
			}
		}
	}

	absWorkspace, err := filepath.Abs(workspaceDir)
	if err != nil {
		return "", err
	}
	if _, statErr := os.Stat(filepath.Join(absWorkspace, "env.local.example")); statErr == nil {
		return "nextjs", nil
	}

	cur := absWorkspace
	for step := 0; step < 8; step++ {
		if packageJSONDeclaresNext(filepath.Join(cur, "package.json")) || nextConfigPresent(cur) {
			return "nextjs", nil
		}
		if _, err := os.Stat(filepath.Join(cur, "go.mod")); err == nil {
			return "go", nil
		}
		if _, err := os.Stat(filepath.Join(cur, "pyproject.toml")); err == nil {
			return "python", nil
		}
		if _, err := os.Stat(filepath.Join(cur, "requirements.txt")); err == nil {
			return "python", nil
		}
		if _, err := os.Stat(filepath.Join(cur, "package.json")); err == nil {
			return "node", nil
		}
		parent := filepath.Dir(cur)
		if parent == cur {
			break
		}
		cur = parent
	}

	if binding, ok, _, err := detectLocalProjectBindingFrom(absWorkspace); err == nil && ok {
		if projectType := strings.ToLower(strings.TrimSpace(binding.ProjectType)); projectType != "" {
			return projectType, nil
		}
		if template := strings.ToLower(strings.TrimSpace(binding.Template)); template != "" {
			return template, nil
		}
	}
	return "standard", nil
}

// syncLocalProjectBindingAfterEnvWrite updates or creates repo-local .agora/project.json
// so non-template repos keep a durable projectType signal for later env/framework decisions.
func syncLocalProjectBindingAfterEnvWrite(workspaceDir, cwd, envFileAbs string, target projectTarget, projectType string) (bool, string, error) {
	projectType = strings.TrimSpace(projectType)
	if projectType == "" {
		projectType = "standard"
	}
	resolveEnvPath := func(root string) string {
		if rel, relErr := filepath.Rel(root, envFileAbs); relErr == nil && rel != "" && !strings.HasPrefix(rel, "..") {
			return filepath.ToSlash(rel)
		}
		return ""
	}

	root, hasRoot, err := detectLocalProjectRoot(workspaceDir)
	if err != nil {
		return false, "", err
	}
	if !hasRoot {
		root = cwd
		binding := localProjectBinding{
			ProjectID:   target.project.ProjectID,
			ProjectName: target.project.Name,
			Region:      target.region,
			ProjectType: projectType,
			EnvPath:     resolveEnvPath(root),
		}
		if err := writeLocalProjectBinding(root, binding); err != nil {
			return false, "", err
		}
		return true, filepath.ToSlash(filepath.Join(localAgoraDirName, localProjectFileName)), nil
	}

	binding, err := loadLocalProjectBinding(root)
	if err != nil {
		return false, "", err
	}
	changed := false
	if binding.ProjectID != target.project.ProjectID {
		binding.ProjectID = target.project.ProjectID
		changed = true
	}
	if binding.ProjectName != target.project.Name {
		binding.ProjectName = target.project.Name
		changed = true
	}
	if strings.TrimSpace(binding.Region) == "" || !strings.EqualFold(binding.Region, target.region) {
		binding.Region = target.region
		changed = true
	}
	// Respect quickstart template bindings; only set projectType when template is unset.
	if strings.TrimSpace(binding.Template) == "" && strings.TrimSpace(binding.ProjectType) == "" {
		binding.ProjectType = projectType
		changed = true
	}
	if strings.TrimSpace(binding.EnvPath) == "" {
		if envPath := resolveEnvPath(root); envPath != "" {
			binding.EnvPath = envPath
			changed = true
		}
	}
	if !changed {
		return false, "", nil
	}
	if err := writeLocalProjectBinding(root, binding); err != nil {
		return false, "", err
	}
	return true, filepath.ToSlash(filepath.Join(localAgoraDirName, localProjectFileName)), nil
}

func detectProjectEnvCredentialLayout(workspaceDir, explicitTemplate string) (projectEnvCredentialLayout, error) {
	projectType, err := detectProjectType(workspaceDir, explicitTemplate)
	if err != nil {
		return projectEnvLayoutStandard, err
	}
	return credentialLayoutFromProjectType(projectType), nil
}

func packageJSONDeclaresNext(packageJSONPath string) bool {
	raw, err := os.ReadFile(packageJSONPath)
	if err != nil {
		return false
	}
	var meta struct {
		Dependencies     map[string]any `json:"dependencies"`
		DevDependencies  map[string]any `json:"devDependencies"`
		PeerDependencies map[string]any `json:"peerDependencies"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		return false
	}
	return depMapHasPackage(meta.Dependencies, "next") ||
		depMapHasPackage(meta.DevDependencies, "next") ||
		depMapHasPackage(meta.PeerDependencies, "next")
}

func depMapHasPackage(section map[string]any, name string) bool {
	if section == nil {
		return false
	}
	_, ok := section[name]
	return ok
}

func nextConfigPresent(dir string) bool {
	for _, name := range []string{"next.config.js", "next.config.mjs", "next.config.ts", "next.config.mts"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err == nil {
			return true
		}
	}
	return false
}

func enabledFeatures(features map[string]bool) []string {
	out := []string{}
	for _, name := range featureIDs() {
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
		"NEXT_PUBLIC_AGORA_APP_ID",
		"NEXT_AGORA_APP_CERTIFICATE",
		"APP_ID",
		"APP_CERTIFICATE",
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

func resolveProjectEnvWriteAbsolutePath(cwd, pathArg string) (string, error) {
	if strings.TrimSpace(pathArg) == "" {
		rel, err := resolveDefaultTargetPath(cwd)
		if err != nil {
			return "", err
		}
		return filepath.Abs(filepath.Join(cwd, rel))
	}
	p := pathArg
	if !filepath.IsAbs(p) {
		p = filepath.Join(cwd, p)
	}
	return filepath.Abs(p)
}

func writeProjectEnvFile(path string, values map[string]any, appendMode, overwrite bool, conflictingKeys []string, defaultPathChosen bool) (envWriteResult, error) {
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
	assignments := strings.TrimRight(renderProjectEnv(values, envDotenv), "\n")
	status := ""
	switch {
	case errors.Is(err, os.ErrNotExist):
		existing = []byte(assignments + "\n")
		status = "created"
	case overwrite:
		existing = []byte(assignments + "\n")
		status = "overwritten"
	default:
		merged, mergeStatus := mergeEnvAssignments(string(existing), values, [][2]string{{"# BEGIN AGORA CLI", "# END AGORA CLI"}}, conflictingKeys)
		if mergeStatus == "appended" && !appendMode && !defaultPathChosen && !isDefaultEnvPath(filePath) {
			return envWriteResult{}, fmt.Errorf("%s already exists. Use --append to append it or --overwrite to replace it.", path)
		}
		existing = []byte(merged)
		status = mergeStatus
		if status == "empty" {
			status = "updated"
		}
	}
	if err := os.WriteFile(filePath, existing, 0o644); err != nil {
		return envWriteResult{}, err
	}
	return envWriteResult{Path: filePath, Status: status}, nil
}

func isDefaultEnvPath(path string) bool {
	name := filepath.Base(path)
	return name == ".env" || name == ".env.local"
}

func detectEOL(v string) string {
	if strings.Contains(v, "\r\n") {
		return "\r\n"
	}
	return "\n"
}

func mergeEnvAssignments(existing string, values map[string]any, oldBlocks [][2]string, conflictingKeys []string) (string, string) {
	eol := detectEOL(existing)
	normalized := strings.ReplaceAll(existing, "\r\n", "\n")
	removedOldBlock := false
	for _, markers := range oldBlocks {
		var removed bool
		normalized, removed = removeDelimitedBlock(normalized, markers[0], markers[1])
		removedOldBlock = removedOldBlock || removed
	}

	trimmed := strings.TrimRight(normalized, "\n")
	lines := []string{}
	if trimmed != "" {
		lines = strings.Split(trimmed, "\n")
	}

	keys := projectEnvKeys(values)
	found := map[string]bool{}
	conflicts := map[string]bool{}
	for _, key := range conflictingKeys {
		if _, expected := values[key]; !expected {
			conflicts[key] = true
		}
	}
	updatedExisting := removedOldBlock
	for index, line := range lines {
		key := dotenvLineKey(line)
		if key == "" {
			continue
		}
		for _, expected := range keys {
			if key == expected {
				if found[expected] {
					lines[index] = commentReplacedEnvLine(line)
				} else {
					lines[index] = expected + "=" + renderDotenvScalar(values[expected])
					found[expected] = true
				}
				updatedExisting = true
				break
			}
		}
		if conflicts[key] {
			lines[index] = commentReplacedEnvLine(line)
			updatedExisting = true
		}
	}

	missing := []string{}
	for _, key := range keys {
		if !found[key] {
			missing = append(missing, key+"="+renderDotenvScalar(values[key]))
		}
	}
	status := "updated"
	if len(lines) == 0 && len(missing) == 0 {
		return "", "empty"
	}
	if len(missing) > 0 {
		if !updatedExisting {
			status = "appended"
		}
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, missing...)
	}
	return strings.Join(lines, eol) + eol, status
}

func removeDelimitedBlock(existing, begin, end string) (string, bool) {
	start := strings.Index(existing, begin)
	if start == -1 {
		return existing, false
	}
	endIndex := strings.Index(existing[start:], end)
	if endIndex == -1 {
		return existing, false
	}
	endIndex = start + endIndex + len(end)
	prefix := strings.TrimRight(existing[:start], "\n")
	suffix := strings.TrimLeft(existing[endIndex:], "\n")
	switch {
	case prefix == "":
		return suffix, true
	case suffix == "":
		return prefix + "\n", true
	default:
		return prefix + "\n" + suffix, true
	}
}

func dotenvLineKey(line string) string {
	trimmed := strings.TrimSpace(line)
	if trimmed == "" || strings.HasPrefix(trimmed, "#") || !strings.Contains(trimmed, "=") {
		return ""
	}
	parts := strings.SplitN(trimmed, "=", 2)
	key := strings.TrimSpace(parts[0])
	if strings.HasPrefix(key, "export ") {
		key = strings.TrimSpace(strings.TrimPrefix(key, "export "))
	}
	return key
}

func commentReplacedEnvLine(line string) string {
	if strings.TrimSpace(line) == "" || strings.HasPrefix(strings.TrimSpace(line), "#") {
		return line
	}
	return "# Replaced by Agora CLI: " + line
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
	return a.enableProjectFeature(feature, target.project, target.region, "")
}
