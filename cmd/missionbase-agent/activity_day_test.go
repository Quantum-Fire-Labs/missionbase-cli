package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestActivityListsTheDayAcrossTheAgentsProjects(t *testing.T) {
	var requests []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "missionbase-dev" {
			t.Errorf("agent slug header = %q", got)
		}
		requests = append(requests, r.Method+" "+r.URL.RequestURI())
		_, _ = w.Write([]byte(`{"date":"2026-11-20","time_zone":"America/Toronto","project":null,"activity_events":[{"id":8,"actor":{"type":"Agent","id":12,"name":"Claude"},"scope":{"project":{"type":"Project","id":5,"name":"Missionbase"}},"action":"task.status_changed","timestamp":"2026-11-20T21:30:00-05:00","summary":"Claude moved task #462 to In Progress."}],"next_cursor":null}`))
	}))
	defer server.Close()
	setAgentEnv(t, server.URL)

	var err error
	output := captureStdout(t, func() { err = run([]string{"activity"}) })
	if err != nil {
		t.Fatal(err)
	}
	want := "Activity for Friday, November 20, 2026 (America/Toronto)\n" +
		"21:30  Missionbase  Claude  Claude moved task #462 to In Progress.\n"
	if output != want {
		t.Fatalf("output = %q", output)
	}

	captureStdout(t, func() { err = run([]string{"activity", "--date", "2026-11-20", "--project", "5"}) })
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"GET /api/v1/activity", "GET /api/v1/activity?date=2026-11-20&project_id=5"}; strings.Join(requests, "|") != strings.Join(want, "|") {
		t.Fatalf("requests = %v", requests)
	}
}

func TestActivityHelpCoversTheDayAndOneProject(t *testing.T) {
	var err error
	output := captureStdout(t, func() { err = run([]string{"activity", "--help"}) })
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"usage: missionbase-agent activity [--date YYYY-MM-DD|today]", "usage: missionbase-agent activity project <id>"} {
		if !strings.Contains(output, want) {
			t.Fatalf("help %q missing %q", output, want)
		}
	}

	err = run([]string{"activity", "--date", "tomorrow"})
	if err == nil || !strings.Contains(err.Error(), "--date must be YYYY-MM-DD or today") {
		t.Fatalf("err = %v", err)
	}
}
