package main

import (
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ---- task review (#349, items #544) ----

func writeTemp(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestTaskReviewPostsTheCompletionMessageAndItems(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"POST /api/v1/tasks/42/review": fixed(201, `{"task":{"id":42,"status":"review","review":{"result":"Live (337e9902)","count":2}}}`),
	})
	setAgentEnv(t, server.URL)
	bodyFile := writeTemp(t, "done.md", "Live (337e9902)\n\nDetails.\n")

	out, err := runCapture(t, "task", "review", "42", "--item", "  Reload Home ", "--body-file", bodyFile, "--item", "Undo puts it back", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if len(*requests) != 1 {
		t.Fatalf("requests = %#v, want one POST", *requests)
	}
	req := (*requests)[0]
	if req.slug != "missionbase-dev" || !strings.HasPrefix(req.payload["body"].(string), "Live (337e9902)") {
		t.Fatalf("request = %#v, want the body as the agent", req)
	}
	items, _ := req.payload["items"].([]any)
	if len(items) != 2 || items[0].(map[string]any)["line"] != "Reload Home" || items[1].(map[string]any)["line"] != "Undo puts it back" {
		t.Fatalf("items = %#v, want the two trimmed lines in order", req.payload["items"])
	}
	if _, ok := items[0].(map[string]any)["detail"]; ok {
		t.Fatalf("items = %#v, an --item has no detail", items)
	}
	for _, key := range []string{"json", "check"} {
		if _, ok := req.payload[key]; ok {
			t.Fatalf("payload = %#v, %s must not be sent", req.payload, key)
		}
	}
	if !strings.Contains(out, `"status":"review"`) {
		t.Fatalf("out = %q, want the server's task JSON", out)
	}
}

func TestTaskReviewReadsItemsWithDetailFromAFile(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"POST /api/v1/tasks/42/review": fixed(201, `{"task":{"id":42,"status":"review"}}`),
	})
	setAgentEnv(t, server.URL)
	bodyFile := writeTemp(t, "done.md", "Shipped\n")
	itemsFile := writeTemp(t, "items.json", `[{"line": "Home shows progress", "detail": "1. Reload [Home](/)\n2. Look at the row"}, {"line": "Undo works"}]`)

	if _, err := runCapture(t, "task", "review", "42", "--body-file", bodyFile, "--items-file", itemsFile); err != nil {
		t.Fatal(err)
	}
	items, _ := (*requests)[0].payload["items"].([]any)
	if len(items) != 2 {
		t.Fatalf("items = %#v, want two", items)
	}
	first := items[0].(map[string]any)
	if first["line"] != "Home shows progress" || first["detail"] != "1. Reload [Home](/)\n2. Look at the row" {
		t.Fatalf("first item = %#v, want its line and detail", first)
	}
}

func TestTaskReviewNeedsABodyAndWellFormedItems(t *testing.T) {
	server, requests := recordServer(t, nil)
	setAgentEnv(t, server.URL)
	bodyFile := writeTemp(t, "done.md", "Shipped\n")
	items := writeTemp(t, "items.json", `[{"line": "Look"}]`)
	typo := writeTemp(t, "typo.json", `[{"check": "Look"}]`)
	notArray := writeTemp(t, "object.json", `{"line": "Look"}`)
	eleven := []string{"task", "review", "42", "--body-file", bodyFile}
	for i := 0; i < 11; i++ {
		eleven = append(eleven, "--item", "Look")
	}

	cases := []struct {
		args []string
		want string
	}{
		{[]string{"task", "review"}, "usage: missionbase-agent task review"},
		{[]string{"task", "review", "42", "--item", "Look"}, "--body-file is required"},
		{[]string{"task", "review", "42", "--body-file", bodyFile}, "what the owner should check is required"},
		{[]string{"task", "review", "42", "--body-file", bodyFile, "--check", "Look"}, "--check is gone"},
		{[]string{"task", "review", "42", "--body-file", bodyFile, "--item", "one\ntwo"}, "is one line"},
		{[]string{"task", "review", "42", "--body-file", bodyFile, "--item", " "}, "has no line"},
		{[]string{"task", "review", "42", "--body-file", bodyFile, "--item", strings.Repeat("x", 281)}, "longer than 280"},
		{eleven, "at most 10 review items"},
		{[]string{"task", "review", "42", "--body-file", bodyFile, "--item", "Look", "--items-file", items}, "either --item or --items-file"},
		{[]string{"task", "review", "42", "--body-file", bodyFile, "--items-file", typo}, "not a valid items file"},
		{[]string{"task", "review", "42", "--body-file", bodyFile, "--items-file", notArray}, "must be a JSON array"},
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
