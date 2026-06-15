package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/textproto"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/config"
	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/httpclient"
	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/update"
)

var (
	Version = "dev"
	Commit  = "unknown"
	Repo    = "Quantum-Fire-Labs/missionbase-cli"
)

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "missionbase-agent: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printHelp()
		return nil
	}

	switch args[0] {
	case "help", "--help", "-h":
		printHelp()
	case "version", "--version", "-v":
		fmt.Printf("Missionbase Agent CLI %s\nCommit: %s\n", Version, Commit)
	case "update":
		return update.Run(update.Options{CurrentVersion: Version, Repo: Repo, BinaryName: "missionbase-agent"}, args[1:])
	case "auth":
		return auth(args[1:])
	case "use":
		return useAgent(args[1:])
	case "me":
		return apiGet("/api/v1/agent/me")
	case "work":
		return work(args[1:])
	case "listen":
		return listen(args[1:])
	case "dm":
		return directMessage(args[1:])
	case "agent":
		return agent(args[1:])
	case "document", "documents", "doc", "docs":
		return document(args[1:])
	case "tasks":
		return apiGet("/api/v1/agent/tasks")
	case "task":
		return task(args[1:])
	case "conversation":
		return conversation(args[1:])
	case "members":
		return members(args[1:])
	case "boxes":
		return boxes(args[1:])
	case "get":
		if len(args) < 2 {
			return fmt.Errorf("usage: missionbase-agent get /api/path")
		}
		return apiGet(args[1])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}

	return nil
}

func work(args []string) error {
	if len(args) == 0 {
		return apiGet("/api/v1/agent/work")
	}

	next := false
	for _, arg := range args {
		switch arg {
		case "--next", "--next-task":
			next = true
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent work [--next|--next-task]")
			return nil
		default:
			return fmt.Errorf("unknown work option %q", arg)
		}
	}

	if next {
		return apiGet("/api/v1/agent/work?next=true")
	}
	return apiGet("/api/v1/agent/work")
}

func auth(args []string) error {
	if len(args) == 0 {
		fmt.Println("usage: missionbase-agent auth <status|set-token>")
		return nil
	}

	switch args[0] {
	case "status":
		cfg, err := config.LoadAgent()
		if err != nil {
			return err
		}
		if cfg.Token == "" {
			fmt.Println("Not authenticated")
			return nil
		}
		fmt.Printf("Authenticated\nBase URL: %s\n", cfg.BaseURL)
		if cfg.AgentSlug != "" {
			fmt.Printf("Agent slug: %s\n", cfg.AgentSlug)
		} else {
			fmt.Println("Agent slug: not set")
		}
		fmt.Printf("Credentials: %s\n", config.CredentialsPath("missionbase-agent"))
		if path, ok := config.LocalAgentConfigPath(); ok {
			fmt.Printf("Directory config: %s\n", path)
		}
	case "set-token":
		if len(args) < 2 {
			return fmt.Errorf("usage: missionbase-agent auth set-token <team-token> [--base-url URL] [--agent slug]")
		}
		cfg, err := config.LoadAgent()
		if err != nil {
			return err
		}
		cfg.Token = args[1]
		for i := 2; i < len(args); i++ {
			switch args[i] {
			case "--base-url":
				if i+1 >= len(args) {
					return fmt.Errorf("--base-url requires a value")
				}
				cfg.BaseURL = args[i+1]
				i++
			case "--agent":
				if i+1 >= len(args) {
					return fmt.Errorf("--agent requires a value")
				}
				cfg.AgentSlug = args[i+1]
				i++
			default:
				return fmt.Errorf("unknown auth option %q", args[i])
			}
		}
		if err := config.SaveAgent(cfg); err != nil {
			return err
		}
		fmt.Printf("Saved credentials to %s\n", config.CredentialsPath("missionbase-agent"))
	default:
		return fmt.Errorf("unknown auth command %q", args[0])
	}

	return nil
}

func listen(args []string) error {
	timeout := "30"
	offset := "0"
	once := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--timeout":
			if i+1 >= len(args) {
				return fmt.Errorf("--timeout requires a value")
			}
			timeout = args[i+1]
			i++
		case "--offset":
			if i+1 >= len(args) {
				return fmt.Errorf("--offset requires a value")
			}
			offset = args[i+1]
			i++
		case "--once":
			once = true
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent listen [--timeout SECONDS] [--offset ID] [--once]")
			return nil
		default:
			return fmt.Errorf("unknown listen option %q", args[i])
		}
	}

	for {
		path := "/api/v1/agent/updates?timeout=" + url.QueryEscape(timeout) + "&offset=" + url.QueryEscape(offset)
		body, err := apiGetBody(path)
		if err != nil {
			return err
		}
		fmt.Println(string(body))

		var response struct {
			NextOffset int `json:"next_offset"`
		}
		if err := json.Unmarshal(body, &response); err != nil {
			return err
		}
		if response.NextOffset > 0 {
			offset = strconv.Itoa(response.NextOffset)
		}
		if once {
			return nil
		}
	}
}

func directMessage(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: missionbase-agent dm <list|show|send>")
	}

	switch args[0] {
	case "list":
		path := "/api/v1/agent/direct_messages"
		path, err := appendLimit(path, args[1:])
		if err != nil {
			return err
		}
		return apiGet(path)
	case "show":
		if len(args) != 2 {
			return fmt.Errorf("usage: missionbase-agent dm show <chat-id>")
		}
		return apiGet("/api/v1/agent/direct_messages/" + url.PathEscape(args[1]))
	case "send":
		return directMessageSend(args[1:])
	default:
		return fmt.Errorf("unknown dm command %q", args[0])
	}
}

func directMessageSend(args []string) error {
	payload := map[string]string{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--chat":
			if i+1 >= len(args) {
				return fmt.Errorf("--chat requires a value")
			}
			payload["chat_id"] = args[i+1]
			i++
		case "--to":
			if i+1 >= len(args) {
				return fmt.Errorf("--to requires a value")
			}
			payload["to"] = args[i+1]
			i++
		case "--body", "--message", "--body-stdin", "--message-stdin":
			return fmt.Errorf("%s is not supported; use --body-file PATH", args[i])
		case "--body-file", "--message-file":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a file path", args[i])
			}
			body, err := readBodyFile(args[i+1])
			if err != nil {
				return err
			}
			payload["body"] = body
			i++
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent dm send (--to <handle> | --chat <chat-id>) --body-file PATH")
			return nil
		default:
			return fmt.Errorf("unknown dm send option %q", args[i])
		}
	}
	if strings.TrimSpace(payload["to"]) == "" && strings.TrimSpace(payload["chat_id"]) == "" {
		return fmt.Errorf("--to or --chat is required")
	}
	payload["body"] = normalizeAgentAuthoredBody(payload["body"])
	if strings.TrimSpace(payload["body"]) == "" {
		return fmt.Errorf("--body is required")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return apiPost("/api/v1/agent/direct_messages", body)
}

