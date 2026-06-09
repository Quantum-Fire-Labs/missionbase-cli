package config

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

const defaultBaseURL = "https://dash.missionbase.app"

type Config struct {
	BaseURL string `json:"base_url"`
	Token   string `json:"token"`
}

func Load() (Config, error) {
	cfg := Config{BaseURL: defaultBaseURL}
	path := CredentialsPath()
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}
	return cfg, nil
}

func Save(cfg Config) error {
	path := CredentialsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o600)
}

func CredentialsPath() string {
	if path := os.Getenv("MISSIONBASE_CREDENTIALS"); path != "" {
		return path
	}
	configHome := os.Getenv("XDG_CONFIG_HOME")
	if configHome == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return ".missionbase-credentials"
		}
		configHome = filepath.Join(home, ".config")
	}
	return filepath.Join(configHome, "missionbase", "credentials")
}
