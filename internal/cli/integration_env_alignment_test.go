package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIProjectEnvWriteNormalizesLegacyCredentialNames(t *testing.T) {
	configHome := t.TempDir()
	repoRoot := t.TempDir()
	api := newFakeCLIBFF()
	t.Cleanup(func() { _ = api.server.Close() })
	project := buildFakeProject("Demo", "prj_env_alignment", "app_env_alignment", "global")
	api.projects[project.ProjectID] = &project
	persistSessionForIntegration(t, configHome)
	if err := saveContext(map[string]string{"XDG_CONFIG_HOME": configHome}, projectContext{
		CurrentProjectID:   &project.ProjectID,
		CurrentProjectName: &project.Name,
		CurrentRegion:      "global",
	}); err != nil {
		t.Fatal(err)
	}

	envPath := filepath.Join(repoRoot, ".env")
	if err := os.WriteFile(envPath, []byte("USER_VALUE=keep\nAPP_ID=old\nAPP_CERTIFICATE=old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	result := runCLI(t, []string{"project", "env", "write", ".env", "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":    configHome,
			"AGORA_API_BASE_URL": api.baseURL,
			"AGORA_LOG_LEVEL":    "error",
		},
		workdir: repoRoot,
	})
	if result.exitCode != 0 {
		t.Fatalf("project env write = exit %d, stdout %s, stderr %s", result.exitCode, result.stdout, result.stderr)
	}
	content := "\n" + readCredentialEnvTestFile(t, envPath)
	if !strings.Contains(content, "\nAGORA_APP_ID=app_env_alignment") || !strings.Contains(content, "\nAGORA_APP_CERTIFICATE=") || strings.Contains(content, "\nAPP_ID=") || strings.Contains(content, "\nAPP_CERTIFICATE=") || !strings.Contains(content, "USER_VALUE=keep") {
		t.Fatalf("unexpected aligned project env: %s", content)
	}
}
