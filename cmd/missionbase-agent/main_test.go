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
	if err := run([]string{"tasks", "--user", "@DanielLemky", "--due", "today", "--box", "2", "--status-category", "open", "--include-closed", "--page", "2", "--per-page", "25", "--json"}); err != nil {
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

func TestWorkNextGetsNextTaskEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Fatalf("method = %s, want GET", r.Method)
		}
		if r.URL.Path != "/api/v1/agent/work" {
			t.Fatalf("path = %s, want /api/v1/agent/work", r.URL.Path)
		}
		if got := r.URL.Query().Get("next"); got != "true" {
			t.Fatalf("next query = %q, want true", got)
		}
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "missionbase-dev" {
			t.Fatalf("agent slug header = %q, want missionbase-dev", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"tasks":[{"id":2420}],"unread_conversations":[],"unread_direct_messages":[],"meta":{"tasks":1,"unread_conversations":0,"unread_direct_messages":0,"total":1,"actionable":true}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"work", "--next"}); err != nil {
		t.Fatalf("run work --next: %v", err)
	}
}

func TestWorkNextTaskAliasGetsNextTaskEndpoint(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/agent/work" {
			t.Fatalf("path = %s, want /api/v1/agent/work", r.URL.Path)
		}
		if got := r.URL.Query().Get("next"); got != "true" {
			t.Fatalf("next query = %q, want true", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"tasks":[],"unread_conversations":[],"unread_direct_messages":[],"meta":{"tasks":0,"unread_conversations":0,"unread_direct_messages":0,"total":0,"actionable":false}}`))
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
		_, _ = w.Write([]byte(`{"discussions":[{"id":7,"title":"Standalone","feed_id":77}],"meta":{"total":1}}`))
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
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "missionbase-dev" {
			t.Fatalf("agent slug header = %q, want missionbase-dev", got)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"files":[{"id":77,"title":"Runbook","type":"document","kind":"document","url":"https://dash.missionbase.app/boxes/2/files/77","fetch_id":77,"fetch_type":"document"}],"meta":{"total":1,"page":2,"per_page":25,"query":"runbook"}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"boxes", "files", "2", "--query", "runbook", "--page", "2", "--per-page", "25"}); err != nil {
		t.Fatalf("run boxes files: %v", err)
	}
}

func TestBoxesFilesRequiresBoxIDAndOptionValues(t *testing.T) {
	if err := run([]string{"boxes", "files"}); err == nil || !strings.Contains(err.Error(), "usage: missionbase-agent boxes files <box-id>") {
		t.Fatalf("err = %v, want usage error", err)
	}
	if err := run([]string{"boxes", "files", "2", "--query"}); err == nil || !strings.Contains(err.Error(), "--query requires a value") {
		t.Fatalf("err = %v, want query value error", err)
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
		_, _ = w.Write([]byte(`{"discussion":{"id":8,"title":"Planning","feed_id":88,"box_id":2}}`))
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
		if payload["comment"] != "General conversation reply" {
			t.Fatalf("comment payload = %q, want General conversation reply", payload["comment"])
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"comment":{"id":654,"feed_id":456,"body":"General conversation reply"}}`))
	}))
	defer server.Close()

	bodyFile := writeTextFile(t, "conversation.md", "General conversation reply")
	setAgentEnv(t, server.URL)
	if err := run([]string{"conversation", "comment", "456", "--body-file", bodyFile}); err != nil {
		t.Fatalf("run conversation comment: %v", err)
	}
}

func TestConversationCommentRejectsBlankBody(t *testing.T) {
	bodyFile := writeTextFile(t, "blank.md", "   ")
	if err := run([]string{"conversation", "comment", "456", "--body-file", bodyFile}); err == nil || !strings.Contains(err.Error(), "--body or at least one attachment") {
		t.Fatalf("err = %v, want blank body error", err)
	}
}

