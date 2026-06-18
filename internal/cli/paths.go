package cli

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// resolveConfigRoot returns the directory used as the *parent* of the Agora
// CLI config directory. Precedence:
//
//  1. $AGORA_HOME — explicit override; used as-is.
//  2. $XDG_CONFIG_HOME — XDG Base Directory standard.
//  3. ~/.agora-cli on macOS (no XDG override).
//  4. $APPDATA on Windows.
//  5. ~/.config on Linux/BSD.
//
// resolveAgoraDirectory layers an "agora-cli" suffix on top of this path
// when the root is not already an explicit Agora-specific override.
func resolveConfigRoot(env map[string]string) (string, error) {
	if v := strings.TrimSpace(env["AGORA_HOME"]); v != "" {
		return v, nil
	}
	if v := strings.TrimSpace(env["XDG_CONFIG_HOME"]); v != "" {
		return v, nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, ".agora-cli"), nil
	}
	if v := strings.TrimSpace(env["APPDATA"]); v != "" {
		return v, nil
	}
	return filepath.Join(home, ".config"), nil
}

// resolveAgoraDirectory returns the canonical Agora CLI config directory,
// suitable for storing config.json, session.json, context.json, logs/, etc.
//
// When AGORA_HOME is set we use it verbatim (it points directly at the
// Agora directory). For XDG_CONFIG_HOME / APPDATA / ~/.config we append
// "agora-cli". The macOS fallback (~/.agora-cli) is already Agora-specific.
func resolveAgoraDirectory(env map[string]string) (string, error) {
	root, err := resolveConfigRoot(env)
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(env["AGORA_HOME"]) != "" {
		return root, nil
	}
	hasExplicitRoot := strings.TrimSpace(env["XDG_CONFIG_HOME"]) != "" || strings.TrimSpace(env["APPDATA"]) != ""
	if runtime.GOOS == "darwin" && !hasExplicitRoot {
		return root, nil
	}
	return filepath.Join(root, "agora-cli"), nil
}

func resolveConfigFilePath(env map[string]string) (string, error) {
	dir, err := resolveAgoraDirectory(env)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "config.json"), nil
}

func resolveSessionFilePath(env map[string]string) (string, error) {
	dir, err := resolveAgoraDirectory(env)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "session.json"), nil
}

func resolveContextFilePath(env map[string]string) (string, error) {
	dir, err := resolveAgoraDirectory(env)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "context.json"), nil
}

// writeSecureJSON writes JSON-encoded value to path with restrictive perms.
// The parent directory is created with 0o700 if missing, and the file
// itself is written with 0o600 — appropriate for files that may carry
// session tokens, OAuth state, or App Certificates.
func writeSecureJSON(path string, value any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

// loadContext reads the persisted project context (currently selected
// project + active control-plane region). A missing file returns the
// zero-value context with region defaulted to "global".
func loadContext(env map[string]string) (projectContext, error) {
	path, err := resolveContextFilePath(env)
	if err != nil {
		return projectContext{}, err
	}
	ctx := projectContext{CurrentRegion: "global"}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return ctx, nil
	}
	if err != nil {
		return projectContext{}, err
	}
	if err := json.Unmarshal(data, &ctx); err != nil {
		return projectContext{}, err
	}
	if ctx.CurrentRegion == "" {
		ctx.CurrentRegion = "global"
	}
	return ctx, nil
}

func saveContext(env map[string]string, ctx projectContext) error {
	path, err := resolveContextFilePath(env)
	if err != nil {
		return err
	}
	return writeSecureJSON(path, ctx)
}

func clearContext(env map[string]string) error {
	path, err := resolveContextFilePath(env)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

// loadSession returns the persisted OAuth session, or nil if no session
// file exists. The caller distinguishes "unauthenticated" (nil) from
// "I/O error" (non-nil err).
func loadSession(env map[string]string) (*session, error) {
	path, err := resolveSessionFilePath(env)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var s session
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func saveSession(env map[string]string, s session) error {
	path, err := resolveSessionFilePath(env)
	if err != nil {
		return err
	}
	return writeSecureJSON(path, s)
}

// clearSession removes the persisted session file. The bool reports
// whether a session existed before the call (true) or whether the file
// was already absent (false).
func clearSession(env map[string]string) (bool, error) {
	path, err := resolveSessionFilePath(env)
	if err != nil {
		return false, err
	}
	if err := os.Remove(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	return true, nil
}
