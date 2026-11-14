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
