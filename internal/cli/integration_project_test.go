package cli

// Integration tests for `agora project` (env, doctor, use, show, feature).
// Shared helpers live in integration_test.go.

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestCLIProjectEnvAndDoctor(t *testing.T) {
	configHome := t.TempDir()
	projectDir := t.TempDir()
	api := newFakeCLIBFF()
	defer api.server.Close()

	alpha := buildFakeProject("Project Alpha", "prj_123456", "app_123456", "global")
	api.projects[alpha.ProjectID] = &alpha
	persistSessionForIntegration(t, configHome)
	if err := saveContext(map[string]string{"XDG_CONFIG_HOME": configHome}, projectContext{
		CurrentProjectID:   &alpha.ProjectID,
		CurrentProjectName: &alpha.Name,
		CurrentRegion:      "global",
		PreferredRegion:    "global",
	}); err != nil {
		t.Fatal(err)
	}

	envResult := runCLI(t, []string{"project", "env"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":    configHome,
		"AGORA_API_BASE_URL": api.baseURL,
		"AGORA_AGENT":        "cursor-test",
		"AGORA_LOG_LEVEL":    "error",
		"AGORA_DEBUG":        "0",
	}})
	if envResult.exitCode != 0 || !strings.Contains(envResult.stdout, "AGORA_PROJECT_ID=prj_123456") {
		t.Fatalf("unexpected project env result: exit=%d stdout=%s stderr=%s", envResult.exitCode, envResult.stdout, envResult.stderr)
	}
	api.mu.Lock()
	sawAgent := false
	for _, request := range api.requests {
		if strings.Contains(request.UserAgent, "agent/cursor-test") {
			sawAgent = true
			break
		}
	}
	api.mu.Unlock()
	if !sawAgent {
		t.Fatalf("expected AGORA_AGENT to be propagated in User-Agent")
	}

	oldwd, _ := os.Getwd()
	defer func() { _ = os.Chdir(oldwd) }()
	if err := os.Chdir(projectDir); err != nil {
		t.Fatal(err)
	}
	writeResult := runCLI(t, []string{"project", "env", "write", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":    configHome,
		"AGORA_API_BASE_URL": api.baseURL,
		"AGORA_LOG_LEVEL":    "error",
		"AGORA_DEBUG":        "0",
	}, workdir: projectDir})
	if writeResult.exitCode != 0 {
		t.Fatalf("unexpected env write result: exit=%d stderr=%s", writeResult.exitCode, writeResult.stderr)
	}
	if _, err := os.Stat(filepath.Join(projectDir, ".env.local")); err != nil {
		t.Fatalf("expected .env.local to be created: %v", err)
	}

	doctorResult := runCLI(t, []string{"project", "doctor", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":    configHome,
		"AGORA_API_BASE_URL": api.baseURL,
		"AGORA_LOG_LEVEL":    "error",
		"AGORA_DEBUG":        "0",
	}})
	if doctorResult.exitCode != 1 {
		t.Fatalf("expected doctor exit 1, got %d stdout=%s stderr=%s", doctorResult.exitCode, doctorResult.stdout, doctorResult.stderr)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(doctorResult.stdout), &envelope); err != nil {
		t.Fatal(err)
	}
	data := envelope["data"].(map[string]any)
	if envelope["ok"] != false {
		t.Fatalf("expected doctor failure envelope ok=false, got %s", doctorResult.stdout)
	}
	meta := envelope["meta"].(map[string]any)
	if meta["exitCode"] != float64(1) {
		t.Fatalf("expected doctor meta.exitCode=1, got %s", doctorResult.stdout)
	}
	if data["status"] != "not_ready" {
		t.Fatalf("expected not_ready doctor result, got %s", doctorResult.stdout)
	}
}

