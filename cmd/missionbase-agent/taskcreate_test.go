package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testCreatedTaskJSON = `{"task":{"id":77,"title":"New work","status":"todo","status_label":"To Do","project":{"id":5,"name":"Missionbase"}}}`

// ---- --json on the older commands ----

func TestTaskCreateJSONPrintsTheCreatedTaskEnvelope(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"POST /api/v1/tasks": fixed(201, testCreatedTaskJSON),
	})
	setAgentEnv(t, server.URL)

	plain, err := runCapture(t, "task", "create", "--title", "New work", "--project", "5")
	if err != nil {
		t.Fatal(err)
	}
	withJSON, err := runCapture(t, "task", "create", "--json", "--title", "New work", "--project", "5")
	if err != nil {
		t.Fatal(err)
	}
	if plain != withJSON {
		t.Fatalf("output differs with --json:\n%s\n%s", plain, withJSON)
	}
	var envelope struct {
		Task struct {
			ID int64 `json:"id"`
		} `json:"task"`
	}
	if err := json.Unmarshal([]byte(withJSON), &envelope); err != nil || envelope.Task.ID != 77 {
		t.Fatalf("output = %q (err %v), want the task envelope with .task.id 77", withJSON, err)
	}
	for _, req := range *requests {
		if _, ok := req.payload["json"]; ok {
			t.Fatalf("payload = %#v, --json must not be sent", req.payload)
		}
	}
}

func TestOlderTaskCommandsAcceptJSON(t *testing.T) {
	cases := []struct {
		args  []string
		route string
	}{
		{[]string{"task", "show", "42", "--json"}, "GET /api/v1/tasks/42"},
		{[]string{"task", "update", "42", "--title", "Renamed", "--json"}, "PATCH /api/v1/tasks/42"},
		{[]string{"task", "status", "42", "todo", "--json"}, "PATCH /api/v1/tasks/42"},
		{[]string{"task", "status", "--json", "42", "complete"}, "PATCH /api/v1/tasks/42/complete"},
		{[]string{"task", "complete", "42", "--json"}, "PATCH /api/v1/tasks/42/complete"},
		{[]string{"task", "assign", "42", "--agent", "claude", "--json"}, "POST /api/v1/tasks/42/assignments"},
		{[]string{"task", "unassign", "42", "--user", "7", "--json"}, "DELETE /api/v1/tasks/42/assignments/7"},
		{[]string{"task", "move", "42", "--project", "9", "--json"}, "PATCH /api/v1/tasks/42"},
		{[]string{"task", "messages", "42", "--json"}, "GET /api/v1/tasks/42/comments"},
		{[]string{"task", "participants", "list", "42", "--json"}, "GET /api/v1/tasks/42/participants"},
		{[]string{"task", "participants", "add", "42", "--agent", "claude", "--json"}, "POST /api/v1/tasks/42/participants"},
		{[]string{"projects", "create", "--name", "Delivery", "--team", "1", "--json"}, "POST /api/v1/projects"},
	}
	for _, tc := range cases {
		t.Run(strings.Join(tc.args, " "), func(t *testing.T) {
			server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
				tc.route: fixed(200, `{"task":{"id":42}}`),
			})
			setAgentEnv(t, server.URL)
			out, err := runCapture(t, tc.args...)
			if err != nil {
				t.Fatal(err)
			}
			if len(*requests) != 1 || strings.TrimSpace(out) != `{"task":{"id":42}}` {
				t.Fatalf("out = %q, requests = %#v", out, *requests)
			}
			if _, ok := (*requests)[0].payload["json"]; ok {
				t.Fatalf("payload = %#v, --json must not be sent", (*requests)[0].payload)
			}
		})
	}
}

func TestTaskMessageAcceptsJSON(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"POST /api/v1/tasks/42/comments": fixed(201, `{"comment":{"id":5}}`),
	})
	setAgentEnv(t, server.URL)
	body := writeTextFile(t, "message.md", "Done.")

	out, err := runCapture(t, "task", "message", "42", "--body-file", body, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != `{"comment":{"id":5}}` || (*requests)[0].payload["message"] != "Done." {
		t.Fatalf("out = %q, requests = %#v", out, *requests)
	}
}

// ---- task create --status ----

