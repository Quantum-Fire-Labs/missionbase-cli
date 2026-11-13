package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	testLocalCoreJSON   = `{"core":{"id":"core-local","name":"omarchy-m3"},"live":3}`
	testMachineLsJSON   = `{"machines":[{"id":"core-m1","name":"omarchy-m1","host":"omarchy-m1","target":"omarchy-m1"},{"id":"core-intel","name":"omarchy-intel","host":"omarchy-mbp-intel","target":"omarchy-intel"}]}`
	testWorkTaskJSON    = `{"id":42,"title":"Ship it","status":"in_progress","status_label":"In Progress","running":{"agent":{"type":"Agent","slug":"missionbase-dev"},"machine_id":"core-local","machine_name":"omarchy-m3","terminal":"t6b","since":"2026-11-12T23:24:15Z","stale":false},"live_status":null,"attention":null}`
	testConflictBodyFmt = `{"error":{"code":"conflict","message":"Work on this task is already running elsewhere","details":{"running":[{"assignment_id":1,"agent":{"type":"Agent","name":"Claude","slug":"claude"},"machine_id":"core-intel","machine_name":"omarchy-intel","terminal":"t7","since":"2026-11-12T23:24:15Z","reason":null}]}}}`
)

// stubHC replaces the hc runner. Keys are the joined args; a missing key fails the test.
func stubHC(t *testing.T, responses map[string]string) *[]string {
	t.Helper()
	var calls []string
	original := runHC
	runHC = func(args ...string) ([]byte, error) {
		key := strings.Join(args, " ")
		calls = append(calls, key)
		out, ok := responses[key]
		if !ok {
			t.Errorf("unexpected hc call: hc %s", key)
			return nil, fmt.Errorf("unexpected hc call")
		}
		return []byte(out), nil
	}
	t.Cleanup(func() { runHC = original })
	return &calls
}

type recordedRequest struct {
	method  string
	path    string
	query   string
	slug    string
	payload map[string]any
}

// recordServer answers each request with the response for "METHOD /path" and records it.
func recordServer(t *testing.T, responses map[string]func(r *http.Request) (int, string)) (*httptest.Server, *[]recordedRequest) {
	t.Helper()
	var requests []recordedRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		rec := recordedRequest{method: r.Method, path: r.URL.Path, query: r.URL.RawQuery, slug: r.Header.Get("X-Missionbase-Agent-Slug")}
		if len(body) > 0 {
			if err := json.Unmarshal(body, &rec.payload); err != nil {
				t.Errorf("request body is not JSON: %q", body)
			}
		}
		requests = append(requests, rec)
		respond, ok := responses[r.Method+" "+r.URL.Path]
		if !ok {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.String())
			w.WriteHeader(http.StatusNotFound)
			return
		}
		status, response := respond(r)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(response))
	}))
	t.Cleanup(server.Close)
	return server, &requests
}

func fixed(status int, body string) func(*http.Request) (int, string) {
	return func(*http.Request) (int, string) { return status, body }
}

func runCapture(t *testing.T, args ...string) (string, error) {
	t.Helper()
	var err error
	out := captureStdout(t, func() { err = run(args) })
	return out, err
}

// ---- task start ----

func TestTaskStartUsesHCTerminalOnLocalCore(t *testing.T) {
	calls := stubHC(t, map[string]string{"server status --json": testLocalCoreJSON})
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"POST /api/v1/tasks/42/work": fixed(200, `{"task":`+testWorkTaskJSON+`,"stopped":[]}`),
	})
	setAgentEnv(t, server.URL)
	t.Setenv("HC_TERMINAL_ID", "t6b")

	out, err := runCapture(t, "task", "start", "42")
	if err != nil {
		t.Fatal(err)
	}
	req := (*requests)[0]
	if req.slug != "missionbase-dev" {
		t.Fatalf("slug = %q", req.slug)
	}
	want := map[string]any{"machine_id": "core-local", "machine_name": "omarchy-m3", "terminal": "t6b"}
	if fmt.Sprint(req.payload) != fmt.Sprint(want) {
		t.Fatalf("payload = %#v, want %#v", req.payload, want)
	}
	if len(*calls) != 1 {
		t.Fatalf("hc calls = %v, want only server status", *calls)
	}
	if !strings.Contains(out, `Started #42 "Ship it" (In Progress) on omarchy-m3 t6b`) {
		t.Fatalf("output = %q", out)
	}
}

