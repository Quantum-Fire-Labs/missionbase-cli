package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/config"
	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/httpclient"
	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/textbody"
)

// Notes, as the signed-in person: daily notes (one free-form Markdown page per
// date, "today" in their time zone) and project notes (one per project). edit
// replaces the whole page; append adds text to the end.

const notesUsage = `usage: missionbase notes search <query>
       missionbase notes daily show [DATE|today] [--json]
       missionbase notes daily list [--before DATE] [--limit N] [--json]
       missionbase notes daily edit [DATE|today] --body TEXT|--body-file PATH [--json]
       missionbase notes daily append [DATE|today] --body TEXT|--body-file PATH [--json]
       missionbase notes project show <project-id> [--json]
       missionbase notes project edit <project-id> --body TEXT|--body-file PATH [--base-version VERSION_ID] [--json]
       missionbase notes project append <project-id> --body TEXT|--body-file PATH [--json]

Project notes hold priorities, decisions and context that tasks can't carry
well, not documentation (that lives on your machine or in Google Drive).
DATE is YYYY-MM-DD; without one it's today in your Missionbase time zone.
edit replaces the whole note: show it first and send the full new text.
append adds the text to the end, after a blank line.
For project notes, --base-version (the version project show prints) keeps text
appended since and refuses other changes made since.`

type notesOptions struct {
	target      string
	body        string
	bodySet     bool
	before      string
	limit       string
	baseVersion string
	json        bool
}

type dailyNotePayload struct {
	Date      string  `json:"date"`
	Body      string  `json:"body"`
	Preview   string  `json:"preview"`
	UpdatedAt *string `json:"updated_at"`
}

type projectNotesVersion struct {
	ID      int64  `json:"id"`
	Summary string `json:"summary"`
	Author  *struct {
		Name string `json:"name"`
	} `json:"author"`
}

type projectNotesPayload struct {
	Body    string               `json:"body"`
	Version *projectNotesVersion `json:"version"`
}

func notes(args []string) error {
	if len(args) == 0 || isHelp(args[0]) {
		fmt.Println(notesUsage)
		return nil
	}
	switch args[0] {
	case "search":
		return notesSearch(args[1:])
	case "daily":
		return dailyNotes(args[1:])
	case "project":
		return projectNotes(args[1:])
	default:
		return fmt.Errorf("unknown notes command %q\n%s", args[0], notesUsage)
	}
}

func notesSearch(args []string) error {
	if len(args) < 1 || isHelp(args[0]) {
		if len(args) == 1 {
			fmt.Println("usage: missionbase notes search <query>")
			return nil
		}
		return fmt.Errorf("usage: missionbase notes search <query>")
	}
	if len(args) > 1 {
		return fmt.Errorf("unknown notes search option %q", args[1])
	}
	values := url.Values{}
	values.Set("query", args[0])
	return apiGet(withQuery("/api/v1/notes/search", values))
}

func dailyNotes(args []string) error {
	if len(args) == 0 || isHelp(args[0]) {
		fmt.Println(notesUsage)
		return nil
	}
	command := args[0]
	allowed := map[string][]string{
		"show":   {"--json"},
		"list":   {"--json", "--before", "--limit"},
		"edit":   {"--json", "--body", "--body-file"},
		"append": {"--json", "--body", "--body-file"},
	}[command]
	if allowed == nil {
		return fmt.Errorf("unknown notes daily command %q\n%s", command, notesUsage)
	}
	opts, help, err := parseNotesArgs("notes daily "+command, args[1:], allowed, command != "list")
	if err != nil || help {
		return err
	}
	date := opts.target
	if date == "" {
		date = "today"
	}
	if err := checkNotesDate(date); err != nil {
		return err
	}
	path := "/api/v1/daily_notes/" + url.PathEscape(date)

	switch command {
	case "show":
		body, err := apiGetBody(path)
		if err != nil || opts.json {
			return printRaw(body, err)
		}
		var response struct {
			DailyNote dailyNotePayload `json:"daily_note"`
		}
		if err := json.Unmarshal(body, &response); err != nil {
			return err
		}
		if strings.TrimSpace(response.DailyNote.Body) == "" {
			fmt.Printf("(no note for %s yet)\n", response.DailyNote.Date)
			return nil
		}
		fmt.Println(response.DailyNote.Body)
		return nil
	case "list":
		return dailyNotesList(opts)
	case "edit":
		if err := requireNotesBody(opts, false); err != nil {
			return err
		}
		body, err := notesWrite("PATCH", path, map[string]any{"body": opts.body})
		return printDailyNoteChange(body, err, opts.json, "Saved")
	default:
		if err := requireNotesBody(opts, true); err != nil {
			return err
		}
		body, err := notesWrite("POST", path+"/appends", map[string]any{"body": opts.body})
		return printDailyNoteChange(body, err, opts.json, "Added to")
	}
}

