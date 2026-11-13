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

	out, err := runCapture(t, "project", "link", "5", "--cwd", root)
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
		if _, err := runCapture(t, args...); err == nil || !strings.Contains(err.Error(), "project link ID") {
			t.Fatalf("%v: err = %v, want usage", args, err)
		}
	}
}
