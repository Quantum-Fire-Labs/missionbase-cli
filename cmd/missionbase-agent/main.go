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
	"time"

	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/config"
	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/httpclient"
	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/textbody"
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
	case "scratchpad":
		return scratchpad(args[1:])
	case "listen":
		return listen(args[1:])
	case "dm":
		return directMessage(args[1:])
	case "agent":
		return agent(args[1:])
	case "document", "documents", "doc", "docs":
		return document(args[1:])
	case "tasks":
		return tasks(args[1:])
	case "task":
		return task(args[1:])
	case "discussion":
		return discussion(args[1:])
	case "conversation":
		return conversation(args[1:])
	case "workspace":
		return workspace(args[1:])
	case "members":
		return members(args[1:])
	case "sidebar":
		return sidebar(args[1:])
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

func scratchpad(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Println("usage: missionbase-agent scratchpad <show|fetch|update|edit> --user USER [--body-file PATH]")
		return nil
	}

	switch args[0] {
	case "show", "fetch":
		user, err := parseScratchpadUser(args[1:])
		if err != nil {
			return err
		}
		query := url.Values{}
		query.Set("user_id", user)
		return apiGet("/api/v1/scratchpad?" + query.Encode())
	case "update", "edit":
		user, body, err := parseAgentScratchpadUpdateArgs(args[1:])
		if err != nil {
			return err
		}
		return apiPatchJSON("/api/v1/scratchpad", map[string]string{"user_id": user, "scratchpad": textbody.Normalize(body)})
	default:
		return fmt.Errorf("unknown scratchpad command %q", args[0])
	}
}

func parseScratchpadUser(args []string) (string, error) {
	user := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--user", "--user-id":
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s requires a value", args[i])
			}
			user = args[i+1]
			i++
		case "--help", "-h":
			return "", fmt.Errorf("usage: missionbase-agent scratchpad show --user USER")
		default:
			return "", fmt.Errorf("unknown scratchpad option %q", args[i])
		}
	}
	if user == "" {
		return "", fmt.Errorf("--user is required")
	}
	return user, nil
}

func parseAgentScratchpadUpdateArgs(args []string) (string, string, error) {
	user := ""
	body := ""
	bodySet := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--user", "--user-id":
			if i+1 >= len(args) {
				return "", "", fmt.Errorf("%s requires a value", args[i])
			}
			user = args[i+1]
			i++
		case "--body", "--content", "--message", "--text", "--body-stdin", "--content-stdin", "--message-stdin", "--text-stdin":
			return "", "", fmt.Errorf("%s is not supported; use --body-file PATH", args[i])
		case "--body-file", "--content-file", "--message-file", "--text-file", "--file":
			if i+1 >= len(args) {
				return "", "", fmt.Errorf("%s requires a value", args[i])
			}
			content, err := os.ReadFile(args[i+1])
			if err != nil {
				return "", "", fmt.Errorf("read body file %q: %w", args[i+1], err)
			}
			body = string(content)
			bodySet = true
			i++
		case "--help", "-h":
			return "", "", fmt.Errorf("usage: missionbase-agent scratchpad edit --user USER --body-file PATH")
		default:
			return "", "", fmt.Errorf("unknown scratchpad option %q", args[i])
		}
	}
	if user == "" {
		return "", "", fmt.Errorf("--user is required")
	}
	if !bodySet {
		return "", "", fmt.Errorf("--body-file is required")
	}
	return user, body, nil
}

func tasks(args []string) error {
	if len(args) == 0 {
		return apiGet("/api/v1/agent/tasks")
	}

	due := ""
	if isDueShortcut(args[0]) {
		due = args[0]
		args = args[1:]
	}

	query := url.Values{}
	if due != "" {
		query.Set("due", due)
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--user":
			if i+1 >= len(args) {
				return fmt.Errorf("--user requires a value")
			}
			query.Set("user", args[i+1])
			i++
		case "--due":
			if i+1 >= len(args) {
				return fmt.Errorf("--due requires a value")
			}
			if !isDueFilter(args[i+1]) {
				return fmt.Errorf("--due must be one of: today, upcoming, overdue, none, all")
			}
			query.Set("due", args[i+1])
			i++
		case "--box":
			if i+1 >= len(args) {
				return fmt.Errorf("--box requires a value")
			}
			query.Set("box", args[i+1])
			i++
		case "--status-category":
			if i+1 >= len(args) {
				return fmt.Errorf("--status-category requires a value")
			}
			query.Set("status_category", args[i+1])
			i++
		case "--status":
			if i+1 >= len(args) {
				return fmt.Errorf("--status requires a value")
			}
			query.Set("status", args[i+1])
			i++
		case "--include-closed", "--include-completed":
			query.Set("include_closed", "true")
		case "--scheduled":
			if i+1 >= len(args) {
				return fmt.Errorf("--scheduled requires a value: actionable, future, or all")
			}
			if !isScheduledFilter(args[i+1]) {
				return fmt.Errorf("--scheduled must be one of: actionable, future, all")
			}
			query.Set("scheduled", args[i+1])
			i++
		case "--page":
			if i+1 >= len(args) {
				return fmt.Errorf("--page requires a value")
			}
			query.Set("page", args[i+1])
			i++
		case "--per-page":
			if i+1 >= len(args) {
				return fmt.Errorf("--per-page requires a value")
			}
			query.Set("per_page", args[i+1])
			i++
		case "--json":
			// JSON is the default output format; accept this flag for script-friendly ergonomics.
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent tasks [today|upcoming|overdue] --user ID|@handle [--due today|upcoming|overdue|none|all] [--box ID] [--status-category open|done|canceled] [--status STATUS] [--include-closed] [--scheduled actionable|future|all] [--page N] [--per-page N] [--json]")
			return nil
		default:
			return fmt.Errorf("unknown tasks option %q", args[i])
		}
	}

	if query.Get("due") != "" && query.Get("due") != "all" && query.Get("user") == "" {
		return fmt.Errorf("--user is required for due-date task listings")
	}

	if query.Get("user") == "" {
		return apiGet("/api/v1/agent/tasks")
	}

	path := "/api/v1/tasks"
	if encoded := query.Encode(); encoded != "" {
		path += "?" + encoded
	}
	return apiGet(path)
}

func isDueShortcut(value string) bool {
	return value == "today" || value == "upcoming" || value == "overdue"
}

func isDueFilter(value string) bool {
	return isDueShortcut(value) || value == "none" || value == "all"
}

func isScheduledFilter(value string) bool {
	return value == "actionable" || value == "future" || value == "all"
}

func work(args []string) error {
	for _, arg := range args {
		switch arg {
		case "--next", "--next-task":
			// Backward-compatible no-op: work is now always task-only and returns at most one task.
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent work [--next|--next-task]")
			return nil
		default:
			return fmt.Errorf("unknown work option %q", arg)
		}
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
		return fmt.Errorf("usage: missionbase-agent agent <create|archive|restore|unarchive|delete|boxes>")
	}

	switch args[0] {
	case "create":
		return agentCreate(args[1:])
	case "archive", "delete", "deactivate":
		return agentArchive(args[1:])
	case "restore", "unarchive", "reactivate":
		return agentRestore(args[1:])
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
		case "--title", "--role-title":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", args[i])
			}
			payload["title"] = args[i+1]
			i++
		case "--description":
			if i+1 >= len(args) {
				return fmt.Errorf("--description requires a value")
			}
			payload["description"] = args[i+1]
			i++
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent agent create --name NAME --slug SLUG [--title TITLE|--role-title TITLE] [--description TEXT]")
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

func agentRestore(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: missionbase-agent agent restore <agent-id-or-slug> --yes")
	}
	agentID := args[0]
	confirmed := false
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--yes", "-y":
			confirmed = true
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent agent restore <agent-id-or-slug> --yes")
			return nil
		default:
			return fmt.Errorf("unknown agent restore option %q", args[i])
		}
	}
	if strings.TrimSpace(agentID) == "" {
		return fmt.Errorf("agent id or slug is required")
	}
	if !confirmed {
		return fmt.Errorf("--yes is required to restore an agent; restore reactivates the agent with its existing identity and box memberships")
	}

	return apiPatchAllowNoAgent("/api/v1/agents/"+url.PathEscape(agentID)+"/restore", nil)
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

