package cli

// Integration test for `agora init` (project + quickstart in one shot).
// Shared helpers live in integration_test.go.

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIInitRTCVideoCallCreatesRTCOnlyProject(t *testing.T) {
	configHome := t.TempDir()
	rootDir := t.TempDir()
	api := newFakeCLIBFF()
	defer api.server.Close()
	persistSessionForIntegration(t, configHome)
	repo := createLocalGitRepo(t, map[string]string{
		"agora.quickstart.json": `{"schemaVersion":1,"template":"nextjs","scenario":"video-call"}`,
		"env.local.example":     "NEXT_PUBLIC_AGORA_APP_ID=\nNEXT_AGORA_APP_CERTIFICATE=\n",
		"package.json":          `{"packageManager":"pnpm@9.15.9","scripts":{"dev":"next dev"}}`,
	})
	target := filepath.Join(rootDir, "rtc-init")
	result := runCLI(t, []string{"init", "rtc-init", "--template", "nextjs", "--scenario", "video-call", "--new-project", "--dir", target, "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":    configHome,
		"AGORA_API_BASE_URL": api.baseURL,
		"AGORA_LOG_LEVEL":    "error",
		"AGORA_QUICKSTART_NEXTJS_VIDEO_CALL_REPO_URL": repo,
	}, workdir: rootDir})
	if result.exitCode != 0 || !strings.Contains(result.stdout, `"enabledFeatures":["rtc"]`) || !strings.Contains(result.stdout, `"scenario":"video-call"`) || !strings.Contains(result.stdout, `"requiredFeatures":["rtc"]`) {
		t.Fatalf("unexpected rtc init result: %+v", result)
	}
	if !strings.Contains(result.stdout, `"packageManager":{"name":"pnpm","requiredVersion":"9.15.9"`) {
		t.Fatalf("rtc init is missing package manager setup metadata: %+v", result)
	}
	if !strings.Contains(result.stdout, `"selectedName":`) {
		t.Fatalf("rtc init is missing selected package manager metadata: %+v", result)
	}
	if strings.Contains(result.stdout, `"rtmDataCenter"`) || strings.Contains(result.stdout, `"convoai"`) {
		t.Fatalf("rtc init enabled unrelated features: %+v", result)
	}
	binding, err := loadLocalProjectBinding(target)
	if err != nil {
		t.Fatal(err)
	}
	if binding.Template != "nextjs" || binding.Scenario != "video-call" {
		t.Fatalf("unexpected rtc init binding: %+v", binding)
	}
}

func TestCLIInitChecksExistingProjectFeaturesBeforeClone(t *testing.T) {
	configHome := t.TempDir()
	rootDir := t.TempDir()
	api := newFakeCLIBFF()
	defer api.server.Close()
	project := buildFakeProject("RTC Only", "prj_rtc_only", "app_rtc_only", "global")
	api.projects[project.ProjectID] = &project
	persistSessionForIntegration(t, configHome)
	target := filepath.Join(rootDir, "must-not-exist")

	result := runCLI(t, []string{"init", "reuse-demo", "--template", "nextjs", "--scenario", "video-call", "--project", project.ProjectID, "--feature", "rtm", "--dir", target, "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":    configHome,
		"AGORA_API_BASE_URL": api.baseURL,
		"AGORA_LOG_LEVEL":    "error",
	}, workdir: rootDir})
	if result.exitCode != 1 || !strings.Contains(result.stdout, `"code":"QUICKSTART_REQUIRED_FEATURE_MISSING"`) || !strings.Contains(result.stdout, "agora project feature enable rtm") {
		t.Fatalf("unexpected missing feature result: %+v", result)
	}
	if _, err := os.Stat(target); !os.IsNotExist(err) {
		t.Fatalf("feature validation must happen before clone, stat err=%v", err)
	}
}

