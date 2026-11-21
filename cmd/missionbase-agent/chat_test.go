package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func testChatJSON(fileServer string) string {
	return `{"chat":{"id":31,"project":{"id":5,"name":"Missionbase"},"listening":true,
	"watch":{"agent":{"id":12,"name":"Claude","slug":"claude"},"terminal":"t51","machine":{"id":"core-local","name":"omarchy-m3"},"label":"Claude · t51","listening":true,"since":"2026-11-19T00:00:00Z"}},
	"messages":[
	 {"id":101,"body":"Why did the deploy fail?\nIt was fine yesterday.","sender":{"type":"User","id":1,"name":"Daniel Lemky","handle":"daniel"},"files":[
	   {"filename":"error log.png","content_type":"image/png","byte_size":4,"image":true,"url":"` + fileServer + `/blob/1"},
	   {"filename":"deploy.log.zip","content_type":"application/zip","byte_size":4,"image":false,"url":"` + fileServer + `/blob/2"},
	   {"filename":"gone.pdf","content_type":"application/pdf","byte_size":4,"image":false,"url":"` + fileServer + `/missing"}],"created_at":"2026-11-19T22:10:00Z","delivered_at":"2026-11-19T22:10:05Z","awaiting_delivery":false},
	 {"id":102,"body":"Looking now.","sender":{"type":"Agent","id":12,"name":"Claude","handle":"claude"},"files":[],"created_at":"2026-11-19T22:11:00Z","delivered_at":null,"awaiting_delivery":false},
	 {"id":103,"body":"Thanks","sender":{"type":"User","id":1,"name":"Daniel Lemky","handle":"daniel"},"files":[],"created_at":"2026-11-19T22:12:00Z","delivered_at":null,"awaiting_delivery":true}
	],"has_more":false}`
}

// testFileServer serves /blob/1 and /blob/2 (signed URLs: no API key may be
// sent) and 404s anything else.
func testFileServer(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Errorf("the signed file URL was sent the API key")
		}
		switch r.URL.Path {
		case "/blob/1":
			_, _ = w.Write([]byte("PNG!"))
		case "/blob/2":
			_, _ = w.Write([]byte("ZIP!"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return server
}

func TestProjectChatShowPrintsTheChatAndSavesEveryFile(t *testing.T) {
	files := testFileServer(t)
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/projects/5/chat": fixed(200, testChatJSON(files.URL)),
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
	dir := filepath.Join(cache, "missionbase", "chats", "5")
	image, zip := filepath.Join(dir, "m101-1-error-log.png"), filepath.Join(dir, "m101-2-deploy.log.zip")
	for _, want := range []string{
		"Missionbase chat · Listening: Claude · t51 on omarchy-m3",
		"Daniel Lemky, message 101:\n  Why did the deploy fail?\n  It was fine yesterday.",
		"  Couldn't save gone.pdf (application/pdf, download failed: HTTP 404); open it at " + files.URL + "/missing\n",
		"  This message includes 2 files. Look at them before acting:\n" +
			"    error log.png (image/png): " + image + "\n" +
			"    deploy.log.zip (application/zip): " + zip + "\n",
		"Claude (agent), message 102:\n  Looking now.",
		"Daniel Lemky, message 103 · not yet delivered:\n  Thanks",
		"Reply with: missionbase-agent project chat reply 5 --body-file PATH",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("out is missing %q:\n%s", want, out)
		}
	}
	for path, want := range map[string]string{image: "PNG!", zip: "ZIP!"} {
		if data, err := os.ReadFile(path); err != nil || string(data) != want {
			t.Fatalf("saved %s = %q, %v", path, data, err)
		}
	}
}

func TestDMShowSavesEachMessagesFilesAndAddsTheirPaths(t *testing.T) {
	files := testFileServer(t)
	server, requests := recordServer(t, map[string]func(*http.Request) (int, string){
		"GET /api/v1/agent/direct_messages/44": fixed(200, `{"chat":{"id":44,"title":"Daniel Lemky","messages":[
		  {"id":7,"body":"The contract and the logo","files":[
		    {"filename":"../contract.pdf","content_type":"application/pdf","byte_size":4,"image":false,"url":"`+files.URL+`/blob/2"},
		    {"filename":"logo.png","content_type":"image/png","byte_size":4,"image":true,"url":"`+files.URL+`/missing"}]},
		  {"id":8,"body":"Thanks","files":[]}]}}`),
	})
	setAgentEnv(t, server.URL)
	t.Setenv("MISSIONBASE_AGENT_SLUG", "prime")
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)

	out, err := runCapture(t, "dm", "show", "44")
	if err != nil {
		t.Fatal(err)
	}
	if len(*requests) != 1 || (*requests)[0].slug != "prime" {
		t.Fatalf("requests = %#v", *requests)
	}
	var response struct {
		Chat struct {
			Messages []struct {
				ID    int64            `json:"id"`
				Body  string           `json:"body"`
				Files []map[string]any `json:"files"`
			} `json:"messages"`
		} `json:"chat"`
	}
	if err := json.Unmarshal([]byte(out), &response); err != nil {
		t.Fatalf("output isn't JSON: %v\n%s", err, out)
	}
	messages := response.Chat.Messages
	if len(messages) != 2 || messages[0].Body != "The contract and the logo" || len(messages[0].Files) != 2 {
		t.Fatalf("messages = %#v", messages)
	}
	saved := filepath.Join(cache, "missionbase", "dms", "44", "m7-1-contract.pdf")
	if got := messages[0].Files[0]["local_path"]; got != saved {
		t.Fatalf("local_path = %v, want %s", got, saved)
	}
	if data, err := os.ReadFile(saved); err != nil || string(data) != "ZIP!" {
		t.Fatalf("saved = %q, %v", data, err)
	}
	if got := messages[0].Files[1]["download_error"]; got != "download failed: HTTP 404" {
		t.Fatalf("download_error = %v", got)
	}
	if _, ok := messages[0].Files[1]["local_path"]; ok {
		t.Fatalf("a failed download has a local_path: %#v", messages[0].Files[1])
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
