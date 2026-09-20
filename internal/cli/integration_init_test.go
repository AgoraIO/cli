package cli

// Integration test for `agora init` (project + quickstart in one shot).
// Shared helpers live in integration_test.go.

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"slices"
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

func TestInitFeatureSelectionCompatibility(t *testing.T) {
	for _, transport := range []string{"cli", "mcp"} {
		for _, tc := range []struct {
			name, scenario, reuse, template string
			features, want                  []string
			invalid                         bool
		}{
			{name: "video defaults", scenario: "video-call", want: []string{"rtc"}},
			{name: "voice defaults", scenario: "voice-agent", want: []string{"rtc", "rtm", "convoai"}},
			{name: "voice explicit rtc", scenario: "voice-agent", features: []string{"rtc"}, want: []string{"rtc"}},
			{name: "video explicit rtm", scenario: "video-call", features: []string{"rtm"}, want: []string{"rtm"}},
			{name: "voice explicit convoai", scenario: "voice-agent", features: []string{"convoai"}, want: []string{"rtm", "convoai"}},
			{name: "reuse voice", scenario: "voice-agent", reuse: "explicit", want: []string{}},
			{name: "reuse voice with features", scenario: "voice-agent", reuse: "explicit", features: []string{"convoai"}, want: []string{}},
			{name: "automatic reuse", scenario: "voice-agent", reuse: "auto", features: []string{"convoai"}, want: []string{}},
			{name: "reuse video", scenario: "video-call", reuse: "explicit", features: []string{"rtm"}, want: []string{}},
			{name: "invalid feature", scenario: "video-call", features: []string{"bad-feature"}, invalid: true},
			{name: "invalid scenario", scenario: "bad-scenario", invalid: true},
			{name: "invalid template", template: "unknown", scenario: "video-call", invalid: true},
			{name: "unsupported combination", template: "python", scenario: "video-call", invalid: true},
		} {
			t.Run(transport+"/"+tc.name, func(t *testing.T) {
				templateID := tc.template
				if templateID == "" {
					templateID = "nextjs"
				}
				root, configHome := t.TempDir(), t.TempDir()
				api := newFakeCLIBFF()
				defer api.server.Close()
				persistSessionForIntegration(t, configHome)
				if tc.reuse != "" {
					project := buildFakeProject("Default Project", "prj_0001", "app_0001", "global")
					api.projects[project.ProjectID] = &project
				}
				repo := createLocalGitRepo(t, map[string]string{
					"agora.quickstart.json": `{"schemaVersion":1,"template":"nextjs","scenario":"` + tc.scenario + `"}`,
					"env.local.example":     "NEXT_PUBLIC_AGORA_APP_ID=\nNEXT_AGORA_APP_CERTIFICATE=\n",
					"package.json":          `{"name":"fixture"}`,
				})
				env := map[string]string{"AGORA_HOME": "", "XDG_CONFIG_HOME": configHome, "AGORA_API_BASE_URL": api.baseURL, "AGORA_LOG_LEVEL": "error", "AGORA_QUICKSTART_NEXTJS_REPO_URL": repo, "AGORA_QUICKSTART_NEXTJS_VIDEO_CALL_REPO_URL": repo}
				target := filepath.Join(root, "demo")
				var data map[string]any
				failed := false
				if transport == "cli" {
					args := []string{"init", "demo", "--template", templateID, "--scenario", tc.scenario, "--dir", target, "--json"}
					switch tc.reuse {
					case "":
						args = append(args, "--new-project")
					case "explicit":
						args = append(args, "--project", "prj_0001")
					}
					for _, f := range tc.features {
						args = append(args, "--feature", f)
					}
					result := runCLI(t, args, cliRunOptions{env: env, workdir: root})
					failed = result.exitCode != 0
					if !failed {
						lines := strings.Split(strings.TrimSpace(result.stdout), "\n")
						var envelope struct {
							Data map[string]any `json:"data"`
						}
						if err := json.Unmarshal([]byte(lines[len(lines)-1]), &envelope); err != nil {
							t.Fatal(err)
						}
						data = envelope.Data
					} else if !tc.invalid {
						t.Fatalf("init failed: %+v", result)
					}
				} else {
					t.Chdir(root)
					for key, value := range env {
						t.Setenv(key, value)
					}
					app, err := NewApp()
					if err != nil {
						t.Fatal(err)
					}
					args := map[string]any{"name": "demo", "template": templateID, "scenario": tc.scenario, "dir": target, "newProject": tc.reuse == "", "features": tc.features}
					if tc.reuse == "explicit" {
						args["project"] = "prj_0001"
					}
					result, err := app.callMCPTool("agora.init", args, nil)
					failed = err != nil
					if !failed {
						data = result.(map[string]any)
					} else if !tc.invalid {
						t.Fatal(err)
					}
				}
				if tc.invalid {
					if !failed {
						t.Fatal("invalid input succeeded")
					}
					api.mu.Lock()
					requests := len(api.requests)
					api.mu.Unlock()
					if requests != 0 {
						t.Fatalf("invalid input reached API: %d requests", requests)
					}
					if _, err := os.Stat(target); !os.IsNotExist(err) {
						t.Fatalf("invalid input created scaffold: %v", err)
					}
					return
				}
				got, err := json.Marshal(data["enabledFeatures"])
				if err != nil {
					t.Fatal(err)
				}
				want, err := json.Marshal(tc.want)
				if err != nil {
					t.Fatal(err)
				}
				if string(got) != string(want) {
					t.Fatalf("features=%s want=%s", got, want)
				}
				api.mu.Lock()
				project := api.projects["prj_0001"]
				rtm, convoai := project.FeatureState.RTMEnabled, project.FeatureState.ConvoAIEnabled
				writes, featureReads := 0, 0
				for _, request := range api.requests {
					if request.Method != http.MethodGet {
						writes++
					}
					if strings.Contains(request.Pathname, "/uap-configs/") || strings.HasSuffix(request.Pathname, "/rtm2-config") {
						featureReads++
					}
				}
				api.mu.Unlock()
				if rtm != slices.Contains(tc.want, "rtm") || convoai != slices.Contains(tc.want, "convoai") {
					t.Fatalf("unexpected API state rtm=%v convoai=%v", rtm, convoai)
				}
				if tc.reuse != "" {
					if writes != 0 || featureReads != 0 {
						t.Fatalf("reuse performed %d writes and %d feature requests", writes, featureReads)
					}
					if data["projectAction"] != "existing" {
						t.Fatalf("unexpected reuse result: %+v", data)
					}
					doctor := runCLI(t, []string{"project", "doctor", "prj_0001", "--feature", "convoai", "--json"}, cliRunOptions{env: env, workdir: root})
					if doctor.exitCode != 1 || !strings.Contains(doctor.stdout, `"status":"not_ready"`) {
						t.Fatalf("doctor failed to report missing functionality: %+v", doctor)
					}
				}
				binding, err := loadLocalProjectBinding(target)
				if err != nil {
					t.Fatal(err)
				}
				if binding.ProjectID != "prj_0001" || binding.Scenario != tc.scenario {
					t.Fatalf("unexpected binding: %+v", binding)
				}
			})
		}
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