func dailyNotesList(opts notesOptions) error {
	values := url.Values{}
	if opts.before != "" {
		values.Set("before", opts.before)
	}
	if opts.limit != "" {
		values.Set("limit", opts.limit)
	}
	body, err := apiGetBody(withQuery("/api/v1/daily_notes", values))
	if err != nil || opts.json {
		return printRaw(body, err)
	}
	var response struct {
		DailyNotes []dailyNotePayload `json:"daily_notes"`
		NextBefore *string            `json:"next_before"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return err
	}
	if len(response.DailyNotes) == 0 {
		fmt.Println("No daily notes.")
		return nil
	}
	for _, note := range response.DailyNotes {
		fmt.Printf("%s  %s\n", note.Date, note.Preview)
	}
	if response.NextBefore != nil {
		fmt.Printf("Older days: missionbase notes daily list --before %s\n", *response.NextBefore)
	}
	return nil
}

func printDailyNoteChange(body []byte, err error, jsonOutput bool, verb string) error {
	if err != nil || jsonOutput {
		return printRaw(body, err)
	}
	var response struct {
		DailyNote dailyNotePayload `json:"daily_note"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return err
	}
	fmt.Printf("%s the daily note for %s.\n", verb, response.DailyNote.Date)
	return nil
}

func projectNotes(args []string) error {
	if len(args) == 0 || isHelp(args[0]) {
		fmt.Println(notesUsage)
		return nil
	}
	command := args[0]
	allowed := map[string][]string{
		"show":   {"--json"},
		"edit":   {"--json", "--body", "--body-file", "--base-version"},
		"append": {"--json", "--body", "--body-file"},
	}[command]
	if allowed == nil {
		return fmt.Errorf("unknown notes project command %q\n%s", command, notesUsage)
	}
	opts, help, err := parseNotesArgs("notes project "+command, args[1:], allowed, true)
	if err != nil || help {
		return err
	}
	if opts.target == "" {
		return fmt.Errorf("a project id is required\n%s", notesUsage)
	}
	if !isPositiveNumber(opts.target) {
		return fmt.Errorf("project id must be a number, got %q", opts.target)
	}
	path := "/api/v1/projects/" + url.PathEscape(opts.target) + "/notes"

	switch command {
	case "show":
		body, err := apiGetBody(path)
		if err != nil || opts.json {
			return printRaw(body, err)
		}
		var response struct {
			Notes projectNotesPayload `json:"notes"`
		}
		if err := json.Unmarshal(body, &response); err != nil {
			return err
		}
		if version := response.Notes.Version; version != nil {
			fmt.Printf("<!-- version %d: %s -->\n", version.ID, describeProjectNotesVersion(*version))
		}
		if strings.TrimSpace(response.Notes.Body) == "" {
			fmt.Println("(no notes yet)")
			return nil
		}
		fmt.Println(response.Notes.Body)
		return nil
	case "edit":
		if err := requireNotesBody(opts, false); err != nil {
			return err
		}
		payload := map[string]any{"body": opts.body}
		if opts.baseVersion != "" {
			id, _ := strconv.ParseInt(opts.baseVersion, 10, 64)
			payload["base_version_id"] = id
		}
		body, err := notesWrite("PATCH", path, payload)
		return printProjectNotesChange(body, err, opts.json, "notes")
	default:
		if err := requireNotesBody(opts, true); err != nil {
			return err
		}
		body, err := notesWrite("POST", path+"/entries", map[string]any{"body": opts.body})
		return printProjectNotesChange(body, err, opts.json, "entry")
	}
}

