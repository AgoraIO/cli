package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	currentAppConfigVersion = 2
	previousAPIBaseURL      = "https://agora-cli-bff.staging.la3.agoralab.co"
	previousOAuthBaseURL    = "https://staging-sso.agora.io"
	previousOAuthClientID   = "cli_demo"
)

type configState struct {
	Config          appConfig
	ConfigFilePath  string
	PreviousVersion *int
	Status          string
}

const agoraBanner = ` █████╗  ██████╗  ██████╗ ██████╗  █████╗ 
██╔══██╗██╔════╝ ██╔═══██╗██╔══██╗██╔══██╗
███████║██║  ███╗██║   ██║██████╔╝███████║
██╔══██║██║   ██║██║   ██║██╔══██╗██╔══██║
██║  ██║╚██████╔╝╚██████╔╝██║  ██║██║  ██║
╚═╝  ╚═╝ ╚═════╝  ╚═════╝ ╚═╝  ╚═╝╚═╝  ╚═╝`

func ensureAppConfigState(env map[string]string) (configState, error) {
	configFilePath, err := resolveConfigFilePath(env)
	if err != nil {
		return configState{}, err
	}

	data, err := os.ReadFile(configFilePath)
	if errors.Is(err, os.ErrNotExist) {
		cfg := defaultConfig()
		cfg.Version = currentAppConfigVersion
		if err := writeSecureJSON(configFilePath, cfg); err != nil {
			return configState{}, err
		}
		return configState{
			Config:         cfg,
			ConfigFilePath: configFilePath,
			Status:         "created",
		}, nil
	}
	if err != nil {
		return configState{}, err
	}

	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		return configState{}, err
	}

	cfg := defaultConfig()
	mergeConfig(&cfg, migratePreviousConfig(raw))
	version, hasVersion := intValue(raw["version"])
	if hasVersion && version > currentAppConfigVersion {
		return configState{}, fmt.Errorf("Config version %d is newer than this CLI supports.", version)
	}
	cfg.Version = currentAppConfigVersion

	status := "loaded"
	var previousVersion *int
	if !hasVersion || version != currentAppConfigVersion {
		status = "migrated"
		if hasVersion {
			v := version
			previousVersion = &v
		}
		if err := writeSecureJSON(configFilePath, cfg); err != nil {
			return configState{}, err
		}
	}

	return configState{
		Config:          cfg,
		ConfigFilePath:  configFilePath,
		PreviousVersion: previousVersion,
		Status:          status,
	}, nil
}

func migratePreviousConfig(raw map[string]any) map[string]any {
	clone := map[string]any{}
	for k, v := range raw {
		clone[k] = v
	}
	version, _ := intValue(raw["version"])
	if version < 2 {
		if v, ok := clone["apiBaseUrl"].(string); ok && v == previousAPIBaseURL {
			clone["apiBaseUrl"] = defaultConfig().APIBaseURL
		}
		if v, ok := clone["oauthBaseUrl"].(string); ok && v == previousOAuthBaseURL {
			clone["oauthBaseUrl"] = defaultConfig().OAuthBaseURL
		}
		if v, ok := clone["oauthClientId"].(string); ok && v == previousOAuthClientID {
			clone["oauthClientId"] = defaultConfig().OAuthClientID
		}
	}
	return clone
}

func intValue(v any) (int, bool) {
	switch x := v.(type) {
	case float64:
		return int(x), true
	case int:
		return x, true
	default:
		return 0, false
	}
}

func shouldPrintConfigBanner(mode outputMode, isTTY bool, status string) bool {
	return isTTY && mode == outputPretty && status != "loaded"
}

func resolveConfiguredOutputMode(raw string, env map[string]string) outputMode {
	if raw == "json" {
		return outputJSON
	}
	if env["AGORA_OUTPUT"] == "json" {
		return outputJSON
	}
	return outputPretty
}

func formatConfigBanner(state configState) string {
	if state.Status == "loaded" {
		return ""
	}
	headline := ""
	switch {
	case state.Status == "created":
		headline = fmt.Sprintf("Config initialized: %s", state.ConfigFilePath)
	case state.PreviousVersion == nil:
		headline = fmt.Sprintf("Config upgraded to version %d from an earlier format: %s", state.Config.Version, state.ConfigFilePath)
	default:
		headline = fmt.Sprintf("Config upgraded from version %d to %d: %s", *state.PreviousVersion, state.Config.Version, state.ConfigFilePath)
	}
	return agoraBanner + "\n\n" + headline + "\nYou can edit this file directly or override values with .env/.env.local during development.\n"
}

