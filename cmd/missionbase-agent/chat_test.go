package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testChatJSON(imageURL string) string {
	return `{"chat":{"id":31,"project":{"id":5,"name":"Missionbase"},"listening":true,
	"watch":{"agent":{"id":12,"name":"Claude","slug":"claude"},"terminal":"t51","machine":{"id":"core-local","name":"omarchy-m3"},"label":"Claude · t51","listening":true,"since":"2026-11-19T00:00:00Z"}},
	"messages":[
	 {"id":101,"body":"Why did the deploy fail?\nIt was fine yesterday.","sender":{"type":"User","id":1,"name":"Daniel Lemky","handle":"daniel"},"images":[{"filename":"error log.png","content_type":"image/png","byte_size":4,"url":"` + imageURL + `"}],"created_at":"2026-11-19T22:10:00Z","delivered_at":"2026-11-19T22:10:05Z","awaiting_delivery":false},
	 {"id":102,"body":"Looking now.","sender":{"type":"Agent","id":12,"name":"Claude","handle":"claude"},"images":[],"created_at":"2026-11-19T22:11:00Z","delivered_at":null,"awaiting_delivery":false},
	 {"id":103,"body":"Thanks","sender":{"type":"User","id":1,"name":"Daniel Lemky","handle":"daniel"},"images":[],"created_at":"2026-11-19T22:12:00Z","delivered_at":null,"awaiting_delivery":true}
	],"has_more":false}`
}

func TestProjectChatShowPrintsTheChatAndSavesItsImages(t *testing.T) {
	images := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("the signed image URL was sent the API key")
		}
		_, _ = w.Write([]byte("PNG!"))
	}))
	t.Cleanup(images.Close)
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/projects/5/chat": fixed(200, testChatJSON(images.URL+"/blob/1")),
	})
	setAgentEnv(t, server.URL)
	t.Setenv("MISSIONBASE_AGENT_SLUG", "claude")
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)

	out, err := runCapture(t, "project", "chat", "show", "5")
	if err != nil {
		t.Fatal(err)
	}
	if len(*requests) != 1 || (*requests)[0].slug != "claude" || (*requests)[0].query != "" {
		t.Fatalf("requests = %#v", *requests)
	}
	saved := filepath.Join(cache, "missionbase", "chats", "5", "m101-1-error-log.png")
	for _, want := range []string{
		"Missionbase chat · Listening: Claude · t51 on omarchy-m3",
		"Daniel Lemky, message 101:\n  Why did the deploy fail?\n  It was fine yesterday.",
		"This message includes 1 image. Look at it before acting: " + saved,
		"Claude (agent), message 102:\n  Looking now.",
		"Daniel Lemky, message 103 · not yet delivered:\n  Thanks",
		"Reply with: missionbase-agent project chat reply 5 --body-file PATH",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("out is missing %q:\n%s", want, out)
		}
	}
	if data, err := os.ReadFile(saved); err != nil || string(data) != "PNG!" {
		t.Fatalf("saved image = %q, %v", data, err)
	}
}

func TestProjectChatShowTakesTheLinkedProjectALimitAndJSON(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/projects/5/chat": fixed(200, `{"chat":{"id":null,"project":{"id":5,"name":"Missionbase"},"listening":false,"watch":null},"messages":[],"has_more":false}`),
	})
	setAgentEnv(t, server.URL)
	root := t.TempDir()
	writeTestProjectFile(t, root, `{"project_id":5,"project_name":"Missionbase"}`)

	out, err := runCapture(t, "project", "chat", "show", "--cwd", root, "--limit", "50")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Missionbase chat · No orchestrator listening") || !strings.Contains(out, "No messages yet.") {
		t.Fatalf("out = %q", out)
	}
	if (*requests)[0].query != "limit=50" {
		t.Fatalf("query = %q", (*requests)[0].query)
	}

	out, err = runCapture(t, "project", "chat", "show", "--project", "5", "--json")
	if err != nil || !strings.HasPrefix(out, `{"chat":{"id":null`) {
		t.Fatalf("json: out = %q, err = %v", out, err)
	}
}

func TestProjectChatReplyPostsAsTheAgent(t *testing.T) {
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"POST /api/v1/projects/5/chat/messages": fixed(201, `{"chat":{"id":31,"project":{"id":5,"name":"Missionbase"}},"message":{"id":104}}`),
	})
	setAgentEnv(t, server.URL)
	t.Setenv("MISSIONBASE_AGENT_SLUG", "claude")
	bodyFile := filepath.Join(t.TempDir(), "reply.md")
	if err := os.WriteFile(bodyFile, []byte("Found it: the migration timed out.\\n\\nOpened #412.\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := runCapture(t, "project", "chat", "reply", "5", "--body-file", bodyFile)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "Replied in the Missionbase chat (message 104)") {
		t.Fatalf("out = %q", out)
	}
	request := (*requests)[0]
	if request.slug != "claude" || request.payload["body"] != "Found it: the migration timed out.\n\nOpened #412." {
		t.Fatalf("request = %#v", request)
	}
}

func TestProjectChatRejectsBadArguments(t *testing.T) {
	setAgentEnv(t, "http://127.0.0.1:1")
	empty := filepath.Join(t.TempDir(), "empty.md")
	if err := os.WriteFile(empty, []byte("  \n"), 0o600); err != nil {
		t.Fatal(err)
	}
	for args, want := range map[string]string{
		"nope":                                 "unknown project chat command",
		"show abc":                             "project id must be a number",
		"show 5 --limit 0":                     "--limit requires a number from 1 to 100",
		"show 5 --limit 101":                   "--limit requires a number from 1 to 100",
		"show 5 --project 5":                   "give the project once",
		"show 5 --nope":                        "unknown project chat option",
		"reply 5":                              "--body-file PATH is required",
		"reply 5 --body hi":                    "use --body-file PATH",
		"reply 5 --body-file " + empty:         "the reply is empty",
		"reply 5 --body-file /no/such/file.md": "read body file",
	} {
		_, err := runCapture(t, append([]string{"project", "chat"}, strings.Fields(args)...)...)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Fatalf("%s: err = %v, want %q", args, err, want)
		}
	}

	_, err := runCapture(t, "project", "chat", "show", "--cwd", t.TempDir())
	if code := exitCodeFor(err); code != exitProjectUnresolved {
		t.Fatalf("no project: err = %v, exit code %d", err, code)
	}
}

func TestProjectChatShowsAPIErrors(t *testing.T) {
	server, _ := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/projects/9/chat": fixed(404, `{"error":{"code":"not_found","message":"Project not found"}}`),
	})
	setAgentEnv(t, server.URL)

	_, err := runCapture(t, "project", "chat", "show", "9")
	if err == nil || !strings.Contains(err.Error(), "Project not found (HTTP 404 not_found)") {
		t.Fatalf("err = %v", err)
	}
}
