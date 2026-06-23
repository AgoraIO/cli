package cli

import "strings"

// appConfig is the persisted CLI configuration shape stored at
// $AGORA_HOME/config.json (or the OS-appropriate fallback path resolved by
// resolveConfigFilePath). All fields are JSON-tagged with their stable
// public names.
type appConfig struct {
	Version          int        `json:"version"`
	BrowserAutoOpen  bool       `json:"browserAutoOpen"`
	LogLevel         string     `json:"logLevel"`
	Output           outputMode `json:"output"`
	TelemetryEnabled bool       `json:"telemetryEnabled"`
	// Debug controls whether `appendAppLog` mirrors structured log
	// records to stderr. v0.2.0 renamed this field from "verbose" to
	// match the canonical --debug flag and AGORA_DEBUG env var.
	// mergeConfig still reads the legacy "verbose" key so existing
	// 0.1.x configs migrate transparently on first load.
	Debug bool `json:"debug"`
}

// defaultConfig returns a fresh appConfig populated with the production
// defaults shipped with the CLI. New installations and missing fields fall
// back to these values.
func defaultConfig() appConfig {
	return appConfig{
		Version:          4,
		BrowserAutoOpen:  true,
		LogLevel:         "info",
		Output:           outputPretty,
		TelemetryEnabled: true,
		Debug:            false,
	}
}

// mergeConfig applies a raw JSON map onto an appConfig in-place, ignoring
// missing or wrong-typed fields. This is the partial-update path used by
// the migration flow in ensureAppConfigState.
func mergeConfig(cfg *appConfig, raw map[string]any) {
	if v, ok := raw["browserAutoOpen"].(bool); ok {
		cfg.BrowserAutoOpen = v
	}
	if v, ok := raw["logLevel"].(string); ok && v != "" {
		cfg.LogLevel = v
	}
	if v, ok := raw["output"].(string); ok && (v == "json" || v == "pretty") {
		cfg.Output = outputMode(v)
	}
	if v, ok := raw["telemetryEnabled"].(bool); ok {
		cfg.TelemetryEnabled = v
	}
	// v0.2.0 renamed "verbose" -> "debug". Read the canonical key
	// first; fall back to the legacy key so existing 0.1.x configs
	// migrate on first load. The next config write drops "verbose"
	// because the appConfig struct only emits "debug".
	if v, ok := raw["debug"].(bool); ok {
		cfg.Debug = v
	} else if v, ok := raw["verbose"].(bool); ok {
		cfg.Debug = v
	}
}

// applyConfigToEnv injects config values into a.env so downstream code paths
// (which prefer env over config) see a consistent view. setEnvIfMissing
// preserves any user-set environment variables; only missing keys are
// populated. DO_NOT_TRACK forces telemetry off regardless of the persisted
// config preference (Console-style telemetry opt-out signal).
func (a *App) applyConfigToEnv() {
	a.setEnvIfMissing("AGORA_OUTPUT", string(a.cfg.Output))
	a.setEnvIfMissing("AGORA_SENTRY_ENABLED", boolString(a.cfg.TelemetryEnabled))
	a.setEnvIfMissing("AGORA_BROWSER_AUTO_OPEN", boolString(a.cfg.BrowserAutoOpen))
	a.setEnvIfMissing("AGORA_LOG_LEVEL", a.cfg.LogLevel)
	a.setEnvIfMissing("AGORA_DEBUG", boolString(a.cfg.Debug))
	if strings.TrimSpace(a.env["DO_NOT_TRACK"]) != "" {
		a.env["AGORA_SENTRY_ENABLED"] = "0"
	}
}

func (a *App) setEnvIfMissing(key, value string) {
	if _, ok := a.env[key]; !ok && value != "" {
		a.env[key] = value
	}
}

func boolString(v bool) string {
	if v {
		return "1"
	}
	return "0"
}
