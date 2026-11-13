package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testProjectFiveJSON = `{"project":{"id":5,"name":"Missionbase","owner":{"type":"Team","id":1,"name":"Quantum Fire Labs"}}}`

func setWatchEnv(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "missionbase-herdr", "watches.json")
	t.Setenv("MISSIONBASE_HERDR_WATCHES", path)
	return path
}

func readTestWatchFile(t *testing.T, path string) watchFile {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var file watchFile
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatalf("watches file is not JSON: %s", data)
	}
	return file
}

func TestProjectWatchWritesTheProjectFromTheProjectFile(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/projects/5": fixed(200, testProjectFiveJSON),
	})
	setAgentEnv(t, server.URL)
	t.Setenv("MISSIONBASE_AGENT_SLUG", "claude")
	path := setWatchEnv(t)
	root := t.TempDir()
	writeTestProjectFile(t, root, `{"project_id":5,"project_name":"Missionbase"}`)

	out, err := runCapture(t, "project", "watch", "--terminal", "t9d", "--cwd", root)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Project 5 Missionbase is watched by t9d as claude") {
		t.Fatalf("out = %q", out)
	}
	if len(*requests) != 1 || (*requests)[0].slug != "claude" {
		t.Fatalf("requests = %#v", *requests)
	}
	file := readTestWatchFile(t, path)
	if file.Version != 1 || len(file.Watches) != 1 {
		t.Fatalf("file = %+v", file)
	}
	w := file.Watches[0]
	if w.ProjectID != 5 || w.ProjectName != "Missionbase" || w.Terminal != "t9d" || w.AgentSlug != "claude" || w.Since == "" {
		t.Fatalf("watch = %+v", w)
	}
}

func TestProjectWatchReplacesTheProjectsTerminalAndKeepsOtherProjects(t *testing.T) {
	server, _ := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/projects/5": fixed(200, testProjectFiveJSON),
		"GET /api/v1/projects/9": fixed(200, `{"project":{"id":9,"name":"Engage Spot"}}`),
	})
	setAgentEnv(t, server.URL)
	path := setWatchEnv(t)

	for _, args := range [][]string{
		{"project", "watch", "--terminal", "t1", "--project", "5"},
		{"project", "watch", "--terminal", "t2", "--project", "9"},
	} {
		if _, err := runCapture(t, args...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	out, err := runCapture(t, "project", "watch", "--terminal", "t3", "--project", "5")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Replaced t1") {
		t.Fatalf("out = %q", out)
	}
	file := readTestWatchFile(t, path)
	if len(file.Watches) != 2 {
		t.Fatalf("watches = %+v", file.Watches)
	}
	if got := file.Watches[0]; got.ProjectID != 5 || got.Terminal != "t3" || got.AgentSlug != "missionbase-dev" {
		t.Fatalf("project 5 watch = %+v", got)
	}
	if got := file.Watches[1]; got.ProjectID != 9 || got.Terminal != "t2" {
		t.Fatalf("project 9 watch = %+v", got)
	}
}

func TestProjectWatchOffRemovesOnlyThatProject(t *testing.T) {
	setAgentEnv(t, "http://127.0.0.1:1")
	path := setWatchEnv(t)
	if err := writeWatchFile(path, watchFile{Watches: []projectWatch{
		{ProjectID: 5, Terminal: "t1", AgentSlug: "claude", Since: "2026-11-13T00:00:00Z"},
		{ProjectID: 9, Terminal: "t2", AgentSlug: "claude", Since: "2026-11-13T00:00:00Z"},
	}}); err != nil {
		t.Fatal(err)
	}

	out, err := runCapture(t, "project", "watch", "--off", "--project", "5")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Stopped watching project 5 (was t1)") {
		t.Fatalf("out = %q", out)
	}
	file := readTestWatchFile(t, path)
	if len(file.Watches) != 1 || file.Watches[0].ProjectID != 9 {
		t.Fatalf("watches = %+v", file.Watches)
	}

	out, err = runCapture(t, "project", "watch", "--off", "--project", "5")
	if err != nil || !strings.Contains(out, "Project 5 isn't watched on this machine") {
		t.Fatalf("second --off: out = %q, err = %v", out, err)
	}
}

func TestProjectWatchShowsTheState(t *testing.T) {
	setAgentEnv(t, "http://127.0.0.1:1")
	path := setWatchEnv(t)

	out, err := runCapture(t, "project", "watch", "--project", "5")
	if err != nil || !strings.Contains(out, "Project 5 isn't watched on this machine") {
		t.Fatalf("out = %q, err = %v", out, err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("showing the state created the file: %v", err)
	}

	if err := writeWatchFile(path, watchFile{Watches: []projectWatch{{ProjectID: 5, ProjectName: "Missionbase", Terminal: "t9d", AgentSlug: "claude", Since: "2026-11-13T19:40:00Z"}}}); err != nil {
		t.Fatal(err)
	}
	out, err = runCapture(t, "project", "watch", "--project", "5", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Watch *projectWatch `json:"watch"`
		Path  string        `json:"path"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("out = %q", out)
	}
	if result.Watch == nil || result.Watch.Terminal != "t9d" || result.Path != path {
		t.Fatalf("result = %+v", result)
	}

	out, err = runCapture(t, "project", "watch", "--cwd", t.TempDir())
	if err != nil || !strings.Contains(out, "Project 5 Missionbase is watched by t9d as claude") {
		t.Fatalf("no project here should list every watch: out = %q, err = %v", out, err)
	}
}

func TestProjectWatchRejectsBadArguments(t *testing.T) {
	setAgentEnv(t, "http://127.0.0.1:1")
	setWatchEnv(t)
	for args, want := range map[string]string{
		"--terminal":                         "--terminal requires",
		"--terminal t1 --off --project 5":    "only one of --terminal or --off",
		"--terminal hal/t1 --project 5":      "kept on this machine",
		"--project 5 --cwd /tmp":             "only one of --cwd or --project",
		"--project abc":                      "--project must be a project id",
		"--nope":                             "unknown project watch option",
		"--terminal t1 --project 5 --json x": "unknown project watch option",
	} {
		_, err := runCapture(t, append([]string{"project", "watch"}, strings.Fields(args)...)...)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s: err = %v, want %q", args, err, want)
		}
	}

	_, err := runCapture(t, "project", "watch", "--terminal", "t1", "--cwd", t.TempDir())
	if code := exitCodeFor(err); code != exitProjectUnresolved {
		t.Fatalf("no project: err = %v, exit code %d", err, code)
	}
}

func TestProjectWatchNeedsAnAgentSlug(t *testing.T) {
	setAgentEnvNoSlug(t, "http://127.0.0.1:1")
	path := setWatchEnv(t)

	_, err := runCapture(t, "project", "watch", "--terminal", "t1", "--project", "5")
	if err == nil || !strings.Contains(err.Error(), "agent slug is not set") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("file written without a slug: %v", err)
	}
}
