package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestMeUsesUserEndpointOnly(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Path != "/api/v1/users/me" {
			t.Fatalf("path = %s, want /api/v1/users/me", r.URL.Path)
		}
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "" {
			t.Fatalf("agent slug header = %q, want empty", got)
		}
		_, _ = w.Write([]byte(`{"user":{"id":1}}`))
	}))
	defer server.Close()

	setUserEnv(t, server.URL)
	if err := run([]string{"me"}); err != nil {
		t.Fatalf("run me: %v", err)
	}
	if !called {
		t.Fatal("server was not called")
	}
}

func TestWorkUsesUserWorkEndpointWithoutAgentHeader(t *testing.T) {
	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		if r.URL.Path != "/api/v1/users/work" {
			t.Fatalf("path = %s, want /api/v1/users/work", r.URL.Path)
		}
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "" {
			t.Fatalf("agent slug header = %q, want empty", got)
		}
		_, _ = w.Write([]byte(`{"user":{"id":1},"tasks":[],"unread_conversations":[],"meta":{"total":0}}`))
	}))
	defer server.Close()

	setUserEnv(t, server.URL)
	if err := run([]string{"work"}); err != nil {
		t.Fatalf("run work: %v", err)
	}
	if !called {
		t.Fatal("server was not called")
	}
}

func TestUserModeIgnoresAgentDirectoryConfigAndHeader(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "" {
			t.Fatalf("agent slug header = %q, want empty", got)
		}
		if r.URL.Path != "/api/v1/teams" {
			t.Fatalf("path = %s, want /api/v1/teams", r.URL.Path)
		}
		_, _ = w.Write([]byte(`{"teams":[]}`))
	}))
	defer server.Close()

	setUserEnv(t, server.URL)
	agentConfig := filepath.Join(t.TempDir(), "agent-config.json")
	if err := os.WriteFile(agentConfig, []byte(`{"base_url":"http://127.0.0.1:1","agent_slug":"agent-from-directory"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MISSIONBASE_AGENT_CONFIG", agentConfig)
	t.Setenv("MISSIONBASE_AGENT_SLUG", "agent-from-env")
	if err := run([]string{"teams"}); err != nil {
		t.Fatalf("run teams: %v", err)
	}
}

func TestBoxesTasksBuildsQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/boxes/42/tasks" {
			t.Fatalf("path = %s, want /api/v1/boxes/42/tasks", r.URL.Path)
		}
		query := r.URL.Query()
		for key, want := range map[string]string{
			"status":          "todo",
			"status_category": "open",
			"task_status_ids": "7,8",
			"page":            "2",
			"per_page":        "25",
		} {
			if got := query.Get(key); got != want {
				t.Fatalf("%s query = %q, want %q", key, got, want)
			}
		}
		_, _ = w.Write([]byte(`{"tasks":[]}`))
	}))
	defer server.Close()

	setUserEnv(t, server.URL)
	if err := run([]string{"boxes", "tasks", "42", "--status", "todo", "--status-category", "open", "--task-status-ids", "7,8", "--page", "2", "--per-page", "25"}); err != nil {
		t.Fatalf("run boxes tasks: %v", err)
	}
}

func TestReadOnlyCommandDispatchRepresentativeEndpoints(t *testing.T) {
	tests := []struct {
		name string
		args []string
		path string
	}{
		{"work", []string{"work"}, "/api/v1/users/work"},
		{"team show", []string{"team", "show", "12"}, "/api/v1/teams/12"},
		{"team members", []string{"team", "members", "12"}, "/api/v1/teams/12/members"},
		{"boxes", []string{"boxes", "--team", "12"}, "/api/v1/boxes"},
		{"box show", []string{"box", "show", "4"}, "/api/v1/boxes/4"},
		{"box discussions", []string{"boxes", "discussions", "4", "--page", "3", "--per-page", "10"}, "/api/v1/boxes/4/discussions"},
		{"box statuses", []string{"boxes", "statuses", "4"}, "/api/v1/boxes/4/task_statuses"},
		{"box task-statuses", []string{"boxes", "task-statuses", "4"}, "/api/v1/boxes/4/task_statuses"},
		{"tasks assigned", []string{"tasks", "assigned", "--page", "1"}, "/api/v1/tasks/assigned"},
		{"tasks visible", []string{"tasks", "visible", "--per-page", "5"}, "/api/v1/tasks"},
		{"task show", []string{"task", "show", "99"}, "/api/v1/tasks/99"},
		{"task feed", []string{"task", "feed", "99", "--limit", "7"}, "/api/v1/tasks/99/comments"},
		{"task comments", []string{"task", "comments", "99"}, "/api/v1/tasks/99/comments"},
		{"conversations", []string{"conversations", "--page", "2"}, "/api/v1/conversations"},
		{"conversation show", []string{"conversation", "show", "abc", "--limit", "6"}, "/api/v1/conversations/abc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != tt.path {
					t.Fatalf("path = %s, want %s", r.URL.Path, tt.path)
				}
				_, _ = w.Write([]byte(`{"ok":true}`))
			}))
			defer server.Close()
			setUserEnv(t, server.URL)
			if err := run(tt.args); err != nil {
				t.Fatalf("run %v: %v", tt.args, err)
			}
		})
	}
}

func TestUsageErrors(t *testing.T) {
	tests := [][]string{
		{"team", "show"},
		{"boxes", "tasks"},
		{"boxes", "tasks", "1", "--status-category", "later"},
		{"task", "feed"},
		{"conversations", "--page"},
		{"conversation", "show"},
	}
	for _, args := range tests {
		if err := run(args); err == nil {
			t.Fatalf("run %v succeeded, want error", args)
		}
	}
}

func TestHelpShowsUserWorkflowCommands(t *testing.T) {
	stdout := captureStdout(t, func() { _ = run([]string{"--help"}) })
	for _, want := range []string{"work", "teams", "team show <team-id>", "boxes tasks <box-id>", "tasks assigned", "task feed <task-id>", "conversations", "conversation show <feed-id>"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("help missing %q:\n%s", want, stdout)
		}
	}
}

func setUserEnv(t *testing.T, baseURL string) {
	t.Helper()
	t.Setenv("MISSIONBASE_BASE_URL", baseURL)
	t.Setenv("MISSIONBASE_TOKEN", "user-token")
	t.Setenv("MISSIONBASE_CREDENTIALS", filepath.Join(t.TempDir(), "credentials"))
}

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stdout
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stdout = writer
	fn()
	_ = writer.Close()
	os.Stdout = original
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func TestTaskCreatePostsJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/tasks" {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "" {
			t.Fatalf("agent slug header = %q, want empty", got)
		}
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
			t.Fatalf("content type = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		for _, want := range []string{`"title":"Write"`, `"box_id":"2"`, `"description":"line1\nline2"`} {
			if !strings.Contains(string(body), want) {
				t.Fatalf("body missing %s: %s", want, body)
			}
		}
		_, _ = w.Write([]byte(`{"task":{"id":123}}`))
	}))
	defer server.Close()
	setUserEnv(t, server.URL)
	if err := run([]string{"task", "create", "--title", "Write", "--box", "2", "--description", `line1\nline2`}); err != nil {
		t.Fatalf("run task create: %v", err)
	}
}

func TestTaskCompletePatchesCompleteEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "" {
			t.Fatalf("agent slug header = %q, want empty", got)
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/v1/users/me" {
			_, _ = w.Write([]byte(`{"user":{"id":44}}`))
			return
		}
		if r.Method != http.MethodPatch || r.URL.Path != "/api/v1/tasks/123/complete" {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"acting_as_user_id":44`) {
			t.Fatalf("body = %s", body)
		}
		_, _ = w.Write([]byte(`{"task":{"id":123,"status":"complete"}}`))
	}))
	defer server.Close()
	setUserEnv(t, server.URL)
	if err := run([]string{"task", "complete", "123"}); err != nil {
		t.Fatalf("run task complete: %v", err)
	}
}

