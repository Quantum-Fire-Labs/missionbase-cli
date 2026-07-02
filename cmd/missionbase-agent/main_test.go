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

func TestScratchpadCommandsUseUserContext(t *testing.T) {
	bodyFile := filepath.Join(t.TempDir(), "scratchpad.md")
	if err := os.WriteFile(bodyFile, []byte("# Agent file\\n\\n- Item"), 0o600); err != nil {
		t.Fatal(err)
	}

	seen := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "missionbase-dev" {
			t.Fatalf("agent slug header = %q, want missionbase-dev", got)
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/scratchpad":
			seen["show"] = true
			if got := r.URL.Query().Get("user_id"); got != "@DanielLemky" {
				t.Fatalf("user_id query = %q, want @DanielLemky", got)
			}
		case "PATCH /api/v1/scratchpad":
			seen["edit"] = true
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["user_id"] != "@DanielLemky" || payload["scratchpad"] != "# Agent file\n\n- Item" {
				t.Fatalf("payload = %#v", payload)
			}
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"scratchpad":{"plain_text":"ok"}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"scratchpad", "show", "--user", "@DanielLemky"}); err != nil {
		t.Fatalf("run scratchpad show: %v", err)
	}
	if err := run([]string{"scratchpad", "edit", "--user", "@DanielLemky", "--body-file", bodyFile}); err != nil {
		t.Fatalf("run scratchpad edit: %v", err)
	}
	for _, key := range []string{"show", "edit"} {
		if !seen[key] {
			t.Fatalf("%s request was not seen", key)
		}
	}
}

func TestScratchpadAgentRejectsInlineBody(t *testing.T) {
	if err := run([]string{"scratchpad", "edit", "--user", "1", "--body", "hello"}); err == nil || !strings.Contains(err.Error(), "use --body-file PATH") {
		t.Fatalf("err = %v, want body-file rejection", err)
	}
}

func TestTasksUserDueBuildsTaskListQuery(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/v1/tasks" {
			t.Fatalf("path = %s, want /api/v1/tasks", r.URL.Path)
		}
		query := r.URL.Query()
		if got := query.Get("user"); got != "@DanielLemky" {
			t.Fatalf("user query = %q, want @DanielLemky", got)
		}
		if got := query.Get("due"); got != "today" {
			t.Fatalf("due query = %q, want today", got)
		}
		if got := query.Get("box"); got != "2" {
			t.Fatalf("box query = %q, want 2", got)
		}
		if got := query.Get("status_category"); got != "open" {
			t.Fatalf("status_category query = %q, want open", got)
		}
		if got := query.Get("include_closed"); got != "true" {
			t.Fatalf("include_closed query = %q, want true", got)
		}
		if got := query.Get("scheduled"); got != "future" {
			t.Fatalf("scheduled query = %q, want future", got)
		}
		if got := query.Get("page"); got != "2" {
			t.Fatalf("page query = %q, want 2", got)
		}
		if got := query.Get("per_page"); got != "25" {
			t.Fatalf("per_page query = %q, want 25", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tasks":[],"meta":{"total":0}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"tasks", "--user", "@DanielLemky", "--due", "today", "--box", "2", "--status-category", "open", "--include-closed", "--scheduled", "future", "--page", "2", "--per-page", "25", "--json"}); err != nil {
		t.Fatalf("run tasks: %v", err)
	}
}

func TestTasksDueShortcutRequiresUser(t *testing.T) {
	if err := run([]string{"tasks", "today"}); err == nil || !strings.Contains(err.Error(), "--user is required") {
		t.Fatalf("err = %v, want --user required", err)
	}
}

func TestTasksWithoutFiltersUsesAssignedAgentEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/tasks" {
			t.Fatalf("path = %s, want /api/v1/agent/tasks", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tasks":[],"meta":{"total":0}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"tasks"}); err != nil {
		t.Fatalf("run tasks: %v", err)
	}
}

func TestWorkNextGetsSelectedTaskEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/v1/agent/work" {
			t.Fatalf("path = %s, want /api/v1/agent/work", r.URL.Path)
		}
		if got := r.URL.RawQuery; got != "" {
			t.Fatalf("query = %q, want empty", got)
		}
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "missionbase-dev" {
			t.Fatalf("agent slug header = %q, want missionbase-dev", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"has_work":true,"task":{"id":2420,"box":{"id":2,"name":"Missionbase","working_directory":"/workspace/missionbase"}}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"work", "--next"}); err != nil {
		t.Fatalf("run work --next: %v", err)
	}
}

func TestWorkNextTaskAliasGetsSelectedTaskEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/work" {
			t.Fatalf("path = %s, want /api/v1/agent/work", r.URL.Path)
		}
		if got := r.URL.RawQuery; got != "" {
			t.Fatalf("query = %q, want empty", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"has_work":false,"task":null}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"work", "--next-task"}); err != nil {
		t.Fatalf("run work --next-task: %v", err)
	}
}

func TestWorkRejectsUnknownOption(t *testing.T) {
	if err := run([]string{"work", "--all"}); err == nil || !strings.Contains(err.Error(), "unknown work option") {
		t.Fatalf("err = %v, want unknown work option", err)
	}
}

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
		if payload["name"] != "Fleet Worker" || payload["slug"] != "fleet-worker" || payload["title"] != "Fleet Architect" || payload["description"] != "Bootstrapper" {
			t.Fatalf("payload = %#v", payload)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"agent":{"id":42,"slug":"fleet-worker"}}`))
	}))
	defer server.Close()

	setAgentEnvNoSlug(t, server.URL)
	if err := run([]string{"agent", "create", "--name", "Fleet Worker", "--slug", "fleet-worker", "--title", "Fleet Architect", "--description", "Bootstrapper"}); err != nil {
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

func TestAgentArchiveDeletesWithoutSelectedAgentWhenConfirmed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("method = %s, want DELETE", r.Method)
		}
		if r.URL.Path != "/api/v1/agents/fleet-worker" {
			t.Fatalf("path = %s, want /api/v1/agents/fleet-worker", r.URL.Path)
		}
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "" {
			t.Fatalf("agent slug header = %q, want empty", got)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"archived":true,"operation":"archive","agent":{"slug":"fleet-worker","status":"archived"}}`))
	}))
	defer server.Close()

	setAgentEnvNoSlug(t, server.URL)
	if err := run([]string{"agent", "archive", "fleet-worker", "--yes"}); err != nil {
		t.Fatalf("run agent archive: %v", err)
	}
}

func TestAgentArchiveRequiresConfirmation(t *testing.T) {
	if err := run([]string{"agent", "archive", "fleet-worker"}); err == nil || !strings.Contains(err.Error(), "--yes is required") {
		t.Fatalf("err = %v, want --yes required", err)
	}
}