// printProjectNotesChange prints "Saved version 12: appended to the end".
func printProjectNotesChange(body []byte, err error, jsonOutput bool, key string) error {
	if err != nil || jsonOutput {
		return printRaw(body, err)
	}
	var response struct {
		Notes projectNotesPayload  `json:"notes"`
		Entry *projectNotesVersion `json:"entry"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return err
	}
	version := response.Notes.Version
	if key == "entry" && response.Entry != nil {
		version = response.Entry
	}
	if version == nil {
		fmt.Println("Notes unchanged.")
		return nil
	}
	fmt.Printf("Saved version %d: %s\n", version.ID, version.Summary)
	return nil
}

func describeProjectNotesVersion(version projectNotesVersion) string {
	if version.Author == nil || version.Author.Name == "" {
		return version.Summary
	}
	return version.Author.Name + " " + version.Summary
}

// parseNotesArgs reads an optional positional target (a date or a project id)
// and the allowed options.
func parseNotesArgs(command string, args []string, allowed []string, takesTarget bool) (notesOptions, bool, error) {
	opts := notesOptions{}
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if isHelp(arg) {
			fmt.Println(notesUsage)
			return opts, true, nil
		}
		if !strings.HasPrefix(arg, "--") {
			if !takesTarget || opts.target != "" {
				return opts, false, fmt.Errorf("unexpected argument %q\n%s", arg, notesUsage)
			}
			opts.target = arg
			continue
		}
		if !containsOption(allowed, arg) {
			return opts, false, fmt.Errorf("unknown %s option %q", command, arg)
		}
		if arg == "--json" {
			opts.json = true
			continue
		}
		if i+1 >= len(args) {
			return opts, false, fmt.Errorf("%s requires a value", arg)
		}
		value := args[i+1]
		i++
		switch arg {
		case "--body":
			if opts.bodySet {
				return opts, false, fmt.Errorf("give --body or --body-file once")
			}
			opts.body, opts.bodySet = textbody.Normalize(value), true
		case "--body-file":
			if opts.bodySet {
				return opts, false, fmt.Errorf("give --body or --body-file once")
			}
			content, err := os.ReadFile(value)
			if err != nil {
				return opts, false, fmt.Errorf("read body file %q: %w", value, err)
			}
			opts.body, opts.bodySet = string(content), true
		case "--before":
			if err := checkNotesDate(value); err != nil || value == "today" {
				return opts, false, fmt.Errorf("--before must be YYYY-MM-DD, got %q", value)
			}
			opts.before = value
		case "--limit":
			if !isPositiveNumber(value) {
				return opts, false, fmt.Errorf("--limit must be a positive number, got %q", value)
			}
			opts.limit = value
		case "--base-version":
			if !isPositiveNumber(value) {
				return opts, false, fmt.Errorf("--base-version must be a version id, got %q", value)
			}
			opts.baseVersion = value
		}
	}
	return opts, false, nil
}

func requireNotesBody(opts notesOptions, nonBlank bool) error {
	if !opts.bodySet {
		return fmt.Errorf("--body or --body-file is required\n%s", notesUsage)
	}
	if nonBlank && strings.TrimSpace(opts.body) == "" {
		return fmt.Errorf("there's nothing to append: the body is empty")
	}
	return nil
}

func checkNotesDate(value string) error {
	if value == "today" {
		return nil
	}
	date, err := time.Parse("2006-01-02", value)
	if err != nil || date.Format("2006-01-02") != value {
		return fmt.Errorf("date must be YYYY-MM-DD or today, got %q", value)
	}
	return nil
}

func notesWrite(method, path string, payload map[string]any) ([]byte, error) {
	request, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	cfg, err := config.LoadUser()
	if err != nil {
		return nil, err
	}
	client := httpclient.NewUser(cfg)
	if method == "PATCH" {
		return client.Patch(path, request)
	}
	return client.Post(path, request)
}

func printRaw(body []byte, err error) error {
	if err != nil {
		return err
	}
	fmt.Println(string(body))
	return nil
}

func containsOption(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func isPositiveNumber(value string) bool {
	number, err := strconv.ParseInt(value, 10, 64)
	return err == nil && number > 0
}
