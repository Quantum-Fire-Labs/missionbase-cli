package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/skillcheck"
)

func TestSkillPrintsTheManual(t *testing.T) {
	t.Setenv("MISSIONBASE_AGENT_SLUG", "")
	var err error
	stdout := captureStdout(t, func() { err = run([]string{"--skill"}) })
	if err != nil {
		t.Fatalf("--skill: %v", err)
	}
	if !strings.HasPrefix(stdout, "# missionbase-agent: track your work in Missionbase") {
		t.Fatalf("--skill printed:\n%s", stdout)
	}
	for _, want := range []string{"## Statuses", "## Questions", "## Task chat", "missionbase-agent task review <id> --item"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("--skill output lacks %q", want)
		}
	}
	if strings.Contains(stdout, "{{") {
		t.Fatalf("--skill left a placeholder:\n%s", stdout)
	}
}

// The wrappers exec missionbase-agent with MISSIONBASE_AGENT_SLUG set; the
// manual then names the wrapper when it's on PATH.
func TestSkillNamesTheWrapperOnPath(t *testing.T) {
	dir := t.TempDir()
	wrapper := filepath.Join(dir, "missionbase-claude")
	if err := os.WriteFile(wrapper, []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)

	t.Setenv("MISSIONBASE_AGENT_SLUG", "claude")
	stdout := captureStdout(t, func() { _ = run([]string{"--skill"}) })
	if !strings.Contains(stdout, "missionbase-claude task start <id> --terminal $HC_TERMINAL_ID") || strings.Contains(stdout, "missionbase-agent task") {
		t.Fatalf("with the claude wrapper on PATH, --skill should name missionbase-claude:\n%s", stdout)
	}

	t.Setenv("MISSIONBASE_AGENT_SLUG", "codex") // no missionbase-codex on PATH
	stdout = captureStdout(t, func() { _ = run([]string{"--skill"}) })
	if !strings.Contains(stdout, "missionbase-agent task start <id>") {
		t.Fatalf("without a wrapper on PATH, --skill should name missionbase-agent:\n%s", stdout)
	}

	t.Setenv("MISSIONBASE_AGENT_SLUG", "../claude")
	if got := skillCLIName(); got != "missionbase-agent" {
		t.Fatalf("skillCLIName() with a path-like slug = %q", got)
	}
}

func TestSkillCommandsExistInHelp(t *testing.T) {
	refs := skillcheck.Refs(skillManual, "{{cli}}")
	if len(refs) < 30 {
		t.Fatalf("found only %d commands in the manual; is the {{cli}} placeholder still used?", len(refs))
	}
	help := captureStdout(t, func() { printHelp() })
	for _, problem := range skillcheck.Check(skillManual, "{{cli}}", help) {
		t.Error(problem)
	}
}

func TestHelpMentionsSkill(t *testing.T) {
	help := captureStdout(t, func() { printHelp() })
	if !strings.Contains(help, "  --skill ") {
		t.Fatalf("help doesn't list --skill:\n%s", help)
	}
}