func TestAgentRestorePatchesWithoutSelectedAgentWhenConfirmed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Fatalf("method = %s, want PATCH", r.Method)
		}
		if r.URL.Path != "/api/v1/agents/fleet-worker/restore" {
			t.Fatalf("path = %s, want /api/v1/agents/fleet-worker/restore", r.URL.Path)
		}
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "" {
			t.Fatalf("agent slug header = %q, want empty", got)
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"restored":true,"operation":"restore","agent":{"slug":"fleet-worker","status":"active"}}`))
	}))
	defer server.Close()

	setAgentEnvNoSlug(t, server.URL)
	if err := run([]string{"agent", "restore", "fleet-worker", "--yes"}); err != nil {
		t.Fatalf("run agent restore: %v", err)
	}
}

func TestAgentRestoreRequiresConfirmation(t *testing.T) {
	if err := run([]string{"agent", "restore", "fleet-worker"}); err == nil || !strings.Contains(err.Error(), "--yes is required") {
		t.Fatalf("err = %v, want --yes required", err)
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

func TestBoxesTaskStatusesGetsBoxTaskStatuses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/v1/boxes/2/task_statuses" {
			t.Fatalf("path = %s, want /api/v1/boxes/2/task_statuses", r.URL.Path)
		}
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "missionbase-dev" {
			t.Fatalf("agent slug header = %q, want missionbase-dev", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"task_statuses":[{"id":1,"key":"todo","name":"To Do","category":"open","position":2,"color":"amber","default_open":true,"primary_done":false,"primary_canceled":false,"archived":false}]}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"boxes", "task-statuses", "2"}); err != nil {
		t.Fatalf("run boxes task-statuses: %v", err)
	}
}

func TestBoxesDiscussionsGetsStandaloneDiscussions(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/v1/boxes/2/discussions" {
			t.Fatalf("path = %s, want /api/v1/boxes/2/discussions", r.URL.Path)
		}
		if got := r.URL.Query().Get("per_page"); got != "10" {
			t.Fatalf("per_page = %q, want 10", got)
		}
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "missionbase-dev" {
			t.Fatalf("agent slug header = %q, want missionbase-dev", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"discussions":[{"id":7,"title":"Standalone"}],"meta":{"total":1}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"boxes", "discussions", "2", "--per-page", "10"}); err != nil {
		t.Fatalf("run boxes discussions: %v", err)
	}
}

func TestBoxesFilesGetsListAndSearchesDocuments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/v1/boxes/2/files" {
			t.Fatalf("path = %s, want /api/v1/boxes/2/files", r.URL.Path)
		}
		if got := r.URL.Query().Get("query"); got != "runbook" {
			t.Fatalf("query = %q, want runbook", got)
		}
		if got := r.URL.Query().Get("page"); got != "2" {
			t.Fatalf("page = %q, want 2", got)
		}
		if got := r.URL.Query().Get("per_page"); got != "25" {
			t.Fatalf("per_page = %q, want 25", got)
		}
		if got := r.URL.Query().Get("folder_id"); got != "67" {
			t.Fatalf("folder_id = %q, want 67", got)
		}
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "missionbase-dev" {
			t.Fatalf("agent slug header = %q, want missionbase-dev", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"files":[{"id":77,"title":"Runbook","type":"document","kind":"document","url":"https://dash.missionbase.app/boxes/2/files/77","fetch_id":77,"fetch_type":"document"}],"meta":{"total":1,"page":2,"per_page":25,"query":"runbook"}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"boxes", "files", "2", "--query", "runbook", "--page", "2", "--per-page", "25", "--folder-id", "67"}); err != nil {
		t.Fatalf("run boxes files: %v", err)
	}
}

func TestBoxesFilesListWithoutFolderIDIsUnchanged(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/v1/boxes/2/files" {
			t.Fatalf("path = %s, want /api/v1/boxes/2/files", r.URL.Path)
		}
		if got := r.URL.Query().Get("folder_id"); got != "" {
			t.Fatalf("folder_id = %q, want empty", got)
		}
		_, _ = w.Write([]byte(`{"files":[],"meta":{"total":0}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"boxes", "files", "2"}); err != nil {
		t.Fatalf("run boxes files: %v", err)
	}
}