func TestTaskStartRemoteMachineTerminalWithForce(t *testing.T) {
	stubHC(t, map[string]string{"server status --json": testLocalCoreJSON, "machine ls --json": testMachineLsJSON})
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"POST /api/v1/tasks/42/work": fixed(200, `{"task":`+testWorkTaskJSON+`,"stopped":[{"agent":{"slug":"claude"},"machine_id":"core-m1","machine_name":"omarchy-m1","terminal":"t2","reason":"reassigned"}]}`),
	})
	setAgentEnv(t, server.URL)
	t.Setenv("HC_TERMINAL_ID", "t6b")

	out, err := runCapture(t, "task", "start", "42", "--terminal", "omarchy-intel/t7", "--force")
	if err != nil {
		t.Fatal(err)
	}
	payload := (*requests)[0].payload
	if payload["machine_id"] != "core-intel" || payload["machine_name"] != "omarchy-intel" || payload["terminal"] != "t7" || payload["force"] != true {
		t.Fatalf("payload = %#v", payload)
	}
	if !strings.Contains(out, "Took over from claude · omarchy-m1 t2") {
		t.Fatalf("output = %q", out)
	}
}

func TestTaskStartMachineMatchesHostAndLocalName(t *testing.T) {
	calls := stubHC(t, map[string]string{"server status --json": testLocalCoreJSON, "machine ls --json": testMachineLsJSON})
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"POST /api/v1/tasks/42/work": fixed(200, `{"task":`+testWorkTaskJSON+`,"stopped":[]}`),
	})
	setAgentEnv(t, server.URL)

	if _, err := runCapture(t, "task", "start", "42", "--terminal", "omarchy-mbp-intel/t3"); err != nil {
		t.Fatal(err)
	}
	if got := (*requests)[0].payload["machine_id"]; got != "core-intel" {
		t.Fatalf("machine_id by host = %v", got)
	}

	*calls = nil
	if _, err := runCapture(t, "task", "start", "42", "--terminal", "omarchy-m3/t9"); err != nil {
		t.Fatal(err)
	}
	if got := (*requests)[1].payload; got["machine_id"] != "core-local" || got["terminal"] != "t9" {
		t.Fatalf("local machine by name payload = %#v", got)
	}
	if strings.Join(*calls, ",") != "server status --json" {
		t.Fatalf("local machine should not list machines; hc calls = %v", *calls)
	}
}

func TestTaskStartConflictUsesDistinctExitCode(t *testing.T) {
	stubHC(t, map[string]string{"server status --json": testLocalCoreJSON})
	server, _ := recordServer(t, map[string]func(*http.Request) (int, string){
		"POST /api/v1/tasks/42/work": fixed(409, testConflictBodyFmt),
	})
	setAgentEnv(t, server.URL)
	t.Setenv("HC_TERMINAL_ID", "t6b")

	out, err := runCapture(t, "task", "start", "42")
	if err == nil {
		t.Fatal("expected conflict error")
	}
	if code := exitCodeFor(err); code != exitConflict {
		t.Fatalf("exit code = %d, want %d", code, exitConflict)
	}
	for _, want := range []string{"already running elsewhere", "running: claude · omarchy-intel t7 since 2026-11-12T23:24:15Z", "--force"} {
		if !strings.Contains(err.Error(), want) {
			t.Fatalf("error %q missing %q", err, want)
		}
	}
	if out != "" {
		t.Fatalf("stdout without --json = %q, want empty", out)
	}

	out, err = runCapture(t, "task", "start", "42", "--json")
	if exitCodeFor(err) != exitConflict {
		t.Fatalf("--json exit code = %d", exitCodeFor(err))
	}
	var envelope map[string]any
	if jsonErr := json.Unmarshal([]byte(out), &envelope); jsonErr != nil || envelope["error"] == nil {
		t.Fatalf("--json stdout = %q, want the error envelope", out)
	}
}

func TestAnyHTTP409MapsToConflictExitCode(t *testing.T) {
	server, _ := recordServer(t, map[string]func(*http.Request) (int, string){
		"DELETE /api/v1/tasks/42/work": fixed(409, `{"error":{"code":"conflict","message":"x"}}`),
		"GET /api/v1/tasks/42":         fixed(500, `{"error":{"code":"boom","message":"x"}}`),
	})
	setAgentEnv(t, server.URL)
	_, err := runCapture(t, "task", "stop", "42")
	if exitCodeFor(err) != exitConflict {
		t.Fatalf("exit code = %d, want %d (err %v)", exitCodeFor(err), exitConflict, err)
	}
	_, err = runCapture(t, "task", "show", "42")
	if exitCodeFor(err) != 1 {
		t.Fatalf("500 exit code = %d, want 1", exitCodeFor(err))
	}
}