func TestTaskCreateStatusSendsTheKey(t *testing.T) {
	for _, status := range []string{"backlog", "planning", "in_progress", "not_doing"} {
		server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
			"POST /api/v1/tasks": fixed(201, testCreatedTaskJSON),
		})
		setAgentEnv(t, server.URL)
		if _, err := runCapture(t, "task", "create", "--title", "New work", "--project", "5", "--status", status); err != nil {
			t.Fatalf("%s: %v", status, err)
		}
		if got := (*requests)[0].payload["status"]; got != status {
			t.Fatalf("status = %#v, want %q", got, status)
		}
	}
}

func TestTaskCreateWithoutStatusSendsNone(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"POST /api/v1/tasks": fixed(201, testCreatedTaskJSON),
	})
	setAgentEnv(t, server.URL)
	if _, err := runCapture(t, "task", "create", "--title", "New work", "--project", "5"); err != nil {
		t.Fatal(err)
	}
	if _, ok := (*requests)[0].payload["status"]; ok {
		t.Fatalf("payload = %#v, want no status", (*requests)[0].payload)
	}
}

func TestTaskCreateStatusLeavesUnknownKeysToTheServer(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"POST /api/v1/tasks": fixed(422, `{"error":{"code":"validation_failed","message":"status must be one of: backlog, planning, todo, in_progress, complete, not_doing"}}`),
	})
	setAgentEnv(t, server.URL)
	_, err := runCapture(t, "task", "create", "--title", "New work", "--project", "5", "--status", "someday")
	if err == nil || !strings.Contains(err.Error(), "status must be one of") || exitCodeFor(err) != 1 {
		t.Fatalf("err = %v (exit %d), want the server's validation error", err, exitCodeFor(err))
	}
	if len(*requests) != 1 || (*requests)[0].payload["status"] != "someday" {
		t.Fatalf("requests = %#v", *requests)
	}
}

func TestTaskCreateStatusRefusesNeedsAttentionAndBlankKeys(t *testing.T) {
	setAgentEnv(t, "http://127.0.0.1:1")
	_, err := runCapture(t, "task", "create", "--title", "New work", "--project", "5", "--status", "needs_attention")
	if err == nil || !strings.Contains(err.Error(), "task ask") {
		t.Fatalf("err = %v, want the needs_attention pointer to task ask", err)
	}
	for _, args := range [][]string{{"--status"}, {"--status", " "}} {
		_, err := runCapture(t, append([]string{"task", "create", "--title", "New work", "--project", "5"}, args...)...)
		if err == nil || !strings.Contains(err.Error(), "--status requires a status key: backlog, planning, todo") {
			t.Fatalf("%v: err = %v", args, err)
		}
	}
}

// ---- task create: project resolution ----

func TestTaskCreateResolvesTheProjectLikeTasksLs(t *testing.T) {
	linked := t.TempDir()
	writeTestProjectFile(t, linked, `{"project_id":12,"project_name":"Other repo","box_id":12}`)
	nested := filepath.Join(linked, "app", "models")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		name  string
		args  []string
		chdir string
		want  string
	}{
		{"--cwd finds the nearest project file", []string{"--cwd", nested}, "", "12"},
		{"the current folder's project file", nil, nested, "12"},
		{"--project wins", []string{"--project", "5"}, nested, "5"},
		{"--box is the old name", []string{"--box", "7"}, "", "7"},
		{"--project-id", []string{"--project-id", "8"}, "", "8"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
				"POST /api/v1/tasks": fixed(201, testCreatedTaskJSON),
			})
			setAgentEnv(t, server.URL)
			if tc.chdir != "" {
				t.Chdir(tc.chdir)
			}
			if _, err := runCapture(t, append([]string{"task", "create", "--title", "New work"}, tc.args...)...); err != nil {
				t.Fatal(err)
			}
			payload := (*requests)[0].payload
			if payload["project_id"] != tc.want || payload["box_id"] != tc.want {
				t.Fatalf("payload = %#v, want project_id and box_id %s", payload, tc.want)
			}
		})
	}
}

func TestTaskCreateUsesTheMachineDefaultProject(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"POST /api/v1/tasks": fixed(201, testCreatedTaskJSON),
	})
	setAgentEnv(t, server.URL)
	if _, err := runCapture(t, "project", "default", "9"); err != nil {
		t.Fatal(err)
	}
	if _, err := runCapture(t, "task", "create", "--title", "New work", "--cwd", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if got := (*requests)[0].payload["project_id"]; got != "9" {
		t.Fatalf("project_id = %#v, want the default 9", got)
	}
}

