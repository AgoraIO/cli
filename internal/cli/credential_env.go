package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

type credentialEnvTarget struct {
	Path              string
	ExamplePath       string
	AppIDKey          string
	AppCertificateKey string
}

type credentialEnvWriteOptions struct {
	AllowAppend bool
	Overwrite   bool
}

type credentialEnvWriteResult struct {
	Path   string
	Status string
	Values map[string]any
}

var credentialEnvLegacyBlocks = [][2]string{
	{"# BEGIN AGORA CLI", "# END AGORA CLI"},
	{"# BEGIN AGORA CLI QUICKSTART", "# END AGORA CLI QUICKSTART"},
}

func credentialEnvKeysForProjectLayout(layout projectEnvCredentialLayout) (string, string) {
	if layout == projectEnvLayoutNextjs {
		return "NEXT_PUBLIC_AGORA_APP_ID", "NEXT_AGORA_APP_CERTIFICATE"
	}
	return "AGORA_APP_ID", "AGORA_APP_CERTIFICATE"
}

func credentialEnvValues(project projectDetail, appIDKey, certificateKey string) (map[string]any, error) {
	if project.SignKey == nil || *project.SignKey == "" {
		return nil, &cliError{Message: fmt.Sprintf("project %q does not have an app certificate. Enable one in Agora Console or use a different project with `agora project use`.", project.Name), Code: "PROJECT_NO_CERTIFICATE"}
	}
	return map[string]any{
		appIDKey:       project.AppID,
		certificateKey: *project.SignKey,
	}, nil
}

func conflictingCredentialEnvKeys(appIDKey, certificateKey string) []string {
	all := []string{
		"NEXT_PUBLIC_AGORA_APP_ID",
		"NEXT_AGORA_APP_CERTIFICATE",
		"AGORA_APP_ID",
		"AGORA_APP_CERTIFICATE",
		"APP_ID",
		"APP_CERTIFICATE",
	}
	conflicts := make([]string, 0, len(all)-2)
	for _, key := range all {
		if key != appIDKey && key != certificateKey {
			conflicts = append(conflicts, key)
		}
	}
	return conflicts
}

func writeCredentialEnv(target credentialEnvTarget, project projectDetail, options credentialEnvWriteOptions) (credentialEnvWriteResult, error) {
	if target.Path == "" || target.AppIDKey == "" || target.AppCertificateKey == "" {
		return credentialEnvWriteResult{}, errors.New("credential env target requires a path, App ID key, and App Certificate key")
	}
	values, err := credentialEnvValues(project, target.AppIDKey, target.AppCertificateKey)
	if err != nil {
		return credentialEnvWriteResult{}, err
	}
	file, err := writeEnvAssignmentsFile(
		target.Path,
		target.ExamplePath,
		values,
		options.Overwrite,
		options.AllowAppend,
		conflictingCredentialEnvKeys(target.AppIDKey, target.AppCertificateKey),
		credentialEnvLegacyBlocks,
	)
	if err != nil {
		return credentialEnvWriteResult{}, err
	}
	return credentialEnvWriteResult{Path: file.Path, Status: file.Status, Values: values}, nil
}

func writeEnvAssignmentsFile(path, examplePath string, values map[string]any, overwrite, allowAppend bool, conflictingKeys []string, oldBlocks [][2]string) (envWriteResult, error) {
	filePath, err := filepath.Abs(path)
	if err != nil {
		return envWriteResult{}, err
	}
	existing, readErr := os.ReadFile(filePath)
	missing := errors.Is(readErr, os.ErrNotExist)
	if readErr != nil && !missing {
		return envWriteResult{}, readErr
	}
	if missing && examplePath != "" {
		example, exampleErr := os.ReadFile(examplePath)
		if exampleErr == nil {
			existing = example
		} else if !errors.Is(exampleErr, os.ErrNotExist) {
			return envWriteResult{}, exampleErr
		}
	}

	status := ""
	switch {
	case missing:
		merged, _ := mergeEnvAssignments(string(existing), values, oldBlocks, conflictingKeys)
		existing = []byte(merged)
		status = "created"
	case overwrite:
		existing = []byte(renderProjectEnv(values, envDotenv))
		status = "overwritten"
	default:
		merged, mergeStatus := mergeEnvAssignments(string(existing), values, oldBlocks, conflictingKeys)
		if mergeStatus == "appended" && !allowAppend {
			return envWriteResult{}, fmt.Errorf("%s already exists. Use --append to append it or --overwrite to replace it.", path)
		}
		existing = []byte(merged)
		status = mergeStatus
		if status == "empty" {
			status = "updated"
		}
	}

	if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
		return envWriteResult{}, err
	}
	if err := os.WriteFile(filePath, existing, 0o644); err != nil {
		return envWriteResult{}, err
	}
	return envWriteResult{Path: filePath, Status: status}, nil
}