func readBodyFile(path string) (string, error) {
	if path == "-" {
		return "", fmt.Errorf("stdin body input is not supported; use --body-file PATH")
	}
	body, err := os.ReadFile(path)
	if err != nil {
		return "", fmt.Errorf("read body file %q: %w", path, err)
	}
	return string(body), nil
}

func normalizeAgentAuthoredBody(body string) string {
	if !strings.Contains(body, `\n`) && !strings.Contains(body, `\r`) {
		return body
	}

	var out strings.Builder
	out.Grow(len(body))
	inSingleQuote := false
	inDoubleQuote := false
	inBacktick := false
	inFence := false
	escapedInQuote := false

	for i := 0; i < len(body); i++ {
		ch := body[i]
		inProtectedContext := inSingleQuote || inDoubleQuote || inBacktick

		if ch == '`' && !inSingleQuote && !inDoubleQuote {
			runLength := 1
			for i+runLength < len(body) && body[i+runLength] == '`' {
				runLength++
			}
			if runLength >= 3 {
				inFence = !inFence
				out.WriteString(body[i : i+runLength])
				i += runLength - 1
				escapedInQuote = false
				continue
			}
		}

		if ch == '\\' && !inProtectedContext && (i == 0 || body[i-1] != '\\') && i+1 < len(body) {
			switch body[i+1] {
			case 'n':
				out.WriteByte('\n')
				i++
				continue
			case 'r':
				out.WriteByte('\n')
				i++
				if i+1 < len(body) && body[i+1] == '\\' && i+2 < len(body) && body[i+2] == 'n' {
					i += 2
				}
				continue
			}
		}

		if ch == '`' && !inSingleQuote && !inDoubleQuote && !inFence {
			inBacktick = !inBacktick
		} else if ch == '\'' && !inDoubleQuote && !inBacktick && !inFence && !escapedInQuote {
			if inSingleQuote && isSingleQuoteClosingBoundary(body, i) {
				inSingleQuote = false
			} else if !inSingleQuote && isSingleQuoteOpeningBoundary(body, i) {
				inSingleQuote = true
			}
		} else if ch == '"' && !inSingleQuote && !inBacktick && !inFence && !escapedInQuote {
			inDoubleQuote = !inDoubleQuote
		}

		out.WriteByte(ch)

		if (inSingleQuote || inDoubleQuote) && ch == '\\' && !escapedInQuote {
			escapedInQuote = true
		} else {
			escapedInQuote = false
		}
	}

	return out.String()
}

func isSingleQuoteOpeningBoundary(s string, i int) bool {
	if i+1 >= len(s) || isASCIISpace(s[i+1]) {
		return false
	}
	return i == 0 || isASCIISpace(s[i-1]) || strings.ContainsRune("([{=:", rune(s[i-1]))
}

func isSingleQuoteClosingBoundary(s string, i int) bool {
	return i+1 == len(s) || isASCIISpace(s[i+1]) || strings.ContainsRune(")]},.;:", rune(s[i+1])) || isEscapedNewlineAt(s, i+1)
}

func isEscapedNewlineAt(s string, i int) bool {
	return i+1 < len(s) && s[i] == '\\' && (s[i+1] == 'n' || s[i+1] == 'r')
}

func isASCIISpace(ch byte) bool {
	switch ch {
	case ' ', '\t', '\n', '\r', '\f', '\v':
		return true
	default:
		return false
	}
}

func agent(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: missionbase-agent agent <create|archive|delete|boxes>")
	}

	switch args[0] {
	case "create":
		return agentCreate(args[1:])
	case "archive", "delete", "deactivate":
		return agentArchive(args[1:])
	case "boxes":
		return agentBoxes(args[1:])
	default:
		return fmt.Errorf("unknown agent command %q", args[0])
	}
}

func agentCreate(args []string) error {
	payload := map[string]string{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--name":
			if i+1 >= len(args) {
				return fmt.Errorf("--name requires a value")
			}
			payload["name"] = args[i+1]
			i++
		case "--slug":
			if i+1 >= len(args) {
				return fmt.Errorf("--slug requires a value")
			}
			payload["slug"] = args[i+1]
			i++
		case "--description":
			if i+1 >= len(args) {
				return fmt.Errorf("--description requires a value")
			}
			payload["description"] = args[i+1]
			i++
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent agent create --name NAME --slug SLUG [--description TEXT]")
			return nil
		default:
			return fmt.Errorf("unknown agent create option %q", args[i])
		}
	}

	if strings.TrimSpace(payload["name"]) == "" {
		return fmt.Errorf("--name is required")
	}
	if strings.TrimSpace(payload["slug"]) == "" {
		return fmt.Errorf("--slug is required")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return apiPostAllowNoAgent("/api/v1/agents", body)
}

func agentArchive(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: missionbase-agent agent archive <agent-id-or-slug> --yes")
	}
	agentID := args[0]
	confirmed := false
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--yes", "-y":
			confirmed = true
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent agent archive <agent-id-or-slug> --yes")
			return nil
		default:
			return fmt.Errorf("unknown agent archive option %q", args[i])
		}
	}
	if strings.TrimSpace(agentID) == "" {
		return fmt.Errorf("agent id or slug is required")
	}
	if !confirmed {
		return fmt.Errorf("--yes is required to archive an agent; archival deactivates the agent, revokes agent-owned API keys, and preserves historical attribution")
	}

	return apiDeleteAllowNoAgent("/api/v1/agents/" + url.PathEscape(agentID))
}

