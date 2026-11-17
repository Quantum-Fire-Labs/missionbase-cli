package main

import (
	"os"
	"path/filepath"
	"reflect"
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

func TestPiInvocationForwardsOneShotAndTask(t *testing.T) {
	host, remote, showHelp, err := piInvocation("agents", []string{"--one-shot", "--task", "3086"})
	if err != nil {
		t.Fatal(err)
	}
	if host != "agents" {
		t.Fatalf("host = %q", host)
	}
	if showHelp {
		t.Fatal("showHelp = true")
	}
	want := []string{"sudo", "-n", "/opt/missionbase-runner/bin/mb-pi", "--one-shot", "--task", "3086"}
	if !reflect.DeepEqual(remote, want) {
		t.Fatalf("remote = %#v, want %#v", remote, want)
	}
}

func TestPiInvocationRejectsTeam(t *testing.T) {
	_, _, _, err := piInvocation("agents", []string{"--team", "Quantum Fire Labs"})
	if err == nil || err.Error() != `unknown pi option "--team"` {
		t.Fatalf("err = %v, want unknown --team option", err)
	}
}
