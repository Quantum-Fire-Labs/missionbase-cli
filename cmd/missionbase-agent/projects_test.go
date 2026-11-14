package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Boxes are now projects: the CLI takes both names and calls the projects routes.

func TestProjectsAndBoxesCommandsCallProjectRoutes(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/projects/2/tasks":         fixed(200, `{"tasks":[]}`),
		"GET /api/v1/projects/2/task_statuses": fixed(200, `{"task_statuses":[]}`),
		"GET /api/v1/projects/2/discussions":   fixed(200, `{"discussions":[]}`),
	})
	setAgentEnv(t, server.URL)

	for _, args := range [][]string{
		{"projects", "tasks", "2"},
		{"boxes", "tasks", "2"},
		{"projects", "task-statuses", "2"},
		{"projects", "discussions", "2"},
	} {
		if _, err := runCapture(t, args...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	want := []string{"/api/v1/projects/2/tasks", "/api/v1/projects/2/tasks", "/api/v1/projects/2/task_statuses", "/api/v1/projects/2/discussions"}
	if len(*requests) != len(want) {
		t.Fatalf("requests = %#v", *requests)
	}
	for i, path := range want {
		if (*requests)[i].path != path {
			t.Fatalf("request %d path = %s, want %s", i, (*requests)[i].path, path)
		}
	}
}

func TestProjectFlagsSendProjectAndBoxKeys(t *testing.T) {
	bodyFile := writeTextFile(t, "body.md", "Body")
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"POST /api/v1/tasks":                fixed(201, `{"task":{"id":1}}`),
		"POST /api/v1/projects/7/documents": fixed(201, `{"document":{"id":2}}`),
	})
	setAgentEnv(t, server.URL)

	for _, args := range [][]string{
		{"task", "create", "--title", "One", "--project", "7"},
		{"task", "create", "--title", "Two", "--box", "7"},
		{"document", "create", "--project-id", "7", "--title", "Doc", "--body-file", bodyFile},
	} {
		if _, err := runCapture(t, args...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	for _, request := range (*requests)[:2] {
		if request.payload["project_id"] != "7" || request.payload["box_id"] != "7" {
			t.Fatalf("payload = %#v, want project_id and box_id 7", request.payload)
		}
	}
	if (*requests)[2].path != "/api/v1/projects/7/documents" {
		t.Fatalf("document path = %s", (*requests)[2].path)
	}
}

func TestProjectsCommandsFallBackToBoxRoutesOnAnOlderServer(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/projects/2/tasks": fixed(404, `{"status":404,"error":"Not Found"}`),
		"GET /api/v1/boxes/2/tasks":    fixed(200, `{"tasks":[]}`),
	})
	setAgentEnv(t, server.URL)

	if _, err := runCapture(t, "projects", "tasks", "2"); err != nil {
		t.Fatal(err)
	}
	if len(*requests) != 2 || (*requests)[1].path != "/api/v1/boxes/2/tasks" {
		t.Fatalf("requests = %#v", *requests)
	}
}

func TestProjectFileReadsProjectKeysThenBoxKeys(t *testing.T) {
	setAgentEnv(t, "http://127.0.0.1:1")
	for content, want := range map[string]projectOutput{
		`{"project_id":5,"project_name":"Missionbase","box_id":5,"box_name":"Missionbase"}`: {ProjectID: "5", ProjectName: "Missionbase"},
		`{"project_id":"6","project_name":"New"}`:                                           {ProjectID: "6", ProjectName: "New"},
		`{"team_id":1,"box_id":10,"box_name":"Engage Spot"}`:                                {ProjectID: "10", ProjectName: "Engage Spot"},
	} {
		root := t.TempDir()
		writeTestProjectFile(t, root, content)

		out, err := runCapture(t, "project", "--cwd", root, "--json")
		if err != nil {
			t.Fatalf("%s: %v", content, err)
		}
		var result struct {
			Project projectOutput `json:"project"`
		}
		if err := json.Unmarshal([]byte(out), &result); err != nil {
			t.Fatal(err)
		}
		got := result.Project
		if got.ProjectID != want.ProjectID || got.ProjectName != want.ProjectName || got.BoxID != want.ProjectID || got.BoxName != want.ProjectName {
			t.Fatalf("%s: project = %+v, want project and box pairs %s %q", content, got, want.ProjectID, want.ProjectName)
		}
	}
}