func discussion(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: missionbase-agent discussion <show|message> ...")
	}

	switch args[0] {
	case "show":
		if len(args) < 2 {
			return fmt.Errorf("usage: missionbase-agent discussion show <discussion-id> [--limit N]")
		}
		path := "/api/v1/conversations/" + url.PathEscape(args[1])
		path, err := appendLimit(path, args[2:])
		if err != nil {
			return err
		}
		return apiGet(path)
	case "message", "create-message", "comment", "create-comment", "reply":
		return discussionMessage(args[1:], "discussion message")
	case "convert", "convert-to-task", "task":
		return discussionConvert(args[1:])
	default:
		return fmt.Errorf("unknown discussion command %q", args[0])
	}
}

func conversation(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: missionbase-agent conversation <show|message> ... (deprecated; use discussion <show|message>)")
	}
	return discussion(args)
}

func discussionConvert(args []string) error {
	usage := "usage: missionbase-agent discussion convert <discussion-id> [--title TITLE] [--description-file PATH] [--deadline YYYY-MM-DD] [--status STATUS] [--task-status-id ID] [--assign-user ID] [--assign-agent ID_OR_SLUG]"
	if len(args) < 1 {
		return fmt.Errorf("%s", usage)
	}
	discussionID := args[0]
	payload := map[string]string{}
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--title", "--deadline", "--status", "--task-status-id", "--assign-user", "--assign-agent":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", args[i])
			}
			key := map[string]string{"--title": "title", "--deadline": "deadline", "--status": "status", "--task-status-id": "task_status_id", "--assign-user": "assign_to_user_id", "--assign-agent": "assign_to_agent_slug"}[args[i]]
			payload[key] = args[i+1]
			i++
		case "--description-file", "--body-file":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a file path", args[i])
			}
			description, err := readBodyFile(args[i+1])
			if err != nil {
				return err
			}
			payload["description"] = description
			i++
		case "--description", "--body", "--description-stdin", "--body-stdin":
			return fmt.Errorf("%s is not supported; use --description-file PATH", args[i])
		case "--help", "-h":
			fmt.Println(usage)
			return nil
		default:
			return fmt.Errorf("unknown discussion convert option %q", args[i])
		}
	}
	if payload["deadline"] != "" {
		if _, err := time.Parse("2006-01-02", payload["deadline"]); err != nil {
			return fmt.Errorf("deadline must be a valid date in YYYY-MM-DD format")
		}
	}
	payload["description"] = textbody.Normalize(payload["description"])
	return apiPostJSON("/api/v1/conversations/"+url.PathEscape(discussionID)+"/task_conversion", payload)
}

func discussionMessage(args []string, commandName string) error {
	usage := "usage: missionbase-agent " + commandName + " <discussion-id> --body-file PATH [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]"
	if len(args) < 1 {
		return fmt.Errorf("%s", usage)
	}
	discussionID := args[0]
	return postDiscussionMessage(discussionID, args[1:], usage, commandName)
}

func postDiscussionMessage(discussionID string, args []string, usage string, commandName string) error {
	payload := map[string]string{}
	var attaches, blobs []string

	for i := 0; i < len(args); i++ {
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
			payload["message"] = body
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
			fmt.Println(usage)
			return nil
		default:
			return fmt.Errorf("unknown %s option %q", commandName, args[i])
		}
	}

	payload["message"] = normalizeAgentAuthoredBody(payload["message"])
	if strings.TrimSpace(payload["message"]) == "" && len(attaches) == 0 && len(blobs) == 0 {
		return fmt.Errorf("--body or at least one attachment is required")
	}
	path := "/api/v1/conversations/" + url.PathEscape(discussionID) + "/comments"
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
		return fmt.Errorf("usage: missionbase-agent document show <document-id> [--format markdown|html|plain-text]\n       missionbase-agent document message <document-id> --body-file PATH [--attach PATH]\n       missionbase-agent document create --box BOX_ID --title TITLE --body-file PATH [--folder FOLDER_ID|--root]\n       missionbase-agent document edit <document-id> [--title TITLE] --body-file PATH")
	}

	switch args[0] {
	case "show", "fetch", "get":
		return documentFetch(args[1:])
	case "message", "comment", "reply":
		return documentMessage(args[1:])
	case "create":
		return documentCreate(args[1:])
	case "edit", "update":
		return documentEdit(args[1:])
	case "--help", "-h":
		fmt.Println("usage: missionbase-agent document show <document-id> [--format markdown|html|plain-text]\n       missionbase-agent document message <document-id> --body-file PATH [--attach PATH]\n       missionbase-agent document create --box BOX_ID --title TITLE --body-file PATH [--folder FOLDER_ID|--root]\n       missionbase-agent document edit <document-id> [--title TITLE] --body-file PATH\n\nExamples:\n  missionbase-agent document show 77\n  missionbase-agent document message 77 --body-file /tmp/reply.md\n  missionbase-agent document show 77 --format markdown\n  missionbase-agent document show 77 --format html\n  missionbase-agent document show 77 --format plain-text\n  missionbase-agent document create --box 2 --title Runbook --body-file /tmp/runbook.md --folder 67")
		return nil
	default:
		return fmt.Errorf("unknown document command %q", args[0])
	}
}

func documentFetch(args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Println("usage: missionbase-agent document show <document-id> [--format markdown|html|plain-text]\n       missionbase-agent document fetch <document-id> [--format markdown|html|plain-text]")
		return nil
	}
	if len(args) < 1 {
		return fmt.Errorf("usage: missionbase-agent document show <document-id> [--format markdown|html|plain-text]\n       missionbase-agent document fetch <document-id> [--format markdown|html|plain-text]")
	}
	documentID := strings.TrimSpace(args[0])
	if documentID == "" {
		return fmt.Errorf("document id is required")
	}

	format := "markdown"
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--format":
			if i+1 >= len(args) {
				return fmt.Errorf("--format requires a value")
			}
			format = normalizeDocumentFetchFormat(args[i+1])
			i++
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent document show <document-id> [--format markdown|html|plain-text]\n       missionbase-agent document fetch <document-id> [--format markdown|html|plain-text]")
			return nil
		default:
			return fmt.Errorf("unknown document fetch option %q", args[i])
		}
	}
	if !validDocumentFetchFormat(format) {
		return fmt.Errorf("invalid document format %q; supported formats: markdown, html, plain-text", format)
	}

	body, err := apiGetBody("/api/v1/documents/" + url.PathEscape(documentID) + "?format=" + url.QueryEscape(format))
	if err != nil {
		return err
	}
	var response struct {
		Document struct {
			Body string `json:"body"`
			URL  string `json:"url"`
		} `json:"document"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return err
	}
	fmt.Print(response.Document.Body)
	if !strings.HasSuffix(response.Document.Body, "\n") {
		fmt.Println()
	}
	if strings.TrimSpace(response.Document.URL) != "" {
		fmt.Fprintf(os.Stderr, "Document URL: %s\n", response.Document.URL)
	}
	return nil
}

func documentMessage(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: missionbase-agent document message <document-id> --body-file PATH [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]")
	}
	documentID := strings.TrimSpace(args[0])
	if documentID == "" {
		return fmt.Errorf("document id is required")
	}
	discussionID, err := documentDiscussionID(documentID)
	if err != nil {
		return err
	}
	return postDiscussionMessage(discussionID, args[1:], "usage: missionbase-agent document message <document-id> --body-file PATH [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]", "document message")
}

func documentDiscussionID(documentID string) (string, error) {
	body, err := apiGetBody("/api/v1/documents/" + url.PathEscape(documentID) + "?format=plain-text")
	if err != nil {
		return "", err
	}
	var response struct {
		Document struct {
			DiscussionID any `json:"discussion_id"`
		} `json:"document"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", err
	}
	discussionID := fmt.Sprint(response.Document.DiscussionID)
	if discussionID == "" || discussionID == "<nil>" {
		return "", fmt.Errorf("document %s does not have a discussion_id", documentID)
	}
	return discussionID, nil
}

