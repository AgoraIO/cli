package cli

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestCLIAnalyticsInitStagesAndFailure(t *testing.T) {
	for _, fail := range []bool{false, true} {
		t.Run(map[bool]string{false: "success", true: "clone failure"}[fail], func(t *testing.T) {
			collector, read := analyticsRecorder(t)
			configHome, root := t.TempDir(), t.TempDir()
			api := newFakeCLIBFF()
			defer api.server.Close()
			persistSessionForIntegration(t, configHome)
			enable := runCLI(t, []string{"telemetry", "enable", "--json"}, cliRunOptions{env: map[string]string{"AGORA_HOME": "", "XDG_CONFIG_HOME": configHome}})
			if enable.exitCode != 0 {
				t.Fatalf("consent failed: %+v", enable)
			}
			files := map[string]string{"agora.quickstart.json": `{"schemaVersion":1,"template":"nextjs","scenario":"video-call"}`, "env.local.example": "NEXT_PUBLIC_AGORA_APP_ID=\nNEXT_AGORA_APP_CERTIFICATE=\n", "package.json": `{"name":"fixture"}`}

			repo := createLocalGitRepo(t, files)
			if fail {
				repo = filepath.Join(root, "missing-private-repo")
			}
			args := []string{"init", "analytics-demo", "--template", "nextjs", "--scenario", "video-call", "--new-project", "--dir", filepath.Join(root, "demo"), "--source-distinct-id", "browser", "--source-session-id", "session", "--json"}
			result := runCLI(t, args, cliRunOptions{env: map[string]string{"AGORA_HOME": "", "XDG_CONFIG_HOME": configHome, "AGORA_API_BASE_URL": api.baseURL, "AGORA_QUICKSTART_NEXTJS_VIDEO_CALL_REPO_URL": repo, "AGORA_TELEMETRY_URL": collector.URL, "DO_NOT_TRACK": "", "AGORA_SENTRY_ENABLED": "1", "AGORA_TELEMETRY_ENABLED": "1"}, workdir: root})
			if !fail && result.exitCode != 0 {
				t.Fatalf("init failed: %+v", result)
			}
			if fail && result.exitCode == 0 {
				t.Fatal("missing repository should fail")
			}
			events := read()
			stages := map[string]bool{}
			finals := 0
			for _, e := range events {
				if e.DistinctID != "browser" || e.Properties["source_session_id"] != "session" {
					t.Fatalf("lost attribution: %+v", e)
				}
				if e.Event == "cli_init_stage_completed" {
					stage, _ := e.Properties["stage"].(string)
					if stages[stage] {
						t.Errorf("stage %q reported more than once", stage)
					}
					stages[stage] = true
				}
				if e.Event == "cli_init_completed" {
					finals++
					want := "success"
					if fail {
						want = "failure"
					}
					if e.Properties["outcome"] != want {
						t.Fatalf("wrong outcome: %+v", e)
					}
				}
			}
			if finals != 1 || !stages["clone"] || (!fail && !stages["env_write"]) {
				t.Fatalf("missing lifecycle stages: %+v", events)
			}
			if !fail && (!stages["binding_write"] || !stages["context_write"]) {
				t.Fatalf("incomplete init lifecycle: %+v", events)
			}
			payload, _ := json.Marshal(events)
			if strings.Contains(string(payload), root) || strings.Contains(string(payload), "analytics-demo") || strings.Contains(string(payload), "app_0001") {
				t.Fatalf("business data leaked: %s", payload)
			}
		})
	}
}

func TestCLIAnalyticsRecipesJSONUnchanged(t *testing.T) {
	collector, read := analyticsRecorder(t)
	catalog := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"schemaVersion":1,"type":"all","total":1,"items":[{"slug":"hotel","title":"Hotel","mainRepoUrl":"https://github.com/AgoraIO/hotel","type":"ai","official":true}]}`))
	}))
	defer catalog.Close()
	env := map[string]string{"AGORA_HOME": t.TempDir(), "AGORA_RECIPES_BASE_URL": catalog.URL, "AGORA_TELEMETRY_URL": collector.URL, "DO_NOT_TRACK": "", "AGORA_SENTRY_ENABLED": "1", "AGORA_TELEMETRY_ENABLED": "1"}
	enable := runCLI(t, []string{"telemetry", "enable", "--json"}, cliRunOptions{env: env})
	if enable.exitCode != 0 {
		t.Fatalf("consent failed: %+v", enable)
	}
	result := runCLI(t, []string{"recipes", "list", "--json"}, cliRunOptions{env: env})
	var envelope map[string]any
	if result.exitCode != 0 || json.Unmarshal([]byte(result.stdout), &envelope) != nil || envelope["ok"] != true {
		t.Fatalf("JSON contract changed: %+v", result)
	}
	events := waitAnalyticsEvents(t, read, 1)
	if len(events) != 1 || events[0].Properties["result_count"] != float64(1) {
		t.Fatalf("wrong recipe event: %+v", events)
	}
}