func TestBoxesFilesShowUploadUpdateAndDownload(t *testing.T) {
	var sawUpload, sawUpdate, sawDownload, sawVersions, sawUploadVersion, sawDownloadVersion bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "missionbase-dev" {
			t.Fatalf("agent slug header = %q, want missionbase-dev", got)
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/boxes/2/files/77":
			_, _ = w.Write([]byte(`{"file":{"id":77,"type":"file"}}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/boxes/2/files":
			sawUpload = true
			if err := r.ParseMultipartForm(1024 * 1024); err != nil {
				t.Fatalf("parse multipart: %v", err)
			}
			if got := r.MultipartForm.Value["title"][0]; got != "Upload" {
				t.Fatalf("title = %q", got)
			}
			if got := r.MultipartForm.Value["folder_id"][0]; got != "67" {
				t.Fatalf("folder_id = %q, want 67", got)
			}
			if len(r.MultipartForm.File["file"]) != 1 {
				t.Fatalf("file count = %d, want 1", len(r.MultipartForm.File["file"]))
			}
			_, _ = w.Write([]byte(`{"file":{"id":78}}`))
		case r.Method == http.MethodPatch && r.URL.Path == "/api/v1/boxes/2/files/78":
			sawUpdate = true
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatalf("decode update: %v", err)
			}
			if payload["description"] != "Changed" {
				t.Fatalf("description = %q", payload["description"])
			}
			_, _ = w.Write([]byte(`{"file":{"id":78}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/boxes/2/files/78/download":
			sawDownload = true
			_, _ = w.Write([]byte("download-body"))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/boxes/2/files/78/versions":
			sawVersions = true
			_, _ = w.Write([]byte(`{"versions":[{"id":5,"version_number":1}]}`))
		case r.Method == http.MethodPost && r.URL.Path == "/api/v1/boxes/2/files/78/versions":
			sawUploadVersion = true
			if err := r.ParseMultipartForm(1024 * 1024); err != nil {
				t.Fatalf("parse version multipart: %v", err)
			}
			if len(r.MultipartForm.File["file"]) != 1 {
				t.Fatalf("version file count = %d, want 1", len(r.MultipartForm.File["file"]))
			}
			_, _ = w.Write([]byte(`{"version":{"id":6}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/boxes/2/files/78/versions/5/download":
			sawDownloadVersion = true
			_, _ = w.Write([]byte("old-version-body"))
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	setAgentEnv(t, server.URL)
	file := writeTextFile(t, "upload.txt", "upload-body")
	output := filepath.Join(t.TempDir(), "download.txt")

	if err := run([]string{"boxes", "files", "show", "2", "77"}); err != nil {
		t.Fatalf("run show: %v", err)
	}
	if err := run([]string{"boxes", "files", "upload", "2", "--file", file, "--title", "Upload", "--folder", "67"}); err != nil {
		t.Fatalf("run upload: %v", err)
	}
	if err := run([]string{"boxes", "files", "update", "2", "78", "--description", "Changed"}); err != nil {
		t.Fatalf("run update: %v", err)
	}
	if err := run([]string{"boxes", "files", "download", "2", "78", "--output", output}); err != nil {
		t.Fatalf("run download: %v", err)
	}
	if got, err := os.ReadFile(output); err != nil || string(got) != "download-body" {
		t.Fatalf("download output = %q, %v", got, err)
	}
	if err := run([]string{"boxes", "files", "versions", "2", "78"}); err != nil {
		t.Fatalf("run versions: %v", err)
	}
	if err := run([]string{"boxes", "files", "upload-version", "2", "78", "--file", file}); err != nil {
		t.Fatalf("run upload-version: %v", err)
	}
	versionOutput := filepath.Join(t.TempDir(), "version.txt")
	if err := run([]string{"boxes", "files", "download", "2", "78", "--version", "5", "--output", versionOutput}); err != nil {
		t.Fatalf("run version download: %v", err)
	}
	if got, err := os.ReadFile(versionOutput); err != nil || string(got) != "old-version-body" {
		t.Fatalf("version download output = %q, %v", got, err)
	}
	if !sawUpload || !sawUpdate || !sawDownload || !sawVersions || !sawUploadVersion || !sawDownloadVersion {
		t.Fatalf("saw upload/update/download/versions/uploadVersion/downloadVersion = %v/%v/%v/%v/%v/%v", sawUpload, sawUpdate, sawDownload, sawVersions, sawUploadVersion, sawDownloadVersion)
	}
}

func TestBoxesFilesCreateArtifact(t *testing.T) {
	var sawArtifact bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/boxes/2/files" {
			t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
		}
		sawArtifact = true
		if err := r.ParseMultipartForm(1024 * 1024); err != nil {
			t.Fatalf("parse artifact multipart: %v", err)
		}
		if got := r.MultipartForm.Value["file_type"][0]; got != "missionbase_artifact" {
			t.Fatalf("file_type = %q", got)
		}
		if got := r.MultipartForm.Value["title"][0]; got != "Counter" {
			t.Fatalf("title = %q", got)
		}
		if got := r.MultipartForm.Value["folder_id"][0]; got != "root" {
			t.Fatalf("folder_id = %q", got)
		}
		files := r.MultipartForm.File["file"]
		if len(files) != 1 {
			t.Fatalf("artifact file count = %d, want 1", len(files))
		}
		if got := files[0].Header.Get("Content-Type"); got != "text/html; charset=utf-8" {
			t.Fatalf("artifact content type = %q", got)
		}
		opened, err := files[0].Open()
		if err != nil {
			t.Fatalf("open artifact upload: %v", err)
		}
		defer opened.Close()
		body, _ := io.ReadAll(opened)
		if !strings.Contains(string(body), "saveState") {
			t.Fatalf("artifact body missing saveState: %q", body)
		}
		_, _ = w.Write([]byte(`{"file":{"id":99,"kind":"missionbase_artifact"}}`))
	}))
	defer server.Close()
	setAgentEnv(t, server.URL)
	file := writeTextFile(t, "artifact.html", "<button onclick=\"saveState({count:1})\">Save</button>")

	if err := run([]string{"boxes", "files", "create-artifact", "2", "--file", file, "--title", "Counter", "--root"}); err != nil {
		t.Fatalf("run create-artifact: %v", err)
	}
	if !sawArtifact {
		t.Fatal("server did not see artifact create")
	}
}

func TestBoxesFilesHelpDocumentsFolderPlacement(t *testing.T) {
	stdout := captureStdout(t, func() {
		printHelp()
	})
	for _, want := range []string{
		"[--folder-id FOLDER_ID|--folder FOLDER_ID|--root] [--recursive]",
		"boxes files upload <box-id> --file PATH [--title TITLE] [--description TEXT] [--folder FOLDER_ID|--root]",
		"boxes files create-artifact <box-id> (--file PATH|--stdin) --title TITLE [--description TEXT] [--folder FOLDER_ID|--root]",
		"sandboxed interactive HTML with persisted JSON state",
		"boxes files mkdir <box-id> --title TITLE [--folder FOLDER_ID|--root]",
		"boxes files mv <box-id> <file-id> (--folder FOLDER_ID|--root)",
		"boxes files update <box-id> <file-id> [--title TITLE] [--description TEXT]",
		"boxes files versions <box-id> <file-id>",
		"boxes files upload-version <box-id> <file-id> --file PATH",
		"boxes files download <box-id> <file-id> --output PATH [--version VERSION_ID]",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("top-level help missing %q in:\n%s", want, stdout)
		}
	}

	stdout = captureStdout(t, func() {
		if err := run([]string{"boxes", "files", "--help"}); err != nil {
			t.Fatalf("run boxes files --help: %v", err)
		}
	})
	for _, want := range []string{
		"missionbase-agent boxes files upload <box-id> --file PATH [--title TITLE] [--description TEXT] [--folder FOLDER_ID|--root]",
		"missionbase-agent boxes files create-artifact <box-id> (--file PATH|--stdin) --title TITLE [--description TEXT] [--folder FOLDER_ID|--root]",
		"loadState()/saveState(data)",
		"missionbase-agent boxes files update <box-id> <file-id> [--title TITLE] [--description TEXT] [--folder FOLDER_ID|--root]",
		"missionbase-agent boxes files upload-version <box-id> <file-id> --file PATH",
	} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("boxes files help missing %q in:\n%s", want, stdout)
		}
	}

	stdout = captureStdout(t, func() {
		if err := run([]string{"boxes", "files", "upload", "--help"}); err != nil {
			t.Fatalf("run upload --help: %v", err)
		}
	})
	if want := "missionbase-agent boxes files upload <box-id> --file PATH [--title TITLE] [--description TEXT] [--folder FOLDER_ID|--root]"; !strings.Contains(stdout, want) {
		t.Fatalf("upload help missing %q in:\n%s", want, stdout)
	}

	stdout = captureStdout(t, func() {
		if err := run([]string{"boxes", "files", "upload-version", "--help"}); err != nil {
			t.Fatalf("run upload-version --help: %v", err)
		}
	})
	if want := "missionbase-agent boxes files upload-version <box-id> <file-id> --file PATH"; !strings.Contains(stdout, want) {
		t.Fatalf("upload-version help missing %q in:\n%s", want, stdout)
	}

	stdout = captureStdout(t, func() {
		if err := run([]string{"boxes", "files", "create-artifact", "--help"}); err != nil {
			t.Fatalf("run create-artifact --help: %v", err)
		}
	})
	for _, want := range []string{"missionbase-agent boxes files create-artifact <box-id> (--file PATH|--stdin) --title TITLE", "sandboxed interactive HTML", "loadState()", "Static .html uploads remain static previews"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("create-artifact help missing %q in:\n%s", want, stdout)
		}
	}

	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"show", []string{"boxes", "files", "show", "--help"}, "missionbase-agent boxes files show <box-id> <file-id>"},
		{"mkdir", []string{"boxes", "files", "mkdir", "--help"}, "missionbase-agent boxes files mkdir <box-id> --title TITLE [--folder FOLDER_ID|--root]"},
		{"mv", []string{"boxes", "files", "mv", "--help"}, "missionbase-agent boxes files mv <box-id> <file-id> (--folder FOLDER_ID|--root)"},
		{"versions", []string{"boxes", "files", "versions", "--help"}, "missionbase-agent boxes files versions <box-id> <file-id>"},
		{"download", []string{"boxes", "files", "download", "--help"}, "missionbase-agent boxes files download <box-id> <file-id> --output PATH [--version VERSION_ID]"},
	} {
		stdout := captureStdout(t, func() {
			if err := run(tc.args); err != nil {
				t.Fatalf("run %s --help: %v", tc.name, err)
			}
		})
		if !strings.Contains(stdout, tc.want) {
			t.Fatalf("%s help missing %q in:\n%s", tc.name, tc.want, stdout)
		}
	}
}

func TestBoxesFilesRequiresBoxIDAndOptionValues(t *testing.T) {
	if err := run([]string{"boxes", "files"}); err == nil || !strings.Contains(err.Error(), "usage: missionbase-agent boxes files <box-id>") {
		t.Fatalf("err = %v, want usage error", err)
	}
	if err := run([]string{"boxes", "files", "2", "--query"}); err == nil || !strings.Contains(err.Error(), "--query requires a value") {
		t.Fatalf("err = %v, want query value error", err)
	}
	if err := run([]string{"boxes", "files", "2", "--folder-id"}); err == nil || !strings.Contains(err.Error(), "--folder-id requires a value") {
		t.Fatalf("err = %v, want folder-id value error", err)
	}
}

func TestBoxesDiscussionsRequiresBoxID(t *testing.T) {
	if err := run([]string{"boxes", "discussions"}); err == nil || !strings.Contains(err.Error(), "usage: missionbase-agent boxes discussions <box-id>") {
		t.Fatalf("err = %v, want usage error", err)
	}
}

func TestBoxesDiscussionsCreatePostsDiscussion(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/boxes/2/discussions" {
			t.Fatalf("path = %s, want /api/v1/boxes/2/discussions", r.URL.Path)
		}
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "missionbase-dev" {
			t.Fatalf("agent slug header = %q, want missionbase-dev", got)
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if got := payload["title"]; got != "Planning" {
			t.Fatalf("title = %q, want Planning", got)
		}
		if got := payload["body"]; got != "Line 1\nLine 2" {
			t.Fatalf("body = %q, want real multiline body", got)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"discussion":{"id":8,"title":"Planning","box_id":2}}`))
	}))
	defer server.Close()

	bodyFile := writeTextFile(t, "discussion.md", `Line 1\nLine 2`)
	setAgentEnv(t, server.URL)
	if err := run([]string{"boxes", "discussions", "create", "2", "--title", "Planning", "--body-file", bodyFile}); err != nil {
		t.Fatalf("run boxes discussions create: %v", err)
	}
}

func TestBoxesDiscussionsCreateRequiresTitleAndBody(t *testing.T) {
	bodyFile := writeTextFile(t, "discussion.md", "Body")
	if err := run([]string{"boxes", "discussions", "create", "2", "--body-file", bodyFile}); err == nil || !strings.Contains(err.Error(), "--title is required") {
		t.Fatalf("err = %v, want title required", err)
	}
	if err := run([]string{"boxes", "discussions", "create", "2", "--title", "Title"}); err == nil || !strings.Contains(err.Error(), "--body is required") {
		t.Fatalf("err = %v, want body required", err)
	}
}

func TestBoxesStatusesAliasGetsBoxTaskStatuses(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/boxes/box-2/task_statuses" {
			t.Fatalf("path = %s, want /api/v1/boxes/box-2/task_statuses", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"task_statuses":[]}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"boxes", "statuses", "box-2"}); err != nil {
		t.Fatalf("run boxes statuses: %v", err)
	}
}