func agentBoxes(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: missionbase-agent agent boxes add <agent-id-or-slug> --box BOX_ID [--box BOX_ID]")
	}
	if args[0] != "add" {
		return fmt.Errorf("unknown agent boxes command %q", args[0])
	}
	if len(args) < 2 {
		return fmt.Errorf("usage: missionbase-agent agent boxes add <agent-id-or-slug> --box BOX_ID [--box BOX_ID]")
	}
	agentID := args[1]
	var boxIDs []string
	for i := 2; i < len(args); i++ {
		switch args[i] {
		case "--box", "--box-id":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", args[i])
			}
			boxIDs = append(boxIDs, args[i+1])
			i++
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent agent boxes add <agent-id-or-slug> --box BOX_ID [--box BOX_ID]")
			return nil
		default:
			return fmt.Errorf("unknown agent boxes add option %q", args[i])
		}
	}
	if strings.TrimSpace(agentID) == "" {
		return fmt.Errorf("agent id or slug is required")
	}
	if len(boxIDs) == 0 {
		return fmt.Errorf("at least one --box is required")
	}

	body, err := json.Marshal(map[string][]string{"box_ids": boxIDs})
	if err != nil {
		return err
	}
	return apiPostAllowNoAgent("/api/v1/agents/"+url.PathEscape(agentID)+"/boxes", body)
}

func conversation(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: missionbase-agent conversation <show|comment> ...")
	}

	switch args[0] {
	case "show":
		if len(args) < 2 {
			return fmt.Errorf("usage: missionbase-agent conversation show <feed-id> [--limit N]")
		}
		path := "/api/v1/conversations/" + url.PathEscape(args[1])
		path, err := appendLimit(path, args[2:])
		if err != nil {
			return err
		}
		return apiGet(path)
	case "comment", "create-comment", "reply":
		return conversationComment(args[1:])
	default:
		return fmt.Errorf("unknown conversation command %q", args[0])
	}
}

func conversationComment(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: missionbase-agent conversation comment <feed-id> --body-file PATH [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]")
	}
	feedID := args[0]
	payload := map[string]string{}
	var attaches, blobs []string

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--body", "--comment", "--message", "--text", "--body-stdin", "--comment-stdin", "--message-stdin", "--text-stdin":
			return fmt.Errorf("%s is not supported; use --body-file PATH", args[i])
		case "--body-file", "--comment-file", "--message-file", "--text-file":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a file path", args[i])
			}
			body, err := readBodyFile(args[i+1])
			if err != nil {
				return err
			}
			payload["comment"] = body
			i++
		case "--attach":
			if i+1 >= len(args) {
				return fmt.Errorf("--attach requires a file path")
			}
			attaches = append(attaches, args[i+1])
			i++
		case "--attach-blob":
			if i+1 >= len(args) {
				return fmt.Errorf("--attach-blob requires a signed_id or sgid")
			}
			blobs = append(blobs, args[i+1])
			i++
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent conversation comment <feed-id> --body-file PATH [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]")
			return nil
		default:
			return fmt.Errorf("unknown conversation comment option %q", args[i])
		}
	}

	payload["comment"] = normalizeAgentAuthoredBody(payload["comment"])
	if strings.TrimSpace(payload["comment"]) == "" && len(attaches) == 0 && len(blobs) == 0 {
		return fmt.Errorf("--body or at least one attachment is required")
	}
	path := "/api/v1/conversations/" + url.PathEscape(feedID) + "/comments"
	if len(attaches) > 0 || len(blobs) > 0 {
		return apiPostMultipart(path, payload, attaches, blobs)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return apiPost(path, body)
}

func document(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: missionbase-agent document create --box BOX_ID --title TITLE --body-file PATH\n       missionbase-agent document edit <document-id> [--title TITLE] --body-file PATH")
	}

	switch args[0] {
	case "create":
		return documentCreate(args[1:])
	case "edit", "update":
		return documentEdit(args[1:])
	case "--help", "-h":
		fmt.Println("usage: missionbase-agent document create --box BOX_ID --title TITLE --body-file PATH\n       missionbase-agent document edit <document-id> [--title TITLE] --body-file PATH")
		return nil
	default:
		return fmt.Errorf("unknown document command %q", args[0])
	}
}

func documentCreate(args []string) error {
	payload := map[string]string{}
	boxID := ""

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--box", "--box-id":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", args[i])
			}
			boxID = args[i+1]
			i++
		case "--title":
			if i+1 >= len(args) {
				return fmt.Errorf("--title requires a value")
			}
			payload["title"] = args[i+1]
			i++
		case "--body", "--content", "--message", "--text", "--body-stdin", "--content-stdin", "--message-stdin", "--text-stdin":
			return fmt.Errorf("%s is not supported; use --body-file PATH", args[i])
		case "--body-file", "--content-file", "--message-file", "--text-file":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a file path", args[i])
			}
			body, err := readBodyFile(args[i+1])
			if err != nil {
				return err
			}
			payload["body"] = body
			i++
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent document create --box BOX_ID --title TITLE --body-file PATH")
			return nil
		default:
			return fmt.Errorf("unknown document create option %q", args[i])
		}
	}

	if strings.TrimSpace(boxID) == "" {
		return fmt.Errorf("--box is required")
	}
	if strings.TrimSpace(payload["title"]) == "" {
		return fmt.Errorf("--title is required")
	}
	payload["body"] = normalizeAgentAuthoredBody(payload["body"])
	if strings.TrimSpace(payload["body"]) == "" {
		return fmt.Errorf("--body is required")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return apiPost("/api/v1/boxes/"+url.PathEscape(boxID)+"/documents", body)
}

func documentEdit(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: missionbase-agent document edit <document-id> [--title TITLE] --body-file PATH")
	}

	documentID := strings.TrimSpace(args[0])
	if documentID == "" {
		return fmt.Errorf("document id is required")
	}

	payload := map[string]string{}
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--title":
			if i+1 >= len(args) {
				return fmt.Errorf("--title requires a value")
			}
			payload["title"] = args[i+1]
			i++
		case "--body", "--content", "--message", "--text", "--body-stdin", "--content-stdin", "--message-stdin", "--text-stdin":
			return fmt.Errorf("%s is not supported; use --body-file PATH", args[i])
		case "--body-file", "--content-file", "--message-file", "--text-file":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a file path", args[i])
			}
			body, err := readBodyFile(args[i+1])
			if err != nil {
				return err
			}
			payload["body"] = body
			i++
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent document edit <document-id> [--title TITLE] --body-file PATH")
			return nil
		default:
			return fmt.Errorf("unknown document edit option %q", args[i])
		}
	}

	payload["body"] = normalizeAgentAuthoredBody(payload["body"])
	if strings.TrimSpace(payload["body"]) == "" {
		return fmt.Errorf("--body is required")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return apiPatch("/api/v1/documents/"+url.PathEscape(documentID), body)
}