func TestTaskStartReportsMissingHC(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	setAgentEnv(t, "http://127.0.0.1:1")
	t.Setenv("HC_TERMINAL_ID", "t1")

	_, err := runCapture(t, "task", "start", "42")
	if err == nil || !strings.Contains(err.Error(), "hc is not installed or not on PATH") {
		t.Fatalf("err = %v, want missing hc", err)
	}
	if code := exitCodeFor(err); code != exitHCUnavailable {
		t.Fatalf("exit code = %d, want %d", code, exitHCUnavailable)
	}
}

func TestTaskStartTerminalResolutionErrors(t *testing.T) {
	stubHC(t, map[string]string{"server status --json": testLocalCoreJSON, "machine ls --json": testMachineLsJSON})
	setAgentEnv(t, "http://127.0.0.1:1")
	t.Setenv("HC_TERMINAL_ID", "")

	cases := []struct {
		args []string
		want string
		code int
	}{
		{[]string{"task", "start", "42"}, "HC_TERMINAL_ID is not set", exitHCUnavailable},
		{[]string{"task", "start", "42", "--terminal", "nowhere/t1"}, "known machines: omarchy-m1, omarchy-intel", exitHCUnavailable},
		{[]string{"task", "start", "42", "--terminal", "/t1"}, "machine name is empty", 1},
		{[]string{"task", "start", "42", "--terminal", "m1/"}, "is not a terminal address", 1},
		{[]string{"task", "start", "42", "--terminal"}, "--terminal requires a value", 1},
		{[]string{"task", "start", "abc", "--terminal", "t1"}, "task id must be a number", 1},
		{[]string{"task", "start"}, "usage: missionbase-agent task start", 1},
		{[]string{"task", "start", "42", "--bogus"}, `unknown task start option "--bogus"`, 1},
	}
	for _, tc := range cases {
		_, err := runCapture(t, tc.args...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%v: err = %v, want %q", tc.args, err, tc.want)
		}
		if code := exitCodeFor(err); code != tc.code {
			t.Fatalf("%v: exit code = %d, want %d", tc.args, code, tc.code)
		}
	}
}

func TestHCTimeoutAndFailureMessages(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "hc")
	if err := os.WriteFile(script, []byte("#!/bin/sh\necho 'server not running' >&2\nexit 2\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	_, err := runHC("server", "status", "--json")
	if err == nil || !strings.Contains(err.Error(), "hc server status --json failed: server not running") {
		t.Fatalf("err = %v", err)
	}
}

// ---- task stop ----

func TestTaskStopSendsReasonInAgentMode(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"DELETE /api/v1/tasks/42/work": func(r *http.Request) (int, string) {
			return 200, `{"task":{"id":42,"running":null},"stopped":[{"agent":{"slug":"missionbase-dev"},"machine_id":"core-local","machine_name":"omarchy-m3","terminal":"t6b","reason":"done"}]}`
		},
	})
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "task", "stop", "42", "--reason", "done")
	if err != nil {
		t.Fatal(err)
	}
	if got := (*requests)[0].payload; len(got) != 1 || got["reason"] != "done" {
		t.Fatalf("payload = %#v, want reason only", got)
	}
	if !strings.Contains(out, "Stopped #42 on omarchy-m3 t6b (done)") {
		t.Fatalf("output = %q", out)
	}

	if _, err := runCapture(t, "task", "stop", "42"); err != nil {
		t.Fatal(err)
	}
	if got := (*requests)[1].payload["reason"]; got != "manual" {
		t.Fatalf("default reason = %v, want manual", got)
	}

	if _, err := runCapture(t, "task", "stop", "42", "--reason", "bored"); err == nil || !strings.Contains(err.Error(), "--reason must be one of") {
		t.Fatalf("err = %v, want reason validation", err)
	}
}