func TestBoxesTaskStatusesRequiresBoxID(t *testing.T) {
	if err := run([]string{"boxes", "task-statuses"}); err == nil || !strings.Contains(err.Error(), "usage: missionbase-agent boxes task-statuses <box-id>") {
		t.Fatalf("err = %v, want usage error", err)
	}
}

func TestTaskShowGetsFullTaskEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/v1/tasks/123" {
			t.Fatalf("path = %s, want /api/v1/tasks/123", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"task":{"id":123,"description_rich_text":{"plain_text":"full context"}}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "show", "123"}); err != nil {
		t.Fatalf("run task show: %v", err)
	}
}

func TestTaskAssignPostsUserAssignment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/tasks/123/assignments" {
			t.Fatalf("path = %s, want /api/v1/tasks/123/assignments", r.URL.Path)
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if payload["user_id"] != "42" {
			t.Fatalf("user_id = %q, want 42", payload["user_id"])
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"assignment":{"task_id":123}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "assign", "123", "--user", "42"}); err != nil {
		t.Fatalf("run task assign user: %v", err)
	}
}

func TestTaskAssignPostsAgentAssignment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if payload["agent_slug"] != "alden" {
			t.Fatalf("agent_slug = %q, want alden", payload["agent_slug"])
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"assignment":{"task_id":123}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "assign", "123", "--agent", "alden"}); err != nil {
		t.Fatalf("run task assign agent: %v", err)
	}
}

func TestTaskUnassignDeletesUserAssignment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("method = %s, want DELETE", r.Method)
		}
		if r.URL.Path != "/api/v1/tasks/123/assignments/42" {
			t.Fatalf("path = %s, want /api/v1/tasks/123/assignments/42", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"Assignment removed successfully"}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "unassign", "123", "--user", "42"}); err != nil {
		t.Fatalf("run task unassign user: %v", err)
	}
}

func TestTaskUnassignSelfDeletesCurrentAgentAssignment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Fatalf("method = %s, want DELETE", r.Method)
		}
		if r.URL.Path != "/api/v1/tasks/123/assignments/agent" {
			t.Fatalf("path = %s, want /api/v1/tasks/123/assignments/agent", r.URL.Path)
		}
		if got := r.URL.Query().Get("agent_slug"); got != "missionbase-dev" {
			t.Fatalf("agent_slug query = %q, want missionbase-dev", got)
		}
		if got := r.URL.Query().Get("assignee_type"); got != "Agent" {
			t.Fatalf("assignee_type query = %q, want Agent", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"message":"Assignment removed successfully"}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "unassign", "123", "--self"}); err != nil {
		t.Fatalf("run task unassign self: %v", err)
	}
}

func TestTaskAssignAndUnassignValidateOptions(t *testing.T) {
	if err := run([]string{"task", "assign", "123", "--bogus", "42"}); err == nil || !strings.Contains(err.Error(), "unknown task assign option") {
		t.Fatalf("err = %v, want unknown assign option", err)
	}
	if err := run([]string{"task", "unassign", "123", "--self", "extra"}); err == nil || !strings.Contains(err.Error(), "usage:") {
		t.Fatalf("err = %v, want self usage error", err)
	}
}

func TestNormalizeAgentAuthoredBodyConvertsEscapedNewlinesOutsideCode(t *testing.T) {
	body := `Here's the summary:\n\n- first\n- use ` + "`printf 'a\\nb'`" + `\n- shell 'a\nb'\n- JSON {"text":"a\nb"}`
	want := "Here's the summary:\n\n- first\n- use `printf 'a\\nb'`\n- shell 'a\\nb'\n- JSON {\"text\":\"a\\nb\"}"
	if got := normalizeAgentAuthoredBody(body); got != want {
		t.Fatalf("normalized body = %q, want %q", got, want)
	}
}

func TestNormalizeAgentAuthoredBodyConvertsEscapedNewlinesInFencedCode(t *testing.T) {
	body := "Summary:\\n\\n```text\\nssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQ user@host\\n```\\n\\nNext `printf 'a\\\\nb'`."
	want := "Summary:\n\n```text\nssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQ user@host\n```\n\nNext `printf 'a\\\\nb'`."
	if got := normalizeAgentAuthoredBody(body); got != want {
		t.Fatalf("normalized body = %q, want %q", got, want)
	}
}

func TestDirectMessageSendNormalizesEscapedNewlines(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/agent/direct_messages" {
			t.Fatalf("path = %s, want /api/v1/agent/direct_messages", r.URL.Path)
		}

		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if payload["body"] != "Line one\nLine two" {
			t.Fatalf("body payload = %q, want normalized newline", payload["body"])
		}

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"message":{"id":321}}`))
	}))
	defer server.Close()

	bodyFile := writeTextFile(t, "dm.md", `Line one\nLine two`)
	setAgentEnv(t, server.URL)
	if err := run([]string{"dm", "send", "--to", "missionbase-lead", "--body-file", bodyFile}); err != nil {
		t.Fatalf("run dm send: %v", err)
	}
}

func TestConversationCommentPostsComment(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/conversations/456/comments" {
			t.Fatalf("path = %s, want /api/v1/conversations/456/comments", r.URL.Path)
		}
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "missionbase-dev" {
			t.Fatalf("agent slug header = %q, want missionbase-dev", got)
		}

		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if payload["message"] != "General conversation reply" {
			t.Fatalf("message payload = %q, want General conversation reply", payload["message"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"comment":{"id":654,"body":"General conversation reply"}}`))
	}))
	defer server.Close()

	bodyFile := writeTextFile(t, "discussion.md", "General conversation reply")
	setAgentEnv(t, server.URL)
	if err := run([]string{"discussion", "message", "456", "--body-file", bodyFile}); err != nil {
		t.Fatalf("run discussion message: %v", err)
	}
}

func TestDiscussionConvertPostsPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/conversations/456/task_conversion" {
			t.Fatalf("%s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "missionbase-dev" {
			t.Fatalf("agent slug header = %q, want missionbase-dev", got)
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if payload["title"] != "Converted" || payload["description"] != "Body" || payload["assign_to_agent_slug"] != "missionbase-dev" {
			t.Fatalf("payload = %#v", payload)
		}
		_, _ = w.Write([]byte(`{"task":{"id":99}}`))
	}))
	defer server.Close()

	bodyFile := writeTextFile(t, "description.md", "Body")
	setAgentEnv(t, server.URL)
	if err := run([]string{"discussion", "convert", "456", "--title", "Converted", "--description-file", bodyFile, "--assign-agent", "missionbase-dev"}); err != nil {
		t.Fatalf("run discussion convert: %v", err)
	}
}

func TestConversationCommentRejectsBlankBody(t *testing.T) {
	bodyFile := writeTextFile(t, "blank.md", "   ")
	if err := run([]string{"discussion", "message", "456", "--body-file", bodyFile}); err == nil || !strings.Contains(err.Error(), "--body or at least one attachment") {
		t.Fatalf("err = %v, want blank body error", err)
	}
}