func boxes(args []string) error {
	if len(args) == 0 {
		fmt.Println("usage: missionbase-agent boxes <tasks|discussions|statuses|task-statuses>")
		return nil
	}

	switch args[0] {
	case "tasks":
		return boxTasks(args[1:])
	case "discussions":
		return boxDiscussions(args[1:])
	case "statuses", "task-statuses":
		return boxTaskStatuses(args[1:])
	default:
		return fmt.Errorf("unknown boxes command %q", args[0])
	}
}

func boxDiscussions(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: missionbase-agent boxes discussions <box-id> [--page N] [--per-page N]\n       missionbase-agent boxes discussions create <box-id> --title TITLE --body-file PATH")
	}
	if args[0] == "create" {
		return boxDiscussionsCreate(args[1:])
	}

	boxID := args[0]
	values := url.Values{}
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--page":
			if i+1 >= len(args) {
				return fmt.Errorf("--page requires a value")
			}
			values.Set("page", args[i+1])
			i++
		case "--per-page":
			if i+1 >= len(args) {
				return fmt.Errorf("--per-page requires a value")
			}
			values.Set("per_page", args[i+1])
			i++
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent boxes discussions <box-id> [--page N] [--per-page N]\n       missionbase-agent boxes discussions create <box-id> --title TITLE --body-file PATH")
			return nil
		default:
			return fmt.Errorf("unknown boxes discussions option %q", args[i])
		}
	}

	path := "/api/v1/boxes/" + url.PathEscape(boxID) + "/discussions"
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return apiGet(path)
}

func boxDiscussionsCreate(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: missionbase-agent boxes discussions create <box-id> --title TITLE --body-file PATH")
	}

	boxID := strings.TrimSpace(args[0])
	if boxID == "" {
		return fmt.Errorf("box id is required")
	}

	payload := map[string]string{}
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--title":
			if i+1 >= len(args) {
				return fmt.Errorf("--title requires a value")
			}
			payload["title"] = args[i+1]
			i++
		case "--body", "--content", "--message", "--text", "--body-stdin", "--content-stdin", "--message-stdin", "--text-stdin":
			return fmt.Errorf("%s is not supported; use --body-file PATH", args[i])
		case "--body-file", "--content-file", "--message-file", "--text-file":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a file path", args[i])
			}
			body, err := readBodyFile(args[i+1])
			if err != nil {
				return err
			}
			payload["body"] = body
			i++
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent boxes discussions create <box-id> --title TITLE --body-file PATH")
			return nil
		default:
			return fmt.Errorf("unknown boxes discussions create option %q", args[i])
		}
	}

	if strings.TrimSpace(payload["title"]) == "" {
		return fmt.Errorf("--title is required")
	}
	payload["body"] = normalizeAgentAuthoredBody(payload["body"])
	if strings.TrimSpace(payload["body"]) == "" {
		return fmt.Errorf("--body is required")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return apiPost("/api/v1/boxes/"+url.PathEscape(boxID)+"/discussions", body)
}

func boxTaskStatuses(args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Println("usage: missionbase-agent boxes task-statuses <box-id>")
		return nil
	}
	if len(args) != 1 {
		return fmt.Errorf("usage: missionbase-agent boxes task-statuses <box-id>")
	}

	boxID := args[0]
	if strings.TrimSpace(boxID) == "" {
		return fmt.Errorf("box id is required")
	}
	return apiGet("/api/v1/boxes/" + url.PathEscape(boxID) + "/task_statuses")
}

func boxTasks(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: missionbase-agent boxes tasks <box-id> [--status STATUS|--status-category open|done|canceled|--task-status-ids IDS] [--page N] [--per-page N]")
	}

	boxID := args[0]
	values := url.Values{}
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--status":
			if i+1 >= len(args) {
				return fmt.Errorf("--status requires a value")
			}
			values.Set("status", args[i+1])
			i++
		case "--status-category":
			if i+1 >= len(args) {
				return fmt.Errorf("--status-category requires a value")
			}
			values.Set("status_category", args[i+1])
			i++
		case "--task-status-ids":
			if i+1 >= len(args) {
				return fmt.Errorf("--task-status-ids requires a value")
			}
			values.Set("task_status_ids", args[i+1])
			i++
		case "--page":
			if i+1 >= len(args) {
				return fmt.Errorf("--page requires a value")
			}
			values.Set("page", args[i+1])
			i++
		case "--per-page":
			if i+1 >= len(args) {
				return fmt.Errorf("--per-page requires a value")
			}
			values.Set("per_page", args[i+1])
			i++
		default:
			return fmt.Errorf("unknown boxes tasks option %q", args[i])
		}
	}

	path := "/api/v1/boxes/" + url.PathEscape(boxID) + "/tasks"
	if encoded := values.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return apiGet(path)
}

func appendLimit(path string, args []string) (string, error) {
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--limit":
			if i+1 >= len(args) {
				return "", fmt.Errorf("--limit requires a value")
			}
			path += "?limit=" + args[i+1]
			i++
		case "--help", "-h":
			return "", fmt.Errorf("usage includes optional [--limit N]")
		default:
			return "", fmt.Errorf("unknown option %q", args[i])
		}
	}
	return path, nil
}

func members(args []string) error {
	path := "/api/v1/agent/members"
	filtered := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--box", "--group":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", args[i])
			}
			path += "?box_id=" + args[i+1]
			filtered = true
			i++
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent members [--box ID]")
			return nil
		default:
			return fmt.Errorf("unknown members option %q", args[i])
		}
	}

	body, err := membersBody(path, filtered)
	if err != nil {
		return err
	}
	fmt.Println(string(body))
	return nil
}

