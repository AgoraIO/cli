package cli

import (
	"fmt"
	"strings"
)

func validateDoctorFeature(feature string) error {
	switch feature {
	case "rtc", "rtm", "convoai":
		return nil
	default:
		return fmt.Errorf("%q must be one of: rtc, rtm, convoai.", feature)
	}
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

func createDoctorAuthErrorResult(feature string, deep bool, message, suggested string) projectDoctorResult {
	item := doctorCheckItem{Name: "session_valid", Message: message, Status: "fail"}
	if suggested != "" {
		item.SuggestedCommand = suggested
	}
	issue := doctorIssue{Code: "AUTH_REQUIRED", Message: message}
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
		configItems[0] = doctorCheckItem{Name: "app_credentials", Message: "App credentials missing", Status: "fail"}
		blocking = append(blocking, doctorIssue{Code: "APP_CREDENTIALS_MISSING", Message: "App credentials missing"})
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
	}
	if deep {
		readinessItems = append(readinessItems, doctorCheckItem{Name: "runtime_preflight", Message: "Deep runtime preflight is not available in CLI 0.1.3", Status: "skipped"})
	}
	checks := []doctorCheckCategory{
		{Category: "auth", Items: authItems, Status: summarizeCategoryStatus(authItems)},
		{Category: "project", Items: projectItems, Status: summarizeCategoryStatus(projectItems)},
		{Category: "features", Items: featureItems, Status: summarizeCategoryStatus(featureItems)},
		{Category: "configuration", Items: configItems, Status: summarizeCategoryStatus(configItems)},
		{Category: "readiness", Items: readinessItems, Status: summarizeCategoryStatus(readinessItems)},
	}
	status := "healthy"
	summary := "Project is ready for " + targetName
	if len(blocking) > 0 {
		status = "not_ready"
		summary = "Project is not ready for " + targetName
	} else if len(warnings) > 0 {
		status = "warning"
		summary = "Project is partially ready for " + targetName
	}
	return projectDoctorResult{
		Action:         "doctor",
		BlockingIssues: blocking,
		Checks:         checks,
		Feature:        feature,
		Healthy:        len(blocking) == 0,
		Mode:           map[bool]string{true: "deep", false: "default"}[deep],
		Project:        map[string]any{"id": project.ProjectID, "name": project.Name, "region": region},
		Status:         status,
		Summary:        summary,
		Warnings:       warnings,
	}
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
	return buildProjectDoctorResult(target.project, target.region, features, feature, deep)
}