func TestConversationCommentPostsMultipartAttachment(t *testing.T) {
	attachment := writePNG(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/conversations/456/comments" {
			t.Fatalf("path = %s, want /api/v1/conversations/456/comments", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "multipart/form-data;") {
			t.Fatalf("content-type = %q, want multipart/form-data", got)
		}
		if err := r.ParseMultipartForm(6 * 1024 * 1024); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		if got := r.FormValue("message"); got != "See attached" {
			t.Fatalf("message = %q, want See attached", got)
		}
		if len(r.MultipartForm.File["attachments[]"]) != 1 {
			t.Fatalf("attachments count = %d, want 1", len(r.MultipartForm.File["attachments[]"]))
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"comment":{"id":655}}`))
	}))
	defer server.Close()

	bodyFile := writeTextFile(t, "discussion.md", "See attached")
	setAgentEnv(t, server.URL)
	if err := run([]string{"discussion", "reply", "456", "--body-file", bodyFile, "--attach", attachment}); err != nil {
		t.Fatalf("run discussion reply with attachment: %v", err)
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
		if payload["message"] != "Done and documented" {
			t.Fatalf("message payload = %q, want Done and documented", payload["message"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"comment":{"id":321,"content":"Done and documented"}}`))
	}))
	defer server.Close()

	bodyFile := writeTextFile(t, "comment.md", "Done and documented")
	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "message", "123", "--body-file", bodyFile}); err != nil {
		t.Fatalf("run task message: %v", err)
	}
}

func TestTaskCommentNormalizesEscapedNewlines(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if payload["message"] != "Done\n\n- documented" {
			t.Fatalf("message payload = %q, want normalized newlines", payload["message"])
		}

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"comment":{"id":324}}`))
	}))
	defer server.Close()

	bodyFile := writeTextFile(t, "comment.md", `Done\n\n- documented`)
	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "message", "123", "--body-file", bodyFile}); err != nil {
		t.Fatalf("run task message: %v", err)
	}
}

func TestTaskCommentPreservesMarkdownInlineAndFencedCode(t *testing.T) {
	body := "Findings:\\n\\n- Run `git pull --ff-only`, `xcodebuild`, `xcrun simctl`, and `./scripts/ios_build`.\\n- User is `iosagent`.\\n\\n```text\\nssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQ user@host\\n```"
	want := "Findings:\n\n- Run `git pull --ff-only`, `xcodebuild`, `xcrun simctl`, and `./scripts/ios_build`.\n- User is `iosagent`.\n\n```text\nssh-rsa AAAAB3NzaC1yc2EAAAADAQABAAACAQ user@host\n```"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if payload["message"] != want {
			t.Fatalf("message payload = %q, want %q", payload["message"], want)
		}

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"comment":{"id":327}}`))
	}))
	defer server.Close()

	bodyFile := writeTextFile(t, "comment.md", body)
	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "message", "123", "--body-file", bodyFile}); err != nil {
		t.Fatalf("run task message: %v", err)
	}
}

func TestTaskCommentReadsMarkdownBodyFile(t *testing.T) {
	want := "## Findings\n\n- Preserved `context: \"modal\"`\n\n```text\nquoted \"value\" and `ticks`\n```\n"
	bodyFile := filepath.Join(t.TempDir(), "comment.md")
	if err := os.WriteFile(bodyFile, []byte(want), 0o600); err != nil {
		t.Fatalf("write body file: %v", err)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if payload["message"] != want {
			t.Fatalf("message payload = %q, want %q", payload["message"], want)
		}

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"comment":{"id":325}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "message", "123", "--body-file", bodyFile}); err != nil {
		t.Fatalf("run task message with body file: %v", err)
	}
}

func TestConversationCommentRejectsBodyStdin(t *testing.T) {
	err := run([]string{"discussion", "message", "456", "--body-stdin"})
	if err == nil || !strings.Contains(err.Error(), "--body-stdin is not supported") {
		t.Fatalf("err = %v, want body stdin unsupported", err)
	}
}

func TestBodyFileDashRejectsStdinAlias(t *testing.T) {
	err := run([]string{"task", "message", "123", "--body-file", "-"})
	if err == nil || !strings.Contains(err.Error(), "stdin body input is not supported") {
		t.Fatalf("err = %v, want stdin body input unsupported", err)
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
		if payload["message"] != "Alias body" {
			t.Fatalf("message payload = %q, want Alias body", payload["message"])
		}

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"comment":{"id":322}}`))
	}))
	defer server.Close()

	bodyFile := writeTextFile(t, "comment.md", "Alias body")
	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "create-comment", "task-123", "--message-file", bodyFile}); err != nil {
		t.Fatalf("run task create-comment: %v", err)
	}
}

func TestTaskCommentRejectsBlankBody(t *testing.T) {
	bodyFile := writeTextFile(t, "blank.md", "   ")
	if err := run([]string{"task", "message", "123", "--body-file", bodyFile}); err == nil {
		t.Fatal("expected blank comment body error")
	}
}

func TestTaskCommentRejectsInlineBody(t *testing.T) {
	err := run([]string{"task", "message", "123", "--body", "inline"})
	if err == nil || !strings.Contains(err.Error(), "--body is not supported") {
		t.Fatalf("err = %v, want inline body unsupported", err)
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
		if got := r.FormValue("message"); got != "See screenshot" {
			t.Fatalf("message = %q, want See screenshot", got)
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

	bodyFile := writeTextFile(t, "comment.md", "See screenshot")
	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "message", "123", "--body-file", bodyFile, "--attach", attachment}); err != nil {
		t.Fatalf("run task message with attachment: %v", err)
	}
}

func TestTaskCreateReadsMarkdownBodyFile(t *testing.T) {
	want := "## Details\n\n- Preserved `context: \"modal\"`\n\n```text\nliteral `ticks` and \"quotes\"\n```\n"
	bodyFile := writeTextFile(t, "body.md", want)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tasks" {
			t.Fatalf("path = %s, want /api/v1/tasks", r.URL.Path)
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if payload["body"] != want {
			t.Fatalf("body = %q, want %q", payload["body"], want)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"task":{"id":986}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "create", "--title", "With body", "--box", "2", "--assign-agent", "missionbase-dev", "--body-file", bodyFile}); err != nil {
		t.Fatalf("run task create with body file: %v", err)
	}
}

func TestTaskCreateWithoutAssigneePostsUnassignedPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tasks" {
			t.Fatalf("path = %s, want /api/v1/tasks", r.URL.Path)
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if payload["title"] != "Unassigned" {
			t.Fatalf("title = %q, want Unassigned", payload["title"])
		}
		if payload["box_id"] != "2" {
			t.Fatalf("box_id = %q, want 2", payload["box_id"])
		}
		if _, ok := payload["assign_to_agent_slug"]; ok {
			t.Fatalf("assign_to_agent_slug unexpectedly present in payload: %#v", payload)
		}
		if _, ok := payload["assign_to_user_id"]; ok {
			t.Fatalf("assign_to_user_id unexpectedly present in payload: %#v", payload)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"task":{"id":989,"assignees":[]}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "create", "--title", "Unassigned", "--box", "2"}); err != nil {
		t.Fatalf("run task create without assignee: %v", err)
	}
}

func TestTaskCreateWithDeadlinePostsDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if payload["deadline"] != "2026-07-15" {
			t.Fatalf("deadline = %q, want 2026-07-15", payload["deadline"])
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"task":{"id":991,"deadline":"2026-07-15"}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "create", "--title", "With deadline", "--box", "2", "--deadline", "2026-07-15"}); err != nil {
		t.Fatalf("run task create with deadline: %v", err)
	}
}

