package main

import (
	_ "embed"
	"fmt"
)

// skillManual is the manual printed by --skill (Missionbase #464): the user
// CLI as an agent acting for the person. It ships with the binary so it always
// matches its commands; the tests check that every command and flag it names
// is in printHelp.
//
//go:embed skill.md
var skillManual string

func printSkill() {
	fmt.Print(skillManual)
}
