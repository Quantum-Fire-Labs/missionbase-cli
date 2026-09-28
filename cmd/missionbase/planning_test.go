package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUserPlanningCommandsCallScopedEndpoints(t *testing.T) {
	requests := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Missionbase-Agent-Slug") != "" {
			t.Errorf("agent header must not be sent")
		}
		requests = append(requests, r.Method+" "+r.URL.String())
		if r.Method == http.MethodPost || r.Method == http.MethodPatch {
			var body map[string]string
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Error(err)
			}
			if r.Method == http.MethodPost && body["task_id"] != "42" {
				t.Errorf("add body = %#v", body)
			}
			if r.Method == http.MethodPatch && body["after_task_id"] != "42" {
				t.Errorf("order body = %#v", body)
			}
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer server.Close()
	setUserEnv(t, server.URL)

	commands := [][]string{
		{"week", "show", "--team", "9", "--starts-on", "2026-08-03"},
		{"week", "add", "42", "--team", "9", "--starts-on", "2026-08-03"},
		{"week", "order", "17", "--team", "9", "--starts-on", "2026-08-03", "--after-task", "42"},
		{"week", "remove", "17", "--team", "9", "--starts-on", "2026-08-03"},
		{"day", "show", "--date", "2026-08-05"},
		{"day", "add", "42", "--date", "2026-08-05"},
		{"day", "order", "17", "--date", "2026-08-05", "--after-task", "42"},
		{"day", "remove", "17", "--date", "2026-08-05"},
	}
	for _, command := range commands {
		if err := run(command); err != nil {
			t.Fatalf("%v: %v", command, err)
		}
	}
	want := []string{
		"GET /api/v1/weeks?starts_on=2026-08-03&team_id=9",
		"POST /api/v1/weeks/2026-08-03/placements?team_id=9",
		"PATCH /api/v1/weeks/2026-08-03/placements/17?team_id=9",
		"DELETE /api/v1/weeks/2026-08-03/placements/17?team_id=9",
		"GET /api/v1/days/2026-08-05",
		"POST /api/v1/days/2026-08-05/placements",
		"PATCH /api/v1/days/2026-08-05/placements/17",
		"DELETE /api/v1/days/2026-08-05/placements/17",
	}
	if strings.Join(requests, "\n") != strings.Join(want, "\n") {
		t.Fatalf("requests = %v, want %v", requests, want)
	}
}

func TestPlanningCommandsRejectInvalidDatesAndUserCLILock(t *testing.T) {
	for _, command := range [][]string{
		{"week", "add", "42", "--team", "9", "--starts-on", "2026-08-04"},
		{"day", "show", "--date", "2026-02-30"},
		{"week", "show", "--starts-on", "2026-08-03"},
		{"day", "add", "42", "--date", "2026-08-05", "--after-task", "9"},
	} {
		if err := run(command); err == nil {
			t.Fatalf("expected %v to fail", command)
		}
	}
	t.Setenv("MISSIONBASE_ACTOR_MODE", "agent")
	if err := run([]string{"day", "show", "--date", "2026-08-05"}); err == nil || !strings.Contains(err.Error(), "locked") {
		t.Fatalf("agent actor mode must reject user planning: %v", err)
	}
}
