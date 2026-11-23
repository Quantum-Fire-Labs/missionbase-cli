package main

import (
	_ "embed"
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"
)

// skillManual is the agent workflow manual printed by --skill (Missionbase
// #464). It ships with the binary so it always matches its commands; the
// tests check that every command and flag it names is in printHelp.
//
//go:embed skill.md
var skillManual string

var wrapperSlug = regexp.MustCompile(`^[a-z0-9][a-z0-9_-]*$`)

// skillCLIName is the command the manual tells the agent to run. The wrappers
// (missionbase-claude, missionbase-codex, missionbase-prime) exec this binary
// with MISSIONBASE_AGENT_SLUG set, so when missionbase-<slug> is on PATH the
// manual names the wrapper; otherwise it names missionbase-agent.
func skillCLIName() string {
	slug := strings.TrimSpace(os.Getenv("MISSIONBASE_AGENT_SLUG"))
	if slug != "" && wrapperSlug.MatchString(slug) {
		name := "missionbase-" + slug
		if _, err := exec.LookPath(name); err == nil {
			return name
		}
	}
	return "missionbase-agent"
}

func printSkill() {
	fmt.Print(strings.ReplaceAll(skillManual, "{{cli}}", skillCLIName()))
}
