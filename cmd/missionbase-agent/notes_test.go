package main

import (
	"net/http"
	"strings"
	"testing"
	"time"
)

const testNotesJSON = `{"notes":{"project_id":7,"body":"## Goal\nShip it","versions_count":2,"version":{"id":12,"action":"appended","summary":"appended to Log","author":{"type":"Agent","id":3,"name":"Claude","slug":"claude"},"source":"api","lines_added":1,"lines_removed":0,"created_at":"2026-11-13T15:58:00Z"}}}`

func TestNotesShowPrintsTheMarkdownWithItsVersion(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/projects/7/notes": fixed(200, testNotesJSON),
	})
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "projects", "notes", "show", "7")
	if err != nil {
		t.Fatal(err)
	}
	if want := "<!-- version 12: Claude appended to Log -->\n## Goal\nShip it\n"; out != want {
		t.Fatalf("out = %q, want %q", out, want)
	}
	if len(*requests) != 1 || (*requests)[0].slug != "missionbase-dev" {
		t.Fatalf("requests = %#v", *requests)
	}

	out, err = runCapture(t, "projects", "notes", "show", "7", "--json")
	if err != nil || strings.TrimSpace(out) != testNotesJSON {
		t.Fatalf("--json out = %q, err = %v", out, err)
	}
}

func TestNotesShowSaysWhenThereAreNoNotes(t *testing.T) {
	server, _ := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/projects/7/notes": fixed(200, `{"notes":{"body":"","versions_count":0,"version":null}}`),
	})
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "boxes", "notes", "show", "7")
	if err != nil || out != "(no notes yet)\n" {
		t.Fatalf("out = %q, err = %v", out, err)
	}
}

func TestNotesUseTheProjectFileWhenNoIDIsGiven(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/projects/5/notes":          fixed(200, testNotesJSON),
		"GET /api/v1/projects/9/notes/versions": fixed(200, `{"versions":[]}`),
	})
	setAgentEnv(t, server.URL)
	root := t.TempDir()
	writeTestProjectFile(t, root, `{"project_id":5,"project_name":"Missionbase"}`)

	if _, err := runCapture(t, "projects", "notes", "show", "--cwd", root); err != nil {
		t.Fatal(err)
	}
	out, err := runCapture(t, "projects", "notes", "history", "--project", "9")
	if err != nil || out != "No versions yet.\n" {
		t.Fatalf("history out = %q, err = %v", out, err)
	}
	if (*requests)[0].path != "/api/v1/projects/5/notes" || (*requests)[1].path != "/api/v1/projects/9/notes/versions" {
		t.Fatalf("requests = %#v", *requests)
	}
}

func TestNotesWithoutAProjectExitWithCode5(t *testing.T) {
	setAgentEnv(t, "http://127.0.0.1:1")

	_, err := runCapture(t, "projects", "notes", "show", "--cwd", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "no project here") {
		t.Fatalf("err = %v", err)
	}
	if code := exitCodeFor(err); code != exitProjectUnresolved {
		t.Fatalf("exit code = %d, want %d", code, exitProjectUnresolved)
	}
}

func TestNotesEditSendsTheBodyAndBaseVersion(t *testing.T) {
	bodyFile := writeTextFile(t, "notes.md", "## Now\n- Testing\n")
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"PATCH /api/v1/projects/7/notes": fixed(200, `{"notes":{"body":"## Now\n- Testing","version":{"id":13,"action":"edited","summary":"edited Now","lines_added":2,"lines_removed":1}}}`),
	})
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "projects", "notes", "edit", "7", "--body-file", bodyFile, "--base-version", "12")
	if err != nil {
		t.Fatal(err)
	}
	if out != "Saved version 13: edited Now (+2 -1)\n" {
		t.Fatalf("out = %q", out)
	}
	payload := (*requests)[0].payload
	if payload["body"] != "## Now\n- Testing\n" || payload["base_version_id"] != 12.0 {
		t.Fatalf("payload = %#v", payload)
	}
}

func TestNotesEditWithoutBaseVersionReplaces(t *testing.T) {
	bodyFile := writeTextFile(t, "notes.md", "Replaced")
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"PATCH /api/v1/projects/7/notes": fixed(200, testNotesJSON),
	})
	setAgentEnv(t, server.URL)

	if _, err := runCapture(t, "projects", "notes", "edit", "7", "--body-file", bodyFile); err != nil {
		t.Fatal(err)
	}
	if _, ok := (*requests)[0].payload["base_version_id"]; ok {
		t.Fatalf("payload = %#v, want no base_version_id", (*requests)[0].payload)
	}
}

