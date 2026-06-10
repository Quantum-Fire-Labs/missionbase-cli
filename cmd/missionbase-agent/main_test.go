package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestTaskCommentPostsComment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/tasks/123/comments" {
			t.Fatalf("path = %s, want /api/v1/tasks/123/comments", r.URL.Path)
		}
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "missionbase-dev" {
			t.Fatalf("agent slug header = %q, want missionbase-dev", got)
		}

		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if payload["comment"] != "Done and documented" {
			t.Fatalf("comment payload = %q, want Done and documented", payload["comment"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"comment":{"id":321,"content":"Done and documented"}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "comment", "123", "--body", "Done and documented"}); err != nil {
		t.Fatalf("run task comment: %v", err)
	}
}

func TestTaskCommentSupportsAliasAndMessageFlag(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/tasks/task-123/comments" {
			t.Fatalf("path = %s, want /api/v1/tasks/task-123/comments", r.URL.Path)
		}

		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if payload["comment"] != "Alias body" {
			t.Fatalf("comment payload = %q, want Alias body", payload["comment"])
		}

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"comment":{"id":322}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "create-comment", "task-123", "--message", "Alias body"}); err != nil {
		t.Fatalf("run task create-comment: %v", err)
	}
}

func TestTaskCommentRejectsBlankBody(t *testing.T) {
	if err := run([]string{"task", "comment", "123", "--body", "   "}); err == nil {
		t.Fatal("expected blank comment body error")
	}
}

func setAgentEnv(t *testing.T, baseURL string) {
	t.Helper()
	t.Setenv("MISSIONBASE_BASE_URL", baseURL)
	t.Setenv("MISSIONBASE_TOKEN", "test-token")
	t.Setenv("MISSIONBASE_AGENT_SLUG", "missionbase-dev")
	t.Setenv("MISSIONBASE_AGENT_CREDENTIALS", filepath.Join(t.TempDir(), "credentials"))
	configPath := filepath.Join(t.TempDir(), "agent-config.json")
	if err := os.WriteFile(configPath, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("write agent config: %v", err)
	}
	t.Setenv("MISSIONBASE_AGENT_CONFIG", configPath)
}
