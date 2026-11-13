package main

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

// answeredWithImages is an answered question with text and the given attachments.
func answeredWithImages(id, position int, answer string, attachments ...string) string {
	body := "null"
	if answer != "" {
		body = strconv.Quote(answer)
	}
	return `{"id":` + strconv.Itoa(id) + `,"task_id":42,"batch":"b1","position":` + strconv.Itoa(position) + `,"body":"Question ` + strconv.Itoa(position) + `?","context":null,"options":[],"status":"answered","asked_by":` + testAskedBy + `,"asked_at":"2026-11-13T05:12:40Z","answer_body":` + body + `,"answered_by":` + testOwner + `,"answered_at":"2026-11-13T06:00:00Z","dismissed_by":null,"dismissed_at":null,"answer_attachments":[` + strings.Join(attachments, ",") + `]}`
}

func attachmentJSON(filename, link string, size int) string {
	return `{"filename":"` + filename + `","content_type":"image/png","byte_size":` + strconv.Itoa(size) + `,"url":"` + link + `"}`
}

type imageServer struct {
	images   map[string]string // path -> bytes
	requests []*http.Request
}

func (s *imageServer) handler(path string) func(*http.Request) (int, string) {
	return func(r *http.Request) (int, string) {
		s.requests = append(s.requests, r)
		body, ok := s.images[path]
		if !ok {
			return 404, "not found"
		}
		return 200, body
	}
}

func setCacheHome(t *testing.T) string {
	t.Helper()
	cache := t.TempDir()
	t.Setenv("XDG_CACHE_HOME", cache)
	return filepath.Join(cache, "missionbase", "answers", "42")
}

func TestTaskQuestionsDownloadsAnswerImagesAndPointsAtThem(t *testing.T) {
	dir := setCacheHome(t)
	images := &imageServer{images: map[string]string{"/blobs/a/layout.png": "PNG-A", "/blobs/b/bug shot.png": "PNG-BB", "/blobs/c/only.png": "PNG-C"}}
	routes := map[string]func(*http.Request) (int, string){}
	for path := range images.images {
		routes["GET "+path] = images.handler(path)
	}
	server, _ := recordServer(t, routes)
	routes["GET /api/v1/tasks/42/questions"] = fixed(200, questionsList(0,
		answeredWithImages(2, 1, "This one", attachmentJSON("layout.png", server.URL+"/blobs/a/layout.png", 5), attachmentJSON("bug shot.png", "/blobs/b/bug%20shot.png", 6)),
		answeredWithImages(3, 2, "", attachmentJSON("only.png", server.URL+"/blobs/c/only.png", 5)),
	))
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "task", "questions", "42")
	if err != nil {
		t.Fatal(err)
	}
	first, second, only := filepath.Join(dir, "q2-1-layout.png"), filepath.Join(dir, "q2-2-bug-shot.png"), filepath.Join(dir, "q3-1-only.png")
	assertContainsAll(t, out,
		"     Answer (John Doe): This one\n     This answer includes 2 images. Look at them before acting: "+first+", "+second+"\n",
		"     Answer (John Doe): [1 image]\n     This answer includes 1 image. Look at it before acting: "+only+"\n",
	)
	for path, want := range map[string]string{first: "PNG-A", second: "PNG-BB", only: "PNG-C"} {
		if data, err := os.ReadFile(path); err != nil || string(data) != want {
			t.Fatalf("%s = %q, %v; want %q", path, data, err, want)
		}
	}
	for _, r := range images.requests {
		if r.Header.Get("Authorization") != "" || r.Header.Get("X-Missionbase-Agent-Slug") != "" {
			t.Fatalf("image download sent credentials: %v", r.Header)
		}
	}

	// Saved images are reused.
	before := len(images.requests)
	if _, err := runCapture(t, "task", "questions", "42"); err != nil {
		t.Fatal(err)
	}
	if len(images.requests) != before {
		t.Fatalf("downloaded again: %d requests, want %d", len(images.requests), before)
	}
}

