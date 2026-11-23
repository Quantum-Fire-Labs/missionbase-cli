package main

import (
	"strings"
	"testing"

	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/skillcheck"
)

func TestSkillPrintsTheManual(t *testing.T) {
	var err error
	stdout := captureStdout(t, func() { err = run([]string{"--skill"}) })
	if err != nil {
		t.Fatalf("--skill: %v", err)
	}
	if !strings.HasPrefix(stdout, "# missionbase: Missionbase as the person") {
		t.Fatalf("--skill printed:\n%s", stdout)
	}
	for _, want := range []string{"## Notes", "## Activity", "missionbase notes daily append"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("--skill output lacks %q", want)
		}
	}
}

// A Pi session locked to an agent refuses user commands, but the manual is
// local information like --help.
func TestSkillWorksInAgentLockedSession(t *testing.T) {
	t.Setenv("MISSIONBASE_ACTOR_MODE", "agent")
	t.Setenv("MISSIONBASE_AGENT_SLUG", "claude")
	var err error
	_ = captureStdout(t, func() { err = run([]string{"--skill"}) })
	if err != nil {
		t.Fatalf("--skill in an agent-locked session: %v", err)
	}
}

func TestSkillCommandsExistInHelp(t *testing.T) {
	refs := skillcheck.Refs(skillManual, "missionbase")
	if len(refs) < 30 {
		t.Fatalf("found only %d commands in the manual", len(refs))
	}
	help := captureStdout(t, func() { printHelp() })
	for _, problem := range skillcheck.Check(skillManual, "missionbase", help) {
		t.Error(problem)
	}
}

func TestHelpMentionsSkill(t *testing.T) {
	help := captureStdout(t, func() { printHelp() })
	if !strings.Contains(help, "  --skill ") {
		t.Fatalf("help doesn't list --skill:\n%s", help)
	}
}
