package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

const defaultBaseURL = "https://dash.missionbase.app"

type Config struct {
	BaseURL   string `json:"base_url"`
	Token     string `json:"token"`
	AgentSlug string `json:"agent_slug,omitempty"`
}

func LoadUser() (Config, error) {
	cfg, err := loadFromPath(CredentialsPath("missionbase"))
	if err != nil {
		return cfg, err
	}
	applyUserEnv(&cfg)
	cfg.AgentSlug = ""
	return cfg, nil
}

func LoadAgent() (Config, error) {
	cfg, err := loadFromPath(CredentialsPath("missionbase-agent"))
	if err != nil {
		return cfg, err
	}
	local, err := LoadLocalAgentConfig()
	if err != nil {
		return cfg, err
	}
	if local.BaseURL != "" {
		cfg.BaseURL = local.BaseURL
	}
	if local.AgentSlug != "" {
		cfg.AgentSlug = local.AgentSlug
	}
	applyAgentEnv(&cfg)
	return cfg, nil
}

func Load() (Config, error) {
	return LoadUser()
}

func SaveUser(cfg Config) error {
	return SaveToPath(cfg, CredentialsPath("missionbase"))
}

func SaveAgent(cfg Config) error {
	return SaveToPath(cfg, CredentialsPath("missionbase-agent"))
}

func Save(cfg Config) error {
	return SaveUser(cfg)
}

func loadFromPath(path string) (Config, error) {
	cfg := Config{BaseURL: defaultBaseURL}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		// Older Missionbase CLI credentials were stored as a plain token.
		// Preserve compatibility so existing agent boxes keep working after upgrading.
		cfg.Token = strings.TrimSpace(string(data))
		return cfg, nil
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	return cfg, nil
}

func SaveToPath(cfg Config, path string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func CredentialsPath(app string) string {
	if app == "missionbase-agent" {
		if path := os.Getenv("MISSIONBASE_AGENT_CREDENTIALS"); path != "" {
			return path
		}
	} else if path := os.Getenv("MISSIONBASE_CREDENTIALS"); path != "" {
		return path
	}

	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "." + app + "-credentials"
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, app, "credentials")
}

func LocalAgentConfigPath() (string, bool) {
	if path := os.Getenv("MISSIONBASE_AGENT_CONFIG"); path != "" {
		return path, true
	}
	dir, err := os.Getwd()
	if err != nil {
		return "", false
	}
	for {
		path := filepath.Join(dir, ".missionbase-agent.json")
		if _, err := os.Stat(path); err == nil {
			return path, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func LoadLocalAgentConfig() (Config, error) {
	var cfg Config
	path, ok := LocalAgentConfigPath()
	if !ok {
		return cfg, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

func SaveLocalAgentConfig(cfg Config) (string, error) {
	path := filepath.Join(mustGetwd(), ".missionbase-agent.json")
	data, err := json.MarshalIndent(Config{BaseURL: cfg.BaseURL, AgentSlug: cfg.AgentSlug}, "", "  ")
	if err != nil {
		return path, err
	}
	return path, os.WriteFile(path, append(data, '\n'), 0o600)
}

func mustGetwd() string {
	dir, err := os.Getwd()
	if err != nil {
		return "."
	}
	return dir
}

func applyUserEnv(cfg *Config) {
	if baseURL := os.Getenv("MISSIONBASE_BASE_URL"); baseURL != "" {
		cfg.BaseURL = baseURL
	}
	if token := os.Getenv("MISSIONBASE_TOKEN"); token != "" {
		cfg.Token = token
	}
}

func applyAgentEnv(cfg *Config) {
	applyUserEnv(cfg)
	if agentSlug := os.Getenv("MISSIONBASE_AGENT_SLUG"); agentSlug != "" {
		cfg.AgentSlug = agentSlug
	}
}