func membersBody(path string, filtered bool) ([]byte, error) {
	body, err := apiGetBody(path)
	if err == nil {
		return body, nil
	}
	if filtered || !strings.Contains(err.Error(), "404") {
		return nil, err
	}

	// Backward-compatible fallback for Missionbase deployments that expose team
	// members before the agent-specific members endpoint is available.
	meBody, meErr := apiGetBody("/api/v1/agent/me")
	if meErr != nil {
		return nil, err
	}
	var me struct {
		Agent struct {
			Team struct {
				ID int `json:"id"`
			} `json:"team"`
		} `json:"agent"`
	}
	if jsonErr := json.Unmarshal(meBody, &me); jsonErr != nil || me.Agent.Team.ID == 0 {
		return nil, err
	}
	body, fallbackErr := apiGetBody(fmt.Sprintf("/api/v1/teams/%d/members", me.Agent.Team.ID))
	if fallbackErr != nil {
		return nil, err
	}
	return body, nil
}

func task(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: missionbase-agent task create --title TITLE --box ID (--assign-agent slug | --assign-user ID|@mention) [--description-file PATH] [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID] OR missionbase-agent task assign <task-id> (--user ID|@mention | --agent slug) OR missionbase-agent task unassign <task-id> (--user ID|@mention | --agent slug | --self) OR missionbase-agent task comment <task-id> --body-file PATH [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID] OR missionbase-agent task status <task-id> <status> OR missionbase-agent task complete <task-id> OR missionbase-agent task <feed|comments> <task-id> [--limit N] OR missionbase-agent task participants <list|add> <task-id> [--user ID|@mention | --agent slug]")
	}

	switch args[0] {
	case "create":
		return taskCreate(args[1:])
	case "comment", "create-comment", "reply":
		return taskComment(args[1:])
	case "assign":
		return taskAssign(args[1:])
	case "unassign":
		return taskUnassign(args[1:])
	case "status":
		return taskStatus(args[1:])
	case "complete":
		return taskComplete(args[1:])
	case "feed", "comments":
		if len(args) < 2 {
			return fmt.Errorf("usage: missionbase-agent task %s <task-id> [--limit N]", args[0])
		}

		path := "/api/v1/tasks/" + url.PathEscape(args[1]) + "/comments"
		path, err := appendLimit(path, args[2:])
		if err != nil {
			return err
		}
		return apiGet(path)
	case "participants":
		if len(args) < 3 {
			return fmt.Errorf("usage: missionbase-agent task participants <list|add> <task-id> [--user ID|@mention | --agent slug]")
		}
		command := args[1]
		taskID := args[2]
		switch command {
		case "list":
			if len(args) != 3 {
				return fmt.Errorf("usage: missionbase-agent task participants list <task-id>")
			}
			return apiGet("/api/v1/tasks/" + url.PathEscape(taskID) + "/participants")
		case "add":
			return taskParticipantsAdd(taskID, args[3:])
		default:
			return fmt.Errorf("unknown task participants command %q", command)
		}
	default:
		return fmt.Errorf("unknown task command %q", args[0])
	}
}

func taskAssign(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("usage: missionbase-agent task assign <task-id> --user ID|@mention OR --agent slug")
	}

	taskID := args[0]
	payload := map[string]string{}
	switch args[1] {
	case "--user":
		userID, err := resolveUserID(args[2])
		if err != nil {
			return err
		}
		payload["user_id"] = userID
	case "--agent":
		if strings.TrimSpace(args[2]) == "" {
			return fmt.Errorf("--agent requires a non-empty slug")
		}
		payload["agent_slug"] = args[2]
	case "--help", "-h":
		fmt.Println("usage: missionbase-agent task assign <task-id> --user ID|@mention OR --agent slug")
		return nil
	default:
		return fmt.Errorf("unknown task assign option %q", args[1])
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return apiPost("/api/v1/tasks/"+url.PathEscape(taskID)+"/assignments", body)
}

func taskUnassign(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: missionbase-agent task unassign <task-id> (--user ID|@mention | --agent slug | --self)")
	}

	taskID := args[0]
	switch args[1] {
	case "--user":
		if len(args) != 3 {
			return fmt.Errorf("usage: missionbase-agent task unassign <task-id> --user ID|@mention")
		}
		userID, err := resolveUserID(args[2])
		if err != nil {
			return err
		}
		return apiDelete("/api/v1/tasks/" + url.PathEscape(taskID) + "/assignments/" + url.PathEscape(userID))
	case "--agent":
		if len(args) != 3 {
			return fmt.Errorf("usage: missionbase-agent task unassign <task-id> --agent slug")
		}
		if strings.TrimSpace(args[2]) == "" {
			return fmt.Errorf("--agent requires a non-empty slug")
		}
		return taskUnassignAgent(taskID, args[2])
	case "--self":
		if len(args) != 2 {
			return fmt.Errorf("usage: missionbase-agent task unassign <task-id> --self")
		}
		cfg, err := config.LoadAgent()
		if err != nil {
			return err
		}
		if strings.TrimSpace(cfg.AgentSlug) == "" {
			return fmt.Errorf("agent slug is not set; run `missionbase-agent use <slug>` in this directory or set MISSIONBASE_AGENT_SLUG")
		}
		return taskUnassignAgent(taskID, cfg.AgentSlug)
	case "--help", "-h":
		fmt.Println("usage: missionbase-agent task unassign <task-id> (--user ID|@mention | --agent slug | --self)")
		return nil
	default:
		return fmt.Errorf("unknown task unassign option %q", args[1])
	}
}

func taskUnassignAgent(taskID, slug string) error {
	path := "/api/v1/tasks/" + url.PathEscape(taskID) + "/assignments/agent?assignee_type=Agent&agent_slug=" + url.QueryEscape(slug)
	return apiDelete(path)
}

