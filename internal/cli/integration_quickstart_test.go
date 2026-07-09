package cli

// Integration tests for `agora quickstart` (list, create, env write).
// Shared helpers live in integration_test.go.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIQuickstartListAndCreate(t *testing.T) {
	configHome := t.TempDir()
	api := newFakeCLIBFF()
	defer api.server.Close()
	nextjsRepo := createLocalGitRepo(t, map[string]string{
		"README.md":         "# Next.js Quickstart\n",
		"env.local.example": "NEXT_PUBLIC_AGORA_APP_ID=\nNEXT_AGORA_APP_CERTIFICATE=\n",
		"package.json":      `{"name":"nextjs-quickstart"}`,
		"app/page.tsx":      "export default function Page() { return null }\n",
	})
	pythonRepo := createLocalGitRepo(t, map[string]string{
		"README.md":               "# Python Quickstart\n",
		"server/.env.example":     "AGORA_APP_ID=\nAGORA_APP_CERTIFICATE=\nPORT=8000\n",
		"server/main.py":          "print('hello')\n",
		"server/requirements.txt": "",
		"web/package.json":        `{"name":"python-quickstart-web"}`,
	})
	goRepo := createLocalGitRepo(t, map[string]string{
		"README.md":           "# Go Quickstart\n",
		"server/.env.example": "AGORA_APP_ID=\nAGORA_APP_CERTIFICATE=\nPORT=8080\n",
		"server/go.mod":       "module agent-quickstart-go/server\n",
		"server/main.go":      "package main\nfunc main() {}\n",
		"client/package.json": `{"name":"go-quickstart-web"}`,
	})

	project := buildFakeProject("Project Alpha", "prj_123456", "app_123456", "global")
	api.projects[project.ProjectID] = &project
	persistSessionForIntegration(t, configHome)
	if err := saveContext(map[string]string{"XDG_CONFIG_HOME": configHome}, projectContext{
		CurrentProjectID:   &project.ProjectID,
		CurrentProjectName: &project.Name,
		CurrentRegion:      "global",
	}); err != nil {
		t.Fatal(err)
	}

	list := runCLI(t, []string{"quickstart", "list", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME": configHome,
		"AGORA_LOG_LEVEL": "error",
	}})
	if list.exitCode != 0 || !strings.Contains(list.stdout, `"id":"nextjs"`) || !strings.Contains(list.stdout, `"id":"python"`) || !strings.Contains(list.stdout, `"id":"go"`) {
		t.Fatalf("unexpected quickstart list result: %+v", list)
	}
	listAll := runCLI(t, []string{"quickstart", "list", "--show-all", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME": configHome,
		"AGORA_LOG_LEVEL": "error",
	}})
	if listAll.exitCode != 0 || !strings.Contains(listAll.stdout, `"id":"go"`) {
		t.Fatalf("unexpected quickstart list --show-all result: %+v", listAll)
	}

	rootDir := t.TempDir()
	unboundTarget := filepath.Join(rootDir, "video-demo")
	createUnbound := runCLI(t, []string{"quickstart", "create", "video-demo", "--template", "nextjs", "--dir", unboundTarget, "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":                  t.TempDir(),
			"AGORA_LOG_LEVEL":                  "error",
			"AGORA_QUICKSTART_NEXTJS_REPO_URL": nextjsRepo,
		},
		workdir: rootDir,
	})
	if createUnbound.exitCode != 0 || !strings.Contains(createUnbound.stdout, `"envStatus":"template-only"`) {
		t.Fatalf("unexpected unbound quickstart create result: %+v", createUnbound)
	}
	if !strings.Contains(createUnbound.stdout, `"written":[]`) {
		t.Fatalf("expected unbound quickstart written field to be an empty array, got: %s", createUnbound.stdout)
	}
	if _, err := os.Stat(filepath.Join(unboundTarget, ".git")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("did not expect upstream .git in unbound scaffold, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(unboundTarget, ".env.local")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("did not expect .env.local in unbound scaffold, got %v", err)
	}

	boundTarget := filepath.Join(rootDir, "agent-demo")
	createBound := runCLI(t, []string{"quickstart", "create", "agent-demo", "--template", "python", "--dir", boundTarget, "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":                  configHome,
			"AGORA_API_BASE_URL":               api.baseURL,
			"AGORA_LOG_LEVEL":                  "error",
			"AGORA_QUICKSTART_PYTHON_REPO_URL": pythonRepo,
		},
		workdir: rootDir,
	})
	if createBound.exitCode != 0 || !strings.Contains(createBound.stdout, `"envStatus":"configured"`) || !strings.Contains(createBound.stdout, `"projectId":"prj_123456"`) {
		t.Fatalf("unexpected bound quickstart create result: %+v", createBound)
	}
	localEnv, err := os.ReadFile(filepath.Join(boundTarget, "server", ".env.local"))
	if err != nil {
		t.Fatalf("expected .env.local in bound scaffold: %v", err)
	}
	if !strings.Contains(string(localEnv), "AGORA_APP_ID=app_123456") || !strings.Contains(string(localEnv), "AGORA_APP_CERTIFICATE=") || !strings.Contains(string(localEnv), "PORT=8000") || strings.Contains(string(localEnv), "# Project ID:") || strings.Contains(string(localEnv), "# Project Name:") || strings.Contains(string(localEnv), "BEGIN AGORA CLI QUICKSTART") {
		t.Fatalf("unexpected .env.local contents: %s", string(localEnv))
	}
	metadataRaw, err := os.ReadFile(filepath.Join(boundTarget, ".agora", "project.json"))
	if err != nil {
		t.Fatalf("expected .agora/project.json in bound scaffold: %v", err)
	}
	if !strings.Contains(string(metadataRaw), `"projectId": "prj_123456"`) || !strings.Contains(string(metadataRaw), `"template": "python"`) {
		t.Fatalf("unexpected .agora/project.json contents: %s", string(metadataRaw))
	}
	if strings.Contains(string(localEnv), "AGORA_PROJECT_ID=") || strings.Contains(string(localEnv), "AGORA_PROJECT_NAME=") {
		t.Fatalf("did not expect project metadata env vars in python env file: %s", string(localEnv))
	}
	readme, err := os.ReadFile(filepath.Join(boundTarget, "README.md"))
	if err != nil {
		t.Fatalf("expected README in bound scaffold: %v", err)
	}
	if !strings.Contains(string(readme), "Python Quickstart") {
		t.Fatalf("unexpected README contents: %s", string(readme))
	}

	nextjsBoundTarget := filepath.Join(rootDir, "nextjs-demo")
	createNextjsBound := runCLI(t, []string{"quickstart", "create", "nextjs-demo", "--template", "nextjs", "--dir", nextjsBoundTarget, "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":                  configHome,
			"AGORA_API_BASE_URL":               api.baseURL,
			"AGORA_LOG_LEVEL":                  "error",
			"AGORA_QUICKSTART_NEXTJS_REPO_URL": nextjsRepo,
		},
		workdir: rootDir,
	})
	if createNextjsBound.exitCode != 0 || !strings.Contains(createNextjsBound.stdout, `"envStatus":"configured"`) {
		t.Fatalf("unexpected bound nextjs quickstart create result: %+v", createNextjsBound)
	}
	nextjsEnv, err := os.ReadFile(filepath.Join(nextjsBoundTarget, ".env.local"))
	if err != nil {
		t.Fatalf("expected .env.local in bound nextjs scaffold: %v", err)
	}
	if !strings.Contains(string(nextjsEnv), "NEXT_PUBLIC_AGORA_APP_ID=app_123456") || !strings.Contains(string(nextjsEnv), "NEXT_AGORA_APP_CERTIFICATE=") || strings.Contains(string(nextjsEnv), "# Project ID:") || strings.Contains(string(nextjsEnv), "# Project Name:") || strings.Contains(string(nextjsEnv), "BEGIN AGORA CLI QUICKSTART") {
		t.Fatalf("unexpected nextjs .env.local contents: %s", string(nextjsEnv))
	}
	if strings.Contains(string(nextjsEnv), "AGORA_PROJECT_ID=") || strings.Contains(string(nextjsEnv), "AGORA_PROJECT_NAME=") {
		t.Fatalf("did not expect project metadata env vars in nextjs env file: %s", string(nextjsEnv))
	}

	writeNextjsEnv := runCLI(t, []string{"quickstart", "env", "write", nextjsBoundTarget, "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":    configHome,
			"AGORA_API_BASE_URL": api.baseURL,
			"AGORA_LOG_LEVEL":    "error",
		},
		workdir: rootDir,
	})
	if writeNextjsEnv.exitCode != 0 || !strings.Contains(writeNextjsEnv.stdout, `"template":"nextjs"`) {
		t.Fatalf("unexpected quickstart env write result: %+v", writeNextjsEnv)
	}

	writePythonEnv := runCLI(t, []string{"quickstart", "env", "write", boundTarget, "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":    configHome,
			"AGORA_API_BASE_URL": api.baseURL,
			"AGORA_LOG_LEVEL":    "error",
		},
		workdir: rootDir,
	})
	if writePythonEnv.exitCode != 0 || !strings.Contains(writePythonEnv.stdout, `"template":"python"`) {
		t.Fatalf("unexpected python quickstart env write result: %+v", writePythonEnv)
	}

	repoScopedConfig := t.TempDir()
	persistSessionForIntegration(t, repoScopedConfig)
	repoShow := runCLI(t, []string{"project", "show", "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":    repoScopedConfig,
			"AGORA_API_BASE_URL": api.baseURL,
			"AGORA_LOG_LEVEL":    "error",
		},
		workdir: filepath.Join(boundTarget, "server"),
	})
	if repoShow.exitCode != 0 || !strings.Contains(repoShow.stdout, `"projectId":"prj_123456"`) {
		t.Fatalf("expected repo-local .agora binding to resolve project context, got %+v", repoShow)
	}

	goBoundTarget := filepath.Join(rootDir, "go-demo")
	createGoBound := runCLI(t, []string{"quickstart", "create", "go-demo", "--template", "go", "--dir", goBoundTarget, "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":              configHome,
			"AGORA_API_BASE_URL":           api.baseURL,
			"AGORA_LOG_LEVEL":              "error",
			"AGORA_QUICKSTART_GO_REPO_URL": goRepo,
		},
		workdir: rootDir,
	})
	if createGoBound.exitCode != 0 || !strings.Contains(createGoBound.stdout, `"envStatus":"configured"`) {
		t.Fatalf("unexpected bound go quickstart create result: %+v", createGoBound)
	}
	goEnv, err := os.ReadFile(filepath.Join(goBoundTarget, "server", ".env.local"))
	if err != nil {
		t.Fatalf("expected .env.local in bound go scaffold: %v", err)
	}
	if !strings.Contains(string(goEnv), "AGORA_APP_ID=app_123456") || !strings.Contains(string(goEnv), "AGORA_APP_CERTIFICATE=") || !strings.Contains(string(goEnv), "PORT=8080") {
		t.Fatalf("unexpected go .env.local contents: %s", string(goEnv))
	}
	writeGoEnv := runCLI(t, []string{"quickstart", "env", "write", goBoundTarget, "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":    configHome,
			"AGORA_API_BASE_URL": api.baseURL,
			"AGORA_LOG_LEVEL":    "error",
		},
		workdir: rootDir,
	})
	if writeGoEnv.exitCode != 0 || !strings.Contains(writeGoEnv.stdout, `"template":"go"`) || strings.Contains(writeGoEnv.stdout, `"template":"python"`) {
		t.Fatalf("unexpected go quickstart env write result: %+v", writeGoEnv)
	}

	noCertProject := buildFakeProject("No Cert", "prj_nocert", "app_nocert", "global")
	noCertProject.SignKey = nil
	noCertProject.CertificateEnabled = false
	api.projects[noCertProject.ProjectID] = &noCertProject
	rollbackTarget := filepath.Join(rootDir, "rollback-demo")
	createRollback := runCLI(t, []string{"quickstart", "create", "rollback-demo", "--template", "python", "--dir", rollbackTarget, "--project", "prj_nocert", "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":                  configHome,
			"AGORA_API_BASE_URL":               api.baseURL,
			"AGORA_LOG_LEVEL":                  "error",
			"AGORA_QUICKSTART_PYTHON_REPO_URL": pythonRepo,
		},
		workdir: rootDir,
	})
	if createRollback.exitCode != 1 || !strings.Contains(createRollback.stdout, `"ok":false`) || !strings.Contains(createRollback.stdout, `failed to configure quickstart env after clone`) {
		t.Fatalf("unexpected rollback quickstart result: %+v", createRollback)
	}
	if _, err := os.Stat(rollbackTarget); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected rollback target to be removed, got %v", err)
	}
}