func normalizeDocumentFetchFormat(format string) string {
	return strings.ReplaceAll(strings.TrimSpace(format), "_", "-")
}

func validDocumentFetchFormat(format string) bool {
	switch format {
	case "markdown", "html", "plain-text":
		return true
	default:
		return false
	}
}

func documentCreate(args []string) error {
	payload := map[string]string{}
	boxID := ""
	folderSet := false
	rootSet := false

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
		case "--folder", "--folder-id":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", args[i])
			}
			payload["folder_id"] = args[i+1]
			folderSet = true
			i++
		case "--root":
			payload["folder_id"] = "root"
			rootSet = true
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent document create --box BOX_ID --title TITLE --body-file PATH [--folder FOLDER_ID|--root]")
			return nil
		default:
			return fmt.Errorf("unknown document create option %q", args[i])
		}
	}

	if strings.TrimSpace(boxID) == "" {
		return fmt.Errorf("--box is required")
	}
	if folderSet && rootSet {
		return fmt.Errorf("use only one of --folder or --root")
	}
	if folderSet && strings.TrimSpace(payload["folder_id"]) == "" {
		return fmt.Errorf("--folder requires a value")
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
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Println("usage: missionbase-agent document edit <document-id> [--title TITLE] --body-file PATH")
		return nil
	}
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

func workspace(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: missionbase-agent workspace <get|create|update> --chat-id CHAT_ID")
	}

	switch args[0] {
	case "get", "show":
		chatID, err := workspaceChatID(args[1:])
		if err != nil {
			return err
		}
		if chatID == "" {
			return nil
		}
		return apiGet("/api/v1/chats/" + url.PathEscape(chatID) + "/workspace")
	case "create":
		return workspaceCreate(args[1:])
	case "update":
		return workspaceUpdate(args[1:])
	default:
		return fmt.Errorf("unknown workspace command %q", args[0])
	}
}

func workspaceCreate(args []string) error {
	chatID := ""
	payload := map[string]string{}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--chat-id", "--chat":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", args[i])
			}
			chatID = args[i+1]
			i++
		case "--title":
			if i+1 >= len(args) {
				return fmt.Errorf("--title requires a value")
			}
			payload["title"] = args[i+1]
			i++
		case "--body", "--markdown", "--content", "--text":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", args[i])
			}
			payload["markdown"] = args[i+1]
			i++
		case "--file", "--body-file", "--markdown-file", "--content-file", "--text-file":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a path", args[i])
			}
			content, err := os.ReadFile(args[i+1])
			if err != nil {
				return err
			}
			payload["markdown"] = string(content)
			i++
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent workspace create --chat-id CHAT_ID [--title TITLE] [--file PATH|--markdown TEXT]")
			return nil
		default:
			return fmt.Errorf("unknown workspace create option %q", args[i])
		}
	}
	if strings.TrimSpace(chatID) == "" {
		return fmt.Errorf("--chat-id is required")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return apiPost("/api/v1/chats/"+url.PathEscape(chatID)+"/workspace", body)
}

func workspaceUpdate(args []string) error {
	chatID := ""
	payload := map[string]string{}
	contentProvided := false

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--chat-id", "--chat":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", args[i])
			}
			chatID = args[i+1]
			i++
		case "--title":
			if i+1 >= len(args) {
				return fmt.Errorf("--title requires a value")
			}
			payload["title"] = args[i+1]
			i++
		case "--file", "--body-file", "--markdown-file", "--content-file", "--text-file":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a path", args[i])
			}
			content, err := os.ReadFile(args[i+1])
			if err != nil {
				return err
			}
			payload["markdown"] = string(content)
			contentProvided = true
			i++
		case "--body", "--markdown", "--content", "--text":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", args[i])
			}
			payload["markdown"] = args[i+1]
			contentProvided = true
			i++
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent workspace update --chat-id CHAT_ID [--title TITLE] [--file PATH|--markdown TEXT]\n       cat draft.md | missionbase-agent workspace update --chat-id CHAT_ID")
			return nil
		default:
			return fmt.Errorf("unknown workspace update option %q", args[i])
		}
	}
	if strings.TrimSpace(chatID) == "" {
		return fmt.Errorf("--chat-id is required")
	}
	if !contentProvided {
		stdin, err := readWorkspaceStdinIfPiped()
		if err != nil {
			return err
		}
		if stdin != nil {
			payload["markdown"] = string(stdin)
			contentProvided = true
		}
	}
	if !contentProvided && strings.TrimSpace(payload["title"]) == "" {
		return fmt.Errorf("--file, --markdown, stdin, or --title is required")
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return apiPatch("/api/v1/chats/"+url.PathEscape(chatID)+"/workspace", body)
}

func workspaceChatID(args []string) (string, error) {
	chatID := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--chat-id", "--chat":
			if i+1 >= len(args) {
				return "", fmt.Errorf("%s requires a value", args[i])
			}
			chatID = args[i+1]
			i++
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent workspace get --chat-id CHAT_ID")
			return "", nil
		default:
			return "", fmt.Errorf("unknown workspace option %q", args[i])
		}
	}
	if strings.TrimSpace(chatID) == "" {
		return "", fmt.Errorf("--chat-id is required")
	}
	return chatID, nil
}

func readWorkspaceStdinIfPiped() ([]byte, error) {
	info, err := os.Stdin.Stat()
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeCharDevice != 0 {
		return nil, nil
	}
	return io.ReadAll(os.Stdin)
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
	case "files":
		return boxFiles(args[1:])
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

func boxFiles(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("%s", boxFilesUsage())
	}
	switch args[0] {
	case "list", "search":
		return boxFilesList(args[1:])
	case "show", "get", "preview":
		return boxFileShow(args[1:])
	case "upload", "add":
		return boxFileUpload(args[1:])
	case "create-artifact", "artifact":
		return boxFileCreateArtifact(args[1:])
	case "mkdir", "folder":
		return boxFileMkdir(args[1:])
	case "mv", "move":
		return boxFileMove(args[1:])
	case "update", "edit":
		return boxFileUpdate(args[1:])
	case "versions", "version-list":
		return boxFileVersions(args[1:])
	case "upload-version", "new-version":
		return boxFileUploadVersion(args[1:])
	case "message", "comment", "reply":
		return boxFileMessage(args[1:])
	case "download", "fetch":
		return boxFileDownload(args[1:])
	case "--help", "-h":
		fmt.Println(boxFilesUsage())
		return nil
	default:
		return boxFilesList(args)
	}
}

