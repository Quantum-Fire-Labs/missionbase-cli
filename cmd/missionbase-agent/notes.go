package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// Project notes: one Markdown page per project with version history.
//
//	missionbase-agent projects notes show [project-id] [--json]
//	missionbase-agent projects notes edit [project-id] --body-file PATH [--base-version ID] [--json]
//	missionbase-agent projects notes append [project-id] --body-file PATH [--date YYYY-MM-DD] [--json]
//	missionbase-agent projects notes history [project-id] [--limit N] [--before ID] [--json]
//
// Without a project id the project comes from --project, then the nearest
// .missionbase/project.json (from --cwd or the current folder), then the
// machine's default project.

func notesUsage() string {
	return "usage: missionbase-agent projects notes show [project-id] [--json]\n" +
		"       missionbase-agent projects notes edit [project-id] --body-file PATH [--base-version VERSION_ID] [--json]\n" +
		"       missionbase-agent projects notes append [project-id] --body-file PATH [--date YYYY-MM-DD] [--json]\n" +
		"       missionbase-agent projects notes history [project-id] [--limit N] [--before VERSION_ID] [--json]\n\n" +
		"The project is the id given, else --project, else the nearest .missionbase/project.json (from --cwd or here), else the default project.\n" +
		"edit replaces the whole page; pass --base-version (the version you read) to keep entries appended since and refuse other changes.\n" +
		"append adds a dated entry under ## Log (newest first, under the day's heading; --date defaults to today here) without rewriting the page."
}

type notesOptions struct {
	projectID   string
	projectFlag string
	cwd         string
	bodyFile    string
	baseVersion string
	date        string
	limit       string
	before      string
	json        bool
}

type notesVersion struct {
	ID           int64        `json:"id"`
	Action       string       `json:"action"`
	Summary      string       `json:"summary"`
	Author       *notesAuthor `json:"author"`
	Source       string       `json:"source"`
	LinesAdded   int          `json:"lines_added"`
	LinesRemoved int          `json:"lines_removed"`
	CreatedAt    string       `json:"created_at"`
}

type notesAuthor struct {
	Type string `json:"type"`
	Name string `json:"name"`
	Slug string `json:"slug"`
}

type notesPayload struct {
	Body          string        `json:"body"`
	VersionsCount int           `json:"versions_count"`
	Version       *notesVersion `json:"version"`
}

func projectNotes(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Println(notesUsage())
		return nil
	}

	command := args[0]
	switch command {
	case "show", "edit", "append", "history":
	default:
		return fmt.Errorf("unknown projects notes command %q\n%s", command, notesUsage())
	}

	opts, help, err := parseNotesOptions(command, args[1:])
	if err != nil {
		return err
	}
	if help {
		fmt.Println(notesUsage())
		return nil
	}

	projectID, err := notesProjectID(opts)
	if err != nil {
		return err
	}
	base := "/api/v1/projects/" + url.PathEscape(projectID) + "/notes"

	switch command {
	case "show":
		return notesShow(base, opts)
	case "edit":
		return notesEdit(base, opts)
	case "append":
		return notesAppend(base, opts)
	default:
		return notesHistory(base, opts)
	}
}

func parseNotesOptions(command string, args []string) (notesOptions, bool, error) {
	opts := notesOptions{}
	value := func(i int) (string, error) {
		if i+1 >= len(args) || strings.HasPrefix(args[i+1], "--") {
			return "", fmt.Errorf("%s requires a value", args[i])
		}
		return args[i+1], nil
	}
	allowed := map[string][]string{
		"show":    {"--project", "--cwd", "--json"},
		"edit":    {"--project", "--cwd", "--json", "--body-file", "--base-version"},
		"append":  {"--project", "--cwd", "--json", "--body-file", "--date"},
		"history": {"--project", "--cwd", "--json", "--limit", "--before"},
	}[command]

	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--help" || arg == "-h" {
			return opts, true, nil
		}
		if !strings.HasPrefix(arg, "--") {
			if opts.projectID != "" {
				return opts, false, fmt.Errorf("unexpected argument %q\n%s", arg, notesUsage())
			}
			if !isPositiveInteger(arg) {
				return opts, false, fmt.Errorf("project id must be a number, got %q", arg)
			}
			opts.projectID = arg
			continue
		}
		if !containsString(allowed, arg) {
			return opts, false, fmt.Errorf("unknown projects notes %s option %q", command, arg)
		}
		if arg == "--json" {
			opts.json = true
			continue
		}
		v, err := value(i)
		if err != nil {
			return opts, false, err
		}
		i++
		switch arg {
		case "--project":
			opts.projectFlag = v
		case "--cwd":
			opts.cwd = v
		case "--body-file":
			opts.bodyFile = v
		case "--base-version":
			if !isPositiveInteger(v) {
				return opts, false, fmt.Errorf("--base-version must be a version id, got %q", v)
			}
			opts.baseVersion = v
		case "--date":
			if _, err := time.Parse("2006-01-02", v); err != nil {
				return opts, false, fmt.Errorf("--date must be YYYY-MM-DD, got %q", v)
			}
			opts.date = v
		case "--limit":
			if !isPositiveInteger(v) {
				return opts, false, fmt.Errorf("--limit must be a positive number, got %q", v)
			}
			opts.limit = v
		case "--before":
			if !isPositiveInteger(v) {
				return opts, false, fmt.Errorf("--before must be a version id, got %q", v)
			}
			opts.before = v
		}
	}

	if (command == "edit" || command == "append") && strings.TrimSpace(opts.bodyFile) == "" {
		return opts, false, fmt.Errorf("--body-file is required\n%s", notesUsage())
	}
	if opts.projectID != "" && opts.projectFlag != "" && opts.projectID != opts.projectFlag {
		return opts, false, fmt.Errorf("project id %s and --project %s differ; give one", opts.projectID, opts.projectFlag)
	}
	return opts, false, nil
}

