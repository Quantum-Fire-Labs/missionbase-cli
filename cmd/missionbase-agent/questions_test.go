package main

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

const (
	testAskedBy        = `{"type":"Agent","id":5,"name":"Publisher","slug":"publisher","handle":"publisher"}`
	testOwner          = `{"type":"User","id":9,"name":"John Doe","handle":"JohnDoe"}`
	testAskTaskJSON    = `{"id":42,"title":"Ship it","status":"needs_attention","status_label":"Needs Attention","attention":{"reason":null,"comment_id":null,"since":"2026-11-13T05:12:40Z"},"questions":{"open_count":2,"latest_batch_at":"2026-11-13T05:12:40Z"}}`
	testAskResponse    = `{"batch":"b1","questions":[` + testQuestionQ1Open + `,` + testQuestionQ2Open + `],"task":` + testAskTaskJSON + `}`
	testQuestionQ0     = `{"id":1,"task_id":42,"batch":"b0","position":1,"body":"Old question?","context":null,"options":[],"status":"answered","asked_by":` + testAskedBy + `,"asked_at":"2026-11-12T09:00:00Z","answer_body":"Old answer","answered_by":` + testOwner + `,"answered_at":"2026-11-12T10:00:00Z","dismissed_by":null,"dismissed_at":null}`
	testQuestionQ1Open = `{"id":2,"task_id":42,"batch":"b1","position":1,"body":"Should a 409 count as delivered?","context":"Optimus returns 409\nfor duplicates.","options":["Yes","No"],"status":"open","asked_by":` + testAskedBy + `,"asked_at":"2026-11-13T05:12:40Z","answer_body":null,"answered_by":null,"answered_at":null,"dismissed_by":null,"dismissed_at":null}`
	testQuestionQ2Open = `{"id":3,"task_id":42,"batch":"b1","position":2,"body":"Cap retries at 1h or 24h?","context":null,"options":[],"status":"open","asked_by":` + testAskedBy + `,"asked_at":"2026-11-13T05:12:40Z","answer_body":null,"answered_by":null,"answered_at":null,"dismissed_by":null,"dismissed_at":null}`
	testQuestionQ1Done = `{"id":2,"task_id":42,"batch":"b1","position":1,"body":"Should a 409 count as delivered?","context":null,"options":["Yes","No"],"status":"answered","asked_by":` + testAskedBy + `,"asked_at":"2026-11-13T05:12:40Z","answer_body":"Yes, treat as delivered\nand log it.","answered_by":` + testOwner + `,"answered_at":"2026-11-13T06:00:00Z","dismissed_by":null,"dismissed_at":null}`
	testQuestionQ2Done = `{"id":3,"task_id":42,"batch":"b1","position":2,"body":"Cap retries at 1h or 24h?","context":null,"options":[],"status":"dismissed","asked_by":` + testAskedBy + `,"asked_at":"2026-11-13T05:12:40Z","answer_body":null,"answered_by":null,"answered_at":null,"dismissed_by":` + testOwner + `,"dismissed_at":"2026-11-13T06:00:00Z"}`
)

func questionsList(openCount int, questions ...string) string {
	return `{"task_id":42,"open_count":` + strconv.Itoa(openCount) + `,"questions":[` + strings.Join(questions, ",") + `]}`
}

func canonicalJSON(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func assertContainsAll(t *testing.T, out string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(out, want) {
			t.Fatalf("output missing %q:\n%s", want, out)
		}
	}
}

func shortPolls(t *testing.T) {
	t.Helper()
	original := questionsPollInterval
	questionsPollInterval = 5 * time.Millisecond
	t.Cleanup(func() { questionsPollInterval = original })
}

// ---- task ask ----