func taskComment(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: missionbase-agent task comment <task-id> --body-file PATH [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]")
	}
	taskID := args[0]
	payload := map[string]string{}
	var attaches, blobs []string

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--body", "--comment", "--message", "--text", "--body-stdin", "--comment-stdin", "--message-stdin", "--text-stdin":
			return fmt.Errorf("%s is not supported; use --body-file PATH", args[i])
		case "--body-file", "--comment-file", "--message-file", "--text-file":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a file path", args[i])
			}
			body, err := readBodyFile(args[i+1])
			if err != nil {
				return err
			}
			payload["comment"] = body
			i++
		case "--attach":
			if i+1 >= len(args) {
				return fmt.Errorf("--attach requires a file path")
			}
			attaches = append(attaches, args[i+1])
			i++
		case "--attach-blob":
			if i+1 >= len(args) {
				return fmt.Errorf("--attach-blob requires a signed_id or sgid")
			}
			blobs = append(blobs, args[i+1])
			i++
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent task comment <task-id> --body-file PATH [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]")
			return nil
		default:
			return fmt.Errorf("unknown task comment option %q", args[i])
		}
	}

	payload["comment"] = normalizeAgentAuthoredBody(payload["comment"])
	if strings.TrimSpace(payload["comment"]) == "" && len(attaches) == 0 && len(blobs) == 0 {
		return fmt.Errorf("--body or at least one attachment is required")
	}
	if len(attaches) > 0 || len(blobs) > 0 {
		return apiPostMultipart("/api/v1/tasks/"+url.PathEscape(taskID)+"/comments", payload, attaches, blobs)
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return apiPost("/api/v1/tasks/"+url.PathEscape(taskID)+"/comments", body)
}

func taskStatus(args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: missionbase-agent task status <task-id> <status>")
	}

	taskID := args[0]
	status := args[1]
	if strings.TrimSpace(status) == "" {
		return fmt.Errorf("status is required")
	}
	if status == "complete" {
		return taskComplete([]string{taskID})
	}

	body, err := json.Marshal(map[string]string{"status": status})
	if err != nil {
		return err
	}
	return apiPatch("/api/v1/tasks/"+url.PathEscape(taskID), body)
}

func taskComplete(args []string) error {
	if len(args) != 1 {
		return fmt.Errorf("usage: missionbase-agent task complete <task-id>")
	}
	return apiPatch("/api/v1/tasks/"+url.PathEscape(args[0])+"/complete", nil)
}

func taskCreate(args []string) error {
	payload := map[string]string{}
	var participantUsers []string
	var attaches, blobs []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--title":
			if i+1 >= len(args) {
				return fmt.Errorf("--title requires a value")
			}
			payload["title"] = args[i+1]
			i++
		case "--description":
			return fmt.Errorf("--description is not supported; use --description-file PATH")
		case "--description-file":
			if i+1 >= len(args) {
				return fmt.Errorf("--description-file requires a file path")
			}
			description, err := readBodyFile(args[i+1])
			if err != nil {
				return err
			}
			payload["description"] = description
			i++
		case "--description-stdin":
			return fmt.Errorf("--description-stdin is not supported; use --description-file PATH")
		case "--box":
			if i+1 >= len(args) {
				return fmt.Errorf("--box requires a value")
			}
			payload["box_id"] = args[i+1]
			i++
		case "--assign-agent":
			if i+1 >= len(args) {
				return fmt.Errorf("--assign-agent requires a value")
			}
			payload["assign_to_agent_slug"] = args[i+1]
			i++
		case "--assign-user":
			if i+1 >= len(args) {
				return fmt.Errorf("--assign-user requires a value")
			}
			userID, err := resolveUserID(args[i+1])
			if err != nil {
				return err
			}
			payload["assign_to_user_id"] = userID
			i++
		case "--participant-user":
			if i+1 >= len(args) {
				return fmt.Errorf("--participant-user requires a value")
			}
			userID, err := resolveUserID(args[i+1])
			if err != nil {
				return err
			}
			participantUsers = append(participantUsers, userID)
			i++
		case "--attach":
			if i+1 >= len(args) {
				return fmt.Errorf("--attach requires a file path")
			}
			attaches = append(attaches, args[i+1])
			i++
		case "--attach-blob":
			if i+1 >= len(args) {
				return fmt.Errorf("--attach-blob requires a signed_id or sgid")
			}
			blobs = append(blobs, args[i+1])
			i++
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent task create --title TITLE --box ID (--assign-agent slug | --assign-user ID|@mention) [--description-file PATH] [--participant-user ID|@mention] [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]")
			return nil
		default:
			return fmt.Errorf("unknown task create option %q", args[i])
		}
	}

	if strings.TrimSpace(payload["title"]) == "" {
		return fmt.Errorf("--title is required")
	}
	if strings.TrimSpace(payload["box_id"]) == "" {
		return fmt.Errorf("--box is required")
	}
	if payload["assign_to_agent_slug"] == "" && payload["assign_to_user_id"] == "" {
		return fmt.Errorf("one of --assign-agent or --assign-user is required")
	}
	if payload["assign_to_agent_slug"] != "" && payload["assign_to_user_id"] != "" {
		return fmt.Errorf("use only one of --assign-agent or --assign-user")
	}

	var responseBody []byte
	var err error
	if len(attaches) > 0 || len(blobs) > 0 {
		responseBody, err = apiPostMultipartBody("/api/v1/tasks", payload, attaches, blobs)
	} else {
		var requestBody []byte
		requestBody, err = json.Marshal(payload)
		if err != nil {
			return err
		}
		responseBody, err = apiPostBody("/api/v1/tasks", requestBody)
	}
	if err != nil {
		return err
	}

	if len(participantUsers) > 0 {
		var response struct {
			Task struct {
				ID int `json:"id"`
			} `json:"task"`
		}
		if err := json.Unmarshal(responseBody, &response); err != nil {
			return err
		}
		if response.Task.ID == 0 {
			return fmt.Errorf("created task response did not include task.id")
		}
		for _, userID := range participantUsers {
			participantBody, err := json.Marshal(map[string]string{"user_id": userID})
			if err != nil {
				return err
			}
			if _, err := apiPostBody("/api/v1/tasks/"+strconv.Itoa(response.Task.ID)+"/participants", participantBody); err != nil {
				return err
			}
		}
	}

	fmt.Println(string(responseBody))
	return nil
}

