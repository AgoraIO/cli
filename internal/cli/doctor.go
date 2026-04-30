package cli

import (
	"os"
	"path/filepath"
	"strings"
)

// validateDoctorFeature defers to the canonical feature catalog so the
// list of accepted `--feature` values stays in lockstep with the rest
// of the CLI.
func validateDoctorFeature(feature string) error {
	return validateFeatureID(feature)
}

func doctorFeatureDependencies(feature string) map[string]bool {
	required := map[string]bool{"rtc": true}
	if feature == "rtm" || feature == "convoai" {
		required["rtm"] = true
	}
	if feature == "convoai" {
		required["convoai"] = true
	}
	return required
}

func summarizeCategoryStatus(items []doctorCheckItem) string {
	hasPass := false
	hasWarn := false
	for _, item := range items {
		switch item.Status {
		case "fail":
			return "fail"
		case "warn":
			hasWarn = true
		case "pass":
			hasPass = true
		}
	}
	if hasWarn {
		return "warn"
	}
	if hasPass {
		return "pass"
	}
	return "skipped"
}

func quickstartAppIDKey(templateID string) string {
	switch templateID {
	case "nextjs":
		return "NEXT_PUBLIC_AGORA_APP_ID"
	case "python", "go":
		return "APP_ID"
	default:
		return ""
	}
}

func lookupDotenvValue(content, key string) (string, bool) {
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		parts := strings.SplitN(trimmed, "=", 2)
		if len(parts) != 2 {
			continue
		}
		if strings.TrimSpace(parts[0]) != key {
			continue
		}
		value := strings.TrimSpace(parts[1])
		value = strings.Trim(value, `"'`)
		return value, true
	}
	return "", false
}

func lookupManagedMetadataValue(content, key string) (string, bool) {
	prefix := "# " + key + ":"
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, prefix)), true
		}
	}
	return "", false
}

func upsertDoctorReadiness(result *projectDoctorResult) {
	targetName := strings.ToUpper(result.Feature)
	status := "pass"
	message := "Project is ready for " + targetName + " development"
	if len(result.BlockingIssues) > 0 {
		status = "fail"
		message = "Blocking readiness issues found"
	} else if len(result.Warnings) > 0 {
		status = "warn"
		message = "Project has non-blocking readiness warnings"
	}
	readiness := doctorCheckCategory{
		Category: "readiness",
		Items:    []doctorCheckItem{{Name: "control_plane_readiness", Message: message, Status: status}},
	}
	readiness.Status = summarizeCategoryStatus(readiness.Items)
	for index, check := range result.Checks {
		if check.Category == "readiness" {
			result.Checks[index] = readiness
			return
		}
	}
	result.Checks = append(result.Checks, readiness)
}

func finalizeDoctorOutcome(result *projectDoctorResult) {
	upsertDoctorReadiness(result)
	targetName := strings.ToUpper(result.Feature)
	result.Healthy = len(result.BlockingIssues) == 0
	result.Status = "healthy"
	result.Summary = "Project is ready for " + targetName
	if len(result.BlockingIssues) > 0 {
		result.Status = "not_ready"
		result.Summary = "Project is not ready for " + targetName
	} else if len(result.Warnings) > 0 {
		result.Status = "warning"
		result.Summary = "Project is partially ready for " + targetName
	}
}