func TestNotesEditConflictExitsWithCode3(t *testing.T) {
	bodyFile := writeTextFile(t, "notes.md", "Mine")
	server, _ := recordServer(t, map[string]func(*http.Request) (int, string){
		"PATCH /api/v1/projects/7/notes": fixed(409, `{"error":{"code":"conflict","message":"The notes changed since version 12: edited Now.","details":{}}}`),
	})
	setAgentEnv(t, server.URL)

	_, err := runCapture(t, "projects", "notes", "edit", "7", "--body-file", bodyFile, "--base-version", "12")
	if err == nil || !strings.Contains(err.Error(), "changed since version 12") {
		t.Fatalf("err = %v", err)
	}
	if code := exitCodeFor(err); code != exitConflict {
		t.Fatalf("exit code = %d, want %d", code, exitConflict)
	}
}

func TestNotesAppendPostsAnEntryDatedToday(t *testing.T) {
	bodyFile := writeTextFile(t, "entry.md", "Merged the check-in form.")
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"POST /api/v1/projects/7/notes/entries": fixed(201, `{"notes":{"body":"x"},"entry":{"id":14,"action":"appended","summary":"appended to Log","lines_added":3,"lines_removed":0}}`),
	})
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "projects", "notes", "append", "7", "--body-file", bodyFile)
	if err != nil {
		t.Fatal(err)
	}
	if out != "Saved version 14: appended to Log (+3 -0)\n" {
		t.Fatalf("out = %q", out)
	}
	payload := (*requests)[0].payload
	if payload["body"] != "Merged the check-in form." || payload["date"] != time.Now().Format("2006-01-02") {
		t.Fatalf("payload = %#v", payload)
	}

	if _, err := runCapture(t, "projects", "notes", "append", "7", "--body-file", bodyFile, "--date", "2026-11-12"); err != nil {
		t.Fatal(err)
	}
	if (*requests)[1].payload["date"] != "2026-11-12" {
		t.Fatalf("payload = %#v", (*requests)[1].payload)
	}
}

func TestNotesHistoryListsVersionsAndPaging(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/projects/7/notes/versions": fixed(200, `{"versions":[`+
			`{"id":12,"action":"appended","summary":"appended to Log","author":{"name":"Claude"},"lines_added":1,"lines_removed":0,"created_at":"2026-11-13T15:58:00Z"},`+
			`{"id":3,"action":"imported","summary":"imported the project description","author":null,"lines_added":4,"lines_removed":0,"created_at":"2026-11-01T09:00:00Z"}],"next_before":3}`),
	})
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "projects", "notes", "history", "7", "--limit", "2", "--before", "20")
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != 3 ||
		!strings.HasPrefix(lines[0], "12  ") || !strings.HasSuffix(lines[0], "Claude appended to Log  +1 -0") ||
		!strings.HasSuffix(lines[1], "  imported the project description  +4 -0") ||
		lines[2] != "Older versions: --before 3" {
		t.Fatalf("out = %q", out)
	}
	if (*requests)[0].query != "before=20&limit=2" {
		t.Fatalf("query = %q", (*requests)[0].query)
	}
}

func TestNotesArgumentErrors(t *testing.T) {
	setAgentEnv(t, "http://127.0.0.1:1")
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"projects", "notes", "publish", "7"}, `unknown projects notes command "publish"`},
		{[]string{"projects", "notes", "edit", "7"}, "--body-file is required"},
		{[]string{"projects", "notes", "append", "7"}, "--body-file is required"},
		{[]string{"projects", "notes", "show", "abc"}, "project id must be a number"},
		{[]string{"projects", "notes", "show", "7", "8"}, `unexpected argument "8"`},
		{[]string{"projects", "notes", "show", "7", "--body-file", "x"}, `unknown projects notes show option "--body-file"`},
		{[]string{"projects", "notes", "append", "7", "--body-file", "x", "--date", "13/11/2026"}, "--date must be YYYY-MM-DD"},
		{[]string{"projects", "notes", "edit", "7", "--body-file", "x", "--base-version", "latest"}, "--base-version must be a version id"},
		{[]string{"projects", "notes", "history", "7", "--limit"}, "--limit requires a value"},
		{[]string{"projects", "notes", "show", "7", "--project", "8"}, "differ"},
		{[]string{"projects", "notes", "append", "7", "--body-file", "/no/such/file.md"}, "read body file"},
	} {
		if _, err := runCapture(t, tc.args...); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%v: err = %v, want %q", tc.args, err, tc.want)
		}
	}

	out, err := runCapture(t, "projects", "notes", "--help")
	if err != nil || !strings.Contains(out, "projects notes append [project-id] --body-file PATH") {
		t.Fatalf("help out = %q, err = %v", out, err)
	}
}