func boxFilesUsage() string {
	return "usage: missionbase-agent boxes files <box-id> [--query QUERY] [--filter all|docs|files] [--sort newest|name|type] [--page N] [--per-page N] [--folder-id FOLDER_ID|--folder FOLDER_ID|--root] [--recursive]\n" +
		"       missionbase-agent boxes files show <box-id> <file-id>\n" +
		"       missionbase-agent boxes files upload <box-id> --file PATH [--title TITLE] [--description TEXT] [--folder FOLDER_ID|--root]\n" +
		"       missionbase-agent boxes files create-artifact <box-id> (--file PATH|--stdin) --title TITLE [--description TEXT] [--folder FOLDER_ID|--root]\n" +
		"       missionbase-agent boxes files update <box-id> <file-id> [--title TITLE] [--description TEXT] [--folder FOLDER_ID|--root]\n" +
		"       missionbase-agent boxes files message <box-id> <file-id> --body-file PATH [--attach PATH]\n" +
		"       missionbase-agent boxes files versions <box-id> <file-id>\n" +
		"       missionbase-agent boxes files upload-version <box-id> <file-id> --file PATH\n" +
		"       missionbase-agent boxes files download <box-id> <file-id> --output PATH [--version VERSION_ID]\n\n" +
		"Missionbase artifacts are sandboxed interactive HTML files. Artifact JavaScript runs outside the main Missionbase app origin and cannot access app DOM, local storage, auth tokens, or normal Missionbase APIs. Use window.MissionbaseArtifact.loadState()/saveState(data) (or loadState()/saveState(data)) for one shared persisted JSON state blob. Static .html uploads remain static previews."
}

func boxFilesList(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: missionbase-agent boxes files <box-id> [--query QUERY] [--filter all|docs|files] [--sort newest|name|type] [--page N] [--per-page N] [--folder-id FOLDER_ID|--folder FOLDER_ID|--root] [--recursive]")
	}
	values := url.Values{}
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--query", "-q":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", args[i])
			}
			values.Set("query", args[i+1])
			i++
		case "--filter":
			if i+1 >= len(args) {
				return fmt.Errorf("--filter requires a value")
			}
			values.Set("filter", args[i+1])
			i++
		case "--sort":
			if i+1 >= len(args) {
				return fmt.Errorf("--sort requires a value")
			}
			values.Set("sort", args[i+1])
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
		case "--folder", "--folder-id":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", args[i])
			}
			values.Set("folder_id", args[i+1])
			i++
		case "--root":
			values.Set("folder_id", "root")
		case "--recursive", "--all-folders":
			values.Set("scope", "recursive")
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent boxes files <box-id> [--query QUERY] [--filter all|docs|files] [--sort newest|name|type] [--page N] [--per-page N] [--folder-id FOLDER_ID|--folder FOLDER_ID|--root] [--recursive]")
			return nil
		default:
			return fmt.Errorf("unknown boxes files option %q", args[i])
		}
	}
	return apiGet(withQuery("/api/v1/boxes/"+url.PathEscape(args[0])+"/files", values))
}

func boxFileShow(args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Println("usage: missionbase-agent boxes files show <box-id> <file-id>")
		return nil
	}
	if len(args) != 2 {
		return fmt.Errorf("usage: missionbase-agent boxes files show <box-id> <file-id>")
	}
	return apiGet("/api/v1/boxes/" + url.PathEscape(args[0]) + "/files/" + url.PathEscape(args[1]))
}

func boxFileMessage(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: missionbase-agent boxes files message <box-id> <file-id> --body-file PATH [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]")
	}
	boxID := strings.TrimSpace(args[0])
	fileID := strings.TrimSpace(args[1])
	if boxID == "" || fileID == "" {
		return fmt.Errorf("box id and file id are required")
	}
	discussionID, err := boxFileDiscussionID(boxID, fileID)
	if err != nil {
		return err
	}
	return postDiscussionMessage(discussionID, args[2:], "usage: missionbase-agent boxes files message <box-id> <file-id> --body-file PATH [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]", "boxes files message")
}

