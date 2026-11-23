package activity

import (
	"bytes"
	"strings"
	"testing"
)

func TestParseAndPath(t *testing.T) {
	opts, help, err := Parse(nil)
	if err != nil || help || opts.Path() != "/api/v1/activity" {
		t.Fatalf("no flags: %+v %v %v path=%s", opts, help, err, opts.Path())
	}

	opts, _, err = Parse([]string{"--date", "2026-11-20", "--project", "5", "--cursor", "812", "--limit", "50", "--json"})
	if err != nil {
		t.Fatal(err)
	}
	if got, want := opts.Path(), "/api/v1/activity?cursor=812&date=2026-11-20&limit=50&project_id=5"; got != want {
		t.Fatalf("path = %s, want %s", got, want)
	}
	if !opts.JSON {
		t.Fatal("--json not set")
	}

	if _, help, _ := Parse([]string{"--help"}); !help {
		t.Fatal("--help not seen")
	}
}

func TestParseRefusesBadInput(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{"--date", "2026-02-30"}, "--date must be YYYY-MM-DD or today"},
		{[]string{"--date", "yesterday"}, "--date must be YYYY-MM-DD or today"},
		{[]string{"--project", "abc"}, "--project must be a project id"},
		{[]string{"--cursor", "0"}, "--cursor must be the id the last page gave"},
		{[]string{"--limit", "-1"}, "--limit must be a positive number"},
		{[]string{"--date"}, "--date requires a value"},
		{[]string{"--since", "1h"}, `unknown activity option "--since"`},
		{[]string{"today"}, `unexpected argument "today"`},
	} {
		_, _, err := Parse(tc.args)
		if err == nil || !strings.Contains(err.Error(), tc.want) {
			t.Errorf("Parse(%v) err = %v, want %q", tc.args, err, tc.want)
		}
	}
}

func TestPrintTextJSONAndEmpty(t *testing.T) {
	body := []byte(`{"date":"2026-11-20","time_zone":"America/Toronto","project":null,"activity_events":[
		{"id":7,"actor":{"type":"User","id":1,"name":"Daniel Lemky"},"scope":{"project":{"type":"Project","id":5,"name":"Missionbase"}},"action":"task.created","timestamp":"2026-11-20T09:14:00-05:00","summary":"Daniel Lemky created task Plan the retreat."},
		{"id":8,"actor":{"type":"Agent","id":12,"name":"Claude"},"scope":{"project":{"type":"Project","id":9,"name":"Home"}},"action":"task.status_changed","timestamp":"2026-11-20T21:30:00-05:00","summary":"Claude moved task #462 to In Progress."},
		{"id":9,"scope":{"project":{"type":"Project","id":9,"name":"Home"}},"action":"task.start_date_arrived","timestamp":"2026-11-20T23:59:00-05:00","summary":"Task Pay rent moved to To Do."}
	],"next_cursor":9}`)

	var out bytes.Buffer
	if err := Print(&out, body, "missionbase", Options{Project: "", Limit: "3"}); err != nil {
		t.Fatal(err)
	}
	want := `Activity for Friday, November 20, 2026 (America/Toronto)
09:14  Missionbase  Daniel Lemky  Daniel Lemky created task Plan the retreat.
21:30  Home         Claude        Claude moved task #462 to In Progress.
23:59  Home         Missionbase   Task Pay rent moved to To Do.
More: missionbase activity --date 2026-11-20 --limit 3 --cursor 9
`
	if out.String() != want {
		t.Fatalf("text =\n%s\nwant\n%s", out.String(), want)
	}

	out.Reset()
	if err := Print(&out, body, "missionbase", Options{JSON: true}); err != nil || !strings.HasPrefix(out.String(), `{"date":"2026-11-20"`) {
		t.Fatalf("json = %q, err = %v", out.String(), err)
	}

	out.Reset()
	empty := []byte(`{"date":"2026-11-19","time_zone":"America/Toronto","project":{"type":"Project","id":5,"name":"Missionbase"},"activity_events":[],"next_cursor":null}`)
	if err := Print(&out, empty, "missionbase-agent", Options{Project: "5"}); err != nil {
		t.Fatal(err)
	}
	if want := "Activity for Thursday, November 19, 2026 (America/Toronto) in Missionbase\nNo activity.\n"; out.String() != want {
		t.Fatalf("empty = %q", out.String())
	}
}