func resolveLogDirectoryPath(env map[string]string) (string, error) {
	dir, err := resolveAgoraDirectory(env)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "logs"), nil
}

func resolveLogFilePath(env map[string]string) (string, error) {
	dir, err := resolveLogDirectoryPath(env)
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "agora-cli.log"), nil
}

func resolveLogFilePathForDisplay(env map[string]string) string {
	path, err := resolveLogFilePath(env)
	if err != nil {
		return "agora-cli.log"
	}
	return path
}

var sensitiveFieldPattern = regexp.MustCompile(`(?i)token|secret|password|api[_-]?key|authorization`)
var logMu sync.Mutex

func appendAppLog(level, event string, env map[string]string, fields map[string]any) error {
	if env["AGORA_LOG_ENABLED"] == "0" {
		return nil
	}
	if !shouldEmitLogLevel(level, resolveLogLevel(env)) {
		return nil
	}
	logFilePath, err := resolveLogFilePath(env)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(logFilePath), 0o700); err != nil {
		return err
	}

	logMu.Lock()
	defer logMu.Unlock()

	if err := rotateLogFiles(logFilePath, parsePositiveInteger(env["AGORA_LOG_MAX_BYTES"], 1_000_000), parsePositiveInteger(env["AGORA_LOG_MAX_FILES"], 5)); err != nil {
		return err
	}

	payload, err := json.Marshal(map[string]any{
		"event":     event,
		"fields":    sanitizeFields(fields),
		"level":     level,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	})
	if err != nil {
		return err
	}
	f, err := os.OpenFile(logFilePath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.Write(append(payload, '\n')); err != nil {
		return err
	}
	if env["AGORA_VERBOSE"] == "1" {
		_, _ = fmt.Fprintf(os.Stderr, "[agora-cli] %s\n", string(payload))
	}
	return nil
}

func parsePositiveInteger(value string, fallback int64) int64 {
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func resolveLogLevel(env map[string]string) string {
	switch strings.ToLower(strings.TrimSpace(env["AGORA_LOG_LEVEL"])) {
	case "debug", "warn", "error":
		return strings.ToLower(strings.TrimSpace(env["AGORA_LOG_LEVEL"]))
	default:
		return "info"
	}
}

func shouldEmitLogLevel(level, threshold string) bool {
	levels := map[string]int{"debug": 10, "info": 20, "warn": 30, "error": 40}
	return levels[level] >= levels[threshold]
}

func sanitizeFields(fields map[string]any) map[string]any {
	if fields == nil {
		return map[string]any{}
	}
	out := make(map[string]any, len(fields))
	for key, value := range fields {
		out[key] = sanitizeValue(key, value)
	}
	return out
}

func sanitizeValue(key string, value any) any {
	if sensitiveFieldPattern.MatchString(key) {
		return "[REDACTED]"
	}
	switch x := value.(type) {
	case map[string]any:
		out := make(map[string]any, len(x))
		for nestedKey, nestedValue := range x {
			out[nestedKey] = sanitizeValue(nestedKey, nestedValue)
		}
		return out
	case []any:
		out := make([]any, len(x))
		for i, entry := range x {
			out[i] = sanitizeValue(key, entry)
		}
		return out
	case []string:
		out := make([]any, len(x))
		for i, entry := range x {
			out[i] = sanitizeValue(key, entry)
		}
		return out
	default:
		return value
	}
}

func rotateLogFiles(logFilePath string, maxBytes, maxFiles int64) error {
	info, err := os.Stat(logFilePath)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Size() < maxBytes {
		return nil
	}

	oldest := fmt.Sprintf("%s.%d", logFilePath, maxFiles-1)
	_ = os.Remove(oldest)
	for index := maxFiles - 2; index >= 1; index-- {
		source := fmt.Sprintf("%s.%d", logFilePath, index)
		dest := fmt.Sprintf("%s.%d", logFilePath, index+1)
		if err := os.Rename(source, dest); err != nil && !errors.Is(err, os.ErrNotExist) {
			return err
		}
	}
	if err := os.Rename(logFilePath, logFilePath+".1"); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return nil
}

func sortedMapKeys(values map[string]any) []string {
	keys := make([]string, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
