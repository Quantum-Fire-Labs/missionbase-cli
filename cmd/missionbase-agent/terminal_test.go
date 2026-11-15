package main

import (
	"net/http"
	"strings"
	"testing"
)

const (
	testTerminalJSON     = `{"agent":{"id":16,"slug":"missionbase-dev"},"terminal":{"address":"omarchy-m3/t2c","machine":"omarchy-m3","id":"t2c","registered_at":"2026-11-15T05:00:00Z"}}`
	testNoTerminalJSON   = `{"agent":{"id":16,"slug":"missionbase-dev"},"terminal":null}`
	testTerminalErrorFmt = `{"error":{"code":"validation_error","message":"Terminal could not be registered","details":{"terminal":["Terminal must be a terminal address like t7 or machine/t7"]}}}`
)

func TestTerminalRegisterDefaultsToHCTerminalOnThisMachine(t *testing.T) {
	calls := stubHC(t, map[string]string{"server status --json": testLocalCoreJSON})
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"PUT /api/v1/agent/terminal": fixed(200, testTerminalJSON),
	})
	setAgentEnv(t, server.URL)
	t.Setenv("HC_TERMINAL_ID", "t2c")

	out, err := runCapture(t, "terminal", "register")
	if err != nil {
		t.Fatal(err)
	}
	if len(*requests) != 1 {
		t.Fatalf("requests = %#v", *requests)
	}
	req := (*requests)[0]
	if req.slug != "missionbase-dev" {
		t.Fatalf("slug = %q", req.slug)
	}
	if len(req.payload) != 1 || req.payload["terminal"] != "omarchy-m3/t2c" {
		t.Fatalf("payload = %#v", req.payload)
	}
	if strings.Join(*calls, ";") != "server status --json" {
		t.Fatalf("hc calls = %v", *calls)
	}
	if out != "Registered omarchy-m3/t2c as missionbase-dev's terminal\n" {
		t.Fatalf("output = %q", out)
	}
}

func TestTerminalRegisterQualifiesABareTerminalWithThisMachine(t *testing.T) {
	stubHC(t, map[string]string{"server status --json": testLocalCoreJSON})
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"PUT /api/v1/agent/terminal": fixed(200, testTerminalJSON),
	})
	setAgentEnv(t, server.URL)
	t.Setenv("HC_TERMINAL_ID", "t6b")

	if _, err := runCapture(t, "terminal", "register", "--terminal", "t9"); err != nil {
		t.Fatal(err)
	}
	if got := (*requests)[0].payload["terminal"]; got != "omarchy-m3/t9" {
		t.Fatalf("terminal = %v", got)
	}
}

func TestTerminalRegisterResolvesAnotherMachineByHost(t *testing.T) {
	stubHC(t, map[string]string{"server status --json": testLocalCoreJSON, "machine ls --json": testMachineLsJSON})
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"PUT /api/v1/agent/terminal": fixed(200, testTerminalJSON),
	})
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "terminal", "register", "--terminal", "omarchy-mbp-intel/t7", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if got := (*requests)[0].payload["terminal"]; got != "omarchy-intel/t7" {
		t.Fatalf("terminal = %v, want hc's machine name", got)
	}
	if strings.TrimSpace(out) != testTerminalJSON {
		t.Fatalf("--json output = %q", out)
	}
}

func TestTerminalRegisterWithoutHCTerminalExits4(t *testing.T) {
	stubHC(t, map[string]string{})
	server, requests := recordServer(t, nil)
	setAgentEnv(t, server.URL)
	t.Setenv("HC_TERMINAL_ID", "")

	_, err := runCapture(t, "terminal", "register")
	if err == nil || !strings.Contains(err.Error(), "HC_TERMINAL_ID is not set") {
		t.Fatalf("err = %v", err)
	}
	if code := exitCodeFor(err); code != exitHCUnavailable {
		t.Fatalf("exit code = %d, want %d", code, exitHCUnavailable)
	}
	if len(*requests) != 0 {
		t.Fatalf("sent %#v", *requests)
	}
}

func TestTerminalRegisterUnknownMachineExits4(t *testing.T) {
	stubHC(t, map[string]string{"server status --json": testLocalCoreJSON, "machine ls --json": testMachineLsJSON})
	server, requests := recordServer(t, nil)
	setAgentEnv(t, server.URL)

	_, err := runCapture(t, "terminal", "register", "--terminal", "nowhere/t7")
	if err == nil || !strings.Contains(err.Error(), `machine "nowhere" is not known to hc`) {
		t.Fatalf("err = %v", err)
	}
	if code := exitCodeFor(err); code != exitHCUnavailable {
		t.Fatalf("exit code = %d", code)
	}
	if len(*requests) != 0 {
		t.Fatalf("sent %#v", *requests)
	}
}

