package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParsePiLaunchArgsRequiresExplicitAgentAndSeparator(t *testing.T) {
	slug, args, err := parsePiLaunchArgs([]string{"--agent", "missionbase-dev", "--", "--model", "gpt-5.6", "Continue work"})
	if err != nil {
		t.Fatal(err)
	}
	if slug != "missionbase-dev" {
		t.Fatalf("slug = %q", slug)
	}
	want := []string{"--model", "gpt-5.6", "Continue work"}
	if !reflect.DeepEqual(args, want) {
		t.Fatalf("args = %#v, want %#v", args, want)
	}

	if _, _, err := parsePiLaunchArgs(nil); err == nil || !strings.Contains(err.Error(), "--agent is required") {
		t.Fatalf("missing agent error = %v", err)
	}
	if _, _, err := parsePiLaunchArgs([]string{"--agent", "missionbase-dev", "--model", "gpt-5.6"}); err == nil || !strings.Contains(err.Error(), "after --") {
		t.Fatalf("unseparated Pi args error = %v", err)
	}
}

func TestPiPreflightsAgentAndLaunchesWithLockedIdentity(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/me" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "missionbase-dev" {
			t.Fatalf("agent header = %q", got)
		}
		_, _ = w.Write([]byte(`{"agent":{"id":7,"slug":"missionbase-dev","name":"Missionbase Dev","status":"active"}}`))
	}))
	defer server.Close()

	capturePath := filepath.Join(t.TempDir(), "pi-launch.txt")
	binDir := t.TempDir()
	piPath := filepath.Join(binDir, "pi")
	script := `#!/usr/bin/env bash
set -eu
{
  printf 'mode=%s\n' "$MISSIONBASE_ACTOR_MODE"
  printf 'slug=%s\n' "$MISSIONBASE_AGENT_SLUG"
  printf 'arg=%s\n' "$@"
} > "$PI_CAPTURE_PATH"
`
	if err := os.WriteFile(piPath, []byte(script), 0o700); err != nil {
		t.Fatal(err)
	}

	t.Setenv("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("PI_CAPTURE_PATH", capturePath)
	t.Setenv("MISSIONBASE_BASE_URL", server.URL)
	t.Setenv("MISSIONBASE_TOKEN", "team-token")
	t.Setenv("MISSIONBASE_AGENT_SLUG", "wrong-agent")

	if err := pi([]string{"--agent", "missionbase-dev", "--", "--name", "Local work"}); err != nil {
		t.Fatal(err)
	}

	captured, err := os.ReadFile(capturePath)
	if err != nil {
		t.Fatal(err)
	}
	output := string(captured)
	for _, expected := range []string{
		"mode=agent",
		"slug=missionbase-dev",
		"arg=--append-system-prompt",
		"identity for this Pi process is agent \"missionbase-dev\"",
		"arg=--name",
		"arg=Local work",
	} {
		if !strings.Contains(output, expected) {
			t.Fatalf("capture missing %q:\n%s", expected, output)
		}
	}
}

func TestPiAgentsListsWithoutAgentHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agents" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "" {
			t.Fatalf("unexpected agent header = %q", got)
		}
		_, _ = w.Write([]byte(`{"agents":[{"id":7,"slug":"missionbase-dev","name":"Missionbase Dev","title":"Rails Developer","status":"active"}],"meta":{"total":1}}`))
	}))
	defer server.Close()

	t.Setenv("MISSIONBASE_BASE_URL", server.URL)
	t.Setenv("MISSIONBASE_TOKEN", "team-token")
	t.Setenv("MISSIONBASE_AGENT_SLUG", "configured-agent")

	output := captureStdout(t, func() {
		if err := pi([]string{"agents"}); err != nil {
			t.Fatal(err)
		}
	})
	for _, expected := range []string{"SLUG", "NAME", "TITLE", "missionbase-dev", "Missionbase Dev", "Rails Developer"} {
		if !strings.Contains(output, expected) {
			t.Fatalf("output missing %q:\n%s", expected, output)
		}
	}
}
