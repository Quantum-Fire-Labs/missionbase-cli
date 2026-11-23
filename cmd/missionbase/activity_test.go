package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const activityDayResponse = `{"date":"2026-11-20","time_zone":"America/Toronto","project":{"type":"Project","id":5,"name":"Missionbase"},"activity_events":[{"id":7,"actor":{"type":"User","id":1,"name":"Daniel Lemky"},"scope":{"project":{"type":"Project","id":5,"name":"Missionbase"}},"action":"task.created","timestamp":"2026-11-20T09:14:00-05:00","summary":"Daniel Lemky created task Plan the retreat."}],"next_cursor":7}`

func activityServer(t *testing.T, requests *[]string) func() {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("X-Missionbase-Agent-Slug"); got != "" {
			t.Errorf("agent slug header = %q, want none: the user CLI acts as the person", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer user-token" {
			t.Errorf("authorization = %q", got)
		}
		*requests = append(*requests, r.Method+" "+r.URL.RequestURI())
		_, _ = w.Write([]byte(activityDayResponse))
	}))
	setUserEnv(t, server.URL)
	return server.Close
}

func TestActivityListsTodayByDefault(t *testing.T) {
	var requests []string
	defer activityServer(t, &requests)()

	var err error
	output := captureStdout(t, func() { err = run([]string{"activity"}) })
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 1 || requests[0] != "GET /api/v1/activity" {
		t.Fatalf("requests = %v", requests)
	}
	want := "Activity for Friday, November 20, 2026 (America/Toronto) in Missionbase\n" +
		"09:14  Missionbase  Daniel Lemky  Daniel Lemky created task Plan the retreat.\n" +
		"More: missionbase activity --date 2026-11-20 --cursor 7\n"
	if output != want {
		t.Fatalf("output = %q", output)
	}
}

func TestActivitySendsDateProjectCursorAndPrintsJSON(t *testing.T) {
	var requests []string
	defer activityServer(t, &requests)()

	var err error
	output := captureStdout(t, func() {
		err = run([]string{"activity", "--date", "2026-11-20", "--project", "5", "--cursor", "3", "--limit", "20", "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := "GET /api/v1/activity?cursor=3&date=2026-11-20&limit=20&project_id=5"; len(requests) != 1 || requests[0] != want {
		t.Fatalf("requests = %v", requests)
	}
	if output != activityDayResponse+"\n" {
		t.Fatalf("json output = %q", output)
	}
}

func TestActivityRefusesBadFlagsWithoutARequest(t *testing.T) {
	var requests []string
	defer activityServer(t, &requests)()

	err := run([]string{"activity", "--date", "20-11-2026"})
	if err == nil || !strings.Contains(err.Error(), "--date must be YYYY-MM-DD or today") || !strings.Contains(err.Error(), "usage: missionbase activity") {
		t.Fatalf("err = %v", err)
	}
	if len(requests) != 0 {
		t.Fatalf("requests = %v", requests)
	}

	output := captureStdout(t, func() { err = run([]string{"activity", "--help"}) })
	if err != nil || !strings.HasPrefix(output, "usage: missionbase activity [--date YYYY-MM-DD|today]") {
		t.Fatalf("help = %q, err = %v", output, err)
	}
}