func TestTaskStopReportsNothingRunning(t *testing.T) {
	server, _ := recordServer(t, map[string]func(*http.Request) (int, string){
		"DELETE /api/v1/tasks/42/work": fixed(200, `{"task":{"id":42},"stopped":[]}`),
	})
	setAgentEnv(t, server.URL)
	out, err := runCapture(t, "task", "stop", "42")
	if err != nil || !strings.Contains(out, "Nothing of yours was running on #42") {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	out, err = runCapture(t, "task", "stop", "42", "--json")
	if err != nil || strings.TrimSpace(out) != `{"task":{"id":42},"stopped":[]}` {
		t.Fatalf("--json out = %q, err = %v", out, err)
	}
}

// ---- task attention ----

func TestTaskAttentionPostsReasonAndClears(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"POST /api/v1/tasks/42/attention":   fixed(201, `{"task":{"id":42,"title":"Ship it","status":"in_progress","status_label":"In Progress","needs_you":true,"attention":{"reason":"Short or long copy?"}}}`),
		"DELETE /api/v1/tasks/42/attention": fixed(200, `{"task":{"id":42,"title":"Ship it","status":"in_progress","status_label":"In Progress","needs_you":false,"attention":null}}`),
	})
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "task", "attention", "42", "Short or long copy?")
	if err != nil {
		t.Fatal(err)
	}
	if got := (*requests)[0]; got.method != "POST" || got.payload["reason"] != "Short or long copy?" || got.slug != "missionbase-dev" {
		t.Fatalf("request = %#v", got)
	}
	if !strings.Contains(out, `#42 "Ship it" needs you: Short or long copy? (In Progress)`) {
		t.Fatalf("output = %q", out)
	}

	out, err = runCapture(t, "task", "attention", "clear", "42")
	if err != nil {
		t.Fatal(err)
	}
	if got := (*requests)[1]; got.method != "DELETE" || got.payload != nil {
		t.Fatalf("clear request = %#v", got)
	}
	if !strings.Contains(out, `#42 "Ship it": open questions dismissed (In Progress)`) {
		t.Fatalf("clear output = %q", out)
	}

	bodyFile := writeTextFile(t, "reason.md", "Which `copy`?\n\n- short\n- long\n")
	if _, err := runCapture(t, "task", "attention", "42", "--body-file", bodyFile, "--json"); err != nil {
		t.Fatal(err)
	}
	if got := (*requests)[2].payload["reason"]; got != "Which `copy`?\n\n- short\n- long" {
		t.Fatalf("body-file reason = %q", got)
	}
}

func TestTaskAttentionValidatesArguments(t *testing.T) {
	cases := map[string][]string{
		"a reason is required":                     {"task", "attention", "42"},
		"a reason is required: task attention":     {"task", "attention", "42", "   "},
		"usage: missionbase-agent task attention":  {"task", "attention"},
		`unexpected argument "now"`:                {"task", "attention", "clear", "42", "now"},
		"is not used with task attention clear":    {"task", "attention", "clear", "42", "--body-file", "x"},
		`unknown task attention option "--reason"`: {"task", "attention", "42", "--reason", "x"},
	}
	setAgentEnv(t, "http://127.0.0.1:1")
	for want, args := range cases {
		if _, err := runCapture(t, args...); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%v: err = %v, want %q", args, err, want)
		}
	}
}

// ---- project resolution and tasks ls ----

func writeProjectFile(t *testing.T, root, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, ".missionbase"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".missionbase", "project.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func boxTask(id int, status string, position int, extra string) string {
	return fmt.Sprintf(`{"id":%d,"title":"Task %d","status":"%s","status_label":"%s","status_category":"open","box":{"id":113,"name":"Engage Spot"},"box_position":%d,"assignees":[],"running":null,"live_status":null,"attention":null%s}`, id, id, status, status, position, extra)
}

const emptyAccessible = `{"tasks":[],"meta":{"total":0,"page":1,"per_page":500,"has_more":false}}`