func TestCLIProjectCreateDefaultsToCoreFeatures(t *testing.T) {
	configHome := t.TempDir()
	api := newFakeCLIBFF()
	defer api.server.Close()
	persistSessionForIntegration(t, configHome)

	dryRun := runCLI(t, []string{"project", "create", "Project Dry Run", "--dry-run", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":    configHome,
		"AGORA_API_BASE_URL": api.baseURL,
		"AGORA_LOG_LEVEL":    "error",
	}})
	if dryRun.exitCode != 0 {
		t.Fatalf("unexpected dry-run result: %+v", dryRun)
	}
	for _, feature := range []string{`"rtc"`, `"rtm"`, `"convoai"`} {
		if !strings.Contains(dryRun.stdout, feature) {
			t.Fatalf("expected default feature %s in dry-run result: %+v", feature, dryRun)
		}
	}

	create := runCLI(t, []string{"project", "create", "Project Gamma", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":    configHome,
		"AGORA_API_BASE_URL": api.baseURL,
		"AGORA_LOG_LEVEL":    "error",
	}})
	if create.exitCode != 0 {
		t.Fatalf("unexpected create result: %+v", create)
	}
	if !strings.Contains(create.stdout, `"rtmDataCenter":"NA"`) {
		t.Fatalf("expected default RTM data center in create result: %+v", create)
	}
	for _, feature := range []string{`"rtc"`, `"rtm"`, `"convoai"`} {
		if !strings.Contains(create.stdout, feature) {
			t.Fatalf("expected default feature %s in create result: %+v", feature, create)
		}
	}
	api.mu.Lock()
	defaultProject := api.projects["prj_0001"]
	api.mu.Unlock()
	if defaultProject == nil || defaultProject.FeatureState.RTMRegion != "NA" {
		t.Fatalf("expected omitted RTM data center to default to NA, got %+v", defaultProject)
	}

	withDataCenter := runCLI(t, []string{"project", "create", "Project Delta", "--rtm-data-center", "eu", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":    configHome,
		"AGORA_API_BASE_URL": api.baseURL,
		"AGORA_LOG_LEVEL":    "error",
	}})
	if withDataCenter.exitCode != 0 || !strings.Contains(withDataCenter.stdout, `"rtmDataCenter":"EU"`) {
		t.Fatalf("unexpected create with data center result: %+v", withDataCenter)
	}
	api.mu.Lock()
	dataCenterProject := api.projects["prj_0002"]
	api.mu.Unlock()
	if dataCenterProject == nil || dataCenterProject.FeatureState.RTMRegion != "EU" {
		t.Fatalf("expected RTM data center EU, got %+v", dataCenterProject)
	}

	rtcOnly := runCLI(t, []string{"project", "create", "Project RTC Only", "--feature", "rtc", "--dry-run", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":    configHome,
		"AGORA_API_BASE_URL": api.baseURL,
		"AGORA_LOG_LEVEL":    "error",
	}})
	if rtcOnly.exitCode != 0 || strings.Contains(rtcOnly.stdout, `"rtmDataCenter"`) || strings.Contains(rtcOnly.stdout, `"rtm"`) {
		t.Fatalf("unexpected rtc-only dry-run result: %+v", rtcOnly)
	}

	convoAIOnly := runCLI(t, []string{"project", "create", "Project ConvoAI", "--feature", "convoai", "--dry-run", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":    configHome,
		"AGORA_API_BASE_URL": api.baseURL,
		"AGORA_LOG_LEVEL":    "error",
	}})
	if convoAIOnly.exitCode != 0 || !strings.Contains(convoAIOnly.stdout, `"convoai"`) || !strings.Contains(convoAIOnly.stdout, `"rtm"`) || !strings.Contains(convoAIOnly.stdout, `"rtmDataCenter":"NA"`) {
		t.Fatalf("expected convoai dry-run to include rtm dependency: %+v", convoAIOnly)
	}
}

