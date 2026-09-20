package cli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestResolveQuickstartEnvWriteTargetSupportsCurrentAndLegacyLayouts(t *testing.T) {
	tests := []struct {
		name               string
		files              map[string]string
		wantTemplate       string
		wantEnvPath        string
		wantAppIDKey       string
		wantCertificateKey string
	}{
		{
			name: "current python",
			files: map[string]string{
				"server/requirements.txt": "",
				"web/package.json":        "{}",
			},
			wantTemplate:       "python",
			wantEnvPath:        "server/.env",
			wantAppIDKey:       "AGORA_APP_ID",
			wantCertificateKey: "AGORA_APP_CERTIFICATE",
		},
		{
			name: "legacy python",
			files: map[string]string{
				"server/env.example":      "APP_ID=\nAPP_CERTIFICATE=\n",
				"web-client/package.json": "{}",
			},
			wantTemplate:       "python",
			wantEnvPath:        "server/.env",
			wantAppIDKey:       "AGORA_APP_ID",
			wantCertificateKey: "AGORA_APP_CERTIFICATE",
		},
		{
			name: "current go",
			files: map[string]string{
				"server/go.mod":       "module example/server",
				"client/package.json": "{}",
			},
			wantTemplate:       "go",
			wantEnvPath:        "server/.env",
			wantAppIDKey:       "AGORA_APP_ID",
			wantCertificateKey: "AGORA_APP_CERTIFICATE",
		},
		{
			name: "legacy go is not python",
			files: map[string]string{
				"server-go/env.example":   "APP_ID=\nAPP_CERTIFICATE=\n",
				"web-client/package.json": "{}",
			},
			wantTemplate:       "go",
			wantEnvPath:        "server-go/.env",
			wantAppIDKey:       "AGORA_APP_ID",
			wantCertificateKey: "AGORA_APP_CERTIFICATE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			for path, content := range tt.files {
				filePath := filepath.Join(root, filepath.FromSlash(path))
				if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filePath, []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}

			template, layout, err := resolveQuickstartEnvWriteTarget(root, "", "")
			if err != nil {
				t.Fatal(err)
			}
			if template.ID != tt.wantTemplate || layout.EnvTargetPath != tt.wantEnvPath || layout.AppIDKey != tt.wantAppIDKey || layout.AppCertificateKey != tt.wantCertificateKey {
				t.Fatalf("resolved template=%q layout=%+v, want template=%q envPath=%q appIDKey=%q certificateKey=%q", template.ID, layout, tt.wantTemplate, tt.wantEnvPath, tt.wantAppIDKey, tt.wantCertificateKey)
			}
		})
	}
}

func TestResolveQuickstartEnvWriteTargetPreservesBoundLegacyLayout(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "server"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "server", "go.mod"), []byte("module example/server\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := writeLocalProjectBinding(root, localProjectBinding{Template: "go", EnvPath: "server-go/.env"}); err != nil {
		t.Fatal(err)
	}

	template, layout, err := resolveQuickstartEnvWriteTarget(root, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if template.ID != "go" || layout.EnvTargetPath != "server-go/.env" || layout.AppIDKey != "AGORA_APP_ID" {
		t.Fatalf("expected bound legacy go layout, got template=%q layout=%+v", template.ID, layout)
	}
}

func TestSeedQuickstartEnvNormalizesLegacyCredentialNames(t *testing.T) {
	template, ok := findQuickstartTemplate("go")
	if !ok {
		t.Fatal("go template not found")
	}
	layout, ok := quickstartEnvLayoutForEnvPath(*template, "server-go/.env")
	if !ok {
		t.Fatal("legacy go layout not found")
	}
	certificate := "cert_123"
	project := projectDetail{AppID: "app_123", SignKey: &certificate}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "server-go"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "server-go", "env.example"), []byte("APP_ID=\nAPP_CERTIFICATE=\nPORT=8080\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	envPath, _, err := seedQuickstartEnv(root, *template, layout, project)
	if err != nil {
		t.Fatal(err)
	}
	if envPath != "server-go/.env" {
		t.Fatalf("env path = %q, want server-go/.env", envPath)
	}
	raw, err := os.ReadFile(filepath.Join(root, "server-go", ".env"))
	if err != nil {
		t.Fatal(err)
	}
	content := string(raw)
	legacyContent := "\n" + content
	if !strings.Contains(content, "AGORA_APP_ID=app_123") || !strings.Contains(content, "AGORA_APP_CERTIFICATE=cert_123") || strings.Contains(legacyContent, "\nAPP_ID=") || strings.Contains(legacyContent, "\nAPP_CERTIFICATE=") || !strings.Contains(content, "PORT=8080") {
		t.Fatalf("unexpected legacy env contents: %s", content)
	}
}