func TestTaskAskOneQuestionSendsBodyAndPrintsBatch(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"POST /api/v1/tasks/42/questions": fixed(201, testAskResponse),
	})
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "task", "ask", "42", "  Should a 409 count as delivered?  ", "--context", `Optimus returns 409\nfor duplicates.`, "--option", "Yes", "--option", " No ")
	if err != nil {
		t.Fatal(err)
	}
	req := (*requests)[0]
	if req.method != "POST" || req.slug != "missionbase-dev" {
		t.Fatalf("request = %#v", req)
	}
	want := `{"questions":[{"body":"Should a 409 count as delivered?","context":"Optimus returns 409\nfor duplicates.","options":["Yes","No"]}]}`
	if got := canonicalJSON(t, req.payload); got != want {
		t.Fatalf("payload = %s\nwant      %s", got, want)
	}
	assertContainsAll(t, out,
		`Asked 2 questions on #42 "Ship it"`,
		"  1. Should a 409 count as delivered?\n",
		"     Context: Optimus returns 409\n       for duplicates.\n",
		"     Options: Yes · No\n",
		"  2. Cap retries at 1h or 24h?\n",
		"Status: Needs Attention\n",
		server.URL+"/tasks/42\n",
		"missionbase-agent task questions 42 --wait",
	)
}

func TestTaskAskQuestionWithoutContextOmitsIt(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"POST /api/v1/tasks/42/questions": fixed(201, testAskResponse),
	})
	setAgentEnv(t, server.URL)
	if _, err := runCapture(t, "task", "ask", "42", "Short or long copy?", "--json"); err != nil {
		t.Fatal(err)
	}
	if got := canonicalJSON(t, (*requests)[0].payload); got != `{"questions":[{"body":"Short or long copy?"}]}` {
		t.Fatalf("payload = %s", got)
	}
}

func TestTaskAskFileSendsEveryQuestionAndJSONIsRaw(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"POST /api/v1/tasks/42/questions": fixed(201, testAskResponse),
	})
	setAgentEnv(t, server.URL)
	file := writeTextFile(t, "questions.json", `[
  {"body": "Should a 409 count as delivered?", "context": "Optimus returns 409.", "options": ["Yes", "No"]},
  {"body": "Cap retries at 1h or 24h?"}
]`)

	out, err := runCapture(t, "task", "ask", "42", "--file", file, "--json")
	if err != nil {
		t.Fatal(err)
	}
	want := `{"questions":[{"body":"Should a 409 count as delivered?","context":"Optimus returns 409.","options":["Yes","No"]},{"body":"Cap retries at 1h or 24h?"}]}`
	if got := canonicalJSON(t, (*requests)[0].payload); got != want {
		t.Fatalf("payload = %s\nwant      %s", got, want)
	}
	if strings.TrimSpace(out) != testAskResponse {
		t.Fatalf("--json output = %q, want the raw response", out)
	}
}