func TestConversationCommentPostsMultipartAttachment(t *testing.T) {
	attachment := writePNG(t)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/conversations/feed-456/comments" {
			t.Fatalf("path = %s, want /api/v1/conversations/feed-456/comments", r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); !strings.HasPrefix(got, "multipart/form-data;") {
			t.Fatalf("content-type = %q, want multipart/form-data", got)
		}
		if err := r.ParseMultipartForm(6 * 1024 * 1024); err != nil {
			t.Fatalf("parse multipart: %v", err)
		}
		if got := r.FormValue("comment"); got != "See attached" {
			t.Fatalf("comment = %q, want See attached", got)
		}
		if len(r.MultipartForm.File["attachments[]"]) != 1 {
			t.Fatalf("attachments count = %d, want 1", len(r.MultipartForm.File["attachments[]"]))
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"comment":{"id":655}}`))
	}))
	defer server.Close()

	bodyFile := writeTextFile(t, "conversation.md", "See attached")
	setAgentEnv(t, server.URL)
	if err := run([]string{"conversation", "reply", "feed-456", "--body-file", bodyFile, "--attach", attachment}); err != nil {
		t.Fatalf("run conversation reply with attachment: %v", err)
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

	bodyFile := writeTextFile(t, "comment.md", "Done and documented")
	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "comment", "123", "--body-file", bodyFile}); err != nil {
		t.Fatalf("run task comment: %v", err)
	}
}

func TestTaskCommentNormalizesEscapedNewlines(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if payload["comment"] != "Done\n\n- documented" {
			t.Fatalf("comment payload = %q, want normalized newlines", payload["comment"])
		}

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"comment":{"id":324}}`))
	}))
	defer server.Close()

	bodyFile := writeTextFile(t, "comment.md", `Done\n\n- documented`)
	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "comment", "123", "--body-file", bodyFile}); err != nil {
		t.Fatalf("run task comment: %v", err)
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
		if payload["comment"] != want {
			t.Fatalf("comment payload = %q, want %q", payload["comment"], want)
		}

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"comment":{"id":327}}`))
	}))
	defer server.Close()

	bodyFile := writeTextFile(t, "comment.md", body)
	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "comment", "123", "--body-file", bodyFile}); err != nil {
		t.Fatalf("run task comment: %v", err)
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
		if payload["comment"] != want {
			t.Fatalf("comment payload = %q, want %q", payload["comment"], want)
		}

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"comment":{"id":325}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "comment", "123", "--body-file", bodyFile}); err != nil {
		t.Fatalf("run task comment with body file: %v", err)
	}
}

func TestConversationCommentRejectsBodyStdin(t *testing.T) {
	err := run([]string{"conversation", "comment", "456", "--body-stdin"})
	if err == nil || !strings.Contains(err.Error(), "--body-stdin is not supported") {
		t.Fatalf("err = %v, want body stdin unsupported", err)
	}
}

func TestBodyFileDashRejectsStdinAlias(t *testing.T) {
	err := run([]string{"task", "comment", "123", "--body-file", "-"})
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
		if payload["comment"] != "Alias body" {
			t.Fatalf("comment payload = %q, want Alias body", payload["comment"])
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
	if err := run([]string{"task", "comment", "123", "--body-file", bodyFile}); err == nil {
		t.Fatal("expected blank comment body error")
	}
}

func TestTaskCommentRejectsInlineBody(t *testing.T) {
	err := run([]string{"task", "comment", "123", "--body", "inline"})
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

	bodyFile := writeTextFile(t, "comment.md", "See screenshot")
	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "comment", "123", "--body-file", bodyFile, "--attach", attachment}); err != nil {
		t.Fatalf("run task comment with attachment: %v", err)
	}
}

func TestTaskCreateReadsMarkdownDescriptionFile(t *testing.T) {
	want := "## Details\n\n- Preserved `context: \"modal\"`\n\n```text\nliteral `ticks` and \"quotes\"\n```\n"
	descriptionFile := writeTextFile(t, "description.md", want)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/tasks" {
			t.Fatalf("path = %s, want /api/v1/tasks", r.URL.Path)
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if payload["description"] != want {
			t.Fatalf("description = %q, want %q", payload["description"], want)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"task":{"id":986}}`))
	}))
	defer server.Close()

	setAgentEnv(t, server.URL)
	if err := run([]string{"task", "create", "--title", "With description", "--box", "2", "--assign-agent", "missionbase-dev", "--description-file", descriptionFile}); err != nil {
		t.Fatalf("run task create with description file: %v", err)
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

func TestTaskHelpDocumentsDeadlineOptions(t *testing.T) {
	stdout := captureStdout(t, func() {
		printHelp()
	})
	for _, want := range []string{"--deadline YYYY-MM-DD", "task update <task-id> (--deadline YYYY-MM-DD | --no-deadline)"} {
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

func TestTaskCreateRejectsInlineDescriptionAndStdin(t *testing.T) {
	if err := run([]string{"task", "create", "--title", "Inline", "--box", "2", "--assign-agent", "missionbase-dev", "--description", "inline"}); err == nil || !strings.Contains(err.Error(), "--description is not supported") {
		t.Fatalf("err = %v, want inline description unsupported", err)
	}
	if err := run([]string{"task", "create", "--title", "Stdin", "--box", "2", "--assign-agent", "missionbase-dev", "--description-stdin"}); err == nil || !strings.Contains(err.Error(), "--description-stdin is not supported") {
		t.Fatalf("err = %v, want description stdin unsupported", err)
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

func TestDocumentFetchGetsMarkdownByDefault(t *testing.T) {
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
			if err := run([]string{"document", "fetch", "77"}); err != nil {
				t.Fatalf("run document fetch: %v", err)
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

func TestDocumentFetchSupportsFormats(t *testing.T) {
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
			if err := run([]string{"document", "fetch", "77", "--format", format}); err != nil {
				t.Fatalf("run document fetch --format %s: %v", format, err)
			}
		})
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
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"document":{"id":77,"title":"Runbook","url":"https://dash.missionbase.app/boxes/2/files/77","version_count":1}}`))
	}))
	defer server.Close()

	bodyFile := writeTextFile(t, "document.md", "# Heading\n\nLine 2")
	setAgentEnv(t, server.URL)
	stdout := captureStdout(t, func() {
		if err := run([]string{"document", "create", "--box", "2", "--title", "Runbook", "--body-file", bodyFile}); err != nil {
			t.Fatalf("run document create: %v", err)
		}
	})
	if !strings.Contains(stdout, `"url":"https://dash.missionbase.app/boxes/2/files/77"`) {
		t.Fatalf("stdout = %q, want document url", stdout)
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
	if err := run([]string{"task", "comment", "123", "--attach", path}); err == nil || !strings.Contains(err.Error(), "unsupported attachment type") {
		t.Fatalf("err = %v, want unsupported attachment type", err)
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