func TestChooseQuickstartProject(t *testing.T) {
	items := []projectSummary{
		{ProjectID: "prj_old", Name: "Older", CreatedAt: "2026-04-01T10:00:00Z"},
		{ProjectID: "prj_new", Name: "Newest", CreatedAt: "2026-04-12T10:00:00Z"},
	}

	t.Run("blank selects most recent project", func(t *testing.T) {
		var out strings.Builder
		project, action, err := chooseQuickstartProject(strings.NewReader("\n"), &out, items)
		if err != nil || action != "project" || project.ProjectID != "prj_new" {
			t.Fatalf("unexpected choice: action=%q project=%+v err=%v", action, project, err)
		}
	})

	t.Run("template-only is explicit", func(t *testing.T) {
		var out strings.Builder
		_, action, err := chooseQuickstartProject(strings.NewReader("template-only\n"), &out, items)
		if err != nil || action != "template-only" {
			t.Fatalf("unexpected choice: action=%q err=%v", action, err)
		}
	})

	t.Run("number selects existing project", func(t *testing.T) {
		var out strings.Builder
		project, action, err := chooseQuickstartProject(strings.NewReader("1\n"), &out, items)
		if err != nil || action != "project" || project.ProjectID != "prj_old" {
			t.Fatalf("unexpected choice: action=%q project=%+v err=%v", action, project, err)
		}
	})

	t.Run("cancel aborts creation", func(t *testing.T) {
		var out strings.Builder
		_, action, err := chooseQuickstartProject(strings.NewReader("cancel\n"), &out, items)
		if err != nil || action != "abort" {
			t.Fatalf("unexpected choice: action=%q err=%v", action, err)
		}
	})

	t.Run("empty account has no selection", func(t *testing.T) {
		var out strings.Builder
		_, action, err := chooseQuickstartProject(strings.NewReader("\n"), &out, nil)
		if err != nil || action != "none" {
			t.Fatalf("unexpected choice: action=%q err=%v", action, err)
		}
	})
}

func TestGoVoiceAgentSkillUsesQuickstartWorkflow(t *testing.T) {
	for _, skill := range skillsCatalog() {
		if skill.ID != "create-go-voice-agent" {
			continue
		}
		if len(skill.Steps) != 3 || skill.Steps[2] != "cd my-go-voice-agent && make setup && make dev" {
			t.Fatalf("unexpected Go voice agent steps: %#v", skill.Steps)
		}
		return
	}
	t.Fatal("Go voice agent skill not found")
}

func TestScaffoldSkillsUseConsistentDiscoveryTags(t *testing.T) {
	wantTags := map[string][]string{
		"create-nextjs-video-app":   {"nextjs", "rtc", "video", "video-call", "init"},
		"create-nextjs-voice-agent": {"nextjs", "convoai", "voice", "voice-agent", "init"},
		"create-python-voice-agent": {"python", "convoai", "voice", "voice-agent", "init"},
		"create-go-voice-agent":     {"go", "convoai", "voice", "voice-agent", "init"},
	}

	for _, skill := range skillsCatalog() {
		want, ok := wantTags[skill.ID]
		if !ok {
			continue
		}
		if !reflect.DeepEqual(skill.Tags, want) {
			t.Fatalf("unexpected tags for %s:\n got: %#v\nwant: %#v", skill.ID, skill.Tags, want)
		}
		delete(wantTags, skill.ID)
	}
	if len(wantTags) != 0 {
		t.Fatalf("missing scaffold skills: %#v", wantTags)
	}
}

