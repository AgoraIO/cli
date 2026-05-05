package cli

import "strings"

func agentLabelFromOSEnv(env map[string]string) string {
	if explicit := strings.TrimSpace(env["AGORA_AGENT"]); explicit != "" {
		return explicit
	}
	if truthyEnv(env, "AGORA_AGENT_DISABLE_INFER") {
		return ""
	}
	switch {
	case nonEmptyEnv(env, "CURSOR_AGENT"):
		return "cursor"
	case nonEmptyEnv(env, "CLAUDE_CODE"):
		return "claude-code"
	case hasEnvPrefix(env, "CLINE_"):
		return "cline"
	case hasEnvPrefix(env, "WINDSURF_"):
		return "windsurf"
	case hasEnvPrefix(env, "OPENAI_CODEX_"):
		return "codex"
	case hasEnvPrefix(env, "AIDER_"):
		return "aider"
	default:
		return ""
	}
}

func nonEmptyEnv(env map[string]string, key string) bool {
	return strings.TrimSpace(env[key]) != ""
}

func hasEnvPrefix(env map[string]string, prefix string) bool {
	for key, value := range env {
		if strings.HasPrefix(key, prefix) && strings.TrimSpace(value) != "" {
			return true
		}
	}
	return false
}

func truthyEnv(env map[string]string, key string) bool {
	value := strings.ToLower(strings.TrimSpace(env[key]))
	return value == "1" || value == "true" || value == "yes" || value == "y"
}
