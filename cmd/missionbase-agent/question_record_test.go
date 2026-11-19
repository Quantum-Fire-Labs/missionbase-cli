package main

import (
	"net/http"
	"strconv"
	"strings"
	"testing"
)

const (
	testRecorder        = `{"type":"Agent","id":6,"name":"Claude","slug":"claude","handle":"claude"}`
	testQuestionQ1Taken = `{"id":2,"task_id":42,"batch":"b1","position":1,"body":"Should a 409 count as delivered?","context":null,"options":["Yes","No"],"status":"answered","asked_by":` + testAskedBy + `,"asked_at":"2026-11-13T05:12:40Z","answer_body":"No — retry once","answer_source":"in a call","answered_by":` + testRecorder + `,"answered_at":"2026-11-13T06:00:00Z","dismissed_by":null,"dismissed_at":null,"dismiss_reason":null}`
	testQuestionQ2Gone  = `{"id":3,"task_id":42,"batch":"b1","position":2,"body":"Cap retries at 1h or 24h?","context":null,"options":[],"status":"dismissed","asked_by":` + testAskedBy + `,"asked_at":"2026-11-13T05:12:40Z","answer_body":null,"answer_source":null,"answered_by":null,"answered_at":null,"dismissed_by":` + testRecorder + `,"dismissed_at":"2026-11-13T06:00:00Z","dismiss_reason":"Covered by #44"}`
	testResolveTaskJSON = `{"id":42,"title":"Ship it","status":"planning","status_label":"Planning","needs_you":false,"questions":{"open_count":0}}`
)

func resolveResponseJSON(answered, dismissed, skipped string, open int, questions ...string) string {
	return `{"task":` + testResolveTaskJSON + `,"questions":[` + strings.Join(questions, ",") + `],"answered":[` + answered + `],"dismissed":[` + dismissed + `],"skipped":[` + skipped + `],"open_remaining":` + strconv.Itoa(open) + `}`
}

func TestTaskAnswerRecordsTheOwnersWordsFromChat(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"PATCH /api/v1/tasks/42/questions": fixed(200, resolveResponseJSON("2", "", "", 1, testQuestionQ1Taken, testQuestionQ2Open)),
	})
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "task", "answer", "42", "#2", "  Yes, ship it  ")
	if err != nil {
		t.Fatal(err)
	}
	if len(*requests) != 1 {
		t.Fatalf("requests = %#v, want only the PATCH", *requests)
	}
	req := (*requests)[0]
	if req.method != "PATCH" || req.slug != "missionbase-dev" {
		t.Fatalf("request = %#v", req)
	}
	if got, want := canonicalJSON(t, req.payload), `{"answers":{"2":"Yes, ship it"},"source":"in chat"}`; got != want {
		t.Fatalf("payload = %s\nwant      %s", got, want)
	}
	assertContainsAll(t, out,
		`Recorded 1 answer given in chat on #42 "Ship it"`+"\n",
		"  1. Should a 409 count as delivered?\n",
		"     Answer (given in a call, recorded by Claude): No — retry once\n",
		"1 question still open\n",
	)
	if strings.Contains(out, "Cap retries") {
		t.Fatalf("printed a question it didn't resolve:\n%s", out)
	}
}

func TestTaskAnswerOptionReadsTheQuestionsAndPutsTheOptionFirst(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/tasks/42/questions":   fixed(200, questionsList(2, testQuestionQ1Open, testQuestionQ2Open)),
		"PATCH /api/v1/tasks/42/questions": fixed(200, resolveResponseJSON("2", "", "", 1, testQuestionQ1Taken)),
	})
	setAgentEnv(t, server.URL)

	if _, err := runCapture(t, "task", "answer", "42", "2", "--option", "2", "retry once", "--source", "in a call"); err != nil {
		t.Fatal(err)
	}
	if len(*requests) != 2 || (*requests)[0].method != "GET" {
		t.Fatalf("requests = %#v", *requests)
	}
	if got, want := canonicalJSON(t, (*requests)[1].payload), `{"answers":{"2":"No — retry once"},"source":"in a call"}`; got != want {
		t.Fatalf("payload = %s\nwant      %s", got, want)
	}
}

func TestTaskAnswerFileSendsEveryAnswerAndJSONIsRaw(t *testing.T) {
	response := resolveResponseJSON("2,3", "", "", 0, testQuestionQ1Taken)
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/tasks/42/questions":   fixed(200, questionsList(2, testQuestionQ1Open, testQuestionQ2Open)),
		"PATCH /api/v1/tasks/42/questions": fixed(200, response),
	})
	setAgentEnv(t, server.URL)
	file := writeTextFile(t, "answers.json", `[{"id": 2, "option": 1}, {"id": 3, "answer": " 24 hours "}]`)

	out, err := runCapture(t, "task", "answer", "42", "--file", file, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := canonicalJSON(t, (*requests)[1].payload), `{"answers":{"2":"Yes","3":"24 hours"},"source":"in chat"}`; got != want {
		t.Fatalf("payload = %s\nwant      %s", got, want)
	}
	if strings.TrimSpace(out) != response {
		t.Fatalf("--json output = %q, want the raw response", out)
	}
}

