package skillcheck

import (
	"strings"
	"testing"
)

const help = `Usage:
  tool <command> [args]

Commands:
  --skill                             Print the manual
  task ask <task-id> "question" [--option TEXT]... [--json]
  task ask <task-id> --file PATH [--json]
                                      Ask questions
  task create --title TITLE [--project ID]
      [--start] [--terminal tN]       Create a task
  project [show] [--cwd DIR | --project ID]
  project watch --terminal tN
  project watch --off
  notes daily show|list [DATE]
  conversation show/message           Deprecated aliases
`

func TestCheckAcceptsCommandsInHelp(t *testing.T) {
	manual := "Run `tool task ask <id> \"Q?\" --option Yes` or:\n\n" +
		"```sh\n" +
		"tool task ask <id> --file q.json      # --bogus in a comment is ignored\n" +
		"tool task create --title \"x\" --project 5 --start --terminal t1\n" +
		"tool project\n" +
		"tool project show --cwd .\n" +
		"tool project watch --off\n" +
		"tool notes daily list\n" +
		"tool conversation message 4\n" +
		"```\n\n" +
		"Also `tool project watch`, `tool --skill`, `tool task ask --help`, and `other task nope`.\n"
	if problems := Check(manual, "tool", help); len(problems) != 0 {
		t.Fatalf("unexpected problems:\n%s", strings.Join(problems, "\n"))
	}
	if got := len(Refs(manual, "tool")); got != 11 {
		t.Fatalf("Refs found %d commands, want 11", got)
	}
}

func TestCheckReportsMissingCommandsAndFlags(t *testing.T) {
	manual := "- `tool task review <id> --check LINE`\n" +
		"- `tool task ask <id> --wait`\n" +
		"- `tool project watch --terminal tN --force`\n" +
		"- `tool notes daily edit`\n" +
		"- `tool task\n  create --assign-agent x`\n"
	problems := Check(manual, "tool", help)
	want := []string{
		`no command "task review"`,
		`doesn't give --wait to "task ask"`,
		`doesn't give --force to "project watch"`,
		`no command "notes daily edit"`,
		`doesn't give --assign-agent to "task create"`,
	}
	if len(problems) != len(want) {
		t.Fatalf("problems = %d, want %d:\n%s", len(problems), len(want), strings.Join(problems, "\n"))
	}
	for i, w := range want {
		if !strings.Contains(problems[i], w) {
			t.Errorf("problem %d = %q, want it to mention %q", i, problems[i], w)
		}
	}
}
