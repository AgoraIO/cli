package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestEnv(t *testing.T) map[string]string {
	t.Helper()
	dir := t.TempDir()
	return map[string]string{"AGORA_HOME": dir}
}

func TestSaveAndLoadProjectListCacheRoundTrip(t *testing.T) {
	env := newTestEnv(t)
	list := projectListResponse{
		Items: []projectSummary{
			{ProjectID: "prj_aaa", Name: "Demo One"},
			{ProjectID: "prj_bbb", Name: "Demo Two"},
		},
		Page:     1,
		PageSize: 100,
		Total:    2,
	}
	if err := saveProjectListCache(env, list); err != nil {
		t.Fatalf("saveProjectListCache: %v", err)
	}
	payload, fresh, err := loadProjectListCache(env)
	if err != nil {
		t.Fatalf("loadProjectListCache: %v", err)
	}
	if !fresh {
		t.Fatal("expected fresh=true immediately after write")
	}
	if payload.SchemaVersion != projectListCacheSchemaVersion {
		t.Errorf("schema version = %d, want %d", payload.SchemaVersion, projectListCacheSchemaVersion)
	}
	if len(payload.Items) != 2 || payload.Items[0].ProjectID != "prj_aaa" {
		t.Errorf("unexpected items: %+v", payload.Items)
	}
	if payload.TTLSeconds <= 0 {
		t.Errorf("expected positive TTLSeconds, got %d", payload.TTLSeconds)
	}
}

func TestLoadProjectListCacheTreatsMissingFileAsCacheMiss(t *testing.T) {
	env := newTestEnv(t)
	_, fresh, err := loadProjectListCache(env)
	if err != nil {
		t.Fatalf("missing cache should be a cache-miss not an error, got %v", err)
	}
	if fresh {
		t.Fatal("expected fresh=false for missing cache")
	}
}

func TestLoadProjectListCacheStaleByFileAge(t *testing.T) {
	env := newTestEnv(t)
	if err := saveProjectListCache(env, projectListResponse{Items: []projectSummary{{ProjectID: "prj_old"}}}); err != nil {
		t.Fatal(err)
	}
	path, _ := resolveProjectListCachePath(env)
	data, _ := os.ReadFile(path)
	var payload projectListCachePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		t.Fatal(err)
	}
	payload.FetchedAt = time.Now().Add(-1 * time.Hour).UTC().Format(time.RFC3339Nano)
	out, _ := json.MarshalIndent(payload, "", "  ")
	if err := os.WriteFile(path, out, 0o600); err != nil {
		t.Fatal(err)
	}
	_, fresh, err := loadProjectListCache(env)
	if err != nil {
		t.Fatalf("expected nil error for stale cache, got %v", err)
	}
	if fresh {
		t.Fatal("stale cache must be reported as fresh=false")
	}
}

func TestLoadProjectListCacheRejectsUnknownSchemaVersion(t *testing.T) {
	env := newTestEnv(t)
	path, _ := resolveProjectListCachePath(env)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	bogus := []byte(`{"schemaVersion": 9999, "fetchedAt": "` + time.Now().UTC().Format(time.RFC3339Nano) + `", "items": []}`)
	if err := os.WriteFile(path, bogus, 0o600); err != nil {
		t.Fatal(err)
	}
	_, fresh, err := loadProjectListCache(env)
	if err != nil {
		t.Fatalf("future schema must be a soft cache-miss, got %v", err)
	}
	if fresh {
		t.Fatal("future schema must be reported as fresh=false")
	}
}

func TestClearProjectListCacheIsIdempotent(t *testing.T) {
	env := newTestEnv(t)
	if err := saveProjectListCache(env, projectListResponse{}); err != nil {
		t.Fatal(err)
	}
	if err := clearProjectListCache(env); err != nil {
		t.Fatalf("first clear: %v", err)
	}
	if err := clearProjectListCache(env); err != nil {
		t.Fatalf("second clear should be a no-op, got %v", err)
	}
	_, fresh, _ := loadProjectListCache(env)
	if fresh {
		t.Fatal("cache should be missing after clear")
	}
}

func TestPruneStaleCachesRemovesFilesOlderThanMaxAge(t *testing.T) {
	env := newTestEnv(t)
	dir, _ := resolveCacheDir(env)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	freshPath := filepath.Join(dir, "fresh.json")
	stalePath := filepath.Join(dir, "stale.json")
	if err := os.WriteFile(freshPath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(stalePath, []byte("{}"), 0o600); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-2 * projectListCacheMaxAge)
	if err := os.Chtimes(stalePath, old, old); err != nil {
		t.Fatal(err)
	}
	if err := pruneStaleCaches(env); err != nil {
		t.Fatalf("pruneStaleCaches: %v", err)
	}
	if _, err := os.Stat(freshPath); err != nil {
		t.Fatal("fresh file was incorrectly pruned")
	}
	if _, err := os.Stat(stalePath); !os.IsNotExist(err) {
		t.Fatal("stale file was not pruned")
	}
}