func TestTaskAskValidatesArgumentsBeforeSending(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){})
	setAgentEnv(t, server.URL)
	file := func(body string) string { return writeTextFile(t, "q.json", body) }
	sixQuestions := `[` + strings.TrimSuffix(strings.Repeat(`{"body":"Q"},`, 6), ",") + `]`

	cases := []struct {
		args []string
		want string
	}{
		{[]string{"task", "ask"}, "usage: missionbase-agent task ask"},
		{[]string{"task", "ask", "42"}, "a question is required"},
		{[]string{"task", "ask", "42", "   "}, "the question: body can't be blank"},
		{[]string{"task", "ask", "abc", "Q?"}, `task id must be a number, got "abc"`},
		{[]string{"task", "ask", "42", "Q?", "extra"}, `unexpected argument "extra"`},
		{[]string{"task", "ask", "42", "Q?", "--question", "x"}, `unknown task ask option "--question"`},
		{[]string{"task", "ask", "42", "Q?", "--option"}, "--option requires a value"},
		{[]string{"task", "ask", "42", "Q?", "--option", "a", "--option", "b", "--option", "c", "--option", "d", "--option", "e"}, "the question: at most 4 options (got 5)"},
		{[]string{"task", "ask", "42", "Q?", "--option", " "}, "the question: option 1 is blank"},
		{[]string{"task", "ask", "42", "Q?", "--context", "a", "--context", "b"}, "--context can be given only once"},
		{[]string{"task", "ask", "42", "Q?", "--file", file(`[{"body":"x"}]`)}, "not both"},
		{[]string{"task", "ask", "42", "--option", "Yes", "--file", file(`[{"body":"x"}]`)}, "not both"},
		{[]string{"task", "ask", "42", "--context", "c", "--file", file(`[{"body":"x"}]`)}, "not both"},
		{[]string{"task", "ask", "42", "--file", "-"}, "stdin is not supported"},
		{[]string{"task", "ask", "42", "--file", "/nonexistent/q.json"}, "read questions file"},
		{[]string{"task", "ask", "42", "--file", file(`{"body":"x"}`)}, "must be a JSON array of questions"},
		{[]string{"task", "ask", "42", "--file", file(`[{"body":"x"`)}, "is not a valid questions file"},
		{[]string{"task", "ask", "42", "--file", file(`[{"question":"x"}]`)}, `unknown field "question"`},
		{[]string{"task", "ask", "42", "--file", file(`[{"body":"x","options":"Yes"}]`)}, "is not a valid questions file"},
		{[]string{"task", "ask", "42", "--file", file(`[{"body":"x"}] []`)}, "extra content after the JSON array"},
		{[]string{"task", "ask", "42", "--file", file(`[]`)}, "ask 1 to 5 questions at a time (got 0)"},
		{[]string{"task", "ask", "42", "--file", file(sixQuestions)}, "ask 1 to 5 questions at a time (got 6)"},
		{[]string{"task", "ask", "42", "--file", file(`[{"body":"ok"},{"body":" ","options":["a","b","c","d","e"]}]`)}, "question 2: body can't be blank\nquestion 2: at most 4 options (got 5)"},
	}
	for _, tc := range cases {
		_, err := runCapture(t, tc.args...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%q: err = %v, want %q", tc.args, err, tc.want)
		}
		if exitCodeFor(err) != 1 {
			t.Fatalf("%q: exit code = %d, want 1", tc.args, exitCodeFor(err))
		}
	}
	if len(*requests) != 0 {
		t.Fatalf("invalid input sent requests: %#v", *requests)
	}
}

func TestTaskAskShowsServerValidationDetails(t *testing.T) {
	server, _ := recordServer(t, map[string]func(*http.Request) (int, string){
		"POST /api/v1/tasks/42/questions": fixed(422, `{"error":{"code":"validation_error","message":"Invalid questions","details":["questions[1]: Body is too long (maximum is 10000 characters)","questions[1]: Options must each be at most 500 characters"]}}`),
		"POST /api/v1/tasks/43/questions": fixed(422, `{"error":{"code":"task_closed","message":"Task is closed; reopen it before asking questions","details":{}}}`),
	})
	setAgentEnv(t, server.URL)

	_, err := runCapture(t, "task", "ask", "42", "Q?")
	want := "Invalid questions (HTTP 422 validation_error)\n  questions[1]: Body is too long (maximum is 10000 characters)\n  questions[1]: Options must each be at most 500 characters"
	if err == nil || err.Error() != want || exitCodeFor(err) != 1 {
		t.Fatalf("err = %v (exit %d)\nwant %s", err, exitCodeFor(err), want)
	}
	_, err = runCapture(t, "task", "ask", "43", "Q?")
	if err == nil || err.Error() != "Task is closed; reopen it before asking questions (HTTP 422 task_closed)" {
		t.Fatalf("err = %v", err)
	}
}

// ---- task questions ----

func TestTaskQuestionsListsByBatchWithAnswers(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/tasks/42/questions": fixed(200, questionsList(1, testQuestionQ0, testQuestionQ1Done, testQuestionQ2Open)),
	})
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "task", "questions", "42")
	if err != nil {
		t.Fatal(err)
	}
	if req := (*requests)[0]; req.query != "" || req.slug != "missionbase-dev" {
		t.Fatalf("request = %#v", req)
	}
	want := `#42: 3 questions, 1 open
Asked by Publisher at 2026-11-12T09:00:00Z
  1. [answered] Old question?
     Answer (John Doe): Old answer
Asked by Publisher at 2026-11-13T05:12:40Z
  1. [answered] Should a 409 count as delivered?
     Options: Yes · No
     Answer (John Doe): Yes, treat as delivered
       and log it.
  2. [open] Cap retries at 1h or 24h?
`
	if out != want {
		t.Fatalf("output:\n%s\nwant:\n%s", out, want)
	}
}