func TestTasksLsResolvesNearestProjectFileAndPages(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, `{"team_id":2,"box_id":113}`)
	nested := filepath.Join(root, "app", "models")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/tasks/accessible": func(r *http.Request) (int, string) {
			if r.URL.Query().Get("page") == "1" {
				return 200, `{"tasks":[` + boxTask(1, "todo", 1, "") + `],"meta":{"total":2,"page":1,"per_page":500,"has_more":true,"cursor":"1.1000.0.42"}}`
			}
			return 200, `{"tasks":[` + boxTask(2, "planning", 1, `,"needs_you":true,"attention":{"reason":"Pick one"}`) + `],"meta":{"total":2,"page":2,"per_page":500,"has_more":false,"cursor":"1.2000.0.43"}}`
		},
	})
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "tasks", "ls", "--cwd", nested, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if len(*requests) != 2 || (*requests)[0].query != "box_ids=113&page=1&per_page=500" || (*requests)[1].query != "box_ids=113&page=2&per_page=500" {
		t.Fatalf("requests = %#v", *requests)
	}
	var result struct {
		Project resolvedProject   `json:"project"`
		Tasks   []json.RawMessage `json:"tasks"`
		Cursor  string            `json:"cursor"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("output %q: %v", out, err)
	}
	if result.Project.BoxID != "113" || result.Project.Source != "project_file" || result.Project.Path != filepath.Join(root, ".missionbase", "project.json") || len(result.Tasks) != 2 {
		t.Fatalf("result = %+v", result)
	}
	// The first page's cursor is the earliest; a later page's could skip changes made while paging.
	if result.Cursor != "1.1000.0.42" {
		t.Fatalf("cursor = %q, want the first page's 1.1000.0.42", result.Cursor)
	}

	// Without box_name in project.json, the text header uses the box name from the payload.
	out, err = runCapture(t, "tasks", "ls", "--cwd", nested)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"Engage Spot (2 tasks, from ", "#1", "Task 1", "needs you: Pick one"} {
		if !strings.Contains(out, want) {
			t.Fatalf("text output missing %q:\n%s", want, out)
		}
	}
}

func TestTasksLsProjectPrecedence(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, `{"box_id":"113"}`)
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/tasks/accessible": fixed(200, emptyAccessible),
	})
	setAgentEnv(t, server.URL)
	if err := os.WriteFile(os.Getenv("MISSIONBASE_AGENT_CREDENTIALS"), []byte(`{"token":"file-token","default_project":"55"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		args []string
		box  string
	}{
		{[]string{"tasks", "ls", "--project", "7", "--json"}, "7"},
		{[]string{"tasks", "ls", "--cwd", root, "--json"}, "113"},
		{[]string{"tasks", "ls", "--cwd", t.TempDir(), "--json"}, "55"},
	} {
		before := len(*requests)
		if _, err := runCapture(t, tc.args...); err != nil {
			t.Fatalf("%v: %v", tc.args, err)
		}
		if got := mustParseQuery(t, (*requests)[before].query).Get("box_ids"); got != tc.box {
			t.Fatalf("%v: box_ids = %s, want %s", tc.args, got, tc.box)
		}
	}
}

func TestTasksLsWithoutProjectExitsWithDistinctCode(t *testing.T) {
	setAgentEnv(t, "http://127.0.0.1:1")
	_, err := runCapture(t, "tasks", "ls", "--cwd", t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "no project here") {
		t.Fatalf("err = %v", err)
	}
	if code := exitCodeFor(err); code != exitProjectUnresolved {
		t.Fatalf("exit code = %d, want %d", code, exitProjectUnresolved)
	}

	bad := t.TempDir()
	writeProjectFile(t, bad, `{"team_id":2}`)
	if _, err := runCapture(t, "tasks", "ls", "--cwd", bad); err == nil || !strings.Contains(err.Error(), "has no valid box_id") {
		t.Fatalf("err = %v, want invalid project file", err)
	}
}

func TestTasksLsStatusFiltersAndFlagValidation(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/tasks/accessible": fixed(200, emptyAccessible),
	})
	setAgentEnv(t, server.URL)
	for _, tc := range []struct{ status, key, value string }{
		{"all", "status_category", "open,done,canceled"},
		{"done", "status_category", "done"},
		{"planning", "status", "planning"},
		{"planning,todo", "status", "planning,todo"},
		{"todo,in_progress", "status", "todo,in_progress"},
	} {
		before := len(*requests)
		if _, err := runCapture(t, "tasks", "ls", "--project", "9", "--status", tc.status, "--json"); err != nil {
			t.Fatal(err)
		}
		query := mustParseQuery(t, (*requests)[before].query)
		if query.Get(tc.key) != tc.value || query.Get("box_ids") != "9" {
			t.Fatalf("--status %s: query = %s", tc.status, (*requests)[before].query)
		}
	}

	for want, args := range map[string][]string{
		"use only one of --cwd or --project":     {"tasks", "ls", "--project", "9", "--cwd", "."},
		"use --all or --cwd/--project, not both": {"tasks", "ls", "--all", "--project", "9"},
		"--project must be a project (box) id":   {"tasks", "ls", "--project", "abc"},
		`unknown tasks ls option "--done"`:       {"tasks", "ls", "--done"},
		"use --needs-you to list tasks waiting":  {"tasks", "ls", "--project", "9", "--status", "todo,needs_attention"},
	} {
		if _, err := runCapture(t, args...); err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%v: err = %v, want %q", args, err, want)
		}
	}
}