func TestCLIProjectUseShowFeatureAndDoctorHappyPath(t *testing.T) {
	configHome := t.TempDir()
	api := newFakeCLIBFF()
	defer api.server.Close()

	alpha := buildFakeProject("Project Alpha", "prj_123456", "app_123456", "global")
	beta := buildFakeProject("Project Beta", "prj_9999", "app_9999", "cn")
	beta.FeatureState.ConvoAIEnabled = true
	beta.FeatureState.RTMEnabled = true
	api.projects[alpha.ProjectID] = &alpha
	api.projects[beta.ProjectID] = &beta
	persistSessionForIntegration(t, configHome)

	useResult := runCLI(t, []string{"project", "use", "Project Beta", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":    configHome,
		"AGORA_API_BASE_URL": api.baseURL,
		"AGORA_LOG_LEVEL":    "error",
	}})
	if useResult.exitCode != 0 || !strings.Contains(useResult.stdout, `"projectId":"prj_9999"`) {
		t.Fatalf("unexpected use result: %+v", useResult)
	}

	showPretty := runCLI(t, []string{"project", "show", "--output", "pretty"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":    configHome,
		"AGORA_API_BASE_URL": api.baseURL,
		"AGORA_LOG_LEVEL":    "error",
		"AGORA_OUTPUT":       "pretty",
	}})
	if showPretty.exitCode != 0 || !strings.Contains(showPretty.stdout, "App Certificate") || !strings.Contains(showPretty.stdout, "Region") || !strings.Contains(showPretty.stdout, "[hidden]") || strings.Contains(showPretty.stdout, "4854d28b48a9439c9f2546e2216fc07a") {
		t.Fatalf("unexpected pretty show output (cert must be [hidden]): %+v", showPretty)
	}

	featureStatus := runCLI(t, []string{"project", "feature", "status", "convoai", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":    configHome,
		"AGORA_API_BASE_URL": api.baseURL,
		"AGORA_LOG_LEVEL":    "error",
	}})
	if featureStatus.exitCode != 0 || !strings.Contains(featureStatus.stdout, `"status":"enabled"`) {
		t.Fatalf("unexpected feature status: %+v", featureStatus)
	}

	doctor := runCLI(t, []string{"project", "doctor", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":    configHome,
		"AGORA_API_BASE_URL": api.baseURL,
		"AGORA_LOG_LEVEL":    "error",
	}})
	if doctor.exitCode != 0 || !strings.Contains(doctor.stdout, `"status":"healthy"`) {
		t.Fatalf("unexpected doctor result: %+v", doctor)
	}

	rtmDoctor := runCLI(t, []string{"project", "doctor", "--feature", "rtm", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":    configHome,
		"AGORA_API_BASE_URL": api.baseURL,
		"AGORA_LOG_LEVEL":    "error",
	}})
	if rtmDoctor.exitCode != 0 || !strings.Contains(rtmDoctor.stdout, `"feature":"rtm"`) || !strings.Contains(rtmDoctor.stdout, `"status":"healthy"`) {
		t.Fatalf("unexpected rtm doctor result: %+v", rtmDoctor)
	}
}