func TestCLIInitCreatesProjectAndQuickstart(t *testing.T) {
	configHome := t.TempDir()
	rootDir := t.TempDir()
	api := newFakeCLIBFF()
	defer api.server.Close()
	persistSessionForIntegration(t, configHome)

	nextjsRepo := createLocalGitRepo(t, map[string]string{
		"README.md":         "# Next.js Quickstart\n",
		"env.local.example": "NEXT_PUBLIC_AGORA_APP_ID=\nNEXT_AGORA_APP_CERTIFICATE=\n",
		"package.json":      `{"name":"nextjs-quickstart"}`,
		"app/page.tsx":      "export default function Page() { return null }\n",
	})

	initResult := runCLI(t, []string{"init", "starter-demo", "--template", "nextjs", "--dir", filepath.Join(rootDir, "starter-demo"), "--rtm-data-center", "ap", "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":                  configHome,
			"AGORA_API_BASE_URL":               api.baseURL,
			"AGORA_LOG_LEVEL":                  "error",
			"AGORA_QUICKSTART_NEXTJS_REPO_URL": nextjsRepo,
		},
		workdir: rootDir,
	})
	if initResult.exitCode != 0 || !strings.Contains(initResult.stdout, `"action":"init"`) || !strings.Contains(initResult.stdout, `"projectAction":"created"`) || !strings.Contains(initResult.stdout, `"template":"nextjs"`) {
		t.Fatalf("unexpected init result: %+v", initResult)
	}
	for _, feature := range []string{`"rtc"`, `"rtm"`, `"convoai"`} {
		if !strings.Contains(initResult.stdout, feature) {
			t.Fatalf("expected default feature %s in init result: %+v", feature, initResult)
		}
	}
	if !strings.Contains(initResult.stdout, `"rtmDataCenter":"AP"`) {
		t.Fatalf("expected RTM data center in init result: %+v", initResult)
	}
	api.mu.Lock()
	initProject := api.projects["prj_0001"]
	api.mu.Unlock()
	if initProject == nil || initProject.FeatureState.RTMRegion != "AP" {
		t.Fatalf("expected init to configure RTM data center AP, got %+v", initProject)
	}
	localEnv, err := os.ReadFile(filepath.Join(rootDir, "starter-demo", ".env.local"))
	if err != nil {
		t.Fatalf("expected init env file: %v", err)
	}
	if !strings.Contains(string(localEnv), "NEXT_PUBLIC_AGORA_APP_ID=app_0001") {
		t.Fatalf("unexpected init env contents: %s", string(localEnv))
	}
	if _, err := os.Stat(filepath.Join(rootDir, "starter-demo", ".agora", "project.json")); err != nil {
		t.Fatalf("expected init to create .agora/project.json: %v", err)
	}
	ctx, err := loadContext(map[string]string{"XDG_CONFIG_HOME": configHome})
	if err != nil {
		t.Fatal(err)
	}
	if ctx.CurrentProjectName == nil || *ctx.CurrentProjectName != "starter-demo" {
		t.Fatalf("expected init to persist current project context, got %+v", ctx)
	}
}

func TestCLIInitRequiresTemplateWhenNoInputIsSet(t *testing.T) {
	result := runCLI(t, []string{"init", "starter-demo", "--yes", "--json"}, cliRunOptions{
		env: map[string]string{
			"AGORA_HOME":      t.TempDir(),
			"AGORA_LOG_LEVEL": "error",
		},
	})
	if result.exitCode != 1 || !strings.Contains(result.stdout, `"code":"INIT_SOURCE_REQUIRED"`) {
		t.Fatalf("expected INIT_SOURCE_REQUIRED, got %+v", result)
	}
}