func TestTaskQuestionsStatusFilterAndJSON(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/tasks/42/questions": fixed(200, questionsList(0, testQuestionQ2Done)),
	})
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "task", "questions", "42", "--status", "answered, dismissed", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if got := mustParseQuery(t, (*requests)[0].query).Get("status"); got != "answered,dismissed" {
		t.Fatalf("status query = %q", got)
	}
	if strings.TrimSpace(out) != questionsList(0, testQuestionQ2Done) {
		t.Fatalf("--json output = %q", out)
	}

	out, err = runCapture(t, "task", "questions", "42", "--open")
	if err != nil {
		t.Fatal(err)
	}
	if got := mustParseQuery(t, (*requests)[1].query).Get("status"); got != "open" {
		t.Fatalf("--open query = %q", got)
	}
	assertContainsAll(t, out, "  2. [dismissed] Cap retries at 1h or 24h?\n     Dismissed by John Doe\n")
}

func TestTaskQuestionsEmptyList(t *testing.T) {
	server, _ := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/tasks/42/questions": fixed(200, questionsList(0)),
	})
	setAgentEnv(t, server.URL)
	out, err := runCapture(t, "task", "questions", "42")
	if err != nil || out != "No questions on #42\n" {
		t.Fatalf("out = %q, err = %v", out, err)
	}
}

func TestTaskQuestionsValidatesArguments(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){})
	setAgentEnv(t, server.URL)
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"task", "questions"}, "usage: missionbase-agent task questions"},
		{[]string{"task", "questions", "x1"}, `task id must be a number, got "x1"`},
		{[]string{"task", "questions", "42", "43"}, `unknown task questions option "43"`},
		{[]string{"task", "questions", "42", "--status", "pending"}, "--status must be open, answered or dismissed"},
		{[]string{"task", "questions", "42", "--status", "open,"}, "--status must be open, answered or dismissed"},
		{[]string{"task", "questions", "42", "--status"}, "--status requires a value"},
		{[]string{"task", "questions", "42", "--timeout", "5m"}, "--timeout is only used with --wait"},
		{[]string{"task", "questions", "42", "--wait", "--status", "open"}, "--status can't be used with --wait"},
		{[]string{"task", "questions", "42", "--wait", "--timeout", "soon"}, "--timeout must be a positive duration"},
		{[]string{"task", "questions", "42", "--wait", "--timeout", "-5s"}, "--timeout must be a positive duration"},
		{[]string{"task", "questions", "42", "--wait", "--timeout"}, "--timeout requires a duration"},
	}
	for _, tc := range cases {
		_, err := runCapture(t, tc.args...)
		if err == nil || !strings.Contains(err.Error(), tc.want) || exitCodeFor(err) != 1 {
			t.Fatalf("%q: err = %v (exit %d), want %q", tc.args, err, exitCodeFor(err), tc.want)
		}
	}
	if len(*requests) != 0 {
		t.Fatalf("invalid input sent requests: %#v", *requests)
	}
}

func TestParseWaitTimeoutAcceptsDurationsAndSeconds(t *testing.T) {
	for value, want := range map[string]time.Duration{"90": 90 * time.Second, "90s": 90 * time.Second, "10m": 10 * time.Minute, "1h30m": 90 * time.Minute} {
		if got, err := parseWaitTimeout(value); err != nil || got != want {
			t.Fatalf("%q = %v, %v; want %v", value, got, err, want)
		}
	}
}