func TestTasksLsAllUsesAccessibleTasksInServerOrder(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/tasks/accessible": fixed(200, `{"tasks":[
			{"id":3,"title":"Waiting","status":"in_progress","needs_you":true,"status_category":"open","box":{"id":1,"name":"A"},"box_position":1},
			{"id":1,"title":"Open","status":"todo","status_category":"open","box":{"id":2,"name":"B"},"box_position":2}
		],"meta":{"total":2,"page":1,"per_page":500,"has_more":false,"cursor":"1.1794529810123456.0.42"}}`),
	})
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "tasks", "ls", "--all", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if got := (*requests)[0].query; got != "page=1&per_page=500" {
		t.Fatalf("query = %s, want no box_ids or status filter", got)
	}
	var result struct {
		Project any `json:"project"`
		Tasks   []struct {
			ID int `json:"id"`
		} `json:"tasks"`
		Cursor string `json:"cursor"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if result.Project != nil || result.Cursor != "1.1794529810123456.0.42" || len(result.Tasks) != 2 || result.Tasks[0].ID != 3 || result.Tasks[1].ID != 1 {
		t.Fatalf("result = %+v", result)
	}

	if _, err := runCapture(t, "tasks", "ls", "--all", "--status", "done", "--json"); err != nil {
		t.Fatal(err)
	}
	if q := mustParseQuery(t, (*requests)[1].query); q.Get("status_category") != "done" || q.Get("box_ids") != "" {
		t.Fatalf("query = %s", (*requests)[1].query)
	}

	text, err := runCapture(t, "tasks", "ls", "--all")
	if err != nil || !strings.Contains(text, "All projects (2 tasks)") || !strings.Contains(text, "A  ") {
		t.Fatalf("text = %q, err = %v", text, err)
	}
}

// ---- tasks changes ----

func TestTasksChangesFollowsCursorDedupesAndAppliesDeletions(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/tasks/changes": func(r *http.Request) (int, string) {
			switch r.URL.Query().Get("since") {
			case "1.100.9.4":
				return 200, `{"tasks":[{"id":1,"title":"v1","status":"todo"},{"id":2,"title":"gone soon","status":"todo"}],"deleted_task_ids":[],"cursor":"1.200.2.4","has_more":true}`
			case "1.200.2.4":
				return 200, `{"tasks":[{"id":1,"title":"v2","status":"in_progress"}],"deleted_task_ids":[2,8],"cursor":"1.300.1.6","has_more":false}`
			}
			t.Errorf("unexpected cursor %q", r.URL.Query().Get("since"))
			return 422, `{"error":{"code":"validation_error","message":"since is not a valid cursor"}}`
		},
	})
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "tasks", "changes", "--since", "1.100.9.4", "--project", "1,2", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if len(*requests) != 2 || mustParseQuery(t, (*requests)[0].query).Get("box_ids") != "1,2" {
		t.Fatalf("requests = %#v", *requests)
	}
	var result struct {
		Tasks []struct {
			ID    int    `json:"id"`
			Title string `json:"title"`
		} `json:"tasks"`
		Deleted []int  `json:"deleted_task_ids"`
		Cursor  string `json:"cursor"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if len(result.Tasks) != 1 || result.Tasks[0].Title != "v2" || fmt.Sprint(result.Deleted) != "[2 8]" || result.Cursor != "1.300.1.6" {
		t.Fatalf("result = %+v", result)
	}
}

func TestTasksChangesEmptyKeepsCursorAndPrintsEmptyArrays(t *testing.T) {
	server, _ := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/tasks/changes": fixed(200, `{"tasks":[],"deleted_task_ids":[],"cursor":"1.1.1.1","has_more":false}`),
	})
	setAgentEnv(t, server.URL)
	out, err := runCapture(t, "tasks", "changes", "--since", "1.1.1.1", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != `{"cursor":"1.1.1.1","deleted_task_ids":[],"tasks":[]}` {
		t.Fatalf("out = %s", out)
	}
	if _, err := runCapture(t, "tasks", "changes", "--project", "x"); err == nil || !strings.Contains(err.Error(), "must be project ids") {
		t.Fatalf("err = %v", err)
	}
}

// ---- task next ----

func TestTaskNextInProjectUsesServerNext(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/boxes/9/tasks/next": fixed(200, `{"has_work":true,"task":`+boxTask(5, "todo", 3, "")+`}`),
	})
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "task", "next", "--project", "9")
	if err != nil {
		t.Fatal(err)
	}
	if len(*requests) != 1 || (*requests)[0].slug != "missionbase-dev" || (*requests)[0].query != "" {
		t.Fatalf("requests = %#v", *requests)
	}
	if !strings.Contains(out, `#5 "Task 5"`) || !strings.Contains(out, "task start 5") {
		t.Fatalf("output = %q, want task 5", out)
	}

	out, err = runCapture(t, "task", "next", "--project", "9", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		HasWork bool `json:"has_work"`
		Task    struct {
			ID int `json:"id"`
		} `json:"task"`
		Project resolvedProject `json:"project"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	if !result.HasWork || result.Task.ID != 5 || result.Project.BoxID != "9" || result.Project.Source != "flag" {
		t.Fatalf("result = %+v", result)
	}
}

func TestTaskNextInProjectReportsNoWorkAndErrors(t *testing.T) {
	root := t.TempDir()
	writeProjectFile(t, root, `{"box_id":10}`)
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/boxes/10/tasks/next": fixed(200, `{"has_work":false,"task":null}`),
		"GET /api/v1/boxes/11/tasks/next": fixed(404, `{"error":{"code":"not_found","message":"Box not found"}}`),
	})
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "task", "next", "--cwd", root, "--json")
	if err != nil || !strings.Contains(out, `"has_work":false`) || !strings.Contains(out, `"task":null`) || !strings.Contains(out, `"box_id":"10"`) {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	if (*requests)[0].path != "/api/v1/boxes/10/tasks/next" {
		t.Fatalf("path = %s", (*requests)[0].path)
	}
	out, err = runCapture(t, "task", "next", "--cwd", root)
	if err != nil || !strings.Contains(out, "No work here") {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	if _, err := runCapture(t, "task", "next", "--project", "11"); err == nil || !strings.Contains(err.Error(), "HTTP 404") || exitCodeFor(err) != 1 {
		t.Fatalf("err = %v, want 404 with exit 1", err)
	}
}

func TestTaskNextWithoutProjectUsesAgentWork(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/agent/work": fixed(200, `{"has_work":true,"task":{"id":77,"title":"Assigned","status":"todo","status_label":"To Do"}}`),
	})
	setAgentEnv(t, server.URL)
	out, err := runCapture(t, "task", "next", "--cwd", t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if (*requests)[0].path != "/api/v1/agent/work" || !strings.Contains(out, `#77 "Assigned" (To Do)`) {
		t.Fatalf("requests = %#v, out = %q", *requests, out)
	}
}