func containsString(list []string, value string) bool {
	for _, item := range list {
		if item == value {
			return true
		}
	}
	return false
}

func notesProjectID(opts notesOptions) (string, error) {
	if opts.projectID != "" {
		return opts.projectID, nil
	}
	project, ok, err := resolveProject(opts.projectFlag, opts.cwd)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", withExitCode(exitProjectUnresolved, fmt.Errorf("no project here: give a project id or --project ID, add %s (missionbase-agent project link ID), or set a machine default with `missionbase-agent project default ID`", projectFileName))
	}
	return project.ID, nil
}

func notesShow(base string, opts notesOptions) error {
	body, err := apiGetBody(base)
	if err != nil {
		return err
	}
	if opts.json {
		fmt.Println(string(body))
		return nil
	}
	var response struct {
		Notes notesPayload `json:"notes"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return err
	}
	if response.Notes.Version != nil {
		fmt.Printf("<!-- version %d: %s -->\n", response.Notes.Version.ID, describeNotesVersion(*response.Notes.Version))
	}
	if strings.TrimSpace(response.Notes.Body) == "" {
		fmt.Println("(no notes yet)")
		return nil
	}
	fmt.Println(response.Notes.Body)
	return nil
}

func notesEdit(base string, opts notesOptions) error {
	text, err := readBodyFile(opts.bodyFile)
	if err != nil {
		return err
	}
	payload := map[string]any{"body": text}
	if opts.baseVersion != "" {
		id, _ := strconv.ParseInt(opts.baseVersion, 10, 64)
		payload["base_version_id"] = id
	}
	request, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	body, err := apiPatchBody(base, request)
	if err != nil {
		return err
	}
	return printNotesChange(body, "notes", opts.json)
}

func notesAppend(base string, opts notesOptions) error {
	text, err := readBodyFile(opts.bodyFile)
	if err != nil {
		return err
	}
	if strings.TrimSpace(text) == "" {
		return fmt.Errorf("the body file is empty")
	}
	date := opts.date
	if date == "" {
		date = time.Now().Format("2006-01-02")
	}
	request, err := json.Marshal(map[string]string{"body": normalizeAgentAuthoredBody(text), "date": date})
	if err != nil {
		return err
	}
	body, err := apiPostBody(base+"/entries", request)
	if err != nil {
		return err
	}
	return printNotesChange(body, "entry", opts.json)
}

// printNotesChange prints "Saved version 12: appended to Log (+1 -0)".
func printNotesChange(body []byte, key string, jsonOutput bool) error {
	if jsonOutput {
		fmt.Println(string(body))
		return nil
	}
	var response struct {
		Notes notesPayload  `json:"notes"`
		Entry *notesVersion `json:"entry"`
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
	fmt.Printf("Saved version %d: %s (+%d -%d)\n", version.ID, version.Summary, version.LinesAdded, version.LinesRemoved)
	return nil
}

func notesHistory(base string, opts notesOptions) error {
	values := url.Values{}
	if opts.limit != "" {
		values.Set("limit", opts.limit)
	}
	if opts.before != "" {
		values.Set("before", opts.before)
	}
	body, err := apiGetBody(withQuery(base+"/versions", values))
	if err != nil {
		return err
	}
	if opts.json {
		fmt.Println(string(body))
		return nil
	}
	var response struct {
		Versions   []notesVersion `json:"versions"`
		NextBefore *int64         `json:"next_before"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return err
	}
	if len(response.Versions) == 0 {
		fmt.Println("No versions yet.")
		return nil
	}
	for _, version := range response.Versions {
		fmt.Printf("%d  %s  %s  +%d -%d\n", version.ID, formatNotesTime(version.CreatedAt), describeNotesVersion(version), version.LinesAdded, version.LinesRemoved)
	}
	if response.NextBefore != nil {
		fmt.Printf("Older versions: --before %d\n", *response.NextBefore)
	}
	return nil
}

// describeNotesVersion: "Claude appended to Log", "imported the project description".
func describeNotesVersion(version notesVersion) string {
	if version.Author == nil || version.Author.Name == "" {
		return version.Summary
	}
	return version.Author.Name + " " + version.Summary
}

func formatNotesTime(value string) string {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return parsed.Local().Format("2006-01-02 15:04")
}