func TestTaskCreateWithoutAProjectExits5WithoutSending(t *testing.T) {
	server, requests := recordServer(t, nil)
	setAgentEnv(t, server.URL)
	_, err := runCapture(t, "task", "create", "--title", "New work", "--cwd", t.TempDir())
	if err == nil || exitCodeFor(err) != exitProjectUnresolved || !strings.Contains(err.Error(), "pass --project ID or --cwd DIR") {
		t.Fatalf("err = %v (exit %d), want exit 5", err, exitCodeFor(err))
	}
	if len(*requests) != 0 {
		t.Fatalf("requests = %#v, want none", *requests)
	}
}

func TestTaskCreateRejectsBadProjectArguments(t *testing.T) {
	server, requests := recordServer(t, nil)
	setAgentEnv(t, server.URL)
	cases := map[string][]string{
		"use only one of --cwd or --project":             {"--project", "5", "--cwd", t.TempDir()},
		"--project must be a project id":                 {"--project", "abc"},
		"--project requires a project id":                {"--project"},
		"--cwd requires a directory":                     {"--cwd"},
		"is not a directory":                             {"--cwd", filepath.Join(t.TempDir(), "missing")},
		"--terminal and --force only apply with --start": {"--project", "5", "--terminal", "t7"},
	}
	for want, args := range cases {
		_, err := runCapture(t, append([]string{"task", "create", "--title", "New work"}, args...)...)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%v: err = %v, want %q", args, err, want)
		}
	}
	if _, err := runCapture(t, "task", "create", "--title", "New work", "--project", "5", "--force"); err == nil || !strings.Contains(err.Error(), "only apply with --start") {
		t.Fatalf("--force without --start: err = %v", err)
	}
	if len(*requests) != 0 {
		t.Fatalf("requests = %#v, want none", *requests)
	}
}

// ---- task create --start ----

func TestTaskCreateStartCreatesThenStartsOnTheHCTerminal(t *testing.T) {
	stubHC(t, map[string]string{"server status --json": testLocalCoreJSON})
	workResponse := `{"task":{"id":77,"title":"New work","status":"in_progress","status_label":"In Progress","running":{"agent":{"type":"Agent","slug":"missionbase-dev"},"machine_id":"core-local","machine_name":"omarchy-m3","terminal":"t6b"}},"stopped":[]}`
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"POST /api/v1/tasks":         fixed(201, testCreatedTaskJSON),
		"POST /api/v1/tasks/77/work": fixed(200, workResponse),
	})
	setAgentEnv(t, server.URL)
	t.Setenv("HC_TERMINAL_ID", "t6b")

	var out string
	var err error
	stderr := captureStderr(t, func() {
		out, err = runCapture(t, "task", "create", "--title", "New work", "--project", "5", "--status", "backlog", "--start", "--json")
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(*requests) != 2 || (*requests)[0].path != "/api/v1/tasks" || (*requests)[1].path != "/api/v1/tasks/77/work" {
		t.Fatalf("requests = %#v, want create then work", *requests)
	}
	create, work := (*requests)[0], (*requests)[1]
	if _, ok := create.payload["status"]; ok {
		t.Fatalf("create payload = %#v, --start must win over --status", create.payload)
	}
	if !strings.Contains(stderr, "--status backlog is ignored with --start") {
		t.Fatalf("stderr = %q", stderr)
	}
	want := map[string]any{"machine_id": "core-local", "machine_name": "omarchy-m3", "terminal": "t6b"}
	if fmt.Sprint(work.payload) != fmt.Sprint(want) || work.slug != "missionbase-dev" {
		t.Fatalf("work payload = %#v slug %q, want %#v", work.payload, work.slug, want)
	}
	if strings.TrimSpace(out) != workResponse {
		t.Fatalf("out = %q, want the task start response", out)
	}
}

func TestTaskCreateStartOnARemoteTerminalWithForce(t *testing.T) {
	stubHC(t, map[string]string{"server status --json": testLocalCoreJSON, "machine ls --json": testMachineLsJSON})
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"POST /api/v1/tasks":         fixed(201, testCreatedTaskJSON),
		"POST /api/v1/tasks/77/work": fixed(200, `{"task":{"id":77},"stopped":[]}`),
	})
	setAgentEnv(t, server.URL)
	t.Setenv("HC_TERMINAL_ID", "")

	if _, err := runCapture(t, "task", "create", "--title", "New work", "--project", "5", "--start", "--terminal", "omarchy-intel/t7", "--force"); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"machine_id": "core-intel", "machine_name": "omarchy-intel", "terminal": "t7", "force": true}
	if got := (*requests)[1].payload; fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("work payload = %#v, want %#v", got, want)
	}
}