func TestTaskAnswerArgumentErrorsSendNothing(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/tasks/42/questions": fixed(200, questionsList(2, testQuestionQ1Open, testQuestionQ2Open)),
	})
	setAgentEnv(t, server.URL)
	badFile := writeTextFile(t, "answers.json", `[{"id": 2, "text": "Yes"}]`)

	cases := []struct {
		args []string
		want string
	}{
		{[]string{"task", "answer"}, "usage: missionbase-agent task answer"},
		{[]string{"task", "answer", "abc", "2", "Yes"}, "task id must be a number"},
		{[]string{"task", "answer", "42"}, "a question id is required"},
		{[]string{"task", "answer", "42", "two", "Yes"}, "question id must be a number"},
		{[]string{"task", "answer", "42", "2"}, "give the owner's answer as text, --option N, or both"},
		{[]string{"task", "answer", "42", "2", "  "}, "give the owner's answer as text"},
		{[]string{"task", "answer", "42", "2", "Yes", "extra"}, `unexpected argument "extra"`},
		{[]string{"task", "answer", "42", "2", "Yes", "--source", " "}, "--source can't be blank"},
		{[]string{"task", "answer", "42", "2", "Yes", "--source", strings.Repeat("x", 101)}, "--source must be at most 100 characters"},
		{[]string{"task", "answer", "42", "2", "--option", "5"}, "--option must be an option number from 1 to 4"},
		{[]string{"task", "answer", "42", "2", "--option"}, "--option requires a value"},
		{[]string{"task", "answer", "42", "2", "Yes", "--verbose"}, `unknown task answer option "--verbose"`},
		{[]string{"task", "answer", "42", "2", "Yes", "--file", badFile}, "not both"},
		{[]string{"task", "answer", "42", "--file", badFile}, "is not a valid answers file"},
	}
	for _, tc := range cases {
		_, err := runCapture(t, tc.args...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%v: err = %v, want %q", tc.args, err, tc.want)
		}
		if code := exitCodeFor(err); code != 1 {
			t.Fatalf("%v: exit code = %d, want 1", tc.args, code)
		}
	}
	if len(*requests) != 0 {
		t.Fatalf("argument errors sent requests: %#v", *requests)
	}

	// An option the question doesn't have is caught after reading the questions, before the PATCH.
	_, err := runCapture(t, "task", "answer", "42", "3", "--option", "1")
	if err == nil || !strings.Contains(err.Error(), "question 3 has 0 options; --option 1 doesn't exist") {
		t.Fatalf("err = %v", err)
	}
	_, err = runCapture(t, "task", "answer", "42", "99", "--option", "1")
	if err == nil || !strings.Contains(err.Error(), "question 99 is not on task #42") {
		t.Fatalf("err = %v", err)
	}
	for _, req := range *requests {
		if req.method != "GET" {
			t.Fatalf("sent %s after an option error", req.method)
		}
	}
}

func TestTaskAnswerShowsServerErrorsAndExitsOne(t *testing.T) {
	server, _ := recordServer(t, map[string]func(*http.Request) (int, string){
		"PATCH /api/v1/tasks/42/questions": fixed(404, `{"error":{"code":"not_found","message":"Task not found"}}`),
	})
	setAgentEnv(t, server.URL)

	_, err := runCapture(t, "task", "answer", "42", "2", "Yes")
	if err == nil || !strings.Contains(err.Error(), "Task not found") {
		t.Fatalf("err = %v", err)
	}
	if code := exitCodeFor(err); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
}

func TestTaskDismissSendsIDsAndReason(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"PATCH /api/v1/tasks/42/questions": fixed(200, resolveResponseJSON("", "3", "2", 0, testQuestionQ1Taken, testQuestionQ2Gone)),
	})
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "task", "dismiss", "42", "3", "2", "3", "--reason", " Covered by #44 ")
	if err != nil {
		t.Fatal(err)
	}
	if got, want := canonicalJSON(t, (*requests)[0].payload), `{"dismiss":[3,2],"reason":"Covered by #44"}`; got != want {
		t.Fatalf("payload = %s\nwant      %s", got, want)
	}
	assertContainsAll(t, out,
		`Dismissed 1 question on #42 "Ship it"`+"\n",
		"  2. Cap retries at 1h or 24h?\n",
		"     Dismissed by Claude: Covered by #44\n",
		"Skipped (already answered or dismissed): 2\n",
		"No questions left open\n",
	)

	// Without a reason, none is sent.
	if _, err := runCapture(t, "task", "dismiss", "42", "3", "--json"); err != nil {
		t.Fatal(err)
	}
	if got := canonicalJSON(t, (*requests)[1].payload); got != `{"dismiss":[3]}` {
		t.Fatalf("payload = %s", got)
	}
}

func TestTaskDismissArgumentErrorsSendNothing(t *testing.T) {
	server, requests := recordServer(t, nil)
	setAgentEnv(t, server.URL)

	cases := []struct {
		args []string
		want string
	}{
		{[]string{"task", "dismiss"}, "usage: missionbase-agent task dismiss"},
		{[]string{"task", "dismiss", "42"}, "task attention clear 42"},
		{[]string{"task", "dismiss", "42", "x"}, "question id must be a number"},
		{[]string{"task", "dismiss", "42", "3", "--reason", " "}, "--reason can't be blank"},
		{[]string{"task", "dismiss", "42", "3", "--reason"}, "--reason requires a value"},
		{[]string{"task", "dismiss", "42", "3", "--all"}, `unknown task dismiss option "--all"`},
	}
	for _, tc := range cases {
		_, err := runCapture(t, tc.args...)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Fatalf("%v: err = %v, want %q", tc.args, err, tc.want)
		}
		if code := exitCodeFor(err); code != 1 {
			t.Fatalf("%v: exit code = %d, want 1", tc.args, code)
		}
	}
	if len(*requests) != 0 {
		t.Fatalf("argument errors sent requests: %#v", *requests)
	}
}