func TestTerminalRegisterRejectsBadArguments(t *testing.T) {
	stubHC(t, map[string]string{})
	server, requests := recordServer(t, nil)
	setAgentEnv(t, server.URL)

	for _, args := range [][]string{
		{"terminal", "register", "--terminal"},
		{"terminal", "register", "--terminal", " "},
		{"terminal", "register", "--terminal", "/t7"},
		{"terminal", "register", "--terminal", "m3/t 7"},
		{"terminal", "register", "--bogus"},
		{"terminal", "register", "t7"},
		{"terminal", "show", "--bogus"},
		{"terminal", "clear", "extra"},
		{"terminal", "nope"},
	} {
		_, err := runCapture(t, args...)
		if err == nil {
			t.Fatalf("%v: expected an error", args)
		}
		if code := exitCodeFor(err); code != 1 {
			t.Fatalf("%v: exit code = %d, want 1", args, code)
		}
	}
	if len(*requests) != 0 {
		t.Fatalf("sent %#v", *requests)
	}
}

func TestTerminalRegisterNeedsAnAgentSlug(t *testing.T) {
	stubHC(t, map[string]string{"server status --json": testLocalCoreJSON})
	server, requests := recordServer(t, nil)
	setAgentEnvNoSlug(t, server.URL)
	t.Setenv("HC_TERMINAL_ID", "t2c")

	_, err := runCapture(t, "terminal", "register")
	if err == nil || !strings.Contains(err.Error(), "agent slug is not set") {
		t.Fatalf("err = %v", err)
	}
	if len(*requests) != 0 {
		t.Fatalf("sent %#v", *requests)
	}
}

func TestTerminalRegisterShowsTheServersValidationError(t *testing.T) {
	stubHC(t, map[string]string{"server status --json": testLocalCoreJSON})
	server, _ := recordServer(t, map[string]func(*http.Request) (int, string){
		"PUT /api/v1/agent/terminal": fixed(422, testTerminalErrorFmt),
	})
	setAgentEnv(t, server.URL)
	t.Setenv("HC_TERMINAL_ID", "t2c")

	_, err := runCapture(t, "terminal", "register")
	if err == nil || !strings.Contains(err.Error(), "Terminal could not be registered") {
		t.Fatalf("err = %v", err)
	}
	if code := exitCodeFor(err); code != 1 {
		t.Fatalf("exit code = %d", code)
	}
}

func TestTerminalShow(t *testing.T) {
	stubHC(t, map[string]string{})
	body := testTerminalJSON
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/agent/terminal": func(*http.Request) (int, string) { return 200, body },
	})
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "terminal", "show")
	if err != nil {
		t.Fatal(err)
	}
	if out != "omarchy-m3/t2c (registered 2026-11-15T05:00:00Z)\n" {
		t.Fatalf("output = %q", out)
	}

	body = testNoTerminalJSON
	out, err = runCapture(t, "terminal", "show")
	if err != nil {
		t.Fatal(err)
	}
	if out != "No terminal registered for missionbase-dev\n" {
		t.Fatalf("output = %q", out)
	}

	out, err = runCapture(t, "terminal", "show", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != testNoTerminalJSON {
		t.Fatalf("--json output = %q", out)
	}
	for _, req := range *requests {
		if req.slug != "missionbase-dev" || req.payload != nil {
			t.Fatalf("request = %#v", req)
		}
	}
}

func TestTerminalClear(t *testing.T) {
	stubHC(t, map[string]string{})
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"DELETE /api/v1/agent/terminal": fixed(200, testNoTerminalJSON),
	})
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "terminal", "clear")
	if err != nil {
		t.Fatal(err)
	}
	if out != "Cleared missionbase-dev's terminal\n" {
		t.Fatalf("output = %q", out)
	}
	out, err = runCapture(t, "terminal", "clear", "--json")
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out) != testNoTerminalJSON {
		t.Fatalf("--json output = %q", out)
	}
	if len(*requests) != 2 || (*requests)[0].slug != "missionbase-dev" {
		t.Fatalf("requests = %#v", *requests)
	}
}

func TestTerminalHelp(t *testing.T) {
	for _, args := range [][]string{{"terminal"}, {"terminal", "--help"}, {"terminal", "register", "--help"}} {
		out, err := runCapture(t, args...)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(out, "register [--terminal [machine/]tN] [--json]") {
			t.Fatalf("%v: output = %q", args, out)
		}
	}
}