func TestTaskQuestionsJSONAddsLocalPathsAndKeepsURLs(t *testing.T) {
	dir := setCacheHome(t)
	images := &imageServer{images: map[string]string{"/blobs/a/layout.png": "PNG-A"}}
	routes := map[string]func(*http.Request) (int, string){"GET /blobs/a/layout.png": images.handler("/blobs/a/layout.png")}
	server, _ := recordServer(t, routes)
	link := server.URL + "/blobs/a/layout.png"
	routes["GET /api/v1/tasks/42/questions"] = fixed(200, questionsList(0, answeredWithImages(2, 1, "", attachmentJSON("layout.png", link, 5)), testQuestionQ2Done))
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "task", "questions", "42", "--json")
	if err != nil {
		t.Fatal(err)
	}
	var response struct {
		Questions []struct {
			AnswerAttachments []map[string]any `json:"answer_attachments"`
		} `json:"questions"`
	}
	if err := json.Unmarshal([]byte(out), &response); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, out)
	}
	attachment := response.Questions[0].AnswerAttachments[0]
	if attachment["url"] != link || attachment["local_path"] != filepath.Join(dir, "q2-1-layout.png") || attachment["filename"] != "layout.png" {
		t.Fatalf("attachment = %#v", attachment)
	}
	if response.Questions[1].AnswerAttachments != nil {
		t.Fatalf("question without images = %#v", response.Questions[1])
	}
}

func TestTaskQuestionsJSONWithoutImagesIsRaw(t *testing.T) {
	setCacheHome(t)
	body := questionsList(0, testQuestionQ1Done, testQuestionQ2Done)
	server, _ := recordServer(t, map[string]func(*http.Request) (int, string){"GET /api/v1/tasks/42/questions": fixed(200, body)})
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "task", "questions", "42", "--json")
	if err != nil || strings.TrimSpace(out) != body {
		t.Fatalf("out = %q, err = %v; want the raw response", out, err)
	}
}

func TestTaskQuestionsImageDownloadFailureShowsURLAndSucceeds(t *testing.T) {
	setCacheHome(t)
	images := &imageServer{images: map[string]string{}}
	routes := map[string]func(*http.Request) (int, string){"GET /blobs/gone.png": images.handler("/blobs/gone.png")}
	server, _ := recordServer(t, routes)
	link := server.URL + "/blobs/gone.png"
	routes["GET /api/v1/tasks/42/questions"] = fixed(200, questionsList(0, answeredWithImages(2, 1, "See image", attachmentJSON("gone.png", link, 5))))
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "task", "questions", "42")
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, out, "     Couldn't save image gone.png (download failed: HTTP 404); open it at "+link+"\n")
	if strings.Contains(out, "Look at") {
		t.Fatalf("output points at a file that wasn't saved:\n%s", out)
	}
}

func TestTaskQuestionsWaitDownloadsTheAnswerImages(t *testing.T) {
	shortPolls(t)
	dir := setCacheHome(t)
	images := &imageServer{images: map[string]string{"/blobs/a/layout.png": "PNG-A"}}
	routes := map[string]func(*http.Request) (int, string){"GET /blobs/a/layout.png": images.handler("/blobs/a/layout.png")}
	server, _ := recordServer(t, routes)
	polls := 0
	routes["GET /api/v1/tasks/42/questions"] = func(*http.Request) (int, string) {
		polls++
		if polls == 1 {
			return 200, questionsList(1, testQuestionQ1Open)
		}
		return 200, questionsList(0, answeredWithImages(2, 1, "", attachmentJSON("layout.png", server.URL+"/blobs/a/layout.png", 5)))
	}
	setAgentEnv(t, server.URL)

	out, err := runCapture(t, "task", "questions", "42", "--wait", "--timeout", "10s")
	if err != nil {
		t.Fatal(err)
	}
	assertContainsAll(t, out,
		"  1. [answered] Question 1?\n     Answer (John Doe): [1 image]\n",
		"This answer includes 1 image. Look at it before acting: "+filepath.Join(dir, "q2-1-layout.png")+"\n",
	)
}

func TestAnswerImageFilenameStaysInTheFolder(t *testing.T) {
	for filename, want := range map[string]string{
		"screenshot.png":     "q7-1-screenshot.png",
		"../../etc/passwd":   "q7-1-passwd",
		"My Screen (2).heic": "q7-1-My-Screen-2-.heic",
		"":                   "q7-1-image",
		"..":                 "q7-1-image",
	} {
		if got := answerImageFilename(7, 1, filename); got != want {
			t.Fatalf("answerImageFilename(%q) = %q, want %q", filename, got, want)
		}
	}
}