func TestCLIProjectDoctorDeepDetectsWorkspaceDrift(t *testing.T) {
	configHome := t.TempDir()
	api := newFakeCLIBFF()
	defer api.server.Close()

	project := buildFakeProject("Project Alpha", "prj_123456", "app_123456", "global")
	project.FeatureState.RTMEnabled = true
	project.FeatureState.ConvoAIEnabled = true
	api.projects[project.ProjectID] = &project
	persistSessionForIntegration(t, configHome)
	if err := saveContext(map[string]string{"XDG_CONFIG_HOME": configHome}, projectContext{
		CurrentProjectID:   &project.ProjectID,
		CurrentProjectName: &project.Name,
		CurrentRegion:      "global",
		PreferredRegion:    "global",
	}); err != nil {
		t.Fatal(err)
	}

	repoRoot := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repoRoot, "server-go"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeLocalProjectBinding(repoRoot, localProjectBinding{
		ProjectID:   project.ProjectID,
		ProjectName: project.Name,
		Region:      "global",
		Template:    "go",
		EnvPath:     "server-go/.env",
	}); err != nil {
		t.Fatal(err)
	}
	mismatched := strings.Join([]string{
		"# BEGIN AGORA CLI QUICKSTART",
		"# Project ID: prj_other",
		"# Project Name: Project Other",
		"APP_ID=app_other",
		"APP_CERTIFICATE=other",
		"# END AGORA CLI QUICKSTART",
		"",
	}, "\n")
	if err := os.WriteFile(filepath.Join(repoRoot, "server-go", ".env"), []byte(mismatched), 0o644); err != nil {
		t.Fatal(err)
	}

	doctor := runCLI(t, []string{"project", "doctor", "--deep", "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":    configHome,
			"AGORA_API_BASE_URL": api.baseURL,
			"AGORA_LOG_LEVEL":    "error",
		},
		workdir: repoRoot,
	})
	if doctor.exitCode != 1 || !strings.Contains(doctor.stdout, `"mode":"deep"`) || !strings.Contains(doctor.stdout, `"status":"not_ready"`) || !strings.Contains(doctor.stdout, `"category":"workspace"`) || !strings.Contains(doctor.stdout, `"code":"WORKSPACE_ENV_APP_ID_MISMATCH"`) {
		t.Fatalf("unexpected deep doctor mismatch result: %+v", doctor)
	}
	if !strings.Contains(doctor.stdout, `"workspace":`) || !strings.Contains(doctor.stdout, `"envAppID":"app_other"`) {
		t.Fatalf("expected deep doctor workspace details, got %+v", doctor)
	}
}