func TestTaskCreateWithScheduledAtPostsScheduledAt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if payload["scheduled_at"] != "2026-07-15T09:30:00-04:00" {
			t.Fatalf("scheduled_at = %q, want timestamp", payload["scheduled_at"])
		}
		if _, ok := payload["deadline"]; ok {
			t.Fatalf("deadline unexpectedly present: %#v", payload)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"task":{"id":992,"scheduled_at":"2026-07-15T13:30:00Z"}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	stdout := captureStdout(t, func() {
		if err := run([]string{"task", "create", "--title", "Scheduled", "--box", "2", "--scheduled-at", "2026-07-15T09:30:00-04:00"}); err != nil {
			t.Fatalf("run task create with scheduled_at: %v", err)
		}
	})
	if !strings.Contains(stdout, `"scheduled_at":"2026-07-15T13:30:00Z"`) {
		t.Fatalf("stdout missing scheduled_at: %s", stdout)
	}
}

func TestTaskCreateRejectsInvalidDeadline(t *testing.T) {
	if err := run([]string{"task", "create", "--title", "Bad deadline", "--box", "2", "--deadline", "2026-99-99"}); err == nil || !strings.Contains(err.Error(), "deadline must be a valid date in YYYY-MM-DD format") {
		t.Fatalf("err = %v, want invalid deadline error", err)
	}
}

func TestTaskUpdateDeadlinePatchesDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Fatalf("method = %s, want PATCH", r.Method)
		}
		if r.URL.Path != "/api/v1/tasks/123" {
			t.Fatalf("path = %s, want /api/v1/tasks/123", r.URL.Path)
		}
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if payload["deadline"] != "2026-07-15" {
			t.Fatalf("deadline = %#v, want 2026-07-15", payload["deadline"])
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"task":{"id":123,"deadline":"2026-07-15"}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "update", "123", "--deadline", "2026-07-15"}); err != nil {
		t.Fatalf("run task update deadline: %v", err)
	}
}

func TestTaskUpdateNoDeadlinePatchesNullDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		value, ok := payload["deadline"]
		if !ok {
			t.Fatalf("deadline key missing from payload: %#v", payload)
		}
		if value != nil {
			t.Fatalf("deadline = %#v, want nil", value)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"task":{"id":123,"deadline":null}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "update", "123", "--no-deadline"}); err != nil {
		t.Fatalf("run task update --no-deadline: %v", err)
	}
}

func TestTaskUpdateScheduledAtPatchesScheduledAtWithoutDeadline(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if payload["scheduled_at"] != "2026-07-15 09:30" {
			t.Fatalf("scheduled_at = %#v, want datetime", payload["scheduled_at"])
		}
		if _, ok := payload["deadline"]; ok {
			t.Fatalf("deadline unexpectedly present: %#v", payload)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"task":{"id":123,"scheduled_at":"2026-07-15T13:30:00Z"}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	stdout := captureStdout(t, func() {
		if err := run([]string{"task", "update", "123", "--scheduled-at", "2026-07-15 09:30"}); err != nil {
			t.Fatalf("run task update --scheduled-at: %v", err)
		}
	})
	if !strings.Contains(stdout, `"scheduled_at":"2026-07-15T13:30:00Z"`) {
		t.Fatalf("stdout missing scheduled_at: %s", stdout)
	}
}

func TestTaskUpdateNoScheduledAtPatchesNullScheduledAt(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]any
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		value, ok := payload["scheduled_at"]
		if !ok {
			t.Fatalf("scheduled_at key missing from payload: %#v", payload)
		}
		if value != nil {
			t.Fatalf("scheduled_at = %#v, want nil", value)
		}
		if _, ok := payload["deadline"]; ok {
			t.Fatalf("deadline unexpectedly present: %#v", payload)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"task":{"id":123,"scheduled_at":null}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "update", "123", "--no-scheduled-at"}); err != nil {
		t.Fatalf("run task update --no-scheduled-at: %v", err)
	}
}

func TestTaskUpdateRejectsInvalidDeadline(t *testing.T) {
	if err := run([]string{"task", "update", "123", "--deadline", "not-a-date"}); err == nil || !strings.Contains(err.Error(), "deadline must be a valid date in YYYY-MM-DD format") {
		t.Fatalf("err = %v, want invalid deadline error", err)
	}
}

func TestTaskUpdateRejectsDeadlineAndNoDeadline(t *testing.T) {
	if err := run([]string{"task", "update", "123", "--deadline", "2026-07-15", "--no-deadline"}); err == nil || !strings.Contains(err.Error(), "use only one of --deadline or --no-deadline") {
		t.Fatalf("err = %v, want mutually exclusive deadline error", err)
	}
}

func TestTaskUpdateRejectsScheduledAtAndNoScheduledAt(t *testing.T) {
	if err := run([]string{"task", "update", "123", "--scheduled-at", "2026-07-15 09:30", "--no-scheduled-at"}); err == nil || !strings.Contains(err.Error(), "use only one of --scheduled-at or --no-scheduled-at") {
		t.Fatalf("err = %v, want mutually exclusive scheduled_at error", err)
	}
}

func TestTaskHelpDocumentsDeadlineAndSchedulingOptions(t *testing.T) {
	stdout := captureStdout(t, func() {
		printHelp()
	})
	for _, want := range []string{"--body-file PATH", "Update task deadline", "--deadline YYYY-MM-DD", "--scheduled-at DATETIME", "--no-scheduled-at", "--scheduled actionable|future|all", "scheduled_at separately from deadline"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("help missing %q in:\n%s", want, stdout)
		}
	}
}

func TestTaskCreatePreservesAssignedAgentPayload(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if payload["assign_to_agent_slug"] != "missionbase-dev" {
			t.Fatalf("assign_to_agent_slug = %q, want missionbase-dev", payload["assign_to_agent_slug"])
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"task":{"id":990}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "create", "--title", "Assigned", "--box", "2", "--assign-agent", "missionbase-dev"}); err != nil {
		t.Fatalf("run assigned task create: %v", err)
	}
}

func TestTaskCreateRejectsBothAgentAndUserAssignment(t *testing.T) {
	if err := run([]string{"task", "create", "--title", "Both", "--box", "2", "--assign-agent", "missionbase-dev", "--assign-user", "42"}); err == nil || !strings.Contains(err.Error(), "use only one of --assign-agent or --assign-user") {
		t.Fatalf("err = %v, want mutually exclusive assignment error", err)
	}
}

