// Package activity reads one day of activity across the projects the caller
// can see (GET /api/v1/activity), for `missionbase activity` and
// `missionbase-agent activity`. The day runs from 00:00 to 24:00 in the
// person's Missionbase time zone (an agent's owner's), oldest first.
package activity

import (
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// Options are the command's flags.
type Options struct {
	Date    string
	Project string
	Cursor  string
	Limit   string
	JSON    bool
}

// Usage is the command's help for the given program name.
func Usage(program string) string {
	return fmt.Sprintf(`usage: %[1]s activity [--date YYYY-MM-DD|today] [--project ID] [--cursor ID] [--limit N] [--json]

Lists what happened in your projects on one day, oldest first: today by
default, in your Missionbase time zone, or --date for another day.
--project keeps one project. A long day comes in pages of 100 (--limit 1-200);
the last line gives the --cursor for the next page.`, program)
}

// Parse reads the flags. help is true when --help was asked for.
func Parse(args []string) (opts Options, help bool, err error) {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch arg {
		case "--help", "-h", "help":
			return opts, true, nil
		case "--json":
			opts.JSON = true
			continue
		case "--date", "--project", "--cursor", "--limit":
		default:
			if strings.HasPrefix(arg, "-") {
				return opts, false, fmt.Errorf("unknown activity option %q", arg)
			}
			return opts, false, fmt.Errorf("unexpected argument %q", arg)
		}
		if i+1 >= len(args) {
			return opts, false, fmt.Errorf("%s requires a value", arg)
		}
		value := strings.TrimSpace(args[i+1])
		i++
		switch arg {
		case "--date":
			if !validDate(value) {
				return opts, false, fmt.Errorf("--date must be YYYY-MM-DD or today, got %q", value)
			}
			opts.Date = value
		case "--project":
			if !positive(value) {
				return opts, false, fmt.Errorf("--project must be a project id, got %q", value)
			}
			opts.Project = value
		case "--cursor":
			if !positive(value) {
				return opts, false, fmt.Errorf("--cursor must be the id the last page gave, got %q", value)
			}
			opts.Cursor = value
		case "--limit":
			if !positive(value) {
				return opts, false, fmt.Errorf("--limit must be a positive number, got %q", value)
			}
			opts.Limit = value
		}
	}
	return opts, false, nil
}

// Path is the request for these options.
func (opts Options) Path() string {
	values := url.Values{}
	for key, value := range map[string]string{"date": opts.Date, "project_id": opts.Project, "cursor": opts.Cursor, "limit": opts.Limit} {
		if value != "" {
			values.Set(key, value)
		}
	}
	if len(values) == 0 {
		return "/api/v1/activity"
	}
	return "/api/v1/activity?" + values.Encode()
}

type ref struct {
	Type string `json:"type"`
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type event struct {
	ID    int64 `json:"id"`
	Actor *ref  `json:"actor"`
	Scope struct {
		Project *ref `json:"project"`
	} `json:"scope"`
	Timestamp string `json:"timestamp"`
	Summary   string `json:"summary"`
}

type response struct {
	Date           string  `json:"date"`
	TimeZone       string  `json:"time_zone"`
	Project        *ref    `json:"project"`
	ActivityEvents []event `json:"activity_events"`
	NextCursor     *int64  `json:"next_cursor"`
}

// Print writes the response: raw with --json, otherwise one line per event
// (time, project, who, summary) under a header, and the next page's command.
func Print(w io.Writer, body []byte, program string, opts Options) error {
	if opts.JSON {
		_, err := fmt.Fprintln(w, string(body))
		return err
	}
	var day response
	if err := json.Unmarshal(body, &day); err != nil {
		return err
	}

	header := "Activity for " + day.Date
	if date, err := time.Parse("2006-01-02", day.Date); err == nil {
		header = "Activity for " + date.Format("Monday, January 2, 2006")
	}
	if day.TimeZone != "" {
		header += " (" + day.TimeZone + ")"
	}
	if day.Project != nil {
		header += " in " + day.Project.Name
	}
	fmt.Fprintln(w, header)
	if len(day.ActivityEvents) == 0 {
		fmt.Fprintln(w, "No activity.")
		return nil
	}

	rows := make([][3]string, len(day.ActivityEvents))
	projectWidth, actorWidth := 0, 0
	for i, item := range day.ActivityEvents {
		project, actor := "", "Missionbase"
		if item.Scope.Project != nil {
			project = item.Scope.Project.Name
		}
		if item.Actor != nil && item.Actor.Name != "" {
			actor = item.Actor.Name
		}
		rows[i] = [3]string{clock(item.Timestamp), project, actor}
		projectWidth = max(projectWidth, utf8.RuneCountInString(project))
		actorWidth = max(actorWidth, utf8.RuneCountInString(actor))
	}
	for i, item := range day.ActivityEvents {
		row := rows[i]
		line := fmt.Sprintf("%s  %s  %s  %s", row[0], pad(row[1], projectWidth), pad(row[2], actorWidth), item.Summary)
		fmt.Fprintln(w, strings.TrimRight(line, " "))
	}

	if day.NextCursor != nil {
		next := []string{program, "activity", "--date", day.Date}
		if opts.Project != "" {
			next = append(next, "--project", opts.Project)
		}
		if opts.Limit != "" {
			next = append(next, "--limit", opts.Limit)
		}
		next = append(next, "--cursor", strconv.FormatInt(*day.NextCursor, 10))
		fmt.Fprintln(w, "More: "+strings.Join(next, " "))
	}
	return nil
}

// clock is the event's time of day; the timestamp is already in the person's zone.
func clock(timestamp string) string {
	at, err := time.Parse(time.RFC3339, timestamp)
	if err != nil {
		return timestamp
	}
	return at.Format("15:04")
}

func pad(value string, width int) string {
	return value + strings.Repeat(" ", width-utf8.RuneCountInString(value))
}

func validDate(value string) bool {
	if value == "today" {
		return true
	}
	date, err := time.Parse("2006-01-02", value)
	return err == nil && date.Format("2006-01-02") == value
}

func positive(value string) bool {
	number, err := strconv.ParseInt(value, 10, 64)
	return err == nil && number > 0
}
