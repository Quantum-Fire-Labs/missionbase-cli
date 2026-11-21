package main

// Files in chats (Missionbase #404). A project chat or DM message can carry
// files of any type, up to 25 MB each. The readers (`project chat show`,
// `dm show`) save them under ~/.cache/missionbase/<chats|dms>/<id>/, because
// agents look at local files more reliably than at links, and say where.

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// maxChatFileBytes caps a download; the server allows 25 MB per file.
const maxChatFileBytes int64 = 30 * 1024 * 1024

// chatFileGroup is one message's files, saved as "m<message>-<n>-<name>".
type chatFileGroup struct {
	MessageID int64
	Files     []answerAttachment
}

// downloadChatFiles saves each message's files to ~/.cache/missionbase/chats/<project>/.
// Failures leave the URL to print.
func downloadChatFiles(projectID string, messages []chatMessage) {
	groups := make([]chatFileGroup, len(messages))
	for i, m := range messages {
		groups[i] = chatFileGroup{MessageID: m.ID, Files: m.Files}
	}
	downloadFileGroups("chats", projectID, groups)
	for i := range messages {
		messages[i].Files = groups[i].Files
	}
}

// downloadFileGroups saves every group's files under the cache's <kind>/<id>
// folder, filling in each file's LocalPath or DownloadError. The folder is
// made only once there's a file to save.
func downloadFileGroups(kind, id string, groups []chatFileGroup) {
	var dir string
	var dirErr error
	for gi := range groups {
		group := &groups[gi]
		for fi := range group.Files {
			file := &group.Files[fi]
			if dir == "" && dirErr == nil {
				if dir, dirErr = imageCacheDir(kind, id); dirErr == nil {
					dirErr = os.MkdirAll(dir, 0o700)
				}
			}
			if dirErr != nil {
				file.DownloadError = dirErr.Error()
				continue
			}
			path := filepath.Join(dir, safeFilename("m", group.MessageID, fi+1, file.Filename, "file"))
			if err := downloadAttachment(file, path, maxChatFileBytes); err != nil {
				file.DownloadError = err.Error()
				continue
			}
			file.LocalPath = path
		}
	}
}

// printChatFiles lists a message's saved files, one a line with its name and
// type, and the URL of any file that couldn't be saved.
func printChatFiles(files []answerAttachment) {
	var saved []answerAttachment
	for _, file := range files {
		if file.LocalPath != "" {
			saved = append(saved, file)
			continue
		}
		reason := file.DownloadError
		if reason == "" {
			reason = "not downloaded"
		}
		fmt.Printf("  Couldn't save %s (%s, %s); open it at %s\n", file.Filename, fileType(file), reason, file.URL)
	}
	if len(saved) == 0 {
		return
	}
	them := "them"
	if len(saved) == 1 {
		them = "it"
	}
	fmt.Printf("  This message includes %s. Look at %s before acting:\n", plural(len(saved), "file"), them)
	for _, file := range saved {
		fmt.Printf("    %s (%s): %s\n", file.Filename, fileType(file), file.LocalPath)
	}
}

func fileType(file answerAttachment) string {
	if strings.TrimSpace(file.ContentType) == "" {
		return "unknown type"
	}
	return file.ContentType
}

// directMessageShow prints a DM chat's JSON, with each message's files saved
// to ~/.cache/missionbase/dms/<chat>/ and local_path (or download_error) added
// to each file.
func directMessageShow(chatID string) error {
	body, err := apiGetBody("/api/v1/agent/direct_messages/" + url.PathEscape(chatID))
	if err != nil {
		return err
	}
	body, err = dmJSONWithLocalFiles(chatID, body)
	if err != nil {
		return err
	}
	fmt.Println(string(body))
	return nil
}

func dmJSONWithLocalFiles(chatID string, body []byte) ([]byte, error) {
	var response struct {
		Chat struct {
			Messages []struct {
				ID    int64              `json:"id"`
				Files []answerAttachment `json:"files"`
			} `json:"messages"`
		} `json:"chat"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("unexpected direct message response: %w", err)
	}
	groups := make([]chatFileGroup, 0, len(response.Chat.Messages))
	for _, m := range response.Chat.Messages {
		if len(m.Files) > 0 {
			groups = append(groups, chatFileGroup{MessageID: m.ID, Files: m.Files})
		}
	}
	if len(groups) == 0 {
		return body, nil
	}
	downloadFileGroups("dms", chatID, groups)

	saved := map[int64][]answerAttachment{}
	for _, group := range groups {
		saved[group.MessageID] = group.Files
	}
	var document map[string]any
	if err := json.Unmarshal(body, &document); err != nil {
		return nil, fmt.Errorf("unexpected direct message response: %w", err)
	}
	chat, _ := document["chat"].(map[string]any)
	messages, _ := chat["messages"].([]any)
	for _, item := range messages {
		message, _ := item.(map[string]any)
		id, _ := message["id"].(float64)
		files, _ := message["files"].([]any)
		for i, raw := range files {
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
