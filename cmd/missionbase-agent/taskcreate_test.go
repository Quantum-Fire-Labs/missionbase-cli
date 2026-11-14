package main

import (
	"encoding/json"
	"net/http"
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