func TestCLIProjectEnvFormatsAndWriteRules(t *testing.T) {
	configHome := t.TempDir()
	projectDir := t.TempDir()
	api := newFakeCLIBFF()
	defer api.server.Close()

	project := buildFakeProject("Project Beta", "prj_9999", "app_9999", "global")
	project.FeatureState.ConvoAIEnabled = true
	project.FeatureState.RTMEnabled = true
	api.projects[project.ProjectID] = &project
	persistSessionForIntegration(t, configHome)
	if err := saveContext(map[string]string{"XDG_CONFIG_HOME": configHome}, projectContext{
		CurrentProjectID:   &project.ProjectID,
		CurrentProjectName: &project.Name,
		CurrentRegion:      "global",
		PreferredRegion:    "global",
	}); err != nil {
		t.Fatal(err)
	}

	shellResult := runCLI(t, []string{"project", "env", "--shell", "--with-secrets"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":    configHome,
			"AGORA_API_BASE_URL": api.baseURL,
			"AGORA_LOG_LEVEL":    "error",
		},
		workdir: projectDir,
	})
	if shellResult.exitCode != 0 || !strings.Contains(shellResult.stdout, "export AGORA_APP_CERTIFICATE=") {
		t.Fatalf("unexpected shell env result: %+v", shellResult)
	}

	jsonResult := runCLI(t, []string{"project", "env", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":    configHome,
		"AGORA_API_BASE_URL": api.baseURL,
		"AGORA_LOG_LEVEL":    "error",
	}})
	if jsonResult.exitCode != 0 || !strings.Contains(jsonResult.stdout, `"command":"project env"`) || !strings.Contains(jsonResult.stdout, `"AGORA_FEATURE_CONVOAI":true`) {
		t.Fatalf("unexpected json env result: %+v", jsonResult)
	}

	if err := os.WriteFile(filepath.Join(projectDir, ".env.custom"), []byte("FOO=bar\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	explicitConflict := runCLI(t, []string{"project", "env", "write", ".env.custom"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":    configHome,
			"AGORA_API_BASE_URL": api.baseURL,
			"AGORA_LOG_LEVEL":    "error",
		},
		workdir: projectDir,
	})
	if explicitConflict.exitCode != 1 || !strings.Contains(explicitConflict.stderr, "--append") {
		t.Fatalf("unexpected explicit write conflict: %+v", explicitConflict)
	}

	explicitConflictJSON := runCLI(t, []string{"project", "env", "write", ".env.custom", "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":    configHome,
			"AGORA_API_BASE_URL": api.baseURL,
			"AGORA_LOG_LEVEL":    "error",
		},
		workdir: projectDir,
	})
	if explicitConflictJSON.exitCode != 1 || !strings.Contains(explicitConflictJSON.stdout, `"ok":false`) || !strings.Contains(explicitConflictJSON.stdout, `"command":"project env write"`) || !strings.Contains(explicitConflictJSON.stdout, `--append`) || explicitConflictJSON.stderr != "" {
		t.Fatalf("unexpected explicit write conflict json result: %+v", explicitConflictJSON)
	}

	appendResult := runCLI(t, []string{"project", "env", "write", "--append", "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":    configHome,
			"AGORA_API_BASE_URL": api.baseURL,
			"AGORA_LOG_LEVEL":    "error",
		},
		workdir: projectDir,
	})
	if appendResult.exitCode != 0 {
		t.Fatalf("unexpected append result: %+v", appendResult)
	}

	if err := os.MkdirAll(filepath.Join(projectDir, "apps", "web"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "apps", "web", "package.json"), []byte(`{"dependencies":{"next":"15.0.0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	nestedResult := runCLI(t, []string{"project", "env", "write", "apps/web/.env.local", "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":    configHome,
			"AGORA_API_BASE_URL": api.baseURL,
			"AGORA_LOG_LEVEL":    "error",
		},
		workdir: projectDir,
	})
	if nestedResult.exitCode != 0 {
		t.Fatalf("unexpected nested write result: %+v", nestedResult)
	}
	if _, err := os.Stat(filepath.Join(projectDir, "apps", "web", ".env.local")); err != nil {
		t.Fatalf("expected nested env file, got %v", err)
	}
	nestedEnv, err := os.ReadFile(filepath.Join(projectDir, "apps", "web", ".env.local"))
	if err != nil {
		t.Fatal(err)
	}
	nextLegacyKeys := regexp.MustCompile(`(?m)^\s*(?:export\s+)?AGORA_APP_ID=|^\s*(?:export\s+)?AGORA_APP_CERTIFICATE=`)
	if !strings.Contains(string(nestedEnv), "NEXT_PUBLIC_AGORA_APP_ID=") || !strings.Contains(string(nestedEnv), "NEXT_AGORA_APP_CERTIFICATE=") ||
		nextLegacyKeys.MatchString(string(nestedEnv)) || strings.Contains(string(nestedEnv), "AGORA_PROJECT_ID=") || strings.Contains(string(nestedEnv), "# BEGIN AGORA CLI") {
		t.Fatalf("unexpected nested env contents (expected Next.js credential names): %s", string(nestedEnv))
	}

	explicitDefaultPath := filepath.Join(projectDir, ".env.local")
	if err := os.WriteFile(explicitDefaultPath, []byte("USER_VALUE=keep\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	explicitDefault := runCLI(t, []string{"project", "env", "write", ".env.local", "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":    configHome,
			"AGORA_API_BASE_URL": api.baseURL,
			"AGORA_LOG_LEVEL":    "error",
		},
		workdir: projectDir,
	})
	if explicitDefault.exitCode != 0 || !strings.Contains(explicitDefault.stdout, `"status":"appended"`) {
		t.Fatalf("unexpected explicit .env.local write result: %+v", explicitDefault)
	}
	defaultEnv, err := os.ReadFile(explicitDefaultPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(defaultEnv), "USER_VALUE=keep") || !strings.Contains(string(defaultEnv), "AGORA_APP_ID=app_9999") || !strings.Contains(string(defaultEnv), "AGORA_APP_CERTIFICATE=") || strings.Contains(string(defaultEnv), "AGORA_PROJECT_ID=") || strings.Contains(string(defaultEnv), "# BEGIN AGORA CLI") {
		t.Fatalf("unexpected explicit .env.local contents: %s", string(defaultEnv))
	}
}

func TestCLIProjectEnvWriteRecordsProjectTypeInBinding(t *testing.T) {
	configHome := t.TempDir()
	repoRoot := t.TempDir()
	api := newFakeCLIBFF()
	defer api.server.Close()

	project := buildFakeProject("Project Gamma", "prj_bindmeta", "app_bindmeta", "global")
	project.FeatureState.ConvoAIEnabled = true
	project.FeatureState.RTMEnabled = true
	api.projects[project.ProjectID] = &project
	persistSessionForIntegration(t, configHome)
	if err := saveContext(map[string]string{"XDG_CONFIG_HOME": configHome}, projectContext{
		CurrentProjectID:   &project.ProjectID,
		CurrentProjectName: &project.Name,
		CurrentRegion:      "global",
		PreferredRegion:    "global",
	}); err != nil {
		t.Fatal(err)
	}

	if err := writeLocalProjectBinding(repoRoot, localProjectBinding{
		ProjectID:   project.ProjectID,
		ProjectName: project.Name,
		Region:      "global",
	}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repoRoot, "package.json"), []byte(`{"dependencies":{"next":"15.0.0"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	result := runCLI(t, []string{"project", "env", "write", ".env.local", "--json"}, cliRunOptions{
		env: map[string]string{
			"XDG_CONFIG_HOME":    configHome,
			"AGORA_API_BASE_URL": api.baseURL,
			"AGORA_LOG_LEVEL":    "error",
		},
		workdir: repoRoot,
	})
	if result.exitCode != 0 || !strings.Contains(result.stdout, `"metadataUpdated":true`) || !strings.Contains(result.stdout, `"metadataPath":".agora/project.json"`) {
		t.Fatalf("expected metadata update in result: %+v", result)
	}

	raw, err := os.ReadFile(filepath.Join(repoRoot, ".agora", "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	var meta map[string]any
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatal(err)
	}
	if meta["projectType"] != "nextjs" {
		t.Fatalf("expected projectType nextjs in binding, got %s", string(raw))
	}
	if meta["envPath"] != ".env.local" {
		t.Fatalf("expected envPath .env.local, got %s", string(raw))
	}
}

func TestCLIFeatureEnableAndDoctorAuthError(t *testing.T) {
	configHome := t.TempDir()
	api := newFakeCLIBFF()
	defer api.server.Close()

	project := buildFakeProject("Project Alpha", "prj_123456", "app_123456", "global")
	api.projects[project.ProjectID] = &project
	persistSessionForIntegration(t, configHome)
	if err := saveContext(map[string]string{"XDG_CONFIG_HOME": configHome}, projectContext{
		CurrentProjectID:   &project.ProjectID,
		CurrentProjectName: &project.Name,
		CurrentRegion:      "global",
		PreferredRegion:    "global",
	}); err != nil {
		t.Fatal(err)
	}

	enable := runCLI(t, []string{"project", "feature", "enable", "convoai", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME":    configHome,
		"AGORA_API_BASE_URL": api.baseURL,
		"AGORA_LOG_LEVEL":    "error",
	}})
	if enable.exitCode != 0 || !strings.Contains(enable.stdout, `"status":"enabled"`) {
		t.Fatalf("unexpected feature enable result: %+v", enable)
	}

	unauthDoctor := runCLI(t, []string{"project", "doctor", "--deep", "--json"}, cliRunOptions{env: map[string]string{
		"XDG_CONFIG_HOME": t.TempDir(),
		"AGORA_LOG_LEVEL": "error",
	}})
	if unauthDoctor.exitCode != 3 || !strings.Contains(unauthDoctor.stdout, `"ok":false`) || !strings.Contains(unauthDoctor.stdout, `"code":"AUTH_UNAUTHENTICATED"`) || !strings.Contains(unauthDoctor.stdout, `"status":"auth_error"`) || !strings.Contains(unauthDoctor.stdout, `"mode":"deep"`) {
		t.Fatalf("unexpected unauth doctor result: %+v", unauthDoctor)
	}
}