// ---- project command ----

func TestProjectDefaultWritesOnlyTheCredentialsFile(t *testing.T) {
	setAgentEnv(t, "http://127.0.0.1:1")
	credentials := os.Getenv("MISSIONBASE_AGENT_CREDENTIALS")
	if err := os.WriteFile(credentials, []byte(`{"base_url":"https://example.test","token":"file-token","agent_slug":"file-agent"}`), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := runCapture(t, "project", "default", "113"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(credentials)
	if err != nil {
		t.Fatal(err)
	}
	var stored map[string]string
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatal(err)
	}
	if stored["default_project"] != "113" || stored["token"] != "file-token" || stored["agent_slug"] != "file-agent" || stored["base_url"] != "https://example.test" {
		t.Fatalf("stored = %#v (env values must not leak into the file)", stored)
	}

	out, err := runCapture(t, "project", "--cwd", t.TempDir(), "--json")
	if err != nil || !strings.Contains(out, `"box_id":"113"`) || !strings.Contains(out, `"source":"default"`) {
		t.Fatalf("out = %q, err = %v", out, err)
	}

	if _, err := runCapture(t, "project", "default", "--clear"); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(credentials); strings.Contains(string(data), "default_project") {
		t.Fatalf("default_project not cleared: %s", data)
	}
	if _, err := runCapture(t, "project", "default", "abc"); err == nil {
		t.Fatal("expected invalid default project to fail")
	}
}

func mustParseQuery(t *testing.T, raw string) url.Values {
	t.Helper()
	values, err := url.ParseQuery(raw)
	if err != nil {
		t.Fatal(err)
	}
	return values
}

func TestTasksLsCursorHandsOffToTasksChanges(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/tasks/accessible": fixed(200, `{"tasks":[`+boxTask(1, "todo", 1, "")+`],"meta":{"total":1,"page":1,"per_page":500,"has_more":false,"cursor":"1.5000.0.7"}}`),
		"GET /api/v1/tasks/changes":    fixed(200, `{"tasks":[],"deleted_task_ids":[],"cursor":"1.5000.0.7","has_more":false}`),
	})
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "tasks", "ls", "--project", "113", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var listing struct {
		Cursor string `json:"cursor"`
	}
	if err := json.Unmarshal([]byte(out), &listing); err != nil || listing.Cursor == "" {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	if _, err := runCapture(t, "tasks", "changes", "--since", listing.Cursor, "--json"); err != nil {
		t.Fatal(err)
	}
	if got := mustParseQuery(t, (*requests)[1].query).Get("since"); got != "1.5000.0.7" {
		t.Fatalf("changes since = %q, want the cursor from tasks ls verbatim", got)
	}
}