func buildWorkspaceDoctorDetails(target projectTarget) (doctorCheckCategory, map[string]any, []doctorIssue, []doctorIssue) {
	items := []doctorCheckItem{}
	blocking := []doctorIssue{}
	warnings := []doctorIssue{}
	workspace := map[string]any{
		"detected":     false,
		"metadataPath": filepath.ToSlash(filepath.Join(localAgoraDirName, localProjectFileName)),
	}
	binding, ok, root, err := detectLocalProjectBinding()
	if err != nil {
		items = append(items, doctorCheckItem{Name: "workspace_scan", Message: "Failed to inspect repo-local project binding: " + err.Error(), Status: "warn"})
		warnings = append(warnings, doctorIssue{Code: "WORKSPACE_SCAN_FAILED", Message: err.Error()})
		check := doctorCheckCategory{Category: "workspace", Items: items}
		check.Status = summarizeCategoryStatus(items)
		return check, workspace, blocking, warnings
	}
	if !ok {
		items = append(items, doctorCheckItem{Name: "workspace_binding", Message: "No .agora/project.json detected from current working directory", Status: "skipped"})
		check := doctorCheckCategory{Category: "workspace", Items: items}
		check.Status = summarizeCategoryStatus(items)
		return check, workspace, blocking, warnings
	}

	workspace["detected"] = true
	workspace["root"] = root
	workspace["metadataPath"] = filepath.ToSlash(filepath.Join(root, localAgoraDirName, localProjectFileName))
	workspace["bindingProjectId"] = binding.ProjectID
	workspace["bindingProjectName"] = binding.ProjectName
	workspace["bindingRegion"] = binding.Region

	items = append(items, doctorCheckItem{Name: "workspace_binding", Message: "Detected repo-local project metadata", Status: "pass"})
	if binding.ProjectID == "" {
		items = append(items, doctorCheckItem{Name: "metadata_project_id", Message: ".agora/project.json is missing projectId", Status: "fail"})
		blocking = append(blocking, doctorIssue{Code: "LOCAL_PROJECT_BINDING_INVALID", Message: ".agora/project.json is missing projectId"})
	} else if binding.ProjectID != target.project.ProjectID {
		items = append(items, doctorCheckItem{
			Name:             "metadata_project_match",
			Message:          "Repo binding points to " + binding.ProjectID + " but current project is " + target.project.ProjectID,
			Status:           "fail",
			SuggestedCommand: "agora quickstart env write --project " + target.project.ProjectID,
		})
		blocking = append(blocking, doctorIssue{
			Code:             "LOCAL_PROJECT_BINDING_MISMATCH",
			Message:          "Repo-local project metadata does not match the selected project.",
			SuggestedCommand: "agora quickstart env write --project " + target.project.ProjectID,
		})
	} else {
		items = append(items, doctorCheckItem{Name: "metadata_project_match", Message: "Repo binding matches the selected project", Status: "pass"})
	}

	templateID := strings.TrimSpace(binding.Template)
	if templateID == "" {
		if template, detectErr := resolveQuickstartTemplateForPath(root, ""); detectErr == nil {
			templateID = template.ID
		}
	}
	if templateID == "" {
		items = append(items, doctorCheckItem{Name: "workspace_template", Message: "Could not detect quickstart template for this repo", Status: "warn"})
		warnings = append(warnings, doctorIssue{Code: "WORKSPACE_TEMPLATE_UNKNOWN", Message: "Could not detect quickstart template for this repo"})
		check := doctorCheckCategory{Category: "workspace", Items: items}
		check.Status = summarizeCategoryStatus(items)
		return check, workspace, blocking, warnings
	}
	workspace["template"] = templateID
	items = append(items, doctorCheckItem{Name: "workspace_template", Message: "Detected template: " + templateID, Status: "pass"})

	template, found := findQuickstartTemplate(templateID)
	envRel := strings.TrimSpace(binding.EnvPath)
	if envRel == "" && found {
		envRel = template.EnvTargetPath
	}
	if envRel == "" {
		items = append(items, doctorCheckItem{Name: "workspace_env_path", Message: "Could not determine quickstart env target path", Status: "warn"})
		warnings = append(warnings, doctorIssue{Code: "WORKSPACE_ENV_PATH_UNKNOWN", Message: "Could not determine quickstart env target path"})
		check := doctorCheckCategory{Category: "workspace", Items: items}
		check.Status = summarizeCategoryStatus(items)
		return check, workspace, blocking, warnings
	}

	workspace["envPath"] = filepath.ToSlash(envRel)
	envFilePath := filepath.Join(root, filepath.FromSlash(envRel))
	raw, readErr := os.ReadFile(envFilePath)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			items = append(items, doctorCheckItem{
				Name:             "workspace_env_file",
				Message:          "Expected env file is missing: " + filepath.ToSlash(envRel),
				Status:           "fail",
				SuggestedCommand: "agora quickstart env write . --project " + target.project.ProjectID,
			})
			blocking = append(blocking, doctorIssue{
				Code:             "WORKSPACE_ENV_FILE_MISSING",
				Message:          "Expected quickstart env file is missing.",
				SuggestedCommand: "agora quickstart env write . --project " + target.project.ProjectID,
			})
		} else {
			// The env file exists but cannot be read (permissions, encoding, IO).
			// Re-running `quickstart env write` recreates the file with the
			// correct ownership, mode, and encoding from a known-good source.
			recoveryCmd := "agora quickstart env write . --project " + target.project.ProjectID + " --overwrite"
			items = append(items, doctorCheckItem{
				Name:             "workspace_env_file",
				Message:          "Failed to read quickstart env file: " + readErr.Error(),
				Status:           "fail",
				SuggestedCommand: recoveryCmd,
			})
			blocking = append(blocking, doctorIssue{
				Code:             "WORKSPACE_ENV_READ_FAILED",
				Message:          readErr.Error(),
				SuggestedCommand: recoveryCmd,
			})
		}
		check := doctorCheckCategory{Category: "workspace", Items: items}
		check.Status = summarizeCategoryStatus(items)
		return check, workspace, blocking, warnings
	}

	workspace["envFileExists"] = true
	items = append(items, doctorCheckItem{Name: "workspace_env_file", Message: "Env file exists: " + filepath.ToSlash(envRel), Status: "pass"})
	envContent := string(raw)
	if envProjectID, ok := lookupManagedMetadataValue(envContent, "Project ID"); ok {
		workspace["envProjectId"] = envProjectID
		if envProjectID != target.project.ProjectID {
			items = append(items, doctorCheckItem{
				Name:             "workspace_env_project_match",
				Message:          "Env metadata points to project " + envProjectID + " but current project is " + target.project.ProjectID,
				Status:           "fail",
				SuggestedCommand: "agora quickstart env write . --project " + target.project.ProjectID,
			})
			blocking = append(blocking, doctorIssue{
				Code:             "WORKSPACE_ENV_PROJECT_MISMATCH",
				Message:          "Quickstart env metadata does not match the selected project.",
				SuggestedCommand: "agora quickstart env write . --project " + target.project.ProjectID,
			})
		} else {
			items = append(items, doctorCheckItem{Name: "workspace_env_project_match", Message: "Env metadata matches the selected project", Status: "pass"})
		}
	} else {
		items = append(items, doctorCheckItem{Name: "workspace_env_project_match", Message: "Env metadata is missing project comments from Agora-managed block", Status: "warn"})
		warnings = append(warnings, doctorIssue{Code: "WORKSPACE_ENV_METADATA_MISSING", Message: "Quickstart env file is missing Agora-managed project metadata comments"})
	}

	appIDKey := quickstartAppIDKey(templateID)
	if appIDKey != "" {
		if envAppID, ok := lookupDotenvValue(envContent, appIDKey); !ok {
			items = append(items, doctorCheckItem{
				Name:             "workspace_env_app_id",
				Message:          "Env file is missing required key " + appIDKey,
				Status:           "fail",
				SuggestedCommand: "agora quickstart env write . --project " + target.project.ProjectID,
			})
			blocking = append(blocking, doctorIssue{
				Code:             "WORKSPACE_ENV_APP_ID_MISSING",
				Message:          "Quickstart env file is missing required app ID key.",
				SuggestedCommand: "agora quickstart env write . --project " + target.project.ProjectID,
			})
		} else if envAppID != target.project.AppID {
			workspace["envAppID"] = envAppID
			items = append(items, doctorCheckItem{
				Name:             "workspace_env_app_id",
				Message:          "Env app ID " + envAppID + " does not match project app ID " + target.project.AppID,
				Status:           "fail",
				SuggestedCommand: "agora quickstart env write . --project " + target.project.ProjectID,
			})
			blocking = append(blocking, doctorIssue{
				Code:             "WORKSPACE_ENV_APP_ID_MISMATCH",
				Message:          "Quickstart env app ID does not match the selected project.",
				SuggestedCommand: "agora quickstart env write . --project " + target.project.ProjectID,
			})
		} else {
			workspace["envAppID"] = envAppID
			items = append(items, doctorCheckItem{Name: "workspace_env_app_id", Message: "Env app ID matches the selected project", Status: "pass"})
		}
	}

	check := doctorCheckCategory{Category: "workspace", Items: items}
	check.Status = summarizeCategoryStatus(items)
	return check, workspace, blocking, warnings
}