func boxFileDiscussionID(boxID string, fileID string) (string, error) {
	body, err := apiGetBody("/api/v1/boxes/" + url.PathEscape(boxID) + "/files/" + url.PathEscape(fileID))
	if err != nil {
		return "", err
	}
	var response struct {
		File struct {
			DiscussionID any `json:"discussion_id"`
		} `json:"file"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", err
	}
	discussionID := fmt.Sprint(response.File.DiscussionID)
	if discussionID == "" || discussionID == "<nil>" {
		return "", fmt.Errorf("file %s does not have a discussion_id", fileID)
	}
	return discussionID, nil
}

func boxFileUpload(args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Println("usage: missionbase-agent boxes files upload <box-id> --file PATH [--title TITLE] [--description TEXT] [--folder FOLDER_ID|--root]")
		return nil
	}
	if len(args) < 1 {
		return fmt.Errorf("usage: missionbase-agent boxes files upload <box-id> --file PATH [--title TITLE] [--description TEXT] [--folder FOLDER_ID|--root]")
	}
	fields := map[string]string{}
	filePath := ""
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--file", "--path":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a path", args[i])
			}
			filePath = args[i+1]
			i++
		case "--title":
			if i+1 >= len(args) {
				return fmt.Errorf("--title requires a value")
			}
			fields["title"] = args[i+1]
			i++
		case "--description":
			if i+1 >= len(args) {
				return fmt.Errorf("--description requires a value")
			}
			fields["description"] = args[i+1]
			i++
		case "--folder":
			if i+1 >= len(args) {
				return fmt.Errorf("--folder requires a value")
			}
			fields["folder_id"] = args[i+1]
			i++
		case "--root":
			fields["folder_id"] = "root"
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent boxes files upload <box-id> --file PATH [--title TITLE] [--description TEXT] [--folder FOLDER_ID|--root]")
			return nil
		default:
			return fmt.Errorf("unknown boxes files upload option %q", args[i])
		}
	}
	if strings.TrimSpace(args[0]) == "" {
		return fmt.Errorf("box id is required")
	}
	if strings.TrimSpace(filePath) == "" {
		return fmt.Errorf("--file is required")
	}
	return apiPostSingleFileMultipart("/api/v1/boxes/"+url.PathEscape(args[0])+"/files", fields, "file", filePath)
}

func boxFileCreateArtifact(args []string) error {
	artifactHelp := "usage: missionbase-agent boxes files create-artifact <box-id> (--file PATH|--stdin) --title TITLE [--description TEXT] [--folder FOLDER_ID|--root]\n\nCreates a missionbase_artifact from HTML. Artifacts are sandboxed interactive HTML with JavaScript enabled and a persisted shared JSON state bridge: loadState() and saveState(data). Artifact JavaScript cannot access the Missionbase app origin, DOM, local storage, auth tokens, or normal Missionbase APIs. Static .html uploads remain static previews."
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Println(artifactHelp)
		return nil
	}
	if len(args) < 1 {
		return fmt.Errorf("%s", artifactHelp)
	}
	fields := map[string]string{"file_type": "missionbase_artifact"}
	filePath := ""
	useStdin := false
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--file", "--path":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a path", args[i])
			}
			filePath = args[i+1]
			i++
		case "--stdin":
			useStdin = true
		case "--title":
			if i+1 >= len(args) {
				return fmt.Errorf("--title requires a value")
			}
			fields["title"] = args[i+1]
			i++
		case "--description":
			if i+1 >= len(args) {
				return fmt.Errorf("--description requires a value")
			}
			fields["description"] = args[i+1]
			i++
		case "--folder":
			if i+1 >= len(args) {
				return fmt.Errorf("--folder requires a value")
			}
			fields["folder_id"] = args[i+1]
			i++
		case "--root":
			fields["folder_id"] = "root"
		case "--help", "-h":
			fmt.Println(artifactHelp)
			return nil
		default:
			return fmt.Errorf("unknown boxes files create-artifact option %q", args[i])
		}
	}
	if strings.TrimSpace(args[0]) == "" {
		return fmt.Errorf("box id is required")
	}
	if strings.TrimSpace(fields["title"]) == "" {
		return fmt.Errorf("--title is required")
	}
	if useStdin == (strings.TrimSpace(filePath) != "") {
		return fmt.Errorf("provide exactly one of --file PATH or --stdin")
	}
	return apiPostArtifactMultipart("/api/v1/boxes/"+url.PathEscape(args[0])+"/files", fields, filePath, useStdin)
}

func boxFileMkdir(args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Println("usage: missionbase-agent boxes files mkdir <box-id> --title TITLE [--folder FOLDER_ID|--root]")
		return nil
	}
	if len(args) < 1 {
		return fmt.Errorf("usage: missionbase-agent boxes files mkdir <box-id> --title TITLE [--folder FOLDER_ID|--root]")
	}
	payload := map[string]string{"kind": "folder"}
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--title", "--name":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", args[i])
			}
			payload["title"] = args[i+1]
			i++
		case "--folder":
			if i+1 >= len(args) {
				return fmt.Errorf("--folder requires a value")
			}
			payload["folder_id"] = args[i+1]
			i++
		case "--root":
			payload["folder_id"] = "root"
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent boxes files mkdir <box-id> --title TITLE [--folder FOLDER_ID|--root]")
			return nil
		default:
			return fmt.Errorf("unknown boxes files mkdir option %q", args[i])
		}
	}
	if strings.TrimSpace(payload["title"]) == "" {
		return fmt.Errorf("--title is required")
	}
	return apiPostJSON("/api/v1/boxes/"+url.PathEscape(args[0])+"/files", payload)
}

func boxFileMove(args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Println("usage: missionbase-agent boxes files mv <box-id> <file-id> (--folder FOLDER_ID|--root)")
		return nil
	}
	if len(args) < 2 {
		return fmt.Errorf("usage: missionbase-agent boxes files mv <box-id> <file-id> (--folder FOLDER_ID|--root)")
	}
	payload := map[string]string{}
	for i := 2; i < len(args); i++ {
		switch args[i] {
		case "--folder":
			if i+1 >= len(args) {
				return fmt.Errorf("--folder requires a value")
			}
			payload["parent_folder_id"] = args[i+1]
			i++
		case "--root":
			payload["parent_folder_id"] = "root"
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent boxes files mv <box-id> <file-id> (--folder FOLDER_ID|--root)")
			return nil
		default:
			return fmt.Errorf("unknown boxes files mv option %q", args[i])
		}
	}
	if _, ok := payload["parent_folder_id"]; !ok {
		return fmt.Errorf("one of --folder or --root is required")
	}
	return apiPatchJSON("/api/v1/boxes/"+url.PathEscape(args[0])+"/files/"+url.PathEscape(args[1]), payload)
}

func boxFileUpdate(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: missionbase-agent boxes files update <box-id> <file-id> [--title TITLE] [--description TEXT] [--folder FOLDER_ID|--root]")
	}
	payload := map[string]string{}
	for i := 2; i < len(args); i++ {
		switch args[i] {
		case "--title":
			if i+1 >= len(args) {
				return fmt.Errorf("--title requires a value")
			}
			payload["title"] = args[i+1]
			i++
		case "--description":
			if i+1 >= len(args) {
				return fmt.Errorf("--description requires a value")
			}
			payload["description"] = args[i+1]
			i++
		case "--folder":
			if i+1 >= len(args) {
				return fmt.Errorf("--folder requires a value")
			}
			payload["parent_folder_id"] = args[i+1]
			i++
		case "--root":
			payload["parent_folder_id"] = "root"
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent boxes files update <box-id> <file-id> [--title TITLE] [--description TEXT] [--folder FOLDER_ID|--root]")
			return nil
		default:
			return fmt.Errorf("unknown boxes files update option %q", args[i])
		}
	}
	if len(payload) == 0 {
		return fmt.Errorf("at least one of --title, --description, --folder, or --root is required")
	}
	return apiPatchJSON("/api/v1/boxes/"+url.PathEscape(args[0])+"/files/"+url.PathEscape(args[1]), payload)
}

func boxFileVersions(args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Println("usage: missionbase-agent boxes files versions <box-id> <file-id>")
		return nil
	}
	if len(args) != 2 {
		return fmt.Errorf("usage: missionbase-agent boxes files versions <box-id> <file-id>")
	}
	return apiGet("/api/v1/boxes/" + url.PathEscape(args[0]) + "/files/" + url.PathEscape(args[1]) + "/versions")
}

func boxFileUploadVersion(args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Println("usage: missionbase-agent boxes files upload-version <box-id> <file-id> --file PATH")
		return nil
	}
	if len(args) < 2 {
		return fmt.Errorf("usage: missionbase-agent boxes files upload-version <box-id> <file-id> --file PATH")
	}
	filePath := ""
	for i := 2; i < len(args); i++ {
		switch args[i] {
		case "--file", "--path":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a path", args[i])
			}
			filePath = args[i+1]
			i++
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent boxes files upload-version <box-id> <file-id> --file PATH")
			return nil
		default:
			return fmt.Errorf("unknown boxes files upload-version option %q", args[i])
		}
	}
	if strings.TrimSpace(filePath) == "" {
		return fmt.Errorf("--file is required")
	}
	return apiPostSingleFileMultipart("/api/v1/boxes/"+url.PathEscape(args[0])+"/files/"+url.PathEscape(args[1])+"/versions", map[string]string{}, "file", filePath)
}

func boxFileDownload(args []string) error {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Println("usage: missionbase-agent boxes files download <box-id> <file-id> --output PATH [--version VERSION_ID]")
		return nil
	}
	if len(args) < 2 {
		return fmt.Errorf("usage: missionbase-agent boxes files download <box-id> <file-id> --output PATH [--version VERSION_ID]")
	}
	output := ""
	versionID := ""
	for i := 2; i < len(args); i++ {
		switch args[i] {
		case "--output", "-o":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a path", args[i])
			}
			output = args[i+1]
			i++
		case "--version":
			if i+1 >= len(args) {
				return fmt.Errorf("--version requires a value")
			}
			versionID = args[i+1]
			i++
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent boxes files download <box-id> <file-id> --output PATH [--version VERSION_ID]")
			return nil
		default:
			return fmt.Errorf("unknown boxes files download option %q", args[i])
		}
	}
	if strings.TrimSpace(output) == "" {
		return fmt.Errorf("--output is required")
	}
	path := "/api/v1/boxes/" + url.PathEscape(args[0]) + "/files/" + url.PathEscape(args[1]) + "/download"
	if strings.TrimSpace(versionID) != "" {
		path = "/api/v1/boxes/" + url.PathEscape(args[0]) + "/files/" + url.PathEscape(args[1]) + "/versions/" + url.PathEscape(versionID) + "/download"
	}
	body, err := apiGetBody(path)
	if err != nil {
		return err
	}
	if err := os.WriteFile(output, body, 0o644); err != nil {
		return err
	}
	fmt.Fprintf(os.Stderr, "Downloaded file to %s\n", output)
	return nil
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
		return fmt.Errorf("usage: missionbase-agent boxes tasks <box-id> [--status STATUS|--status-category open|done|canceled|--task-status-ids IDS] [--scheduled actionable|future|all] [--page N] [--per-page N]")
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
		case "--scheduled":
			if i+1 >= len(args) {
				return fmt.Errorf("--scheduled requires a value: actionable, future, or all")
			}
			if !isScheduledFilter(args[i+1]) {
				return fmt.Errorf("--scheduled must be one of: actionable, future, all")
			}
			values.Set("scheduled", args[i+1])
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
		case "--json":
			// JSON is the default output format; accept this flag for consistency.
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent members [--box ID] [--json]")
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
		return fmt.Errorf("usage: missionbase-agent task show <task-id> OR missionbase-agent task create --title TITLE --box ID [--deadline YYYY-MM-DD] [--scheduled-at DATETIME] [--assign-agent slug | --assign-user ID|@mention] [--body-file PATH] [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID] OR missionbase-agent task update <task-id> [--deadline YYYY-MM-DD | --no-deadline] [--scheduled-at DATETIME | --no-scheduled-at] OR missionbase-agent task assign <task-id> (--user ID|@mention | --agent slug) OR missionbase-agent task unassign <task-id> (--user ID|@mention | --agent slug | --self) OR missionbase-agent task message <task-id> --body-file PATH [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID] OR missionbase-agent task status <task-id> <status> OR missionbase-agent task move <task-id> --box BOX_ID OR missionbase-agent task complete <task-id> OR missionbase-agent task messages <task-id> [--limit N] OR missionbase-agent task participants <list|add> <task-id> [--user ID|@mention | --agent slug]")
	}

	switch args[0] {
	case "show", "get":
		if len(args) != 2 {
			return fmt.Errorf("usage: missionbase-agent task show <task-id>")
		}
		return apiGet("/api/v1/tasks/" + url.PathEscape(args[1]))
	case "create":
		return taskCreate(args[1:])
	case "message", "create-message", "comment", "create-comment", "reply":
		return taskMessage(args[1:])
	case "update", "edit":
		return taskUpdate(args[1:])
	case "assign":
		return taskAssign(args[1:])
	case "unassign":
		return taskUnassign(args[1:])
	case "status":
		return taskStatus(args[1:])
	case "move", "box":
		return taskMove(args[1:])
	case "complete":
		return taskComplete(args[1:])
	case "messages", "comments":
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

func taskUpdate(args []string) error {
	usage := "usage: missionbase-agent task update <task-id> [--deadline YYYY-MM-DD | --no-deadline] [--scheduled-at DATETIME | --no-scheduled-at]"
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Println(usage)
		return nil
	}
	if len(args) < 1 {
		return fmt.Errorf("%s", usage)
	}
	taskID := args[0]
	payload := map[string]any{}
	deadlineSet := false
	deadlineCleared := false
	scheduledAtSet := false
	scheduledAtCleared := false

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--deadline":
			if i+1 >= len(args) {
				return fmt.Errorf("--deadline requires a value in YYYY-MM-DD format")
			}
			deadline, err := validateDeadline(args[i+1])
			if err != nil {
				return err
			}
			payload["deadline"] = deadline
			deadlineSet = true
			i++
		case "--no-deadline":
			payload["deadline"] = nil
			deadlineCleared = true
		case "--scheduled-at":
			if i+1 >= len(args) {
				return fmt.Errorf("--scheduled-at requires a datetime value")
			}
			scheduledAt, err := validateScheduledAt(args[i+1])
			if err != nil {
				return err
			}
			payload["scheduled_at"] = scheduledAt
			scheduledAtSet = true
			i++
		case "--no-scheduled-at", "--clear-scheduled-at":
			payload["scheduled_at"] = nil
			scheduledAtCleared = true
		case "--help", "-h":
			fmt.Println(usage)
			return nil
		default:
			return fmt.Errorf("unknown task update option %q", args[i])
		}
	}

	if deadlineSet && deadlineCleared {
		return fmt.Errorf("use only one of --deadline or --no-deadline")
	}
	if scheduledAtSet && scheduledAtCleared {
		return fmt.Errorf("use only one of --scheduled-at or --no-scheduled-at")
	}
	if len(payload) == 0 {
		return fmt.Errorf("one of --deadline, --no-deadline, --scheduled-at, or --no-scheduled-at is required")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return apiPatch("/api/v1/tasks/"+url.PathEscape(taskID), body)
}

func validateDeadline(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("--deadline requires a value in YYYY-MM-DD format")
	}
	if _, err := time.Parse("2006-01-02", value); err != nil {
		return "", fmt.Errorf("deadline must be a valid date in YYYY-MM-DD format")
	}
	return value, nil
}

func validateScheduledAt(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("--scheduled-at requires a datetime value")
	}
	return value, nil
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

func taskMessage(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: missionbase-agent task message <task-id> --body-file PATH [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]")
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
			payload["message"] = body
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
			fmt.Println("usage: missionbase-agent task message <task-id> --body-file PATH [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]")
			return nil
		default:
			return fmt.Errorf("unknown task message option %q", args[i])
		}
	}

	payload["message"] = normalizeAgentAuthoredBody(payload["message"])
	if strings.TrimSpace(payload["message"]) == "" && len(attaches) == 0 && len(blobs) == 0 {
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

func taskMove(args []string) error {
	if len(args) != 3 {
		return fmt.Errorf("usage: missionbase-agent task move <task-id> --box BOX_ID")
	}
	taskID := args[0]
	if args[1] != "--box" {
		return fmt.Errorf("unknown task move option %q", args[1])
	}
	boxID := strings.TrimSpace(args[2])
	if boxID == "" {
		return fmt.Errorf("--box requires a box id")
	}

	body, err := json.Marshal(map[string]string{"box_id": boxID})
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
		case "--body", "--body-stdin":
			return fmt.Errorf("%s is not supported; use --body-file PATH", args[i])
		case "--body-file":
			if i+1 >= len(args) {
				return fmt.Errorf("--body-file requires a file path")
			}
			body, err := readBodyFile(args[i+1])
			if err != nil {
				return err
			}
			payload["body"] = body
			i++
		case "--box":
			if i+1 >= len(args) {
				return fmt.Errorf("--box requires a value")
			}
			payload["box_id"] = args[i+1]
			i++
		case "--deadline":
			if i+1 >= len(args) {
				return fmt.Errorf("--deadline requires a value in YYYY-MM-DD format")
			}
			deadline, err := validateDeadline(args[i+1])
			if err != nil {
				return err
			}
			payload["deadline"] = deadline
			i++
		case "--scheduled-at":
			if i+1 >= len(args) {
				return fmt.Errorf("--scheduled-at requires a datetime value")
			}
			scheduledAt, err := validateScheduledAt(args[i+1])
			if err != nil {
				return err
			}
			payload["scheduled_at"] = scheduledAt
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
			fmt.Println("usage: missionbase-agent task create --title TITLE --box ID [--deadline YYYY-MM-DD] [--scheduled-at DATETIME] [--assign-agent slug | --assign-user ID|@mention] [--body-file PATH] [--participant-user ID|@mention] [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]")
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

func sidebar(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Println("usage: missionbase-agent sidebar <pins|pin|unpin> --user ID|@mention [--type box_file --id ID]")
		return nil
	}

	switch args[0] {
	case "pins", "list":
		if len(args) == 2 && (args[1] == "--help" || args[1] == "-h") {
			fmt.Println("usage: missionbase-agent sidebar pins --user ID|@mention")
			return nil
		}
		userID, err := parseSidebarUserArg(args[1:])
		if err != nil {
			return err
		}
		query := url.Values{}
		query.Set("user_id", userID)
		return apiGet("/api/v1/sidebar_pins?" + query.Encode())
	case "pin":
		if len(args) == 2 && (args[1] == "--help" || args[1] == "-h") {
			fmt.Println("usage: missionbase-agent sidebar pin --user ID|@mention --type box_file --id ID")
			return nil
		}
		userID, typeValue, idValue, err := parseAgentSidebarItemArgs(args[1:])
		if err != nil {
			return err
		}
		body, err := json.Marshal(map[string]string{"user_id": userID, "type": typeValue, "id": idValue})
		if err != nil {
			return err
		}
		return apiPost("/api/v1/sidebar_pins", body)
	case "unpin":
		if len(args) == 2 && (args[1] == "--help" || args[1] == "-h") {
			fmt.Println("usage: missionbase-agent sidebar unpin --user ID|@mention --type box_file --id ID")
			return nil
		}
		userID, typeValue, idValue, err := parseAgentSidebarItemArgs(args[1:])
		if err != nil {
			return err
		}
		query := url.Values{}
		query.Set("user_id", userID)
		query.Set("type", typeValue)
		query.Set("id", idValue)
		return apiDelete("/api/v1/sidebar_pins?" + query.Encode())
	default:
		return fmt.Errorf("unknown sidebar command %q", args[0])
	}
}

func parseSidebarUserArg(args []string) (string, error) {
	if len(args) == 0 {
		return "", fmt.Errorf("usage: missionbase-agent sidebar pins --user ID|@mention")
	}
	userValue := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--user":
			if i+1 >= len(args) {
				return "", fmt.Errorf("--user requires a value")
			}
			userValue = args[i+1]
			i++
		case "--help", "-h":
			return "", fmt.Errorf("usage: missionbase-agent sidebar pins --user ID|@mention")
		default:
			return "", fmt.Errorf("unknown sidebar option %q", args[i])
		}
	}
	if userValue == "" {
		return "", fmt.Errorf("usage: missionbase-agent sidebar pins --user ID|@mention")
	}
	return resolveUserID(userValue)
}

func parseAgentSidebarItemArgs(args []string) (string, string, string, error) {
	userValue := ""
	typeValue := ""
	idValue := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--user":
			if i+1 >= len(args) {
				return "", "", "", fmt.Errorf("--user requires a value")
			}
			userValue = args[i+1]
			i++
		case "--type":
			if i+1 >= len(args) {
				return "", "", "", fmt.Errorf("--type requires a value")
			}
			typeValue = args[i+1]
			i++
		case "--id":
			if i+1 >= len(args) {
				return "", "", "", fmt.Errorf("--id requires a value")
			}
			idValue = args[i+1]
			i++
		case "--help", "-h":
			return "", "", "", fmt.Errorf("usage: missionbase-agent sidebar <pin|unpin> --user ID|@mention --type box_file --id ID")
		default:
			return "", "", "", fmt.Errorf("unknown sidebar option %q", args[i])
		}
	}
	if userValue == "" || typeValue == "" || idValue == "" {
		return "", "", "", fmt.Errorf("usage: missionbase-agent sidebar <pin|unpin> --user ID|@mention --type box_file --id ID")
	}
	userID, err := resolveUserID(userValue)
	if err != nil {
		return "", "", "", err
	}
	return userID, typeValue, idValue, nil
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

func apiPostJSON(path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return apiPost(path, body)
}

func apiPatchJSON(path string, payload any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return apiPatch(path, body)
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

func apiPostArtifactMultipart(path string, fields map[string]string, filePath string, useStdin bool) error {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return err
		}
	}

	filename := strings.TrimSpace(fields["title"])
	if filename == "" {
		filename = "missionbase-artifact"
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, escapeQuotes("file"), escapeQuotes(filename)))
	header.Set("Content-Type", "text/html; charset=utf-8")
	part, err := writer.CreatePart(header)
	if err != nil {
		return err
	}
	if useStdin {
		data, err := io.ReadAll(io.LimitReader(os.Stdin, 100*1024*1024+1))
		if err != nil {
			return err
		}
		if len(data) > 100*1024*1024 {
			return fmt.Errorf("artifact stdin is too large (max 100 MB)")
		}
		if _, err := part.Write(data); err != nil {
			return err
		}
	} else {
		file, err := os.Open(filePath)
		if err != nil {
			return fmt.Errorf("open artifact file %q: %w", filePath, err)
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			return fmt.Errorf("stat artifact file %q: %w", filePath, err)
		}
		if info.IsDir() {
			return fmt.Errorf("artifact file %q is a directory", filePath)
		}
		if info.Size() > 100*1024*1024 {
			return fmt.Errorf("artifact file %q is too large (max 100 MB)", filePath)
		}
		if _, err := io.Copy(part, file); err != nil {
			return err
		}
	}
	if err := writer.Close(); err != nil {
		return err
	}
	body, err := apiPostBodyWithContentType(path, buf.Bytes(), writer.FormDataContentType())
	if err != nil {
		return err
	}
	fmt.Println(string(body))
	return nil
}

func apiPostSingleFileMultipart(path string, fields map[string]string, fieldName, filePath string) error {
	var buf bytes.Buffer
	writer := multipart.NewWriter(&buf)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			return err
		}
	}
	if err := addMultipartFile(writer, fieldName, filePath); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	body, err := apiPostBodyWithContentType(path, buf.Bytes(), writer.FormDataContentType())
	if err != nil {
		return err
	}
	fmt.Println(string(body))
	return nil
}

func addMultipartFile(writer *multipart.Writer, fieldName, path string) error {
	file, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open file %q: %w", path, err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return fmt.Errorf("stat file %q: %w", path, err)
	}
	if info.IsDir() {
		return fmt.Errorf("file %q is a directory", path)
	}
	if info.Size() > 100*1024*1024 {
		return fmt.Errorf("file %q is too large (max 100 MB)", path)
	}
	peek := make([]byte, 512)
	n, err := file.Read(peek)
	if err != nil && err != io.EOF {
		return fmt.Errorf("read file %q: %w", path, err)
	}
	contentType := http.DetectContentType(peek[:n])
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	header := make(textproto.MIMEHeader)
	header.Set("Content-Disposition", fmt.Sprintf(`form-data; name="%s"; filename="%s"`, escapeQuotes(fieldName), escapeQuotes(filepath.Base(path))))
	header.Set("Content-Type", contentType)
	part, err := writer.CreatePart(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(part, file)
	return err
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

func apiPatchAllowNoAgent(path string, requestBody []byte) error {
	body, err := apiPatchBodyAllowNoAgent(path, requestBody)
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

func apiPatchBodyAllowNoAgent(path string, requestBody []byte) ([]byte, error) {
	cfg, err := config.LoadAgent()
	if err != nil {
		return nil, err
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

func withQuery(path string, values url.Values) string {
	if encoded := values.Encode(); encoded != "" {
		return path + "?" + encoded
	}
	return path
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
  work [--next|--next-task]           Show the next actionable assigned task
                                      With --next, return only the next assigned task
  scratchpad show --user USER         Show a team member's scratchpad
  scratchpad edit --user USER --body-file PATH
                                      Update a team member's scratchpad from a file
  listen [--timeout N] [--offset ID] [--once]
                                      Long-poll for agent updates
  dm list [--limit N]                 List agent direct messages
  dm show <chat-id>                   Show an agent DM chat
  dm send --to <handle> --body-file PATH
                                      Start/send a DM to a user or agent
  dm send --chat <chat-id> --body-file PATH
                                      Reply in an existing DM chat
  agent create --name NAME --slug SLUG [--title TITLE|--role-title TITLE] [--description TEXT]
                                      Create an agent on the authenticated team
  agent archive <agent-id-or-slug> --yes
                                      Archive/deactivate an agent safely
  agent restore <agent-id-or-slug> --yes
                                      Restore/reactivate an archived agent
  agent boxes add <agent-id-or-slug> --box BOX_ID [--box BOX_ID]
                                      Add an agent to one or more boxes
  document show <document-id> [--format markdown|html|plain-text]
                                      Print a document body (default: markdown)
  document fetch <document-id> [--format markdown|html|plain-text]
                                      Compatibility alias for document show
  document message <document-id> --body-file PATH [--attach PATH]
                                      Post a Markdown-capable message to a document discussion
  document create --box BOX_ID --title TITLE --body-file PATH [--folder FOLDER_ID|--root]
                                      Create a box document from a Markdown/plain-text file
  document edit <document-id> [--title TITLE] --body-file PATH
                                      Edit a document by creating a new version from a file
  tasks                               Show assigned tasks
  tasks --user ID|@handle [--due today|upcoming|overdue|none|all]
      [--box ID] [--status-category open|done|canceled] [--include-closed]
      [--scheduled actionable|future|all] [--page N] [--per-page N] [--json]
                                      Show open tasks assigned to a target user
  tasks today|upcoming|overdue --user ID|@handle
                                      Convenience due-date task listings
  task show <task-id>                  Show full task working context
  task create --title TITLE --box ID [--deadline YYYY-MM-DD] [--scheduled-at DATETIME]
      [--assign-agent slug | --assign-user ID|@mention]
      [--body-file PATH] [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]
                                      Create a task and print the created task JSON
  task update <task-id> [--deadline YYYY-MM-DD | --no-deadline]
      [--scheduled-at DATETIME | --no-scheduled-at]
                                      Update task deadline or schedule and print the updated task JSON
  task assign <task-id> --user ID|@mention
                                      Assign an existing task to a user
  task assign <task-id> --agent slug  Assign an existing task to an agent
  task unassign <task-id> --user ID|@mention
                                      Remove a user assignment from a task
  task unassign <task-id> --agent slug
                                      Remove an agent assignment from a task
  task unassign <task-id> --self      Remove the current agent from a task
  task message <task-id> --body-file PATH
      [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]
                                      Post a Markdown-capable message to a task discussion
  task status <task-id> <status>      Set status (server validates box-specific statuses)
  task move <task-id> --box BOX_ID    Move a task to another accessible box
  task complete <task-id>             Mark a task complete
  task messages <task-id> [--limit N] Show task discussion messages
  task comments <task-id> [--limit N] Legacy alias for task messages
  task participants list <task-id>    List task participants
  task participants add <task-id> --user ID|@mention
                                      Add a user participant to a task
  task participants add <task-id> --agent slug
                                      Add an agent participant to a task
  discussion show <discussion-id> [--limit N]
                                      Show a discussion by canonical discussion id
  discussion message <discussion-id> --body-file PATH
      [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]
                                      Post a Markdown-capable reply by canonical discussion id
  conversation show/message           Deprecated aliases for discussion show/message
  workspace get --chat-id CHAT_ID     Get the chat workspace as Markdown JSON
  workspace create --chat-id CHAT_ID [--title TITLE]
      [--file PATH|--markdown TEXT]   Create/open a temporary chat workspace
  workspace update --chat-id CHAT_ID [--title TITLE]
      [--file PATH|--markdown TEXT]   Replace workspace content from Markdown or stdin
  members [--box ID] [--json]         List group members and mention handles
  boxes tasks <box-id>                Show open-category tasks in an accessible box by default
      [--status STATUS] [--status-category open|done|canceled] [--task-status-ids IDS]
      [--scheduled actionable|future|all] [--page N] [--per-page N]
  boxes discussions <box-id>          List standalone box discussions (not task conversations)
      [--page N] [--per-page N]
  boxes discussions create <box-id>   Create a standalone Markdown-capable box discussion
      --title TITLE --body-file PATH
  boxes files <box-id>                List/search files and documents in an accessible box
      [--query QUERY] [--filter all|docs|files] [--sort newest|name|type] [--page N] [--per-page N]
      [--folder-id FOLDER_ID|--folder FOLDER_ID|--root] [--recursive]
  boxes files show <box-id> <file-id> Show BoxFile/document metadata and preview fields
  boxes files upload <box-id> --file PATH [--title TITLE] [--description TEXT] [--folder FOLDER_ID|--root]
                                      Upload a file to Docs & Files
  boxes files create-artifact <box-id> (--file PATH|--stdin) --title TITLE [--description TEXT] [--folder FOLDER_ID|--root]
                                      Create sandboxed interactive HTML with persisted JSON state
  boxes files mkdir <box-id> --title TITLE [--folder FOLDER_ID|--root]
                                      Create a Docs & Files folder
  boxes files mv <box-id> <file-id> (--folder FOLDER_ID|--root)
                                      Move a file, document, or folder
  boxes files update <box-id> <file-id> [--title TITLE] [--description TEXT]
      [--folder FOLDER_ID|--root]
                                      Update uploaded file metadata or placement
  boxes files message <box-id> <file-id> --body-file PATH [--attach PATH]
                                      Post a Markdown-capable message to a file/document discussion
  boxes files versions <box-id> <file-id>
                                      List uploaded file versions
  boxes files upload-version <box-id> <file-id> --file PATH
                                      Upload a new version of an uploaded file
  boxes files download <box-id> <file-id> --output PATH [--version VERSION_ID]
                                      Download an uploaded file
  sidebar pins --user ID|@mention
                                      List pinned sidebar pages for a user
  sidebar pin --user ID|@mention --type box_file --id ID
                                      Pin a supported page to a user's sidebar
  sidebar unpin --user ID|@mention --type box_file --id ID
                                      Unpin a supported page from a user's sidebar
  boxes task-statuses <box-id>        List all configured task statuses for a box as JSON
                                      Fields: id, key, name, category, position, color,
                                      default_open, primary_done, primary_canceled, archived
  boxes statuses <box-id>             Alias for boxes task-statuses
  get /api/path                       GET an API path and print JSON
  update [--check] [--force]          Update this CLI from GitHub Releases
  version                             Show CLI version

Scheduling:
  --scheduled-at sends scheduled_at separately from deadline. The Missionbase API parses DATETIME in the acting
  user's timezone when no offset is included; include an ISO-8601 offset or Z for an absolute instant.
  Normal agent work/task endpoints keep using the API default scheduled filter, so future scheduled tasks are hidden
  until actionable. Use --scheduled future or --scheduled all on task listing commands when explicitly discovering them.

Artifacts:
  Missionbase artifacts are sandboxed interactive HTML files. Their JavaScript runs outside the main Missionbase app origin
  and cannot access app DOM, local storage, auth tokens, or normal Missionbase APIs. Use loadState()/saveState(data) or
  window.MissionbaseArtifact.loadState()/saveState(data) for one shared persisted JSON state blob. Static .html uploads remain static previews.

Markdown:
  DM bodies, task/discussion/document/file message bodies, box discussion bodies, and document bodies are Markdown-capable
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