func TestNextJSVideoAppSkillUsesVideoCallQuickstartWorkflow(t *testing.T) {
	for _, skill := range skillsCatalog() {
		if skill.ID != "create-nextjs-video-app" {
			continue
		}
		wantSteps := []string{
			"agora login",
			"agora init my-nextjs-demo --template nextjs --scenario video-call --new-project --json",
			"Run the nextSteps returned by agora init exactly; they select exact pnpm or the native npm fallback for this machine.",
		}
		if !reflect.DeepEqual(skill.Steps, wantSteps) {
			t.Fatalf("unexpected Next.js video app steps:\n got: %#v\nwant: %#v", skill.Steps, wantSteps)
		}
		if skill.Description != "Create a runnable Next.js one-to-one RTC audio and video call." || !slices.Contains(skill.Tags, "video-call") {
			t.Fatalf("unexpected Next.js video app discovery metadata: description=%q tags=%#v", skill.Description, skill.Tags)
		}
		if !slices.Contains(skill.NextSteps, "Run agora project doctor --feature rtc --deep --json to validate the project, manifest, binding, and env.") {
			t.Fatalf("Next.js video app skill is missing RTC deep doctor guidance: %#v", skill.NextSteps)
		}
		return
	}
	t.Fatal("Next.js video app skill not found")
}

func TestNextJSVoiceAgentSkillUsesVoiceAgentQuickstartWorkflow(t *testing.T) {
	for _, skill := range skillsCatalog() {
		if skill.ID != "create-nextjs-voice-agent" {
			continue
		}
		wantSteps := []string{
			"agora login",
			"agora init my-nextjs-voice-agent --template nextjs --scenario voice-agent --new-project --json",
			"cd my-nextjs-voice-agent && pnpm install && pnpm dev",
		}
		if !reflect.DeepEqual(skill.Steps, wantSteps) {
			t.Fatalf("unexpected Next.js voice agent steps:\n got: %#v\nwant: %#v", skill.Steps, wantSteps)
		}
		if skill.Description != "Create a runnable Next.js conversational AI voice agent." || !slices.Contains(skill.Tags, "voice-agent") {
			t.Fatalf("unexpected Next.js voice agent discovery metadata: description=%q tags=%#v", skill.Description, skill.Tags)
		}
		if !slices.Contains(skill.NextSteps, "Run agora project doctor --feature convoai --deep --json to validate project and quickstart readiness.") {
			t.Fatalf("Next.js voice agent skill is missing ConvoAI deep doctor guidance: %#v", skill.NextSteps)
		}
		return
	}
	t.Fatal("Next.js voice agent skill not found")
}

func TestGitQuickstartCloneArgs(t *testing.T) {
	args := gitQuickstartCloneArgs("https://github.com/AgoraIO/example", "/tmp/example", "")
	want := []string{"-c", "credential.helper=", "clone", "--depth", "1", "--", "https://github.com/AgoraIO/example", "/tmp/example"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("unexpected clone args:\n got: %#v\nwant: %#v", args, want)
	}

	args = gitQuickstartCloneArgs("https://github.com/AgoraIO/example", "/tmp/example", " release/v1 ")
	want = []string{"-c", "credential.helper=", "clone", "--depth", "1", "--branch", "release/v1", "--", "https://github.com/AgoraIO/example", "/tmp/example"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("unexpected clone args with ref:\n got: %#v\nwant: %#v", args, want)
	}

	args = gitQuickstartCloneArgs("-https://evil.example/repo", "/tmp/example", "")
	want = []string{"-c", "credential.helper=", "clone", "--depth", "1", "--", "-https://evil.example/repo", "/tmp/example"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("unexpected clone args with dash-prefixed url:\n got: %#v\nwant: %#v", args, want)
	}
}