func createDoctorAuthErrorResult(feature string, deep bool, message, suggested string) projectDoctorResult {
	item := doctorCheckItem{Name: "session_valid", Message: message, Status: "fail"}
	if suggested != "" {
		item.SuggestedCommand = suggested
	}
	issue := doctorIssue{Code: "AUTH_UNAUTHENTICATED", Message: message}
	if suggested != "" {
		issue.SuggestedCommand = suggested
	}
	return projectDoctorResult{
		Action:         "doctor",
		BlockingIssues: []doctorIssue{issue},
		Checks:         []doctorCheckCategory{{Category: "auth", Items: []doctorCheckItem{item}, Status: "fail"}},
		Feature:        feature,
		Healthy:        false,
		Mode:           map[bool]string{true: "deep", false: "default"}[deep],
		Project:        nil,
		Status:         "auth_error",
		Summary:        message,
		Warnings:       []doctorIssue{},
	}
}

func buildProjectDoctorResult(project projectDetail, region string, features []featureItem, feature string, deep bool) projectDoctorResult {
	blocking := []doctorIssue{}
	warnings := []doctorIssue{}
	required := doctorFeatureDependencies(feature)
	authItems := []doctorCheckItem{{Name: "session_valid", Message: "Session is valid", Status: "pass"}, {Name: "project_access", Message: "Project access confirmed", Status: "pass"}}
	projectItems := []doctorCheckItem{{Name: "project_found", Message: "Project found: " + project.Name, Status: "pass"}, {Name: "project_region", Message: "Region: " + region, Status: "pass"}}
	featureItems := []doctorCheckItem{}
	for _, feature := range features {
		item := doctorCheckItem{Name: feature.Feature + "_enabled", Message: feature.Message}
		requiredFeature := required[feature.Feature]
		switch feature.Status {
		case "enabled", "included":
			item.Status = "pass"
		case "provisioning":
			if requiredFeature {
				item.Status = "warn"
				warnings = append(warnings, doctorIssue{Code: "FEATURE_" + strings.ToUpper(feature.Feature) + "_PROVISIONING", Message: feature.Message})
			} else {
				item.Status = "skipped"
			}
		default:
			if requiredFeature {
				item.Status = "fail"
				if feature.Feature != "rtc" {
					item.SuggestedCommand = "agora project feature enable " + feature.Feature
				}
				blocking = append(blocking, doctorIssue{Code: "FEATURE_" + strings.ToUpper(feature.Feature) + "_DISABLED", Message: feature.Message, SuggestedCommand: item.SuggestedCommand})
			} else {
				item.Status = "skipped"
			}
		}
		featureItems = append(featureItems, item)
	}
	configItems := []doctorCheckItem{{Name: "app_credentials", Message: "App credentials available", Status: "pass"}}
	if project.AppID == "" {
		// The remote project exists but the BFF returned no App ID. The
		// most useful next step is to re-fetch the project (which often
		// resolves transient provisioning lag) or re-select the intended
		// project explicitly.
		recoveryCmd := "agora project show --project " + project.ProjectID
		configItems[0] = doctorCheckItem{
			Name:             "app_credentials",
			Message:          "App credentials missing",
			Status:           "fail",
			SuggestedCommand: recoveryCmd,
		}
		blocking = append(blocking, doctorIssue{
			Code:             "APP_CREDENTIALS_MISSING",
			Message:          "App credentials missing",
			SuggestedCommand: recoveryCmd,
		})
	}
	if project.TokenEnabled {
		configItems = append(configItems, doctorCheckItem{Name: "token_capability", Message: "Token capability enabled for the project", Status: "pass"})
	} else {
		configItems = append(configItems, doctorCheckItem{Name: "token_capability", Message: "Token capability is disabled for this project", Status: "warn"})
		warnings = append(warnings, doctorIssue{Code: "TOKEN_CAPABILITY_DISABLED", Message: "Token capability is disabled for this project"})
	}
	targetName := strings.ToUpper(feature)
	readinessItems := []doctorCheckItem{{Name: "control_plane_readiness", Message: "Project is ready for " + targetName + " development", Status: "pass"}}
	if len(blocking) > 0 {
		readinessItems[0] = doctorCheckItem{Name: "control_plane_readiness", Message: "Blocking readiness issues found", Status: "fail"}
	} else if len(warnings) > 0 {
		readinessItems[0] = doctorCheckItem{Name: "control_plane_readiness", Message: "Project has non-blocking readiness warnings", Status: "warn"}
	}
	checks := []doctorCheckCategory{
		{Category: "auth", Items: authItems, Status: summarizeCategoryStatus(authItems)},
		{Category: "project", Items: projectItems, Status: summarizeCategoryStatus(projectItems)},
		{Category: "features", Items: featureItems, Status: summarizeCategoryStatus(featureItems)},
		{Category: "configuration", Items: configItems, Status: summarizeCategoryStatus(configItems)},
		{Category: "readiness", Items: readinessItems, Status: summarizeCategoryStatus(readinessItems)},
	}
	result := projectDoctorResult{
		Action:         "doctor",
		BlockingIssues: blocking,
		Checks:         checks,
		Feature:        feature,
		Healthy:        len(blocking) == 0,
		Mode:           map[bool]string{true: "deep", false: "default"}[deep],
		Project:        map[string]any{"id": project.ProjectID, "name": project.Name, "region": region},
		Status:         "healthy",
		Summary:        "Project is ready for " + targetName,
		Warnings:       warnings,
	}
	finalizeDoctorOutcome(&result)
	return result
}