func TestCLIInitPythonWritesAgoraCredentialNames(t *testing.T) {
	configHome := t.TempDir()
	rootDir := t.TempDir()
	api := newFakeCLIBFF()
	defer api.server.Close()
	persistSessionForIntegration(t, configHome)

	pythonRepo := createLocalGitRepo(t, map[string]string{
		"README.md":               "# Python Quickstart\n",
		"server/.env.example":     "APP_ID=placeholder\nAPP_CERTIFICATE=placeholder\nPORT=8000\n",
		"server/requirements.txt": "",
		"web/package.json":        `{"name":"python-quickstart-web"}`,
	})
	targetDir := filepath.Join(rootDir, "python-demo")

	result := runCLI(t, []string{"init", "python-demo", "--template", "python", "--new-project", "--dir", targetDir, "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":                  configHome,
			"AGORA_API_BASE_URL":               api.baseURL,
			"AGORA_LOG_LEVEL":                  "error",
			"AGORA_QUICKSTART_PYTHON_REPO_URL": pythonRepo,
		},
		workdir: rootDir,
	})
	if result.exitCode != 0 || !strings.Contains(result.stdout, `"template":"python"`) || !strings.Contains(result.stdout, `"envPath":"server/.env"`) {
		t.Fatalf("unexpected Python init result: %+v", result)
	}

	serverEnv, err := os.ReadFile(filepath.Join(targetDir, "server", ".env"))
	if err != nil {
		t.Fatalf("expected Python server env file: %v", err)
	}
	content := string(serverEnv)
	unprefixed := "\n" + content
	if !strings.Contains(content, "AGORA_APP_ID=app_0001") || !strings.Contains(content, "AGORA_APP_CERTIFICATE=4854d28b48a9439c9f2546e2216fc07a") || !strings.Contains(content, "PORT=8000") || strings.Contains(unprefixed, "\nAPP_ID=") || strings.Contains(unprefixed, "\nAPP_CERTIFICATE=") {
		t.Fatalf("unexpected Python server env contents: %s", content)
	}
}

func TestCLIInitCreatesAndroidClientServerQuickstart(t *testing.T) {
	configHome := t.TempDir()
	rootDir := t.TempDir()
	api := newFakeCLIBFF()
	defer api.server.Close()
	persistSessionForIntegration(t, configHome)
	androidRepo := createLocalGitRepo(t, map[string]string{
		"settings.gradle.kts":              "rootProject.name = \"android-quickstart\"\n",
		"gradlew":                          "#!/bin/sh\n",
		"app/src/main/AndroidManifest.xml": "<manifest />\n",
		"server/.env.example":              "AGORA_APP_ID=placeholder\nAGORA_APP_CERTIFICATE=placeholder\nAGORA_AGENT_UID=123456\n",
		"server/requirements-dev.txt":      "fastapi\n",
		"server/run.sh":                    "#!/usr/bin/env bash\n",
	})
	targetDir := filepath.Join(rootDir, "android-demo")

	result := runCLI(t, []string{"init", "android-demo", "--template", "android", "--new-project", "--dir", targetDir, "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":                   configHome,
			"AGORA_API_BASE_URL":                api.baseURL,
			"AGORA_LOG_LEVEL":                   "error",
			"AGORA_QUICKSTART_ANDROID_REPO_URL": androidRepo,
		},
		workdir: rootDir,
	})
	if result.exitCode != 0 || !strings.Contains(result.stdout, `"template":"android"`) || !strings.Contains(result.stdout, `"envPath":"server/.env.local"`) {
		t.Fatalf("unexpected Android init result: %+v", result)
	}

	serverEnv, err := os.ReadFile(filepath.Join(targetDir, "server", ".env.local"))
	if err != nil {
		t.Fatalf("expected Android server env file: %v", err)
	}
	content := string(serverEnv)
	if !strings.Contains(content, "AGORA_APP_ID=app_0001") || !strings.Contains(content, "AGORA_APP_CERTIFICATE=4854d28b48a9439c9f2546e2216fc07a") || !strings.Contains(content, "AGORA_AGENT_UID=123456") {
		t.Fatalf("unexpected Android server env contents: %s", content)
	}
	if _, err := os.Stat(filepath.Join(targetDir, "local.properties")); !os.IsNotExist(err) {
		t.Fatalf("expected init to leave Android local.properties untouched, got %v", err)
	}
}
