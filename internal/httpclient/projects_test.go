package httpclient

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/config"
)

// A missing route is an error: the client never retries a projects path
// under its old /api/v1/boxes name.
func TestProjectPathsAreNotRetriedAsBoxes(t *testing.T) {
	var paths []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte(`{"status":404,"error":"Not Found"}`))
	}))
	defer server.Close()
	client := NewAgent(config.Config{BaseURL: server.URL, Token: "t"})

	_, err := client.Get("/api/v1/projects/5/tasks")
	var httpErr *HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusNotFound || httpErr.Path != "/api/v1/projects/5/tasks" {
		t.Fatalf("err = %v", err)
	}
	if len(paths) != 1 || paths[0] != "/api/v1/projects/5/tasks" {
		t.Fatalf("paths = %v, want only the projects path", paths)
	}
}
