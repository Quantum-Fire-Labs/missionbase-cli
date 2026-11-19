package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- task review (#349) ----

func TestTaskReviewPostsTheCompletionMessageAndCheck(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"POST /api/v1/tasks/42/review": fixed(201, `{"task":{"id":42,"status":"review","review":{"result":"Live (337e9902)","check":"Reload Home"}}}`),
	})
	setAgentEnv(t, server.URL)
	bodyFile := filepath.Join(t.TempDir(), "done.md")
	if err := os.WriteFile(bodyFile, []byte("Live (337e9902)\n\nDetails.\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := runCapture(t, "task", "review", "42", "--check", "  Reload Home ", "--body-file", bodyFile, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if len(*requests) != 1 {
		t.Fatalf("requests = %#v, want one POST", *requests)
	}
	req := (*requests)[0]
	if req.slug != "missionbase-dev" || req.payload["check"] != "Reload Home" || !strings.HasPrefix(req.payload["body"].(string), "Live (337e9902)") {
		t.Fatalf("request = %#v, want the body and the trimmed check as the agent", req)
	}
	if _, ok := req.payload["json"]; ok {
		t.Fatalf("payload = %#v, --json must not be sent", req.payload)
	}
	if !strings.Contains(out, `"status":"review"`) {
		t.Fatalf("out = %q, want the server's task JSON", out)
	}
}

func TestTaskReviewNeedsABodyAndOneCheckLine(t *testing.T) {
	server, requests := recordServer(t, nil)
	setAgentEnv(t, server.URL)
	bodyFile := filepath.Join(t.TempDir(), "done.md")
	if err := os.WriteFile(bodyFile, []byte("Shipped\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		args []string
		want string
	}{
		{[]string{"task", "review"}, "usage: missionbase-agent task review"},
		{[]string{"task", "review", "42", "--check", "Look"}, "--body-file is required"},
		{[]string{"task", "review", "42", "--body-file", bodyFile}, "--check is required"},
		{[]string{"task", "review", "42", "--body-file", bodyFile, "--check", "one\ntwo"}, "--check is one line"},
		{[]string{"task", "review", "42", "--body", "Shipped"}, "unknown task review option"},
	}
	for _, tc := range cases {
		if _, err := runCapture(t, tc.args...); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%v: err = %v, want %q", tc.args, err, tc.want)
		}
	}
	if len(*requests) != 0 {
		t.Fatalf("requests = %#v, want nothing sent", *requests)
	}
}

func TestAgentsCantCompleteOrSetReviewDirectly(t *testing.T) {
	server, requests := recordServer(t, nil)
	setAgentEnv(t, server.URL)

	cases := []struct {
		args []string
		want string
	}{
		{[]string{"task", "complete", "42"}, "agents don't mark tasks complete"},
		{[]string{"task", "status", "42", "complete"}, "agents don't mark tasks complete"},
		{[]string{"task", "status", "42", "review"}, "send a task for review with `missionbase-agent task review"},
		{[]string{"task", "create", "--title", "Born done", "--project", "5", "--status", "complete"}, "agents don't mark tasks complete"},
		{[]string{"task", "create", "--title", "Born reviewed", "--project", "5", "--status", "review"}, "task review"},
	}
	for _, tc := range cases {
		if _, err := runCapture(t, tc.args...); err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%v: err = %v, want %q", tc.args, err, tc.want)
		}
	}
	if len(*requests) != 0 {
		t.Fatalf("requests = %#v, want nothing sent", *requests)
	}
}