func TestTasksLsCursorIsNullWhenServerSendsNone(t *testing.T) {
	server, _ := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/tasks/accessible": fixed(200, emptyAccessible),
	})
	setAgentEnv(t, server.URL)
	out, err := runCapture(t, "tasks", "ls", "--all", "--json")
	if err != nil || strings.TrimSpace(out) != `{"cursor":null,"project":null,"tasks":[]}` {
		t.Fatalf("out = %q, err = %v", out, err)
	}
}

func TestAgentWorkCommandsShowTheAPIErrorMessage(t *testing.T) {
	server, _ := recordServer(t, map[string]func(*http.Request) (int, string){
		"POST /api/v1/tasks/7/work":   fixed(422, `{"error":{"code":"task_closed","message":"Task is closed; reopen it before starting work","details":{}}}`),
		"GET /api/v1/tasks/changes":   fixed(422, `{"error":{"code":"validation_error","message":"since is not a valid cursor","details":{}}}`),
		"DELETE /api/v1/tasks/8/work": fixed(502, `<html>Bad gateway</html>`),
	})
	stubHC(t, map[string]string{"server status --json": testLocalCoreJSON})
	setAgentEnv(t, server.URL)
	t.Setenv("HC_TERMINAL_ID", "t1")

	_, err := runCapture(t, "task", "start", "7")
	if err == nil || err.Error() != "Task is closed; reopen it before starting work (HTTP 422 task_closed)" || exitCodeFor(err) != 1 {
		t.Fatalf("err = %v (exit %d)", err, exitCodeFor(err))
	}
	if _, err := runCapture(t, "tasks", "changes", "--since", "nope"); err == nil || err.Error() != "since is not a valid cursor (HTTP 422 validation_error)" {
		t.Fatalf("err = %v", err)
	}
	// A body that is not the standard envelope keeps the raw message.
	if _, err := runCapture(t, "task", "stop", "8"); err == nil || !strings.Contains(err.Error(), "HTTP 502: <html>Bad gateway</html>") {
		t.Fatalf("err = %v", err)
	}
}

// ---- task status ----

func TestTaskStatusSendsPlanningKey(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"PATCH /api/v1/tasks/42": fixed(200, `{"task":{"id":42,"status":"planning","status_label":"Planning"}}`),
	})
	setAgentEnv(t, server.URL)

	if _, err := runCapture(t, "task", "status", "42", "planning"); err != nil {
		t.Fatal(err)
	}
	if len(*requests) != 1 || fmt.Sprint((*requests)[0].payload) != fmt.Sprint(map[string]any{"status": "planning"}) {
		t.Fatalf("requests = %+v, want one PATCH with status planning", *requests)
	}
}

func TestTasksLsNeedsYouFiltersAndMarksTasks(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/tasks/accessible": fixed(200, `{"tasks":[
			{"id":5,"title":"Pick a plan","status":"planning","status_label":"Planning","needs_you":true,"attention":{"reason":"Which plan?"},"questions":{"open_count":1}},
			{"id":6,"title":"Deploy","status":"in_progress","status_label":"In Progress","needs_you":true,"attention":null,"live_status":{"worker":"claude","state":"permission"}}
		],"meta":{"total":2,"page":1,"per_page":500,"has_more":false}}`),
	})
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "tasks", "ls", "--project", "9", "--needs-you")
	if err != nil {
		t.Fatal(err)
	}
	query := mustParseQuery(t, (*requests)[0].query)
	if query.Get("needs_you") != "true" || query.Get("box_ids") != "9" || query.Get("status") != "" {
		t.Fatalf("query = %s", (*requests)[0].query)
	}
	for _, want := range []string{"#5  Planning", "needs you: Which plan? 1 question", "#6  In Progress", "needs permission (claude) needs you"} {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func TestTaskStatusRefusesNeedsAttention(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){})
	setAgentEnv(t, server.URL)

	_, err := runCapture(t, "task", "status", "42", "needs_attention")
	if err == nil || !strings.Contains(err.Error(), "needs_attention is no longer a status") || !strings.Contains(err.Error(), "task ask") {
		t.Fatalf("err = %v", err)
	}
	if len(*requests) != 0 {
		t.Fatalf("requests = %+v, want none", *requests)
	}
}