func TestTaskCreateRejectsInlineBodyAndStdin(t *testing.T) {
	if err := run([]string{"task", "create", "--title", "Inline", "--box", "2", "--assign-agent", "missionbase-dev", "--body", "inline"}); err == nil || !strings.Contains(err.Error(), "--body is not supported") {
		t.Fatalf("err = %v, want inline body unsupported", err)
	}
	if err := run([]string{"task", "create", "--title", "Stdin", "--box", "2", "--assign-agent", "missionbase-dev", "--body-stdin"}); err == nil || !strings.Contains(err.Error(), "--body-stdin is not supported") {
		t.Fatalf("err = %v, want body stdin unsupported", err)
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

func TestTaskCreatePostsHEICMultipartAttachment(t *testing.T) {
	attachment := writeHEIC(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := r.ParseMultipartForm(6 * 1024 * 1024); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		files := r.MultipartForm.File["attachments[]"]
		if len(files) != 1 {
			t.Fatalf("attachments count = %d, want 1", len(files))
		}
		if got := files[0].Header.Get("Content-Type"); got != "image/heic" {
			t.Fatalf("attachment content type = %q, want image/heic", got)
		}
		if files[0].Filename != filepath.Base(attachment) {
			t.Fatalf("filename = %q", files[0].Filename)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"task":{"id":988}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "create", "--title", "With HEIC attachment", "--box", "2", "--assign-agent", "missionbase-dev", "--attach", attachment}); err != nil {
		t.Fatalf("run task create with HEIC attachment: %v", err)
	}
}

func TestDocumentShowGetsMarkdownByDefault(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/v1/documents/77" {
			t.Fatalf("path = %s, want /api/v1/documents/77", r.URL.Path)
		}
		if got := r.URL.Query().Get("format"); got != "markdown" {
			t.Fatalf("format = %q, want markdown", got)
		}
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "missionbase-dev" {
			t.Fatalf("agent slug header = %q, want missionbase-dev", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"document":{"id":77,"format":"markdown","url":"https://dash.missionbase.app/boxes/2/files/77","body":"# Heading\n\nBody"}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	var stderr string
	stdout := captureStdout(t, func() {
		stderr = captureStderr(t, func() {
			if err := run([]string{"document", "show", "77"}); err != nil {
				t.Fatalf("run document show: %v", err)
			}
		})
	})
	if stdout != "# Heading\n\nBody\n" {
		t.Fatalf("stdout = %q", stdout)
	}
	if !strings.Contains(stderr, "Document URL: https://dash.missionbase.app/boxes/2/files/77") {
		t.Fatalf("stderr = %q, want document URL", stderr)
	}
}

func TestDocumentSubcommandHelpDoesNotCallAPI(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"show", []string{"document", "show", "--help"}, "missionbase-agent document show <document-id> [--format markdown|html|plain-text]"},
		{"fetch", []string{"document", "fetch", "--help"}, "missionbase-agent document fetch <document-id> [--format markdown|html|plain-text]"},
		{"edit", []string{"document", "edit", "--help"}, "missionbase-agent document edit <document-id> [--title TITLE] --body-file PATH"},
	} {
		stdout := captureStdout(t, func() {
			if err := run(tc.args); err != nil {
				t.Fatalf("run %s --help: %v", tc.name, err)
			}
		})
		if !strings.Contains(stdout, tc.want) {
			t.Fatalf("%s help missing %q in:\n%s", tc.name, tc.want, stdout)
		}
	}
}

func TestDocumentShowSupportsFormats(t *testing.T) {
	formats := []string{"markdown", "html", "plain-text"}
	for _, format := range formats {
		t.Run(format, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if got := r.URL.Query().Get("format"); got != format {
					t.Fatalf("format = %q, want %s", got, format)
				}
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"document":{"id":77,"body":"ok"}}`))
			}))
			defer server.Close()

			setAgentEnv(t, server.URL)
			if err := run([]string{"document", "show", "77", "--format", format}); err != nil {
				t.Fatalf("run document show --format %s: %v", format, err)
			}
		})
	}
}

func TestDocumentFetchRemainsCompatibilityAlias(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/documents/77" {
			t.Fatalf("path = %s, want /api/v1/documents/77", r.URL.Path)
		}
		if got := r.URL.Query().Get("format"); got != "plain-text" {
			t.Fatalf("format = %q, want plain-text", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"document":{"id":77,"body":"ok"}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"document", "fetch", "77", "--format", "plain-text"}); err != nil {
		t.Fatalf("run document fetch alias: %v", err)
	}
}

func TestDocumentFetchRejectsInvalidFormat(t *testing.T) {
	if err := run([]string{"document", "fetch", "77", "--format", "json"}); err == nil || !strings.Contains(err.Error(), "invalid document format") {
		t.Fatalf("err = %v, want invalid format", err)
	}
}

func TestDocumentCreatePostsFileBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Fatalf("method = %s, want POST", r.Method)
		}
		if r.URL.Path != "/api/v1/boxes/2/documents" {
			t.Fatalf("path = %s, want /api/v1/boxes/2/documents", r.URL.Path)
		}
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "missionbase-dev" {
			t.Fatalf("agent slug header = %q, want missionbase-dev", got)
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if payload["title"] != "Runbook" {
			t.Fatalf("title = %q, want Runbook", payload["title"])
		}
		if payload["body"] != "# Heading\n\nLine 2" {
			t.Fatalf("body = %q", payload["body"])
		}
		if payload["folder_id"] != "67" {
			t.Fatalf("folder_id = %q, want 67", payload["folder_id"])
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"document":{"id":77,"title":"Runbook","url":"https://dash.missionbase.app/boxes/2/files/77","box_file_id":88,"parent_folder_id":67,"version_count":1}}`))
	}))
	defer server.Close()

	bodyFile := writeTextFile(t, "document.md", "# Heading\n\nLine 2")
	setAgentEnv(t, server.URL)
	stdout := captureStdout(t, func() {
		if err := run([]string{"document", "create", "--box", "2", "--title", "Runbook", "--body-file", bodyFile, "--folder", "67"}); err != nil {
			t.Fatalf("run document create: %v", err)
		}
	})
	if !strings.Contains(stdout, `"url":"https://dash.missionbase.app/boxes/2/files/77"`) {
		t.Fatalf("stdout = %q, want document url", stdout)
	}
	if !strings.Contains(stdout, `"box_file_id":88`) || !strings.Contains(stdout, `"parent_folder_id":67`) {
		t.Fatalf("stdout = %q, want backing BoxFile metadata", stdout)
	}
}

func TestDocumentCreateRootPlacement(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if payload["folder_id"] != "root" {
			t.Fatalf("folder_id = %q, want root", payload["folder_id"])
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"document":{"id":77,"title":"Runbook","box_file_id":88}}`))
	}))
	defer server.Close()

	bodyFile := writeTextFile(t, "document.md", "Body")
	setAgentEnv(t, server.URL)
	if err := run([]string{"document", "create", "--box", "2", "--title", "Runbook", "--body-file", bodyFile, "--root"}); err != nil {
		t.Fatalf("run document create: %v", err)
	}
}

func TestDocumentCreateRejectsFolderAndRootTogether(t *testing.T) {
	bodyFile := writeTextFile(t, "document.md", "Body")
	if err := run([]string{"document", "create", "--box", "2", "--title", "Runbook", "--body-file", bodyFile, "--folder", "67", "--root"}); err == nil || !strings.Contains(err.Error(), "use only one of --folder or --root") {
		t.Fatalf("err = %v, want mutually exclusive folder/root", err)
	}
}

func TestDocumentEditPatchesFileBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Fatalf("method = %s, want PATCH", r.Method)
		}
		if r.URL.Path != "/api/v1/documents/77" {
			t.Fatalf("path = %s, want /api/v1/documents/77", r.URL.Path)
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if payload["title"] != "Updated" || payload["body"] != "Updated body" {
			t.Fatalf("payload = %#v", payload)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"document":{"id":77,"title":"Updated","version_count":2}}`))
	}))
	defer server.Close()

	bodyFile := writeTextFile(t, "document.md", "Updated body")
	setAgentEnv(t, server.URL)
	if err := run([]string{"document", "edit", "77", "--title", "Updated", "--body-file", bodyFile}); err != nil {
		t.Fatalf("run document edit: %v", err)
	}
}

func TestTaskMovePatchesBoxID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch {
			t.Fatalf("method = %s, want PATCH", r.Method)
		}
		if r.URL.Path != "/api/v1/tasks/123" {
			t.Fatalf("path = %s, want /api/v1/tasks/123", r.URL.Path)
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if payload["box_id"] != "42" {
			t.Fatalf("box_id = %q, want 42", payload["box_id"])
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"task":{"id":123,"box":{"id":42}}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "move", "123", "--box", "42"}); err != nil {
		t.Fatalf("run task move: %v", err)
	}
}

func TestTaskMoveRequiresBox(t *testing.T) {
	if err := run([]string{"task", "move", "123"}); err == nil || !strings.Contains(err.Error(), "task move <task-id> --box BOX_ID") {
		t.Fatalf("err = %v, want usage error", err)
	}
	if err := run([]string{"task", "move", "123", "--status", "todo"}); err == nil || !strings.Contains(err.Error(), "unknown task move option") {
		t.Fatalf("err = %v, want unknown option error", err)
	}
}