func TestTaskCommentUsesMultipartWithAttachment(t *testing.T) {
	png := filepath.Join(t.TempDir(), "image.png")
	if err := os.WriteFile(png, []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0}, 0o600); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/tasks/123/comments" {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "multipart/form-data") {
			t.Fatalf("content type = %q", got)
		}
		if err := r.ParseMultipartForm(6 << 20); err != nil {
			t.Fatalf("ParseMultipartForm: %v", err)
		}
		if got := r.FormValue("comment"); got != "hello\nthere" {
			t.Fatalf("comment = %q", got)
		}
		if got := r.FormValue("attachment_blobs[]"); got != "signed-1" {
			t.Fatalf("blob = %q", got)
		}
		if len(r.MultipartForm.File["attachments[]"]) != 1 {
			t.Fatalf("attachments = %#v", r.MultipartForm.File)
		}
		_, _ = w.Write([]byte(`{"comment":{"id":9}}`))
	}))
	defer server.Close()
	setUserEnv(t, server.URL)
	if err := run([]string{"task", "comment", "123", "--body", `hello\nthere`, "--attach", png, "--attach-blob", "signed-1"}); err != nil {
		t.Fatalf("run task comment: %v", err)
	}
}

func TestConversationCommentPostsJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/conversations/feed-1/comments" {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
			t.Fatalf("content type = %q", got)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"comment":"hi"`) {
			t.Fatalf("body = %s", body)
		}
		_, _ = w.Write([]byte(`{"comment":{"id":10}}`))
	}))
	defer server.Close()
	setUserEnv(t, server.URL)
	if err := run([]string{"conversation", "comment", "feed-1", "--message", "hi"}); err != nil {
		t.Fatalf("run conversation comment: %v", err)
	}
}
