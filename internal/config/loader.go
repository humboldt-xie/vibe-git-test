package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// ClaudeSettings represents the structure of ~/.claude/settings.json
type ClaudeSettings struct {
	Env map[string]string `json:"env"`
}

// DefaultConfig holds default values loaded from ~/.claude/settings.json
type DefaultConfig struct {
	AnthropicAPIKey string
	AnthropicBaseURL string
}

// LoadFromClaudeSettings loads default configuration from ~/.claude/settings.json
func LoadFromClaudeSettings() *DefaultConfig {
	config := &DefaultConfig{}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return config
	}

	settingsPath := filepath.Join(homeDir, ".claude", "settings.json")
	data, err := os.ReadFile(settingsPath)
	if err != nil {
		return config
	}

	var settings ClaudeSettings
	if err := json.Unmarshal(data, &settings); err != nil {
		return config
	}

	// Load ANTHROPIC_AUTH_TOKEN as API key (claude code uses this)
	if key, ok := settings.Env["ANTHROPIC_AUTH_TOKEN"]; ok && key != "" {
		config.AnthropicAPIKey = key
	}

	// Also check ANTHROPIC_API_KEY as fallback
	if config.AnthropicAPIKey == "" {
		if key, ok := settings.Env["ANTHROPIC_API_KEY"]; ok && key != "" {
			config.AnthropicAPIKey = key
		}
	}

	// Load base URL if available
	if url, ok := settings.Env["ANTHROPIC_BASE_URL"]; ok && url != "" {
		config.AnthropicBaseURL = url
	}

	return config
}
