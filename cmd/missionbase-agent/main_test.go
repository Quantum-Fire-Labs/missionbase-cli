package main

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAgentCreatePostsAgentPayloadWithoutSelectedAgent(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/agents" {
			t.Fatalf("path = %s, want /api/v1/agents", r.URL.Path)
		}
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "" {
			t.Fatalf("agent slug header = %q, want empty", got)
		}

		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if payload["name"] != "Fleet Worker" || payload["slug"] != "fleet-worker" || payload["description"] != "Bootstrapper" {
			t.Fatalf("payload = %#v", payload)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"agent":{"id":42,"slug":"fleet-worker"}}`))
	}))
	defer server.Close()

	setAgentEnvNoSlug(t, server.URL)
	if err := run([]string{"agent", "create", "--name", "Fleet Worker", "--slug", "fleet-worker", "--description", "Bootstrapper"}); err != nil {
		t.Fatalf("run agent create: %v", err)
	}
}

func TestAgentCreateRequiresNameAndSlug(t *testing.T) {
	if err := run([]string{"agent", "create", "--name", "Only Name"}); err == nil || !strings.Contains(err.Error(), "--slug is required") {
		t.Fatalf("err = %v, want slug required", err)
	}
	if err := run([]string{"agent", "create", "--slug", "only-slug"}); err == nil || !strings.Contains(err.Error(), "--name is required") {
		t.Fatalf("err = %v, want name required", err)
	}
}

func TestAgentBoxesAddPostsBoxIDs(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/agents/fleet-worker/boxes" {
			t.Fatalf("path = %s, want /api/v1/agents/fleet-worker/boxes", r.URL.Path)
		}
		var payload map[string][]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if got := payload["box_ids"]; len(got) != 2 || got[0] != "2" || got[1] != "3" {
			t.Fatalf("box_ids = %#v", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"agent":{"id":42,"slug":"fleet-worker"},"memberships":[{"status":"created"}]}`))
	}))
	defer server.Close()

	setAgentEnvNoSlug(t, server.URL)
	if err := run([]string{"agent", "boxes", "add", "fleet-worker", "--box", "2", "--box", "3"}); err != nil {
		t.Fatalf("run agent boxes add: %v", err)
	}
}

func TestAgentBoxesAddRequiresBox(t *testing.T) {
	if err := run([]string{"agent", "boxes", "add", "fleet-worker"}); err == nil || !strings.Contains(err.Error(), "at least one --box") {
		t.Fatalf("err = %v, want box required", err)
	}
}

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

func TestTaskCommentPostsMultipartAttachment(t *testing.T) {
	attachment := writePNG(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "multipart/form-data;") {
			t.Fatalf("content-type = %q, want multipart/form-data", got)
		}
		if err := r.ParseMultipartForm(6 * 1024 * 1024); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		if got := r.FormValue("comment"); got != "See screenshot" {
			t.Fatalf("comment = %q, want See screenshot", got)
		}
		files := r.MultipartForm.File["attachments[]"]
		if len(files) != 1 {
			t.Fatalf("attachments count = %d, want 1", len(files))
		}
		if files[0].Filename != filepath.Base(attachment) {
			t.Fatalf("filename = %q", files[0].Filename)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"comment":{"id":323}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "comment", "123", "--body", "See screenshot", "--attach", attachment}); err != nil {
		t.Fatalf("run task comment with attachment: %v", err)
	}
}

func TestTaskCreatePostsMultipartAttachmentAndBlob(t *testing.T) {
	attachment := writePNG(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tasks" {
			t.Fatalf("path = %s, want /api/v1/tasks", r.URL.Path)
		}
		if err := r.ParseMultipartForm(6 * 1024 * 1024); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		if got := r.FormValue("title"); got != "With attachment" {
			t.Fatalf("title = %q", got)
		}
		if got := r.MultipartForm.Value["attachment_blobs[]"]; len(got) != 1 || got[0] != "signed123" {
			t.Fatalf("attachment_blobs = %#v", got)
		}
		if len(r.MultipartForm.File["attachments[]"]) != 1 {
			t.Fatalf("attachments count = %d, want 1", len(r.MultipartForm.File["attachments[]"]))
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"task":{"id":987}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "create", "--title", "With attachment", "--box", "2", "--assign-agent", "missionbase-dev", "--attach", attachment, "--attach-blob", "signed123"}); err != nil {
		t.Fatalf("run task create with attachment: %v", err)
	}
}

func TestAttachmentRejectsUnsupportedType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("plain text"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"task", "comment", "123", "--attach", path}); err == nil || !strings.Contains(err.Error(), "unsupported attachment type") {
		t.Fatalf("err = %v, want unsupported attachment type", err)
	}
}

func writePNG(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "screenshot.png")
	png := []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 0, 'I', 'E', 'N', 'D'}
	if err := os.WriteFile(path, png, 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.Copy(io.Discard, file)
	_ = file.Close()
	return path
}

func setAgentEnv(t *testing.T, baseURL string) {
	t.Helper()
	setAgentEnvNoSlug(t, baseURL)
	t.Setenv("MISSIONBASE_AGENT_SLUG", "missionbase-dev")
}

func setAgentEnvNoSlug(t *testing.T, baseURL string) {
	t.Helper()
	t.Setenv("MISSIONBASE_BASE_URL", baseURL)
	t.Setenv("MISSIONBASE_TOKEN", "test-token")
	t.Setenv("MISSIONBASE_AGENT_SLUG", "")
	t.Setenv("MISSIONBASE_AGENT_CREDENTIALS", filepath.Join(t.TempDir(), "credentials"))
	configPath := filepath.Join(t.TempDir(), "agent-config.json")
	if err := os.WriteFile(configPath, []byte(`{}`), 0o600); err != nil {
		t.Fatalf("write agent config: %v", err)
	}
	t.Setenv("MISSIONBASE_AGENT_CONFIG", configPath)
}