func TestStripClonedGitMetadata(t *testing.T) {
	repo := createLocalGitRepo(t, map[string]string{
		"README.md": "# Quickstart\n",
	})
	target := filepath.Join(t.TempDir(), "quickstart")
	if err := cloneQuickstartRepo(repo, target, ""); err != nil {
		t.Fatalf("cloneQuickstartRepo failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, ".git")); err != nil {
		t.Fatalf("expected cloned git repo before strip: %v", err)
	}
	if err := stripClonedGitMetadata(target); err != nil {
		t.Fatalf("stripClonedGitMetadata failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, ".git")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected .git removed after strip, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "README.md")); err != nil {
		t.Fatalf("expected README to remain after strip: %v", err)
	}
}

func TestCloneQuickstartRepoLocal(t *testing.T) {
	repo := createLocalGitRepo(t, map[string]string{
		"README.md": "# Quickstart\n",
	})
	target := filepath.Join(t.TempDir(), "quickstart")

	if err := cloneQuickstartRepo(repo, target, ""); err != nil {
		t.Fatalf("cloneQuickstartRepo failed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, ".git")); err != nil {
		t.Fatalf("expected cloned git repo: %v", err)
	}
}

func TestCloneQuickstartRepoRejectsBadRef(t *testing.T) {
	target := filepath.Join(t.TempDir(), "quickstart")
	err := cloneQuickstartRepo("https://example.invalid/repo.git", target, "-fexploit")
	if err == nil {
		t.Fatal("expected error for dash-prefixed ref, got nil")
	}
	var cliErr *cliError
	if !errors.As(err, &cliErr) {
		t.Fatalf("expected *cliError, got %T: %v", err, err)
	}
	if cliErr.Code != "QUICKSTART_REF_INVALID" {
		t.Fatalf("expected code QUICKSTART_REF_INVALID, got %q", cliErr.Code)
	}
	if _, statErr := os.Stat(target); !os.IsNotExist(statErr) {
		t.Fatalf("expected target not created on validation failure, stat err = %v", statErr)
	}
}

func TestValidateGitRef(t *testing.T) {
	cases := []struct {
		name string
		ref  string
		ok   bool
	}{
		{"empty allowed", "", true},
		{"whitespace allowed (treated as empty)", "  ", true},
		{"normal branch", "main", true},
		{"slash branch", "release/v1", true},
		{"tag with dots", "v1.2.3", true},
		{"leading dash rejected", "-fexploit", false},
		{"embedded space rejected", "release v1", false},
		{"embedded tab rejected", "release\tv1", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateGitRef(tc.ref)
			if tc.ok && err != nil {
				t.Fatalf("expected ref %q to be valid, got error: %v", tc.ref, err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("expected ref %q to be rejected", tc.ref)
			}
		})
	}
}

func TestValidateRepoOverrideURL(t *testing.T) {
	localAbsPath := filepath.Join(os.TempDir(), "example-repo")
	cases := []struct {
		name string
		url  string
		ok   bool
	}{
		{"https", "https://github.com/AgoraIO/example", true},
		{"http", "http://example.com/repo.git", true},
		{"ssh", "ssh://git@github.com/AgoraIO/example.git", true},
		{"git", "git://github.com/AgoraIO/example.git", true},
		{"file", "file:///srv/mirror/example.git", true},
		{"ssh shorthand", "git@github.com:AgoraIO/example.git", true},
		{"absolute local path", localAbsPath, true},
		{"empty rejected", "", false},
		{"dash prefix rejected", "-https://evil/repo", false},
		{"unrecognized form rejected", "example", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRepoOverrideURL(tc.url)
			if tc.ok && err != nil {
				t.Fatalf("expected %q to be valid, got error: %v", tc.url, err)
			}
			if !tc.ok && err == nil {
				t.Fatalf("expected %q to be rejected", tc.url)
			}
		})
	}
}

func TestQuickstartRepoOverrideKey(t *testing.T) {
	if got := quickstartRepoOverrideKey("nextjs"); got != "AGORA_QUICKSTART_NEXTJS_REPO_URL" {
		t.Fatalf("unexpected key for nextjs: %q", got)
	}
	if got := quickstartRepoOverrideKey("my-template"); got != "AGORA_QUICKSTART_MY_TEMPLATE_REPO_URL" {
		t.Fatalf("unexpected key for my-template: %q", got)
	}
}

func TestSelectQuickstartDefinitionUsesScenarioAndPreservesTemplateDefaults(t *testing.T) {
	rtc, err := selectQuickstartDefinition("nextjs", "video-call")
	if err != nil {
		t.Fatal(err)
	}
	if rtc.ID != "nextjs-video-call" || rtc.Template != "nextjs" || rtc.Scenario != "video-call" || !reflect.DeepEqual(rtc.RequiredFeatures, []string{"rtc"}) {
		t.Fatalf("unexpected rtc quickstart definition: %+v", rtc)
	}

	legacy, err := selectQuickstartDefinition("nextjs", "")
	if err != nil {
		t.Fatal(err)
	}
	if legacy.Template != "nextjs" || legacy.Scenario != "voice-agent" || !legacy.DefaultScenario {
		t.Fatalf("unexpected default nextjs quickstart definition: %+v", legacy)
	}

	_, err = selectQuickstartDefinition("go", "video-call")
	assertCLIErrorCode(t, err, "QUICKSTART_SCENARIO_UNSUPPORTED")
	_, err = selectQuickstartDefinition("nextjs", "not-a-scenario")
	assertCLIErrorCode(t, err, "QUICKSTART_SCENARIO_UNKNOWN")
}

func TestResolveQuickstartDefinitionUsesManifestAndRejectsConflicts(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, quickstartManifestFileName), []byte(`{"schemaVersion":1,"template":"nextjs","scenario":"video-call"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	definition, err := resolveQuickstartTemplateForPath(root, "", "")
	if err != nil {
		t.Fatal(err)
	}
	if definition.ID != "nextjs-video-call" {
		t.Fatalf("manifest resolved to %+v", definition)
	}

	if err := writeLocalProjectBinding(root, localProjectBinding{ProjectID: "prj_1", Template: "nextjs", Scenario: "voice-agent"}); err != nil {
		t.Fatal(err)
	}
	_, err = resolveQuickstartTemplateForPath(root, "", "")
	assertCLIErrorCode(t, err, "QUICKSTART_SELECTION_MISMATCH")

	if err := os.WriteFile(filepath.Join(root, quickstartManifestFileName), []byte(`{"schemaVersion":2,"template":"nextjs","scenario":"video-call"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err = resolveQuickstartTemplateForPath(root, "", "")
	assertCLIErrorCode(t, err, "QUICKSTART_MANIFEST_INVALID")
}

func TestQuickstartRepoURLOverride(t *testing.T) {
	tmpl := quickstartTemplate{
		ID:        "nextjs",
		RepoURL:   "https://default.example/repo",
		RepoURLCN: "https://cn.example/repo",
	}
	app := &App{env: map[string]string{"XDG_CONFIG_HOME": t.TempDir()}}

	url, override, err := app.quickstartRepoURL(tmpl)
	if err != nil || override != "" || url != tmpl.RepoURL {
		t.Fatalf("default path: url=%q override=%q err=%v", url, override, err)
	}

	app.env = map[string]string{
		"AGORA_QUICKSTART_NEXTJS_REPO_URL": "https://fork.example/repo",
		"XDG_CONFIG_HOME":                  t.TempDir(),
	}
	url, override, err = app.quickstartRepoURL(tmpl)
	if err != nil || override != "AGORA_QUICKSTART_NEXTJS_REPO_URL" || url != "https://fork.example/repo" {
		t.Fatalf("override path: url=%q override=%q err=%v", url, override, err)
	}

	app.env = map[string]string{
		"AGORA_QUICKSTART_NEXTJS_REPO_URL": "-fexploit",
		"XDG_CONFIG_HOME":                  t.TempDir(),
	}
	if _, _, err := app.quickstartRepoURL(tmpl); err == nil {
		t.Fatal("expected error for invalid override")
	} else {
		var cliErr *cliError
		if !errors.As(err, &cliErr) || cliErr.Code != "QUICKSTART_REPO_OVERRIDE_INVALID" {
			t.Fatalf("expected QUICKSTART_REPO_OVERRIDE_INVALID, got %v", err)
		}
	}
}

func TestQuickstartRepoURLForRegion(t *testing.T) {
	tmpl := quickstartTemplate{
		RepoURL:   "https://global.example/repo",
		RepoURLCN: "https://cn.example/repo",
	}
	if got := quickstartRepoURLForRegion(tmpl, regionGlobal); got != tmpl.RepoURL {
		t.Fatalf("global repo url = %q, want %q", got, tmpl.RepoURL)
	}
	if got := quickstartRepoURLForRegion(tmpl, regionCN); got != tmpl.RepoURLCN {
		t.Fatalf("cn repo url = %q, want %q", got, tmpl.RepoURLCN)
	}
}

func TestQuickstartDocsURLForRegion(t *testing.T) {
	tmpl := quickstartTemplate{
		DocsURL:   "https://global.example/docs",
		DocsURLCN: "https://cn.example/docs",
	}
	if got := quickstartDocsURL(tmpl, regionGlobal); got != tmpl.DocsURL {
		t.Fatalf("global docs url = %q, want %q", got, tmpl.DocsURL)
	}
	if got := quickstartDocsURL(tmpl, regionCN); got != tmpl.DocsURLCN {
		t.Fatalf("cn docs url = %q, want %q", got, tmpl.DocsURLCN)
	}
}

func TestQuickstartTemplatesIncludeAndroid(t *testing.T) {
	var android quickstartTemplate
	found := false
	for _, tmpl := range quickstartTemplates() {
		if tmpl.ID == "android" {
			android = tmpl
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected android quickstart template to exist")
	}
	if android.RepoURL != "https://github.com/AgoraIO-Conversational-AI/agent-quickstart-android" {
		t.Fatalf("unexpected android repo url: %q", android.RepoURL)
	}
	layout, ok := android.defaultEnvLayout()
	if !ok {
		t.Fatal("expected android env layout")
	}
	if layout.EnvExamplePath != "server/.env.example" || layout.EnvTargetPath != "server/.env.local" {
		t.Fatalf("unexpected android env layout: %+v", layout)
	}
	if layout.AppIDKey != "AGORA_APP_ID" || layout.AppCertificateKey != "AGORA_APP_CERTIFICATE" {
		t.Fatalf("unexpected android credential keys: %+v", layout)
	}
	if !android.Available || !android.SupportsInit {
		t.Fatalf("unexpected android flags: available=%v supportsInit=%v", android.Available, android.SupportsInit)
	}
	wantSteps := []string{
		"cd android-demo",
		"python3 -m venv server/.venv && server/.venv/bin/pip install -r server/requirements-dev.txt",
		"./server/run.sh",
		"./server/tunnel.sh --provider ngrok",
		"./server/configure-android.sh https://your-public-host",
		"./gradlew :app:assembleDebug",
	}
	if got := initNextSteps(android, "/tmp/android-demo"); !reflect.DeepEqual(got, wantSteps) {
		t.Fatalf("unexpected Android next steps:\n got: %#v\nwant: %#v", got, wantSteps)
	}
}

func TestResolveQuickstartScenarioInheritance(t *testing.T) {
	for _, tc := range []struct {
		name     string
		binding  *localProjectBinding
		manifest bool
		want     string
	}{
		{name: "manifest", manifest: true, want: "video-call"},
		{name: "binding", binding: &localProjectBinding{ProjectID: "prj_1", Template: "nextjs", Scenario: "video-call"}, want: "video-call"},
		{name: "legacy binding and manifest", binding: &localProjectBinding{ProjectID: "prj_1", Template: "nextjs"}, manifest: true, want: "video-call"},
		{name: "legacy default", binding: &localProjectBinding{ProjectID: "prj_1", Template: "nextjs"}, want: "voice-agent"},
		{name: "no metadata", want: "voice-agent"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.binding != nil {
				if err := writeLocalProjectBinding(root, *tc.binding); err != nil {
					t.Fatal(err)
				}
			}
			if tc.manifest {
				if err := os.WriteFile(filepath.Join(root, quickstartManifestFileName), []byte(`{"schemaVersion":1,"template":"nextjs","scenario":"video-call"}`), 0600); err != nil {
					t.Fatal(err)
				}
			}
			definition, err := resolveQuickstartTemplateForPath(root, "nextjs", "")
			if err != nil {
				t.Fatal(err)
			}
			if definition.Scenario != tc.want {
				t.Fatalf("scenario=%s want=%s", definition.Scenario, tc.want)
			}
		})
	}
}