func taskParticipantsAdd(taskID string, args []string) error {
	if len(args) != 2 {
		return fmt.Errorf("usage: missionbase-agent task participants add <task-id> --user ID|@mention OR --agent slug")
	}

	payload := map[string]string{}
	switch args[0] {
	case "--user":
		userID, err := resolveUserID(args[1])
		if err != nil {
			return err
		}
		payload["user_id"] = userID
	case "--agent":
		payload["agent_slug"] = args[1]
	case "--help", "-h":
		fmt.Println("usage: missionbase-agent task participants add <task-id> --user ID|@mention OR --agent slug")
		return nil
	default:
		return fmt.Errorf("unknown task participants add option %q", args[0])
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return apiPost("/api/v1/tasks/"+url.PathEscape(taskID)+"/participants", body)
}

func resolveUserID(value string) (string, error) {
	if _, err := strconv.Atoi(value); err == nil {
		return value, nil
	}
	mention := strings.TrimPrefix(value, "@")
	if mention == value || mention == "" {
		return "", fmt.Errorf("--user requires a numeric user id or @mention")
	}
	body, err := membersBody("/api/v1/agent/members", false)
	if err != nil {
		return "", err
	}
	var response struct {
		Members []struct {
			UserID  int    `json:"user_id"`
			ID      int    `json:"id"`
			Name    string `json:"name"`
			Email   string `json:"email"`
			Mention string `json:"mention"`
			Handle  string `json:"handle"`
		} `json:"members"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", err
	}
	normalized := normalizeMention(mention)
	var matches []string
	for _, member := range response.Members {
		id := member.UserID
		if id == 0 {
			id = member.ID
		}
		if id == 0 {
			continue
		}
		candidates := []string{member.Mention, member.Handle, member.Name, strings.Split(member.Email, "@")[0]}
		for _, candidate := range candidates {
			if normalizeMention(strings.TrimPrefix(candidate, "@")) == normalized {
				matches = append(matches, strconv.Itoa(id))
				break
			}
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no member found for %s", value)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("multiple members match %s; use a numeric user id", value)
	}
	return matches[0], nil
}

func normalizeMention(value string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func useAgent(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: missionbase-agent use <agent-slug> [--base-url URL]")
	}
	cfg := config.Config{AgentSlug: args[0]}
	for i := 1; i < len(args); i++ {
		if args[i] == "--base-url" && i+1 < len(args) {
			cfg.BaseURL = args[i+1]
			i++
		} else {
			return fmt.Errorf("unknown use option %q", args[i])
		}
	}
	path, err := config.SaveLocalAgentConfig(cfg)
	if err != nil {
		return err
	}
	fmt.Printf("Saved directory agent config to %s\n", path)
	return nil
}

func apiPost(path string, requestBody []byte) error {
	body, err := apiPostBody(path, requestBody)
	if err != nil {
		return err
	}
	fmt.Println(string(body))
	return nil
}

func apiPostAllowNoAgent(path string, requestBody []byte) error {
	body, err := apiPostBodyAllowNoAgent(path, requestBody)
	if err != nil {
		return err
	}
	fmt.Println(string(body))
	return nil
}

func apiPostMultipart(path string, fields map[string]string, attaches []string, blobs []string) error {
	body, err := apiPostMultipartBody(path, fields, attaches, blobs)
	if err != nil {
		return err
	}
	fmt.Println(string(body))
	return nil
}

func apiPostMultipartBody(path string, fields map[string]string, attaches []string, blobs []string) ([]byte, error) {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return nil, err
		}
	}
	for _, blob := range blobs {
		if strings.TrimSpace(blob) == "" {
			return nil, fmt.Errorf("--attach-blob requires a non-empty signed_id or sgid")
		}
		if err := writer.WriteField("attachment_blobs[]", blob); err != nil {
			return nil, err
		}
	}
	for _, path := range attaches {
		if err := addMultipartAttachment(writer, path); err != nil {
			return nil, err
		}
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return apiPostBodyWithContentType(path, buf.Bytes(), writer.FormDataContentType())
}

func addMultipartAttachment(writer *multipart.Writer, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open attachment %q: %w", path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat attachment %q: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("attachment %q is a directory", path)
	}
	if info.Size() > 5*1024*1024 {
		return fmt.Errorf("attachment %q is too large (max 5 MB)", path)
	}
	peek := make([]byte, 512)
	n, err := file.Read(peek)
	if err != nil && err != io.EOF {
		return fmt.Errorf("read attachment %q: %w", path, err)
	}
	contentType := detectAttachmentContentType(peek[:n])
	if !allowedAttachmentContentType(contentType) {
		return fmt.Errorf("unsupported attachment type %q for %q (allowed: PNG, JPEG, GIF, WEBP, HEIC/HEIF)", contentType, path)
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="attachments[]"; filename="%s"`, escapeQuotes(filepath.Base(path))))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(part, file)
	return err
}

func detectAttachmentContentType(peek []byte) string {
	if contentType := detectHEIFContentType(peek); contentType != "" {
		return contentType
	}
	return http.DetectContentType(peek)
}

func detectHEIFContentType(peek []byte) string {
	if len(peek) < 12 || !bytes.Equal(peek[4:8], []byte("ftyp")) {
		return ""
	}
	brand := string(peek[8:12])
	switch brand {
	case "heic", "heix", "hevc", "hevx":
		return "image/heic"
	case "mif1", "msf1":
		return "image/heif"
	default:
		return ""
	}
}

func allowedAttachmentContentType(contentType string) bool {
	switch contentType {
	case "image/png", "image/jpeg", "image/gif", "image/webp", "image/heic", "image/heif":
		return true
	default:
		return false
	}
}

func escapeQuotes(value string) string {
	return strings.ReplaceAll(value, `"`, `\\"`)
}

func apiPatch(path string, requestBody []byte) error {
	body, err := apiPatchBody(path, requestBody)
	if err != nil {
		return err
	}
	fmt.Println(string(body))
	return nil
}

func apiDelete(path string) error {
	body, err := apiDeleteBody(path)
	if err != nil {
		return err
	}
	fmt.Println(string(body))
	return nil
}

func apiDeleteAllowNoAgent(path string) error {
	body, err := apiDeleteBodyAllowNoAgent(path)
	if err != nil {
		return err
	}
	fmt.Println(string(body))
	return nil
}

func apiGet(path string) error {
	body, err := apiGetBody(path)
	if err != nil {
		return err
	}
	fmt.Println(string(body))
	return nil
}

func apiPostBody(path string, requestBody []byte) ([]byte, error) {
	return apiPostBodyWithContentType(path, requestBody, "application/json")
}

func apiPostBodyAllowNoAgent(path string, requestBody []byte) ([]byte, error) {
	cfg, err := config.LoadAgent()
	if err != nil {
		return nil, err
	}
	client := httpclient.New(cfg)
	return client.Post(path, requestBody)
}