func TestProjectLinkWritesProjectAndBoxKeys(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/projects/5": fixed(200, `{"project":{"id":5,"name":"Missionbase","owner":{"type":"Team","id":1,"name":"Quantum Fire Labs"}},"box":{"id":5,"name":"Missionbase"}}`),
	})
	setAgentEnv(t, server.URL)
	root := t.TempDir()
	writeTestProjectFile(t, root, `{"box_id":3,"box_name":"Old","notes":"kept"}`)

	out, err := runCapture(t, "project", "link", "5", "--cwd", root, "--force") // the file links project 3
	if err != nil {
		t.Fatal(err)
	}
	if len(*requests) != 1 || !strings.Contains(out, "project 5 Missionbase") {
		t.Fatalf("out = %q, requests = %#v", out, *requests)
	}
	data, err := os.ReadFile(filepath.Join(root, ".missionbase", "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	var file map[string]any
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"team_id": 1.0, "team_name": "Quantum Fire Labs", "project_id": 5.0, "project_name": "Missionbase", "box_id": 5.0, "box_name": "Missionbase", "notes": "kept"}
	if len(file) != len(want) {
		t.Fatalf("file = %s", data)
	}
	for key, value := range want {
		if file[key] != value {
			t.Fatalf("file[%s] = %#v, want %#v in %s", key, file[key], value, data)
		}
	}
}

func TestProjectLinkReadsAnOlderServersBoxKey(t *testing.T) {
	server, _ := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/projects/8": fixed(404, `{"status":404,"error":"Not Found"}`),
		"GET /api/v1/boxes/8":    fixed(200, `{"box":{"id":8,"name":"Personal","owner":{"type":"User","id":3,"name":"Someone"}}}`),
	})
	setAgentEnv(t, server.URL)
	root := t.TempDir()

	if _, err := runCapture(t, "project", "link", "8", "--cwd", root); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, ".missionbase", "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	var file map[string]any
	if err := json.Unmarshal(data, &file); err != nil {
		t.Fatal(err)
	}
	if file["project_id"] != 8.0 || file["box_name"] != "Personal" || file["team_id"] != nil {
		t.Fatalf("file = %s", data)
	}
}

func TestProjectLinkRequiresAnID(t *testing.T) {
	setAgentEnv(t, "http://127.0.0.1:1")
	for _, args := range [][]string{{"project", "link"}, {"project", "link", "abc"}, {"project", "link", "1", "2"}} {
		if _, err := runCapture(t, args...); err == nil || !strings.Contains(err.Error(), "project link ID [--cwd DIR] [--force] [--json]") {
			t.Fatalf("%v: err = %v, want usage", args, err)
		}
	}
}

func readLinkFile(t *testing.T, root string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, ".missionbase", "project.json"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestProjectLinkWritesTheKeysInTheSetupOrderAtTheGitRoot(t *testing.T) {
	server, _ := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/projects/5": fixed(200, testProjectFiveJSON),
	})
	setAgentEnv(t, server.URL)
	repo := t.TempDir()
	if err := os.WriteFile(filepath.Join(repo, ".git"), []byte("gitdir: /elsewhere\n"), 0o644); err != nil { // a worktree's .git file
		t.Fatal(err)
	}
	nested := filepath.Join(repo, "cmd", "tool")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := runCapture(t, "project", "link", "5", "--cwd", nested)
	if err != nil {
		t.Fatal(err)
	}
	want := `{
  "team_id": 1,
  "team_name": "Quantum Fire Labs",
  "project_id": 5,
  "project_name": "Missionbase",
  "box_id": 5,
  "box_name": "Missionbase"
}
`
	if got := readLinkFile(t, repo); got != want {
		t.Fatalf("file =\n%s\nwant\n%s", got, want)
	}
	if _, err := os.Stat(filepath.Join(nested, ".missionbase")); !os.IsNotExist(err) {
		t.Fatalf("link was written in the nested folder: %v", err)
	}
	if !strings.Contains(out, "Linked "+filepath.Join(repo, ".missionbase", "project.json")+" to project 5 Missionbase") {
		t.Fatalf("out = %q", out)
	}
}

