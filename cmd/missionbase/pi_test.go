package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/config"
)

func TestPiConfigurePersistsSSHHost(t *testing.T) {
	configHome := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configHome)
	t.Setenv("MISSIONBASE_PI_HOST", "")

	if err := piConfigure(config.Config{BaseURL: "https://example.test", Token: "token"}, []string{"--host", "daniel@agents"}); err != nil {
		t.Fatalf("piConfigure: %v", err)
	}
	loaded, err := config.LoadUser()
	if err != nil {
		t.Fatal(err)
	}
	if loaded.PiHost != "daniel@agents" {
		t.Fatalf("PiHost = %q", loaded.PiHost)
	}
	if _, err := os.Stat(filepath.Join(configHome, "missionbase", "credentials")); err != nil {
		t.Fatal(err)
	}
}

func TestPiRejectsUnsafeRemoteArgumentsBeforeSSH(t *testing.T) {
	t.Setenv("MISSIONBASE_PI_HOST", "agents")
	if err := piCommand([]string{"--agent", "agent;touch-pwned"}); err == nil {
		t.Fatal("expected unsafe agent value to be rejected")
	}
}
