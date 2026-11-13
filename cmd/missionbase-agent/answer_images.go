package main

// Images in question answers. task questions downloads an answer's images to
// ~/.cache/missionbase/answers/<task>/ (or $XDG_CACHE_HOME/missionbase/...)
// and prints their paths under the answer, because agents look at local files
// more reliably than at links.

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/config"
)

// maxAnswerImageBytes caps a download; the server allows 5 MB per image.
const maxAnswerImageBytes int64 = 10 * 1024 * 1024

// answerImageClient downloads answer images. Tests swap it.
var answerImageClient = &http.Client{Timeout: 60 * time.Second}

type answerAttachment struct {
	Filename    string `json:"filename"`
	ContentType string `json:"content_type"`
	ByteSize    int64  `json:"byte_size"`
	URL         string `json:"url"`
	// Set after download: the local file, or why it couldn't be saved.
	LocalPath     string `json:"-"`
	DownloadError string `json:"-"`
}

// answerImagesDir is where a task's answer images are saved.
func answerImagesDir(taskID string) (string, error) {
	base := os.Getenv("XDG_CACHE_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("find home folder for the answer image cache: %w", err)
		}
		base = filepath.Join(home, ".cache")
	}
	return filepath.Join(base, "missionbase", "answers", strings.TrimSpace(taskID)), nil
}

// downloadAnswerImages saves the images of every answered question in
// questions, filling in each attachment's LocalPath or DownloadError. A file
// already saved with the expected size is reused. Failures never stop the
// command: the answer then lists the image's URL instead.
func downloadAnswerImages(taskID string, questions []taskQuestion) {
	var dir string
	var dirErr error
	for qi := range questions {
		q := &questions[qi]
		if q.Status != "answered" {
			continue
		}
		for ai := range q.AnswerAttachments {
			attachment := &q.AnswerAttachments[ai]
			if dir == "" && dirErr == nil {
				if dir, dirErr = answerImagesDir(taskID); dirErr == nil {
					dirErr = os.MkdirAll(dir, 0o700)
				}
			}
			if dirErr != nil {
				attachment.DownloadError = dirErr.Error()
				continue
			}
			path := filepath.Join(dir, answerImageFilename(q.ID, ai+1, attachment.Filename))
			if err := downloadAnswerImage(attachment, path); err != nil {
				attachment.DownloadError = err.Error()
				continue
			}
			attachment.LocalPath = path
		}
	}
}

var unsafeFilenameChars = regexp.MustCompile(`[^A-Za-z0-9._-]+`)

// answerImageFilename is "q<question>-<n>-<name>", so images from different
// answers never collide and a name can't leave the folder.
func answerImageFilename(questionID int64, n int, filename string) string {
	name := strings.Trim(unsafeFilenameChars.ReplaceAllString(filepath.Base(filename), "-"), ".-")
	if name == "" {
		name = "image"
	}
	if len(name) > 80 {
		name = name[len(name)-80:]
	}
	return "q" + strconv.FormatInt(questionID, 10) + "-" + strconv.Itoa(n) + "-" + name
}

func downloadAnswerImage(attachment *answerAttachment, path string) error {
	if info, err := os.Stat(path); err == nil && info.Mode().IsRegular() && (attachment.ByteSize == 0 || info.Size() == attachment.ByteSize) {
		return nil
	}
	link, err := answerImageURL(attachment.URL)
	if err != nil {
		return err
	}
	// The URL is signed, so no API key is sent with it.
	response, err := answerImageClient.Get(link)
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode > 299 {
		return fmt.Errorf("download failed: HTTP %d", response.StatusCode)
	}

	temp, err := os.CreateTemp(filepath.Dir(path), ".download-*")
	if err != nil {
		return err
	}
	defer os.Remove(temp.Name())
	written, err := io.Copy(temp, io.LimitReader(response.Body, maxAnswerImageBytes+1))
	if closeErr := temp.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return fmt.Errorf("download failed: %w", err)
	}
	if written > maxAnswerImageBytes {
		return fmt.Errorf("download failed: larger than %d MB", maxAnswerImageBytes/1024/1024)
	}
	return os.Rename(temp.Name(), path)
}

// answerImageURL resolves a relative URL against the configured server.
func answerImageURL(link string) (string, error) {
	parsed, err := url.Parse(link)
	if err != nil || link == "" {
		return "", fmt.Errorf("invalid image URL %q", link)
	}
	if parsed.IsAbs() {
		return link, nil
	}
	cfg, err := config.LoadAgent()
	if err != nil || cfg.BaseURL == "" {
		return "", fmt.Errorf("can't resolve image URL %q without a server URL", link)
	}
	base, err := url.Parse(strings.TrimRight(cfg.BaseURL, "/") + "/")
	if err != nil {
		return "", err
	}
	return base.ResolveReference(parsed).String(), nil
}

// printAnswerImages prints the line that sends the agent to the saved images,
// and the URL of any image that couldn't be saved.
func printAnswerImages(q taskQuestion, inner string) {
	var paths []string
	var failed []answerAttachment
	for _, attachment := range q.AnswerAttachments {
		if attachment.LocalPath != "" {
			paths = append(paths, attachment.LocalPath)
		} else {
			failed = append(failed, attachment)
		}
	}
	if len(paths) > 0 {
		them := "them"
		if len(paths) == 1 {
			them = "it"
		}
		fmt.Printf("%sThis answer includes %s. Look at %s before acting: %s\n", inner, plural(len(paths), "image"), them, strings.Join(paths, ", "))
	}
	for _, attachment := range failed {
		reason := attachment.DownloadError
		if reason == "" {
			reason = "not downloaded"
		}
		fmt.Printf("%sCouldn't save image %s (%s); open it at %s\n", inner, attachment.Filename, reason, attachment.URL)
	}
}

// questionsJSONWithLocalPaths adds local_path (or download_error) to each
// downloaded answer attachment in a task questions response. The body is
// returned unchanged when no question has attachments.
func questionsJSONWithLocalPaths(body []byte, questions []taskQuestion) ([]byte, error) {
	saved := map[int64][]answerAttachment{}
	for _, q := range questions {
		if len(q.AnswerAttachments) > 0 {
			saved[q.ID] = q.AnswerAttachments
		}
	}
	if len(saved) == 0 {
		return body, nil
	}

	var document map[string]any
	if err := json.Unmarshal(body, &document); err != nil {
		return nil, fmt.Errorf("unexpected task questions response: %w", err)
	}
	items, _ := document["questions"].([]any)
	for _, item := range items {
		question, _ := item.(map[string]any)
		id, _ := question["id"].(float64)
		attachments, _ := question["answer_attachments"].([]any)
		for i, raw := range attachments {
			entry, _ := raw.(map[string]any)
			if entry == nil || i >= len(saved[int64(id)]) {
				continue
			}
			local := saved[int64(id)][i]
			if local.LocalPath != "" {
				entry["local_path"] = local.LocalPath
			} else if local.DownloadError != "" {
				entry["download_error"] = local.DownloadError
			}
		}
	}
	return json.Marshal(document)
}
