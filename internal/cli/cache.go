package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Cache layer for read-only API responses that are safe to serve from
// disk for a short window. Today this powers shell tab-completion of
// project names so `agora project use <TAB>` does not hit the API on
// every keystroke. The same machinery is reusable for any "list-y"
// API response we want to amortize.
//
// Storage layout (under the resolved Agora directory, honoring
// AGORA_HOME / XDG_CONFIG_HOME / ~/.agora-cli on macOS):
//
//   <agora-dir>/
//     cache/
//       projects.json   — see projectListCachePayload below
//
// Files are written with 0o600 because they may include project IDs
// the user considers private. The directory is 0o700 to match the
// rest of the Agora config tree.

const (
	// cacheDirName is the on-disk subdirectory under the Agora
	// config directory that holds all transient API caches.
	cacheDirName = "cache"

	// projectListCacheFile is the projects-list cache filename.
	projectListCacheFile = "projects.json"

	// projectListCacheTTL is the longest age a cache file may be
	// served from. After this window the cache is considered stale
	// and ignored (and proactively removed on the next startup
	// sweep).
	projectListCacheTTL = 5 * time.Minute

	// projectListCacheMaxAge is the cutoff used by
	// pruneStaleCaches: anything older than this is removed
	// regardless of whether it is being read.
	projectListCacheMaxAge = 24 * time.Hour

	// projectListCacheSchemaVersion is bumped whenever the on-disk
	// shape changes incompatibly. Older versions are ignored
	// (treated as cache-miss) instead of crashing the CLI.
	projectListCacheSchemaVersion = 1
)

// projectListCachePayload is the persisted shape of the projects-list
// cache. The TTLSeconds field is recorded alongside FetchedAt so a
// future TTL change does not silently invalidate every existing cache;
// readers honor whichever TTL is shorter (the file's recorded TTL or
// the current process's TTL constant) so we are always conservative.
type projectListCachePayload struct {
	SchemaVersion int              `json:"schemaVersion"`
	FetchedAt     string           `json:"fetchedAt"`
	TTLSeconds    int              `json:"ttlSeconds"`
	Items         []projectSummary `json:"items"`
	Page          int              `json:"page"`
	PageSize      int              `json:"pageSize"`
	Total         int              `json:"total"`
}

// resolveCacheDir is the on-disk root for all Agora CLI read caches.
// It always lives under the resolved Agora config directory so it
// follows the same isolation contract: in CI / multi-agent runs,
// setting AGORA_HOME to a tmpdir transparently isolates the cache too.
func resolveCacheDir(env map[string]string) (string, error) {
	dir, err := resolveAgoraDirectory(env)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, cacheDirName), nil
}

func resolveProjectListCachePath(env map[string]string) (string, error) {
	dir, err := resolveCacheDir(env)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, projectListCacheFile), nil
}

// saveProjectListCache writes a fresh cache entry. Errors are
// non-fatal to the caller: a failure to cache is observability noise,
// not a user-visible error. The function therefore returns an error
// for tests but every production caller is expected to ignore it.
func saveProjectListCache(env map[string]string, list projectListResponse) error {
	path, err := resolveProjectListCachePath(env)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	payload := projectListCachePayload{
		SchemaVersion: projectListCacheSchemaVersion,
		FetchedAt:     time.Now().UTC().Format(time.RFC3339Nano),
		TTLSeconds:    int(projectListCacheTTL.Seconds()),
		Items:         list.Items,
		Page:          list.Page,
		PageSize:      list.PageSize,
		Total:         list.Total,
	}
	data, err := json.MarshalIndent(payload, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// loadProjectListCache reads the persisted cache. Returns:
//
//	(payload, true,  nil)   — cache is present, schema-current, and fresh
//	(zero,    false, nil)   — cache is missing, expired, or wrong schema
//	(zero,    false, err)   — disk I/O error or parse error
//
// "Fresh" honors whichever TTL is shorter (the file's recorded TTL
// when it was written, or this process's compiled-in TTL). This means
// shortening the TTL in code immediately tightens what counts as
// fresh, while lengthening it does not retroactively extend a file
// the user wrote under a tighter contract.
func loadProjectListCache(env map[string]string) (projectListCachePayload, bool, error) {
	path, err := resolveProjectListCachePath(env)
	if err != nil {
		return projectListCachePayload{}, false, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return projectListCachePayload{}, false, nil
	}
	if err != nil {
		return projectListCachePayload{}, false, err
	}
	var payload projectListCachePayload
	if err := json.Unmarshal(data, &payload); err != nil {
		return projectListCachePayload{}, false, err
	}
	if payload.SchemaVersion != projectListCacheSchemaVersion {
		return projectListCachePayload{}, false, nil
	}
	fetchedAt, err := time.Parse(time.RFC3339Nano, strings.TrimSpace(payload.FetchedAt))
	if err != nil {
		return projectListCachePayload{}, false, nil
	}
	effectiveTTL := projectListCacheTTL
	if payload.TTLSeconds > 0 {
		recorded := time.Duration(payload.TTLSeconds) * time.Second
		if recorded < effectiveTTL {
			effectiveTTL = recorded
		}
	}
	if time.Since(fetchedAt) > effectiveTTL {
		return projectListCachePayload{}, false, nil
	}
	return payload, true, nil
}

// clearProjectListCache removes the persisted cache. Called when the
// backing list is known to be stale or the user identity changed:
// `agora logout` (no session should imply no cached API snapshot) and
// `agora project create` (a new project exists server-side but is not
// yet in the cached first page). Also cleared when `AGORA_DISABLE_CACHE=1`
// runs the startup prune path.
func clearProjectListCache(env map[string]string) error {
	path, err := resolveProjectListCachePath(env)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// pruneStaleCaches is the "flush on startup" sweep called from
// App.Execute. Any cache file whose age exceeds projectListCacheMaxAge
// (24 h by default) is removed so we never accumulate unbounded
// disk under the Agora config tree, and so a stale cache from an
// earlier auth session can never silently shape today's CLI output.
//
// This runs at most once per process, never blocks, and is best-effort
// silent: any I/O error is swallowed because cache hygiene is not a
// reason to fail a user's command. Errors are still surfaced to tests
// via the return value.
func pruneStaleCaches(env map[string]string) error {
	if isTruthy(env["AGORA_DISABLE_CACHE"]) {
		return clearProjectListCache(env)
	}
	dir, err := resolveCacheDir(env)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	cutoff := time.Now().Add(-projectListCacheMaxAge)
	var firstErr error
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if info.ModTime().Before(cutoff) {
			if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil && firstErr == nil {
				firstErr = err
			}
		}
	}
	return firstErr
}

// isTruthy parses the standard truthy strings the rest of the CLI
// accepts ("1", "true", "yes", "y", case-insensitive).
func isTruthy(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "y":
		return true
	default:
		return false
	}
}

// cacheTTLFromEnv lets tests and power users override the read-cache
// TTL via AGORA_PROJECT_CACHE_TTL_SECONDS without recompiling. A
// missing or invalid value falls back to the package-level default.
func cacheTTLFromEnv(env map[string]string) time.Duration {
	raw := strings.TrimSpace(env["AGORA_PROJECT_CACHE_TTL_SECONDS"])
	if raw == "" {
		return projectListCacheTTL
	}
	seconds, err := strconv.Atoi(raw)
	if err != nil || seconds < 0 {
		return projectListCacheTTL
	}
	return time.Duration(seconds) * time.Second
}
