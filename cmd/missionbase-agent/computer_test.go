package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/config"
)

func TestConnectEnrollsWithoutAuthAndSavesComputerCredential(t *testing.T) {
	credentialsPath := filepath.Join(t.TempDir(), "nested", "computer")
	t.Setenv("MISSIONBASE_COMPUTER_CREDENTIALS", credentialsPath)
	t.Setenv("MISSIONBASE_TOKEN", "must-not-be-sent")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/api/v1/computers/enrollment" {
			t.Fatalf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		if authorization := r.Header.Get("Authorization"); authorization != "" {
			t.Fatalf("authorization = %q, want empty", authorization)
		}
		var payload map[string]string
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload["token"] != "enrollment-secret" || payload["name"] != "Daniel's Mac" {
			t.Fatalf("payload = %#v", payload)
		}
		if payload["hostname"] == "" || payload["platform"] != runtime.GOOS || payload["cli_version"] != Version {
			t.Fatalf("machine payload = %#v", payload)
		}
		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{"computer":{"id":42,"name":"Daniel's Mac"},"credential":"computer-secret"}`))
	}))
	defer server.Close()

	if err := connect([]string{"enrollment-secret", "--base-url", server.URL, "--name", "Daniel's Mac"}); err != nil {
		t.Fatal(err)
	}
	computer, err := config.LoadComputer()
	if err != nil {
		t.Fatal(err)
	}
	if computer.BaseURL != server.URL || computer.ComputerID != 42 || computer.Credential != "computer-secret" {
		t.Fatalf("computer config = %#v", computer)
	}
	body, err := os.ReadFile(credentialsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(body), "enrollment-secret") {
		t.Fatal("saved credentials contain enrollment token")
	}
	info, err := os.Stat(credentialsPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("credentials mode = %o, want 600", info.Mode().Perm())
	}
}

func TestConnectDoesNotExposeEnrollmentResponseOnFailure(t *testing.T) {
	t.Setenv("MISSIONBASE_COMPUTER_CREDENTIALS", filepath.Join(t.TempDir(), "computer"))
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"credential":"must-not-leak"}`))
	}))
	defer server.Close()

	err := connect([]string{"bad-token", "--base-url", server.URL})
	if err == nil || !strings.Contains(err.Error(), "HTTP 401") {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(err.Error(), "must-not-leak") {
		t.Fatalf("error leaked response: %v", err)
	}
}

func TestMaterializeMissionbaseBridgeUsesComputerConfigDirectoryAndPrivateMode(t *testing.T) {
	directory := t.TempDir()
	t.Setenv("MISSIONBASE_COMPUTER_CREDENTIALS", filepath.Join(directory, "credentials", "computer.json"))
	path, err := materializeMissionbaseBridge()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(directory, "credentials", "missionbase-bridge.ts"); path != want {
		t.Fatalf("path = %q, want %q", path, want)
	}
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"session_start", "session_shutdown", "agent_settled", "sendUserMessage", "/api/v1/computer/sessions", "rev-parse", "--show-toplevel", "basename(repositoryRoot || ctx.cwd)", "MAX_RESPONSE_CHARACTERS = 100_000", "body.slice(0, MAX_RESPONSE_CHARACTERS)", "delete process.env.MISSIONBASE_COMPUTER_TOKEN"} {
		if !strings.Contains(string(body), expected) {
			t.Fatalf("extension missing %q", expected)
		}
	}
	if strings.Contains(string(body), "remote.origin.url") {
		t.Fatal("extension must not inspect or report the git remote URL")
	}
	captureIndex := strings.Index(string(body), `requiredEnvironment("MISSIONBASE_COMPUTER_TOKEN")`)
	deleteIndex := strings.Index(string(body), "delete process.env.MISSIONBASE_COMPUTER_TOKEN")
	if captureIndex < 0 || deleteIndex <= captureIndex {
		t.Fatal("extension must capture and then immediately remove the computer token from the process environment")
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("extension mode = %o, want 600", info.Mode().Perm())
	}
}

func TestPiRequiresConnectedComputer(t *testing.T) {
	t.Setenv("MISSIONBASE_COMPUTER_CREDENTIALS", filepath.Join(t.TempDir(), "missing"))
	err := pi([]string{"--agent", "missionbase-dev"})
	if err == nil || !strings.Contains(err.Error(), "missionbase-agent connect TOKEN") {
		t.Fatalf("err = %v, want connect instruction", err)
	}
}
