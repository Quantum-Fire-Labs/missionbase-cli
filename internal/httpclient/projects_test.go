package httpclient

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/config"
)

func TestBoxPath(t *testing.T) {
	for path, want := range map[string]string{
		"/api/v1/projects":                   "/api/v1/boxes",
		"/api/v1/projects?x=1":               "/api/v1/boxes?x=1",
		"/api/v1/projects/5/tasks/next":      "/api/v1/boxes/5/tasks/next",
		"/api/v1/teams/2/projects":           "/api/v1/teams/2/boxes",
		"/api/v1/agents/publisher/projects":  "/api/v1/agents/publisher/boxes",
		"api/v1/projects/5":                  "api/v1/boxes/5",
		"/api/v1/tasks?project_ids=5":        "",
		"/api/v1/projectsx":                  "",
		"/api/v1/teams/2/projects/extra/two": "/api/v1/teams/2/boxes/extra/two",
	} {
		got, ok := BoxPath(path)
		if want == "" {
			if ok || got != path {
				t.Errorf("BoxPath(%q) = %q, %v; want unchanged", path, got, ok)
			}
			continue
		}
		if !ok || got != want {
			t.Errorf("BoxPath(%q) = %q, %v; want %q", path, got, ok, want)
		}
	}
}

func TestProjectPathsFallBackToBoxesOnAnOlderServer(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/api/v1/boxes/5/tasks" || r.URL.Path == "/api/v1/boxes" {
			_, _ = w.Write([]byte(`{"tasks":[]}`))
			return
		}
		// Rails' answer for a route it does not have.
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"status":404,"error":"Not Found"}`))
	}))
	defer server.Close()
	client := NewAgent(config.Config{BaseURL: server.URL, Token: "t"})

	if _, err := client.Get("/api/v1/projects/5/tasks"); err != nil {
		t.Fatal(err)
	}
	if _, err := client.Get("/api/v1/projects"); err != nil {
		t.Fatal(err)
	}

	want := []string{"/api/v1/projects/5/tasks", "/api/v1/boxes/5/tasks", "/api/v1/boxes"}
	if len(paths) != len(want) {
		t.Fatalf("paths = %v, want %v", paths, want)
	}
	for i := range want {
		if paths[i] != want[i] {
			t.Fatalf("paths = %v, want %v", paths, want)
		}
	}
}

func TestProjectPathsKeepAPINotFoundErrors(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"error":{"code":"not_found","message":"Project not found","details":{}}}`))
	}))
	defer server.Close()
	client := NewAgent(config.Config{BaseURL: server.URL, Token: "t"})

	_, err := client.Get("/api/v1/projects/999")
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusNotFound || httpErr.Path != "/api/v1/projects/999" {
		t.Fatalf("err = %v", err)
	}
	if len(paths) != 1 {
		t.Fatalf("paths = %v, want only the projects path", paths)
	}
}