func apiPostBodyWithContentType(path string, requestBody []byte, contentType string) ([]byte, error) {
	cfg, err := config.LoadAgent()
	if err != nil {
		return nil, err
	}
	if cfg.AgentSlug == "" {
		return nil, fmt.Errorf("agent slug is not set; run `missionbase-agent use <slug>` in this directory or set MISSIONBASE_AGENT_SLUG")
	}
	client := httpclient.New(cfg)
	return client.PostWithContentType(path, requestBody, contentType)
}

func apiPatchBody(path string, requestBody []byte) ([]byte, error) {
	cfg, err := config.LoadAgent()
	if err != nil {
		return nil, err
	}
	if cfg.AgentSlug == "" {
		return nil, fmt.Errorf("agent slug is not set; run `missionbase-agent use <slug>` in this directory or set MISSIONBASE_AGENT_SLUG")
	}
	client := httpclient.New(cfg)
	return client.Patch(path, requestBody)
}

func apiDeleteBody(path string) ([]byte, error) {
	cfg, err := config.LoadAgent()
	if err != nil {
		return nil, err
	}
	if cfg.AgentSlug == "" {
		return nil, fmt.Errorf("agent slug is not set; run `missionbase-agent use <slug>` in this directory or set MISSIONBASE_AGENT_SLUG")
	}
	client := httpclient.New(cfg)
	return client.Delete(path)
}

func apiDeleteBodyAllowNoAgent(path string) ([]byte, error) {
	cfg, err := config.LoadAgent()
	if err != nil {
		return nil, err
	}
	client := httpclient.New(cfg)
	return client.Delete(path)
}

func apiGetBody(path string) ([]byte, error) {
	cfg, err := config.LoadAgent()
	if err != nil {
		return nil, err
	}
	if cfg.AgentSlug == "" {
		return nil, fmt.Errorf("agent slug is not set; run `missionbase-agent use <slug>` in this directory or set MISSIONBASE_AGENT_SLUG")
	}
	client := httpclient.New(cfg)
	return client.Get(path)
}

func printHelp() {
	fmt.Println(`Missionbase Agent CLI

Agent-acting Missionbase command-line client.

Usage:
  missionbase-agent <command> [args]

Commands:
  auth status                         Show auth status
  auth set-token <team-token> [--base-url URL] [--agent slug]
                                      Save a team API token
  use <agent-slug> [--base-url URL]   Set the agent for this directory
  me                                  Show the current agent
  work [--next|--next-task]           Show assigned tasks, unread conversations, and DMs
                                      With --next, return only the next assigned task
  listen [--timeout N] [--offset ID] [--once]
                                      Long-poll for agent updates
  dm list [--limit N]                 List agent direct messages
  dm show <chat-id>                   Show an agent DM chat
  dm send --to <handle> --body-file PATH
                                      Start/send a DM to a user or agent
  dm send --chat <chat-id> --body-file PATH
                                      Reply in an existing DM chat
  agent create --name NAME --slug SLUG [--description TEXT]
                                      Create an agent on the authenticated team
  agent archive <agent-id-or-slug> --yes
                                      Archive/deactivate an agent safely
  agent boxes add <agent-id-or-slug> --box BOX_ID [--box BOX_ID]
                                      Add an agent to one or more boxes
  document create --box BOX_ID --title TITLE --body-file PATH
                                      Create a box document from a Markdown/plain-text file
  document edit <document-id> [--title TITLE] --body-file PATH
                                      Edit a document by creating a new version from a file
  tasks                               Show assigned tasks
  task create --title TITLE --box ID (--assign-agent slug | --assign-user ID|@mention)
      [--description-file PATH] [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]
                                      Create a task and print the created task JSON
  task assign <task-id> --user ID|@mention
                                      Assign an existing task to a user
  task assign <task-id> --agent slug  Assign an existing task to an agent
  task unassign <task-id> --user ID|@mention
                                      Remove a user assignment from a task
  task unassign <task-id> --agent slug
                                      Remove an agent assignment from a task
  task unassign <task-id> --self      Remove the current agent from a task
  task comment <task-id> --body-file PATH
      [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]
                                      Post a Markdown-capable comment to a task conversation/feed
  task status <task-id> <status>      Set status (server validates box-specific statuses)
  task complete <task-id>             Mark a task complete
  task feed <task-id> [--limit N]     Show a task feed and comments
  task comments <task-id> [--limit N] Show a task feed and comments
  task participants list <task-id>    List task participants
  task participants add <task-id> --user ID|@mention
                                      Add a user participant to a task
  task participants add <task-id> --agent slug
                                      Add an agent participant to a task
  conversation show <feed-id> [--limit N]
                                      Show a conversation/feed
  conversation comment <feed-id> --body-file PATH
      [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]
                                      Post a Markdown-capable reply to a conversation/feed
  members [--box ID]                  List group members and mention handles
  boxes tasks <box-id>                Show open-category tasks in an accessible box by default
      [--status STATUS] [--status-category open|done|canceled] [--task-status-ids IDS]
      [--page N] [--per-page N]
  boxes discussions <box-id>          List standalone box discussions (not task conversations)
      [--page N] [--per-page N]
  boxes discussions create <box-id>   Create a standalone Markdown-capable box discussion
      --title TITLE --body-file PATH
  boxes task-statuses <box-id>        List all configured task statuses for a box as JSON
                                      Fields: id, key, name, category, position, color,
                                      default_open, primary_done, primary_canceled, archived
  boxes statuses <box-id>             Alias for boxes task-statuses
  get /api/path                       GET an API path and print JSON
  update [--check] [--force]          Update this CLI from GitHub Releases
  version                             Show CLI version

Markdown:
  DM bodies, task comment bodies, conversation comment bodies, box discussion bodies, and document bodies are Markdown-capable
  by default. Missionbase renders headings, emphasis, links, lists, blockquotes, and fenced code blocks as
  sanitized rich text while preserving ordinary plain-text messages. Accidental escaped newline sequences
  (\\n, \\r, \\r\\n) are normalized to real line breaks outside quoted/backticked code contexts.

  Agent-authored posting bodies are read from --body-file PATH only. Stdin body input is intentionally
  unsupported so Markdown, backticks, and shell-sensitive content are never passed through fragile shell
  quoting or piped interactive flows.

Directory config:
  missionbase-agent searches the current directory and parents for
  .missionbase-agent.json. This lets each project/worktree use a different
  agent while sharing the global team token in ~/.config/missionbase-agent/credentials.

Default base URL: https://dash.missionbase.app`)
}