func TestProjectLinkOutsideGitWritesInTheFolderAndPrintsJSON(t *testing.T) {
	server, _ := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/projects/5": fixed(200, testProjectFiveJSON),
	})
	setAgentEnv(t, server.URL)
	dir := t.TempDir()
	t.Chdir(dir)

	out, err := runCapture(t, "project", "link", "5", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var result struct {
		Project struct {
			projectOutput
			Source string `json:"source"`
			Path   string `json:"path"`
		} `json:"project"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatalf("out = %q: %v", out, err)
	}
	p := result.Project
	wantPath := filepath.Join(dir, ".missionbase", "project.json")
	if p.ProjectID != "5" || p.BoxID != "5" || p.ProjectName != "Missionbase" || p.Source != "project_file" || p.Path != wantPath {
		t.Fatalf("project = %+v, want project 5 at %s", p, wantPath)
	}
	if !strings.Contains(readLinkFile(t, dir), `"project_id": 5`) {
		t.Fatal("link file not written")
	}
}

func TestProjectLinkRefusesToReplaceADifferentProjectWithoutForce(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/projects/5": fixed(200, testProjectFiveJSON),
	})
	setAgentEnv(t, server.URL)
	root := t.TempDir()
	original := `{"team_id":2,"box_id":113,"box_name":"Dev database"}`
	writeTestProjectFile(t, root, original)

	_, err := runCapture(t, "project", "link", "5", "--cwd", root)
	if err == nil || !strings.Contains(err.Error(), "already links project 113 Dev database; pass --force to link project 5 instead") || exitCodeFor(err) != 1 {
		t.Fatalf("err = %v (exit %d)", err, exitCodeFor(err))
	}
	if len(*requests) != 0 || readLinkFile(t, root) != original {
		t.Fatalf("requests = %#v, file = %s; want nothing sent or changed", *requests, readLinkFile(t, root))
	}

	// Relinking the same project refreshes it without --force.
	writeTestProjectFile(t, root, `{"project_id":5,"project_name":"Old name","box_id":5}`)
	if _, err := runCapture(t, "project", "link", "5", "--cwd", root); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readLinkFile(t, root), `"project_name": "Missionbase"`) {
		t.Fatalf("file = %s", readLinkFile(t, root))
	}
}

func TestProjectLinkReplacesAnUnreadableFileOnlyWithForce(t *testing.T) {
	server, _ := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/projects/5": fixed(200, testProjectFiveJSON),
	})
	setAgentEnv(t, server.URL)
	root := t.TempDir()
	writeTestProjectFile(t, root, `{not json`)

	if _, err := runCapture(t, "project", "link", "5", "--cwd", root); err == nil || !strings.Contains(err.Error(), "is not a JSON object; pass --force to replace it") {
		t.Fatalf("err = %v", err)
	}
	if _, err := runCapture(t, "project", "link", "5", "--cwd", root, "--force"); err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(readLinkFile(t, root), "{\n  \"team_id\": 1,") {
		t.Fatalf("file = %s", readLinkFile(t, root))
	}
}

func TestProjectLinkForPersonalProjectDropsStaleTeamKeys(t *testing.T) {
	server, _ := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/projects/8": fixed(200, `{"project":{"id":8,"name":"Personal","owner":{"type":"User","id":3,"name":"Someone"}}}`),
	})
	setAgentEnv(t, server.URL)
	root := t.TempDir()
	writeTestProjectFile(t, root, `{"team_id":1,"team_name":"Quantum Fire Labs","project_id":5,"box_id":5}`)

	if _, err := runCapture(t, "project", "link", "8", "--cwd", root, "--force"); err != nil {
		t.Fatal(err)
	}
	if got := readLinkFile(t, root); strings.Contains(got, "team_") || !strings.Contains(got, `"box_id": 8`) {
		t.Fatalf("file = %s", got)
	}
}

func TestProjectLinkReportsAnUnreachableProjectWithoutWriting(t *testing.T) {
	server, _ := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/projects/9": fixed(404, `{"error":{"code":"not_found","message":"Project not found"}}`),
	})
	setAgentEnv(t, server.URL)
	root := t.TempDir()

	_, err := runCapture(t, "project", "link", "9", "--cwd", root)
	if err == nil || !strings.Contains(err.Error(), "can't link project 9: Project not found (HTTP 404 not_found)") {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, ".missionbase")); !os.IsNotExist(err) {
		t.Fatalf(".missionbase was created: %v", err)
	}
}

func TestProjectUnlinkRemovesTheLink(t *testing.T) {
	setAgentEnv(t, "http://127.0.0.1:1") // unlink sends nothing
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	writeTestProjectFile(t, repo, `{"team_id":1,"team_name":"Quantum Fire Labs","project_id":5,"project_name":"Missionbase","box_id":5,"box_name":"Missionbase"}`)
	nested := filepath.Join(repo, "app")
	if err := os.Mkdir(nested, 0o755); err != nil {
		t.Fatal(err)
	}

	out, err := runCapture(t, "project", "unlink", "--cwd", nested)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Unlinked "+filepath.Join(repo, ".missionbase", "project.json")) {
		t.Fatalf("out = %q", out)
	}
	if _, err := os.Stat(filepath.Join(repo, ".missionbase")); !os.IsNotExist(err) {
		t.Fatalf(".missionbase still exists: %v", err)
	}

	out, err = runCapture(t, "project", "unlink", "--cwd", nested, "--json")
	if err != nil {
		t.Fatal(err)
	}
	if want := `{"path":"` + filepath.Join(repo, ".missionbase", "project.json") + `","removed":false}`; strings.TrimSpace(out) != want {
		t.Fatalf("out = %q, want %q", out, want)
	}
}

func TestProjectUnlinkKeepsOtherKeys(t *testing.T) {
	setAgentEnv(t, "http://127.0.0.1:1")
	root := t.TempDir()
	writeTestProjectFile(t, root, `{"project_id":5,"box_id":5,"notes":"kept"}`)

	if _, err := runCapture(t, "project", "unlink", "--cwd", root); err != nil {
		t.Fatal(err)
	}
	if got := readLinkFile(t, root); got != "{\n  \"notes\": \"kept\"\n}\n" {
		t.Fatalf("file = %q", got)
	}
	if _, err := runCapture(t, "project", "unlink", "--cwd", root, "--bogus"); err == nil || !strings.Contains(err.Error(), `unknown project unlink option "--bogus"`) {
		t.Fatalf("err = %v", err)
	}
}

// ---- projects update ----

const testUpdatedProjectJSON = `{"box":{"id":5,"name":"Missionbase","kind":"ongoing","status":"in_progress"},"project":{"id":5,"name":"Missionbase","kind":"ongoing","status":"in_progress"}}`

func TestProjectsUpdateSendsOnlyTheFlagsGiven(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"PATCH /api/v1/projects/5": fixed(200, testUpdatedProjectJSON),
	})
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "projects", "update", "5", "--kind", "ongoing")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != testUpdatedProjectJSON {
		t.Fatalf("out = %q, want the server's JSON", out)
	}
	req := (*requests)[0]
	if req.method != "PATCH" || req.path != "/api/v1/projects/5" || req.slug != "missionbase-dev" {
		t.Fatalf("request = %#v", req)
	}
	if len(req.payload) != 1 || req.payload["kind"] != "ongoing" {
		t.Fatalf("payload = %#v, want only kind", req.payload)
	}
}

func TestProjectsUpdateSendsNameDescriptionAndStatus(t *testing.T) {
	description := writeTextFile(t, "description.md", "Ship the **API**\n")
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"PATCH /api/v1/projects/5": fixed(200, testUpdatedProjectJSON),
	})
	setAgentEnv(t, server.URL)

	if _, err := runCapture(t, "projects", "update", "5", "--name", "Missionbase", "--description-file", description, "--status", "on_hold", "--json"); err != nil {
		t.Fatal(err)
	}
	want := map[string]any{"name": "Missionbase", "description": "Ship the **API**\n", "status": "on_hold"}
	got := (*requests)[0].payload
	if len(got) != len(want) {
		t.Fatalf("payload = %#v, want %#v", got, want)
	}
	for key, value := range want {
		if got[key] != value {
			t.Fatalf("payload[%s] = %#v, want %#v", key, got[key], value)
		}
	}
}

func TestProjectsUpdateAliasesCallTheSameRoute(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"PATCH /api/v1/projects/5": fixed(200, testUpdatedProjectJSON),
	})
	setAgentEnv(t, server.URL)

	for _, args := range [][]string{
		{"projects", "edit", "5", "--name", "One"},
		{"boxes", "update", "5", "--name", "Two"},
	} {
		if _, err := runCapture(t, args...); err != nil {
			t.Fatalf("%v: %v", args, err)
		}
	}
	if len(*requests) != 2 || (*requests)[1].path != "/api/v1/projects/5" || (*requests)[1].payload["name"] != "Two" {
		t.Fatalf("requests = %#v", *requests)
	}
}

func TestProjectsUpdateRefusesBadArgumentsBeforeSending(t *testing.T) {
	setAgentEnv(t, "http://127.0.0.1:1") // nothing listens: any request would fail differently
	for _, tc := range []struct {
		args []string
		want string
	}{
		{[]string{}, "a project id is required"},
		{[]string{"--name", "X"}, "a project id is required"},
		{[]string{"5"}, "nothing to update"},
		{[]string{"5", "6", "--name", "X"}, "takes one project id"},
		{[]string{"5", "--kind", "forever"}, `--kind must be project or ongoing, not "forever"`},
		{[]string{"5", "--name"}, "--name requires a value"},
		{[]string{"5", "--name", " "}, "--name requires a value"},
		{[]string{"5", "--description", "text"}, "use --description-file PATH"},
		{[]string{"5", "--visibility", "team"}, `unknown projects update option "--visibility"`},
		{[]string{"5", "--description-file", "/nonexistent/description.md"}, "read body file"},
	} {
		_, err := runCapture(t, append([]string{"projects", "update"}, tc.args...)...)
		if err == nil || !strings.Contains(err.Error(), tc.want) || exitCodeFor(err) != 1 {
			t.Fatalf("%v: err = %v (exit %d), want %q", tc.args, err, exitCodeFor(err), tc.want)
		}
	}
}

func TestProjectsUpdateHelp(t *testing.T) {
	setAgentEnv(t, "http://127.0.0.1:1")
	out, err := runCapture(t, "projects", "update", "--help")
	if err != nil || !strings.Contains(out, "projects update <project-id> [--name NAME] [--description-file PATH] [--kind project|ongoing]") {
		t.Fatalf("out = %q, err = %v", out, err)
	}
}

func TestProjectsUpdateReportsServerErrors(t *testing.T) {
	server, _ := recordServer(t, map[string]func(*http.Request) (int, string){
		"PATCH /api/v1/projects/5": fixed(422, `{"error":{"code":"validation_error","message":"Failed to update project","details":["Status must be one of: proposed, planning, in_progress, on_hold, complete, not_doing, archived"]}}`),
		"PATCH /api/v1/projects/9": fixed(404, `{"error":{"code":"not_found","message":"Project not found","details":{}}}`),
		"PATCH /api/v1/projects/7": fixed(403, `{"error":{"code":"forbidden","message":"Insufficient permissions for this action","details":{}}}`),
	})
	setAgentEnv(t, server.URL)

	for id, want := range map[string]string{
		"5": "Status must be one of",
		"9": "Project not found",
		"7": "Insufficient permissions",
	} {
		out, err := runCapture(t, "projects", "update", id, "--status", "someday")
		if err == nil || !strings.Contains(err.Error(), want) || exitCodeFor(err) != 1 {
			t.Fatalf("%s: err = %v (exit %d), want %q", id, err, exitCodeFor(err), want)
		}
		if out != "" {
			t.Fatalf("%s: out = %q, want nothing on stdout", id, out)
		}
	}
}