func TestCLIQuickstartEnvWriteUsesTargetRepoBindingPrecedence(t *testing.T) {
	configHome := t.TempDir()
	rootDir := t.TempDir()
	api := newFakeCLIBFF()
	defer api.server.Close()

	alpha := buildFakeProject("Project Alpha", "prj_alpha", "app_alpha", "global")
	alpha.FeatureState.RTMEnabled = true
	alpha.FeatureState.ConvoAIEnabled = true
	beta := buildFakeProject("Project Beta", "prj_beta", "app_beta", "global")
	beta.FeatureState.RTMEnabled = true
	beta.FeatureState.ConvoAIEnabled = true
	api.projects[alpha.ProjectID] = &alpha
	api.projects[beta.ProjectID] = &beta
	persistSessionForIntegration(t, configHome)
	if err := saveContext(map[string]string{"XDG_CONFIG_HOME": configHome}, projectContext{
		CurrentProjectID:   &beta.ProjectID,
		CurrentProjectName: &beta.Name,
		CurrentRegion:      "global",
	}); err != nil {
		t.Fatal(err)
	}

	targetDir := filepath.Join(rootDir, "demo-go")
	if err := os.MkdirAll(filepath.Join(targetDir, "server"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "server", ".env.example"), []byte("AGORA_APP_ID=\nAGORA_APP_CERTIFICATE=\nPORT=8080\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "server", "go.mod"), []byte("module agent-quickstart-go/server\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeLocalProjectBinding(targetDir, localProjectBinding{
		ProjectID:   alpha.ProjectID,
		ProjectName: alpha.Name,
		Region:      "global",
		Template:    "go",
		EnvPath:     "server/.env.local",
	}); err != nil {
		t.Fatal(err)
	}

	result := runCLI(t, []string{"quickstart", "env", "write", targetDir, "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":    configHome,
			"AGORA_API_BASE_URL": api.baseURL,
			"AGORA_LOG_LEVEL":    "error",
		},
		workdir: t.TempDir(),
	})
	if result.exitCode != 0 || !strings.Contains(result.stdout, `"projectId":"prj_alpha"`) {
		t.Fatalf("expected repo-local project binding precedence, got %+v", result)
	}
	envRaw, err := os.ReadFile(filepath.Join(targetDir, "server", ".env.local"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(envRaw), "AGORA_APP_ID=app_alpha") || !strings.Contains(string(envRaw), "PORT=8080") || strings.Contains(string(envRaw), "AGORA_APP_ID=app_beta") {
		t.Fatalf("expected target repo binding project app id in env, got %s", string(envRaw))
	}
}

func TestCLIQuickstartEnvWriteMissingBindingEvenWhenEnvExists(t *testing.T) {
	configHome := t.TempDir()
	targetDir := filepath.Join(t.TempDir(), "demo-go")
	if err := os.MkdirAll(filepath.Join(targetDir, "server"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "server", ".env.example"), []byte("AGORA_APP_ID=\nAGORA_APP_CERTIFICATE=\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "server", "go.mod"), []byte("module agent-quickstart-go/server\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(targetDir, "server", ".env.local"), []byte("AGORA_APP_ID=stale\nAGORA_APP_CERTIFICATE=stale\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result := runCLI(t, []string{"quickstart", "env", "write", targetDir, "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME": configHome,
			"AGORA_LOG_LEVEL": "error",
		},
		workdir: t.TempDir(),
	})
	if result.exitCode != 1 || !strings.Contains(result.stdout, `"ok":false`) || !strings.Contains(result.stdout, ".agora/project.json") || !strings.Contains(result.stdout, "--project") || !strings.Contains(result.stdout, "agora project use") {
		t.Fatalf("unexpected missing binding result: %+v", result)
	}
}