func TestPruneStaleCachesIsNoOpWithoutCacheDir(t *testing.T) {
	env := newTestEnv(t)
	if err := pruneStaleCaches(env); err != nil {
		t.Fatalf("pruneStaleCaches with no cache dir should be a no-op, got %v", err)
	}
}

func TestAGORADisableCacheClearsTheCacheOnStartup(t *testing.T) {
	env := newTestEnv(t)
	if err := saveProjectListCache(env, projectListResponse{Items: []projectSummary{{ProjectID: "prj_x"}}}); err != nil {
		t.Fatal(err)
	}
	env["AGORA_DISABLE_CACHE"] = "1"
	if err := pruneStaleCaches(env); err != nil {
		t.Fatalf("pruneStaleCaches: %v", err)
	}
	_, fresh, _ := loadProjectListCache(env)
	if fresh {
		t.Fatal("AGORA_DISABLE_CACHE should drop the cache on startup")
	}
}

func TestCacheTTLFromEnvHonorsOverride(t *testing.T) {
	if got := cacheTTLFromEnv(map[string]string{}); got != projectListCacheTTL {
		t.Errorf("default TTL = %v, want %v", got, projectListCacheTTL)
	}
	if got := cacheTTLFromEnv(map[string]string{"AGORA_PROJECT_CACHE_TTL_SECONDS": "0"}); got != 0 {
		t.Errorf("override 0s = %v, want 0", got)
	}
	if got := cacheTTLFromEnv(map[string]string{"AGORA_PROJECT_CACHE_TTL_SECONDS": "120"}); got != 2*time.Minute {
		t.Errorf("override 120s = %v, want 2m", got)
	}
	if got := cacheTTLFromEnv(map[string]string{"AGORA_PROJECT_CACHE_TTL_SECONDS": "garbage"}); got != projectListCacheTTL {
		t.Errorf("invalid override should fall back, got %v", got)
	}
}

func TestHasPersistedNonEmptySessionRequiresToken(t *testing.T) {
	env := newTestEnv(t)
	if hasPersistedNonEmptySession(env) {
		t.Fatal("expected false with no session file")
	}
	if err := saveSession(env, session{AccessToken: "t"}); err != nil {
		t.Fatal(err)
	}
	if !hasPersistedNonEmptySession(env) {
		t.Fatal("expected true with non-empty access token")
	}
}

func TestHasPersistedNonEmptySessionRejectsExpiredSession(t *testing.T) {
	env := newTestEnv(t)
	expired := time.Now().Add(-time.Minute).UTC().Format(time.RFC3339Nano)
	if err := saveSession(env, session{AccessToken: "t", ExpiresAt: expired}); err != nil {
		t.Fatal(err)
	}
	if hasPersistedNonEmptySession(env) {
		t.Fatal("expected false with expired session")
	}
	future := time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano)
	if err := saveSession(env, session{AccessToken: "t", ExpiresAt: future}); err != nil {
		t.Fatal(err)
	}
	if !hasPersistedNonEmptySession(env) {
		t.Fatal("expected true with unexpired session")
	}
}

func TestCompletionUsesCacheBeforeNetwork(t *testing.T) {
	env := newTestEnv(t)
	if err := saveSession(env, session{AccessToken: "cached-session-token", TokenType: "Bearer"}); err != nil {
		t.Fatal(err)
	}
	list := projectListResponse{
		Items: []projectSummary{
			{ProjectID: "prj_alpha", Name: "Alpha App"},
			{ProjectID: "prj_beta", Name: "Beta App"},
		},
	}
	if err := saveProjectListCache(env, list); err != nil {
		t.Fatal(err)
	}
	app := &App{env: env}
	items, ok := app.completionProjectsFromCache()
	if !ok {
		t.Fatal("expected cache hit")
	}
	if len(items) != 2 || items[0].Name != "Alpha App" {
		t.Fatalf("unexpected cached items: %+v", items)
	}
	results := filterProjectCompletions(items, "alp")
	if len(results) == 0 {
		t.Fatal("filter should have matched 'alp' against Alpha App")
	}
}

func TestCompletionCacheIgnoredWithoutLocalSession(t *testing.T) {
	env := newTestEnv(t)
	if err := saveProjectListCache(env, projectListResponse{
		Items: []projectSummary{{ProjectID: "prj_only_in_cache", Name: "Ghost"}},
	}); err != nil {
		t.Fatal(err)
	}
	app := &App{env: env}
	if _, ok := app.completionProjectsFromCache(); ok {
		t.Fatal("expected no cache hit without a local session")
	}
}

func TestCompletionCacheRespectsAGORADisableCache(t *testing.T) {
	env := newTestEnv(t)
	if err := saveSession(env, session{AccessToken: "x", TokenType: "Bearer"}); err != nil {
		t.Fatal(err)
	}
	if err := saveProjectListCache(env, projectListResponse{Items: []projectSummary{{ProjectID: "prj_x"}}}); err != nil {
		t.Fatal(err)
	}
	env["AGORA_PROJECT_CACHE_TTL_SECONDS"] = "0"
	app := &App{env: env}
	if _, ok := app.completionProjectsFromCache(); ok {
		t.Fatal("TTL=0 must disable the completion cache")
	}
}
