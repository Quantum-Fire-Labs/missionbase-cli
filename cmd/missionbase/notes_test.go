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

type notesRequest struct {
	method string
	path   string
	query  string
	body   map[string]any
}

// notesServer records each request and answers with the response for its "METHOD path".
func notesServer(t *testing.T, responses map[string]string) (*[]notesRequest, func()) {
	t.Helper()
	var requests []notesRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "" {
			t.Errorf("agent slug header = %q, want none: notes act as the person", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer user-token" {
			t.Errorf("authorization = %q", got)
		}
		request := notesRequest{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery}
		if raw, _ := io.ReadAll(r.Body); len(raw) > 0 {
			if err := json.Unmarshal(raw, &request.body); err != nil {
				t.Errorf("body is not JSON: %s", raw)
			}
		}
		requests = append(requests, request)
		response, ok := responses[r.Method+" "+r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"error":{"code":"not_found","message":"Project not found"}}`))
			return
		}
		_, _ = w.Write([]byte(response))
	}))
	setUserEnv(t, server.URL)
	return &requests, server.Close
}

func runNotes(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var err error
	output := captureStdout(t, func() { err = run(append([]string{"notes"}, args...)) })
	return output, err
}

func TestNotesDailyShowDefaultsToTodayAndPrintsTheMarkdown(t *testing.T) {
	requests, done := notesServer(t, map[string]string{
		"GET /api/v1/daily_notes/today":      `{"daily_note":{"date":"2026-11-22","body":"# Sunday\n- Called the bank","updated_at":"2026-11-22T09:00:00-05:00"}}`,
		"GET /api/v1/daily_notes/2026-11-20": `{"daily_note":{"date":"2026-11-20","body":"","updated_at":null}}`,
	})
	defer done()

	output, err := runNotes(t, "daily", "show")
	if err != nil {
		t.Fatal(err)
	}
	if output != "# Sunday\n- Called the bank\n" {
		t.Fatalf("output = %q", output)
	}

	output, err = runNotes(t, "daily", "show", "2026-11-20")
	if err != nil || output != "(no note for 2026-11-20 yet)\n" {
		t.Fatalf("output = %q, err = %v", output, err)
	}

	output, err = runNotes(t, "daily", "show", "2026-11-20", "--json")
	if err != nil || !strings.Contains(output, `"daily_note":{"date":"2026-11-20"`) {
		t.Fatalf("json output = %q, err = %v", output, err)
	}
	if len(*requests) != 3 || (*requests)[0].method != "GET" {
		t.Fatalf("requests = %+v", *requests)
	}
}

func TestNotesDailyEditAndAppendSendTheBody(t *testing.T) {
	requests, done := notesServer(t, map[string]string{
		"PATCH /api/v1/daily_notes/2026-11-20":        `{"daily_note":{"date":"2026-11-20","body":"Replaced"}}`,
		"POST /api/v1/daily_notes/today/appends":      `{"daily_note":{"date":"2026-11-22","body":"x\n\nMore"}}`,
		"POST /api/v1/daily_notes/2026-11-21/appends": `{"daily_note":{"date":"2026-11-21","body":"From a file"}}`,
	})
	defer done()
	bodyFile := filepath.Join(t.TempDir(), "note.md")
	if err := os.WriteFile(bodyFile, []byte("From a file\\n kept as written\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	output, err := runNotes(t, "daily", "edit", "2026-11-20", "--body", "Replaced")
	if err != nil || output != "Saved the daily note for 2026-11-20.\n" {
		t.Fatalf("edit output = %q, err = %v", output, err)
	}
	output, err = runNotes(t, "daily", "append", "--body", `More\nlines`)
	if err != nil || output != "Added to the daily note for 2026-11-22.\n" {
		t.Fatalf("append output = %q, err = %v", output, err)
	}
	if _, err = runNotes(t, "daily", "append", "2026-11-21", "--body-file", bodyFile); err != nil {
		t.Fatal(err)
	}

	got := *requests
	if got[0].method != "PATCH" || got[0].body["body"] != "Replaced" {
		t.Fatalf("edit request = %+v", got[0])
	}
	if got[1].path != "/api/v1/daily_notes/today/appends" || got[1].body["body"] != "More\nlines" {
		t.Fatalf("append request = %+v", got[1])
	}
	if got[2].body["body"] != "From a file\\n kept as written\n" {
		t.Fatalf("a body file is sent as written, got %q", got[2].body["body"])
	}
}

func TestNotesDailyListPrintsDaysAndTheNextPage(t *testing.T) {
	requests, done := notesServer(t, map[string]string{
		"GET /api/v1/daily_notes": `{"daily_notes":[{"date":"2026-11-21","preview":"Rested","updated_at":"x"},{"date":"2026-11-20","preview":"Planned the retreat","updated_at":"x"}],"next_before":"2026-11-20"}`,
	})
	defer done()

	output, err := runNotes(t, "daily", "list", "--before", "2026-11-22", "--limit", "2")
	if err != nil {
		t.Fatal(err)
	}
	want := "2026-11-21  Rested\n2026-11-20  Planned the retreat\nOlder days: missionbase notes daily list --before 2026-11-20\n"
	if output != want {
		t.Fatalf("output = %q", output)
	}
	if query := (*requests)[0].query; query != "before=2026-11-22&limit=2" {
		t.Fatalf("query = %q", query)
	}
}

func TestNotesProjectShowEditAndAppend(t *testing.T) {
	requests, done := notesServer(t, map[string]string{
		"GET /api/v1/projects/5/notes":          `{"notes":{"project_id":5,"body":"## Goal\nOpen the cafe","version":{"id":12,"summary":"edited Goal","author":{"name":"Daniel Lemky"}}}}`,
		"PATCH /api/v1/projects/5/notes":        `{"notes":{"body":"x","version":{"id":13,"summary":"edited Goal"}}}`,
		"POST /api/v1/projects/5/notes/entries": `{"notes":{"body":"x"},"entry":{"id":14,"summary":"appended to the end"}}`,
	})
	defer done()

	output, err := runNotes(t, "project", "show", "5")
	if err != nil || output != "<!-- version 12: Daniel Lemky edited Goal -->\n## Goal\nOpen the cafe\n" {
		t.Fatalf("show output = %q, err = %v", output, err)
	}
	output, err = runNotes(t, "project", "edit", "5", "--body", "## Goal\nOpen the cafe", "--base-version", "12")
	if err != nil || output != "Saved version 13: edited Goal\n" {
		t.Fatalf("edit output = %q, err = %v", output, err)
	}
	output, err = runNotes(t, "project", "append", "5", "--body", "Ordered the machine")
	if err != nil || output != "Saved version 14: appended to the end\n" {
		t.Fatalf("append output = %q, err = %v", output, err)
	}

	got := *requests
	if got[1].body["base_version_id"] != float64(12) || got[1].body["body"] != "## Goal\nOpen the cafe" {
		t.Fatalf("edit request = %+v", got[1])
	}
	if got[2].body["body"] != "Ordered the machine" || got[2].body["date"] != nil {
		t.Fatalf("append request = %+v", got[2])
	}
}

func TestNotesServerErrorsFail(t *testing.T) {
	_, done := notesServer(t, map[string]string{})
	defer done()

	_, err := runNotes(t, "project", "show", "99")
	if err == nil || !strings.Contains(err.Error(), "HTTP 404") || !strings.Contains(err.Error(), "Project not found") {
		t.Fatalf("err = %v", err)
	}
}

func TestNotesArgumentsAreCheckedBeforeSending(t *testing.T) {
	requests, done := notesServer(t, map[string]string{})
	defer done()
	emptyFile := filepath.Join(t.TempDir(), "empty.md")
	if err := os.WriteFile(emptyFile, []byte("  \n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cases := []struct {
		args []string
		want string
	}{
		{[]string{"daily", "show", "Nov-20"}, `date must be YYYY-MM-DD or today, got "Nov-20"`},
		{[]string{"daily", "show", "2026-02-30"}, `date must be YYYY-MM-DD or today`},
		{[]string{"daily", "show", "2026-11-20", "2026-11-21"}, `unexpected argument "2026-11-21"`},
		{[]string{"daily", "list", "2026-11-20"}, `unexpected argument "2026-11-20"`},
		{[]string{"daily", "list", "--before", "today"}, `--before must be YYYY-MM-DD`},
		{[]string{"daily", "list", "--limit", "0"}, `--limit must be a positive number`},
		{[]string{"daily", "edit"}, `--body or --body-file is required`},
		{[]string{"daily", "edit", "--body", "a", "--body-file", emptyFile}, `give --body or --body-file once`},
		{[]string{"daily", "append", "--body-file", emptyFile}, `nothing to append`},
		{[]string{"daily", "append", "--body-file", "/no/such/file"}, `read body file "/no/such/file"`},
		{[]string{"daily", "show", "--base-version", "3"}, `unknown notes daily show option "--base-version"`},
		{[]string{"daily", "remove"}, `unknown notes daily command "remove"`},
		{[]string{"daily", "edit", "--body"}, `--body requires a value`},
		{[]string{"project", "show"}, `a project id is required`},
		{[]string{"project", "show", "cafe"}, `project id must be a number, got "cafe"`},
		{[]string{"project", "edit", "5", "--body", "x", "--base-version", "latest"}, `--base-version must be a version id`},
		{[]string{"project", "append", "5", "--date", "2026-11-20", "--body", "x"}, `unknown notes project append option "--date"`},
		{[]string{"project", "history", "5"}, `unknown notes project command "history"`},
		{[]string{"weekly"}, `unknown notes command "weekly"`},
	}
	for _, tc := range cases {
		_, err := runNotes(t, tc.args...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("notes %v: err = %v, want %q", tc.args, err, tc.want)
		}
	}
	if len(*requests) != 0 {
		t.Fatalf("sent %d requests for bad arguments", len(*requests))
	}
}

func TestNotesHelpPrintsUsage(t *testing.T) {
	for _, args := range [][]string{{}, {"--help"}, {"daily"}, {"project", "-h"}, {"daily", "edit", "--help"}} {
		output, err := runNotes(t, args...)
		if err != nil || !strings.Contains(output, "missionbase notes daily append [DATE|today]") {
			t.Fatalf("notes %v: output = %q, err = %v", args, output, err)
		}
	}
}