func TestTaskQuestionsWaitPollsUntilResolvedAndPrintsWaitedQuestions(t *testing.T) {
	shortPolls(t)
	polls := 0
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/tasks/42/questions": func(*http.Request) (int, string) {
			polls++
			switch polls {
			case 1:
				return 200, questionsList(2, testQuestionQ0, testQuestionQ1Open, testQuestionQ2Open)
			case 2:
				return 503, `{"error":{"code":"unavailable","message":"Try again"}}`
			case 3:
				return 200, questionsList(1, testQuestionQ0, testQuestionQ1Done, testQuestionQ2Open)
			default:
				return 200, questionsList(0, testQuestionQ0, testQuestionQ1Done, testQuestionQ2Done)
			}
		},
	})
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "task", "questions", "42", "--wait", "--timeout", "10s")
	if err != nil {
		t.Fatal(err)
	}
	if len(*requests) != 4 {
		t.Fatalf("polls = %d, want 4 (a 503 is retried)", len(*requests))
	}
	for _, req := range *requests {
		if req.query != "" {
			t.Fatalf("--wait query = %q, want none", req.query)
		}
	}
	want := `#42: 2 questions, 0 open
Asked by Publisher at 2026-11-13T05:12:40Z
  1. [answered] Should a 409 count as delivered?
     Options: Yes · No
     Answer (John Doe): Yes, treat as delivered
       and log it.
  2. [dismissed] Cap retries at 1h or 24h?
     Dismissed by John Doe
`
	if out != want {
		t.Fatalf("output:\n%s\nwant:\n%s", out, want)
	}
}

func TestTaskQuestionsWaitWithNothingOpenShowsLatestBatchAndJSON(t *testing.T) {
	shortPolls(t)
	body := questionsList(0, testQuestionQ0, testQuestionQ1Done, testQuestionQ2Done)
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/tasks/42/questions": fixed(200, body),
	})
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "task", "questions", "42", "--wait")
	if err != nil {
		t.Fatal(err)
	}
	if len(*requests) != 1 || strings.Contains(out, "Old question") || !strings.Contains(out, "#42: 2 questions, 0 open") {
		t.Fatalf("polls = %d, output:\n%s", len(*requests), out)
	}

	out, err = runCapture(t, "task", "questions", "42", "--wait", "--json")
	if err != nil || strings.TrimSpace(out) != body {
		t.Fatalf("out = %q, err = %v", out, err)
	}
}

func TestTaskQuestionsWaitTimesOutWithDistinctExitCode(t *testing.T) {
	shortPolls(t)
	body := questionsList(2, testQuestionQ1Open, testQuestionQ2Open)
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/tasks/42/questions": fixed(200, body),
	})
	setAgentEnv(t, server.URL)

	start := time.Now()
	out, err := runCapture(t, "task", "questions", "42", "--wait", "--timeout", "40ms")
	if err == nil || exitCodeFor(err) != exitWaitTimeout || exitWaitTimeout != 6 {
		t.Fatalf("err = %v (exit %d), want exit 6", err, exitCodeFor(err))
	}
	if !strings.Contains(err.Error(), "timed out after 40ms: 2 questions still open on #42") {
		t.Fatalf("err = %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("took %s", elapsed)
	}
	if len(*requests) < 2 || out != "" {
		t.Fatalf("polls = %d, stdout = %q", len(*requests), out)
	}

	out, err = runCapture(t, "task", "questions", "42", "--wait", "--timeout", "20ms", "--json")
	if exitCodeFor(err) != exitWaitTimeout || !strings.Contains(out, body) {
		t.Fatalf("--json: out = %q, err = %v", out, err)
	}
}

func TestTaskQuestionsWaitStopsOnClientErrors(t *testing.T) {
	shortPolls(t)
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/tasks/42/questions": fixed(404, `{"error":{"code":"not_found","message":"Task not found"}}`),
	})
	setAgentEnv(t, server.URL)
	_, err := runCapture(t, "task", "questions", "42", "--wait")
	if err == nil || err.Error() != "Task not found (HTTP 404 not_found)" || exitCodeFor(err) != 1 || len(*requests) != 1 {
		t.Fatalf("err = %v (exit %d), polls = %d", err, exitCodeFor(err), len(*requests))
	}
}