func TestDocumentCommandsRequireFileBody(t *testing.T) {
	bodyFile := writeTextFile(t, "document.md", "Body")
	if err := run([]string{"document", "create", "--box", "2", "--title", "Runbook", "--body", "Body"}); err == nil || !strings.Contains(err.Error(), "use --body-file PATH") {
		t.Fatalf("err = %v, want body-file error", err)
	}
	if err := run([]string{"document", "create", "--box", "2", "--title", "Runbook"}); err == nil || !strings.Contains(err.Error(), "--body is required") {
		t.Fatalf("err = %v, want body required", err)
	}
	if err := run([]string{"document", "create", "--title", "Runbook", "--body-file", bodyFile}); err == nil || !strings.Contains(err.Error(), "--box is required") {
		t.Fatalf("err = %v, want box required", err)
	}
	if err := run([]string{"document", "edit", "77", "--body-file", "-"}); err == nil || !strings.Contains(err.Error(), "stdin body input is not supported") {
		t.Fatalf("err = %v, want stdin unsupported", err)
	}
}

func TestAttachmentRejectsUnsupportedType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("plain text"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"task", "message", "123", "--attach", path}); err == nil || !strings.Contains(err.Error(), "unsupported attachment type") {
		t.Fatalf("err = %v, want unsupported attachment type", err)
	}
}

func TestSidebarSubcommandHelpDoesNotResolveUser(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"pins", []string{"sidebar", "pins", "--help"}, "missionbase-agent sidebar pins --user ID|@mention"},
		{"pin", []string{"sidebar", "pin", "--help"}, "missionbase-agent sidebar pin --user ID|@mention --type box_file --id ID"},
		{"unpin", []string{"sidebar", "unpin", "--help"}, "missionbase-agent sidebar unpin --user ID|@mention --type box_file --id ID"},
	} {
		stdout := captureStdout(t, func() {
			if err := run(tc.args); err != nil {
				t.Fatalf("run %s --help: %v", tc.name, err)
			}
		})
		if !strings.Contains(stdout, tc.want) {
			t.Fatalf("%s help missing %q in:\n%s", tc.name, tc.want, stdout)
		}
	}
}

func TestSidebarCommandsUseAgentEndpointWithTargetUser(t *testing.T) {
	seen := map[string]bool{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "missionbase-dev" {
			t.Fatalf("agent slug header = %q, want missionbase-dev", got)
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /api/v1/sidebar_pins":
			seen["pins"] = true
			if r.URL.Query().Get("user_id") != "7" {
				t.Fatalf("query = %s", r.URL.RawQuery)
			}
		case "POST /api/v1/sidebar_pins":
			seen["pin"] = true
			var payload map[string]string
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				t.Fatal(err)
			}
			if payload["user_id"] != "7" || payload["type"] != "box_file" || payload["id"] != "42" {
				t.Fatalf("payload = %#v", payload)
			}
		case "DELETE /api/v1/sidebar_pins":
			seen["unpin"] = true
			query := r.URL.Query()
			if query.Get("user_id") != "7" || query.Get("type") != "box_file" || query.Get("id") != "42" {
				t.Fatalf("query = %s", r.URL.RawQuery)
			}
		default:
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.String())
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	commands := [][]string{
		{"sidebar", "pins", "--user", "7"},
		{"sidebar", "pin", "--user", "7", "--type", "box_file", "--id", "42"},
		{"sidebar", "unpin", "--user", "7", "--type", "box_file", "--id", "42"},
	}
	for _, command := range commands {
		if err := run(command); err != nil {
			t.Fatalf("run %v: %v", command, err)
		}
	}
	for _, key := range []string{"pins", "pin", "unpin"} {
		if !seen[key] {
			t.Fatalf("%s request was not seen", key)
		}
	}
}

func TestActivityCommandBuildsBoxQueryAndPrintsSummary(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "missionbase-dev" {
			t.Fatalf("agent slug header = %q, want missionbase-dev", got)
		}
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/boxes/2/activity_events" {
			t.Fatalf("request = %s %s", r.Method, r.URL.String())
		}
		query := r.URL.Query()
		wants := map[string]string{
			"since":        "2026-07-01T00:00:00Z",
			"until":        "2026-07-02T00:00:00Z",
			"actor":        "Agent:2",
			"subject_type": "Task",
			"subject_id":   "2902",
			"action":       "task.updated",
			"cursor":       "99",
			"limit":        "5",
		}
		for key, want := range wants {
			if got := query.Get(key); got != want {
				t.Fatalf("query %s = %q, want %q (raw %s)", key, got, want, r.URL.RawQuery)
			}
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"activity_events":[{"id":100,"actor":{"type":"Agent","id":2,"name":"MissionbaseDev"},"scope":{"box":{"type":"Box","id":2,"name":"Missionbase"}},"subject":{"type":"Task","id":2902,"name":"Expose activity logging"},"action":"task.updated","timestamp":"2026-07-02T17:00:00Z","summary":"MissionbaseDev updated task 2902","source":"agent","metadata":{"field":"status"},"route":{"url":"https://dash.missionbase.app/tasks/2902"}}],"next_cursor":100}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	stdout := captureStdout(t, func() {
		if err := run([]string{"activity", "box", "2", "--since", "2026-07-01T00:00:00Z", "--until", "2026-07-02T00:00:00Z", "--actor", "Agent:2", "--subject-type", "Task", "--subject-id", "2902", "--action", "task.updated", "--cursor", "99", "--limit", "5"}); err != nil {
			t.Fatalf("run activity box: %v", err)
		}
	})
	for _, want := range []string{"2026-07-02T17:00:00Z", "Box #2 Missionbase", "Agent #2 MissionbaseDev", "task.updated", "Task #2902 Expose activity logging", "MissionbaseDev updated task 2902", "metadata:", "Next cursor: 100"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("stdout missing %q in:\n%s", want, stdout)
		}
	}
}

func TestBoxesActivityAliasAndJSONOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/boxes/2/activity_events" {
			t.Fatalf("request = %s %s", r.Method, r.URL.String())
		}
		if got := r.URL.Query().Get("limit"); got != "1" {
			t.Fatalf("limit = %q, want 1", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"activity_events":[],"next_cursor":null}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	stdout := captureStdout(t, func() {
		if err := run([]string{"boxes", "activity", "2", "--limit", "1", "--json"}); err != nil {
			t.Fatalf("run boxes activity: %v", err)
		}
	})
	if !strings.Contains(stdout, `"activity_events":[]`) {
		t.Fatalf("stdout = %s", stdout)
	}
}

func TestActivityTeamDurationBuildsTeamEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/api/v1/teams/7/activity_events" {
			t.Fatalf("request = %s %s", r.Method, r.URL.String())
		}
		if r.URL.Query().Get("since") == "" {
			t.Fatalf("since missing from duration query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"activity_events":[],"next_cursor":null}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	stdout := captureStdout(t, func() {
		if err := run([]string{"activity", "team", "7", "--duration", "24h"}); err != nil {
			t.Fatalf("run activity team: %v", err)
		}
	})
	if !strings.Contains(stdout, "No activity events found.") {
		t.Fatalf("stdout = %s", stdout)
	}
}

func TestActivityHelpMentionsJSONAndFilters(t *testing.T) {
	stdout := captureStdout(t, func() {
		if err := run([]string{"activity", "--help"}); err != nil {
			t.Fatalf("run activity help: %v", err)
		}
	})
	for _, want := range []string{"missionbase-agent activity <box|team> <id>", "--since TIME", "--cursor ID", "--json"} {
		if !strings.Contains(stdout, want) {
			t.Fatalf("help missing %q in:\n%s", want, stdout)
		}
	}
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

func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	original := os.Stderr
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	os.Stderr = writer
	fn()
	_ = writer.Close()
	os.Stderr = original
	body, err := io.ReadAll(reader)
	if err != nil {
		t.Fatal(err)
	}
	return string(body)
}

func writeTextFile(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
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

func writeHEIC(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "iphone-screenshot.heic")
	heic := []byte{0, 0, 0, 24, 'f', 't', 'y', 'p', 'h', 'e', 'i', 'c', 0, 0, 0, 0, 'm', 'i', 'f', '1'}
	if err := os.WriteFile(path, heic, 0o600); err != nil {
		t.Fatal(err)
	}
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