func (a *App) projectDoctor(projectArg, feature string, deep bool) projectDoctorResult {
	status, err := a.authStatus()
	if err != nil {
		return createDoctorAuthErrorResult(feature, deep, err.Error(), "agora login")
	}
	if auth, _ := status["authenticated"].(bool); !auth {
		return createDoctorAuthErrorResult(feature, deep, "Not logged in", "agora login")
	}
	target, err := a.resolveProjectTarget(projectArg)
	if err != nil {
		suggested := "agora project use <project>"
		if isAuthRequired(err) {
			suggested = "agora login"
		}
		return createDoctorAuthErrorResult(feature, deep, err.Error(), suggested)
	}
	features, err := a.listProjectFeatures(target.project, target.region)
	if err != nil {
		return createDoctorAuthErrorResult(feature, deep, err.Error(), "agora project use <project>")
	}
	result := buildProjectDoctorResult(target.project, target.region, features, feature, deep)
	if deep {
		workspaceCheck, workspace, blocking, warnings := buildWorkspaceDoctorDetails(target)
		result.Checks = append(result.Checks, workspaceCheck)
		result.Workspace = workspace
		if len(blocking) > 0 {
			result.BlockingIssues = append(result.BlockingIssues, blocking...)
		}
		if len(warnings) > 0 {
			result.Warnings = append(result.Warnings, warnings...)
		}
		finalizeDoctorOutcome(&result)
	}
	return result
}