func TestTaskCreateStartWithoutATerminalCreatesNothing(t *testing.T) {
	server, requests := recordServer(t, nil)
	setAgentEnv(t, server.URL)
	t.Setenv("HC_TERMINAL_ID", "")

	_, err := runCapture(t, "task", "create", "--title", "New work", "--project", "5", "--start")
	if err == nil || exitCodeFor(err) != exitHCUnavailable || !strings.Contains(err.Error(), "HC_TERMINAL_ID is not set") {
		t.Fatalf("err = %v (exit %d), want exit 4", err, exitCodeFor(err))
	}

	t.Setenv("HC_TERMINAL_ID", "t6b")
	original := runHC
	runHC = func(args ...string) ([]byte, error) { return nil, fmt.Errorf("hc is not installed or not on PATH") }
	t.Cleanup(func() { runHC = original })
	_, err = runCapture(t, "task", "create", "--title", "New work", "--project", "5", "--start")
	if err == nil || exitCodeFor(err) != exitHCUnavailable || !strings.Contains(err.Error(), "hc is not installed") {
		t.Fatalf("err = %v (exit %d), want exit 4", err, exitCodeFor(err))
	}
	if len(*requests) != 0 {
		t.Fatalf("requests = %#v, want none: no task may be created when the terminal can't be resolved", *requests)
	}
}

func TestTaskCreateStartConflictPrintsTheCreatedTaskAndExits3(t *testing.T) {
	stubHC(t, map[string]string{"server status --json": testLocalCoreJSON})
	server, _ := recordServer(t, map[string]func(*http.Request) (int, string){
		"POST /api/v1/tasks":         fixed(201, testCreatedTaskJSON),
		"POST /api/v1/tasks/77/work": fixed(409, testConflictBodyFmt),
	})
	setAgentEnv(t, server.URL)
	t.Setenv("HC_TERMINAL_ID", "t6b")

	out, err := runCapture(t, "task", "create", "--title", "New work", "--project", "5", "--start")
	if err == nil || exitCodeFor(err) != exitConflict {
		t.Fatalf("err = %v (exit %d), want exit 3", err, exitCodeFor(err))
	}
	for _, want := range []string{"created task #77 but did not start it", "running: claude · omarchy-intel t7", "pass --force"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("err = %v, want %q", err, want)
		}
	}
	if strings.TrimSpace(out) != testCreatedTaskJSON {
		t.Fatalf("out = %q, want only the created task's JSON", out)
	}
}

func TestTaskCreateStartFailureKeepsTheExitCodeOfOtherErrors(t *testing.T) {
	stubHC(t, map[string]string{"server status --json": testLocalCoreJSON})
	server, _ := recordServer(t, map[string]func(*http.Request) (int, string){
		"POST /api/v1/tasks":         fixed(201, testCreatedTaskJSON),
		"POST /api/v1/tasks/77/work": fixed(403, `{"error":{"code":"forbidden","message":"Missing tasks:assign permission"}}`),
	})
	setAgentEnv(t, server.URL)
	t.Setenv("HC_TERMINAL_ID", "t6b")

	_, err := runCapture(t, "task", "create", "--title", "New work", "--project", "5", "--start")
	if err == nil || exitCodeFor(err) != 1 || !strings.Contains(err.Error(), "created task #77 but did not start it: Missing tasks:assign permission (HTTP 403 forbidden)") {
		t.Fatalf("err = %v (exit %d)", err, exitCodeFor(err))
	}
}

func TestTaskCreateHelpShowsStartAndProjectResolution(t *testing.T) {
	out, err := runCapture(t, "task", "create", "--help")
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"[--project ID | --cwd DIR]", "[--status KEY]", "--start [--terminal [machine/]tN] [--force] [--json]"} {
		if !strings.Contains(out, want) {
			t.Fatalf("help = %q, want %q", out, want)
		}
	}
}
