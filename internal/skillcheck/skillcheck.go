// Package skillcheck keeps the --skill manuals (Missionbase #464) in step with
// the CLIs: it finds every command a manual names and checks it against the
// binary's own help. The CLIs' tests run it, so renaming or removing a command
// or flag fails until the manual follows.
package skillcheck

import (
	"fmt"
	"regexp"
	"strings"
)

// Ref is one command the manual names: `<cli> words... args --flags`.
type Ref struct {
	Text  string   // the command as written, after the CLI name
	Words []string // the command path, e.g. ["task", "ask"]
	Flags []string // every --flag in it
}

var (
	inlineCode = regexp.MustCompile("`([^`\n]+)`")
	word       = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
	flag       = regexp.MustCompile(`(?:^|[\s\[(|])(--[a-z][a-z0-9-]*)`)
)

// Refs returns every command the manual names: inline code spans and lines in
// fenced blocks that start with cli followed by a space. A " # comment" at the
// end of a fenced line is dropped.
func Refs(manual, cli string) []Ref {
	var snippets []string
	inFence := false
	for _, line := range strings.Split(manual, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			if i := strings.Index(trimmed, " # "); i >= 0 {
				trimmed = trimmed[:i]
			}
			snippets = append(snippets, trimmed)
			continue
		}
		for _, m := range inlineCode.FindAllStringSubmatch(line, -1) {
			snippets = append(snippets, m[1])
		}
	}
	// Inline spans can wrap across lines in Markdown; join the manual's
	// paragraphs too so a span broken by a newline is still found.
	for _, m := range regexp.MustCompile("`([^`]+)`").FindAllStringSubmatch(manual, -1) {
		if strings.Contains(m[1], "\n") {
			snippets = append(snippets, strings.Join(strings.Fields(m[1]), " "))
		}
	}

	var refs []Ref
	for _, s := range snippets {
		s = strings.TrimSpace(s)
		if !strings.HasPrefix(s, cli+" ") {
			continue
		}
		rest := strings.TrimSpace(strings.TrimPrefix(s, cli))
		ref := Ref{Text: rest}
		for _, token := range strings.Fields(rest) {
			if !word.MatchString(token) {
				break
			}
			ref.Words = append(ref.Words, token)
		}
		for _, m := range flag.FindAllStringSubmatch(" "+rest, -1) {
			ref.Flags = append(ref.Flags, m[1])
		}
		refs = append(refs, ref)
	}
	return refs
}

type entry struct {
	paths [][]string
	text  string
}

// entries splits the help's command list into one entry per command line,
// with its continuation lines. A command line is indented two spaces and
// starts with a lowercase word or --flag; deeper-indented lines continue it.
func entries(help string) []entry {
	var out []entry
	for _, line := range strings.Split(help, "\n") {
		if strings.HasPrefix(line, "  ") && !strings.HasPrefix(line, "   ") {
			body := strings.TrimSpace(line)
			if body != "" && (body[0] >= 'a' && body[0] <= 'z' || strings.HasPrefix(body, "--")) {
				out = append(out, entry{paths: commandPaths(body), text: body})
				continue
			}
		}
		if len(out) > 0 && strings.HasPrefix(line, "   ") {
			out[len(out)-1].text += "\n" + strings.TrimSpace(line)
		}
	}
	return out
}

// commandPaths reads the leading command words of a help line. "a|b" and
// "a/b" are alternatives and "[word]" is optional, so "project [show]" gives
// both ["project"] and ["project", "show"].
func commandPaths(line string) [][]string {
	paths := [][]string{{}}
	for _, token := range strings.Fields(line) {
		optional := strings.HasPrefix(token, "[") && strings.HasSuffix(token, "]")
		bare := strings.Trim(token, "[]")
		alternatives := strings.FieldsFunc(bare, func(r rune) bool { return r == '|' || r == '/' })
		ok := len(alternatives) > 0
		for _, a := range alternatives {
			if !word.MatchString(a) {
				ok = false
			}
		}
		if !ok {
			break
		}
		var next [][]string
		for _, p := range paths {
			if optional {
				next = append(next, p)
			}
			for _, a := range alternatives {
				next = append(next, append(append([]string{}, p...), a))
			}
		}
		paths = next
	}
	return paths
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func hasPrefix(path, prefix []string) bool {
	return len(path) >= len(prefix) && equal(path[:len(prefix)], prefix)
}

// Check returns one problem per command the manual names that the help
// doesn't list, or that names a flag the help doesn't give that command.
// A reference matches a help command with the same words; failing that, the
// help commands its words are a prefix of ("project watch" covers
// "project watch --off"). A reference with no words (just flags) matches the
// whole help. Flags must appear in the matched entries' text.
func Check(manual, cli, help string) []string {
	all := entries(help)
	var problems []string
	for _, ref := range Refs(manual, cli) {
		var matched []entry
		for _, e := range all {
			for _, p := range e.paths {
				if len(p) > 0 && equal(p, ref.Words) {
					matched = append(matched, e)
					break
				}
			}
		}
		if len(matched) == 0 && len(ref.Words) > 0 {
			for _, e := range all {
				for _, p := range e.paths {
					if hasPrefix(p, ref.Words) {
						matched = append(matched, e)
						break
					}
				}
			}
		}
		if len(matched) == 0 && len(ref.Words) == 0 {
			matched = all
		}
		if len(matched) == 0 {
			problems = append(problems, fmt.Sprintf("`%s %s`: no command %q in --help", cli, ref.Text, strings.Join(ref.Words, " ")))
			continue
		}
		text := ""
		for _, e := range matched {
			text += e.text + "\n"
		}
		for _, f := range ref.Flags {
			if f == "--help" { // every command takes it
				continue
			}
			if !regexp.MustCompile(regexp.QuoteMeta(f) + `\b`).MatchString(text) {
				problems = append(problems, fmt.Sprintf("`%s %s`: --help doesn't give %s to %q", cli, ref.Text, f, strings.Join(ref.Words, " ")))
			}
		}
	}
	return problems
}
