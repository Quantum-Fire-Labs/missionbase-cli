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
		return apiGet("/api/v1/agent/work")
	case "listen":
		return listen(args[1:])
	case "dm":
		return directMessage(args[1:])
	case "agent":
		return agent(args[1:])
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
		case "--body", "--message":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", args[i])
			}
			payload["body"] = args[i+1]
			i++
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent dm send (--to <handle> | --chat <chat-id>) --body MESSAGE")
			return nil
		default:
			if payload["body"] == "" {
				payload["body"] = strings.Join(args[i:], " ")
				i = len(args)
			} else {
				return fmt.Errorf("unknown dm send option %q", args[i])
			}
		}
	}
	if strings.TrimSpace(payload["to"]) == "" && strings.TrimSpace(payload["chat_id"]) == "" {
		return fmt.Errorf("--to or --chat is required")
	}
	if strings.TrimSpace(payload["body"]) == "" {
		return fmt.Errorf("--body is required")
	}

	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return apiPost("/api/v1/agent/direct_messages", body)
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
	if len(args) < 2 || args[0] != "show" {
		return fmt.Errorf("usage: missionbase-agent conversation show <feed-id> [--limit N]")
	}
	path := "/api/v1/conversations/" + args[1]
	path, err := appendLimit(path, args[2:])
	if err != nil {
		return err
	}
	return apiGet(path)
}

func boxes(args []string) error {
	if len(args) == 0 {
		fmt.Println("usage: missionbase-agent boxes <tasks>")
		return nil
	}

	switch args[0] {
	case "tasks":
		return boxTasks(args[1:])
	default:
		return fmt.Errorf("unknown boxes command %q", args[0])
	}
}

func boxTasks(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: missionbase-agent boxes tasks <box-id> [--status STATUS] [--page N] [--per-page N]")
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
		return fmt.Errorf("usage: missionbase-agent task create --title TITLE --box ID (--assign-agent slug | --assign-user ID|@mention) [--description TEXT] [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID] OR missionbase-agent task assign <task-id> (--user ID|@mention | --agent slug) OR missionbase-agent task unassign <task-id> (--user ID|@mention | --agent slug | --self) OR missionbase-agent task comment <task-id> --body TEXT [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID] OR missionbase-agent task status <task-id> <status> OR missionbase-agent task complete <task-id> OR missionbase-agent task <feed|comments> <task-id> [--limit N] OR missionbase-agent task participants <list|add> <task-id> [--user ID|@mention | --agent slug]")
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
		return fmt.Errorf("usage: missionbase-agent task comment <task-id> --body TEXT [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]")
	}
	taskID := args[0]
	payload := map[string]string{}
	var attaches, blobs []string

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--body", "--comment", "--message", "--text":
			if i+1 >= len(args) {
				return fmt.Errorf("%s requires a value", args[i])
			}
			payload["comment"] = args[i+1]
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
			fmt.Println("usage: missionbase-agent task comment <task-id> --body TEXT [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]")
			return nil
		default:
			return fmt.Errorf("unknown task comment option %q", args[i])
		}
	}

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
		return fmt.Errorf("usage: missionbase-agent task status <task-id> <backlog|todo|in_progress|complete|not_doing>")
	}

	taskID := args[0]
	status := args[1]
	validStatuses := map[string]bool{
		"backlog":     true,
		"todo":        true,
		"in_progress": true,
		"complete":    true,
		"not_doing":   true,
	}
	if !validStatuses[status] {
		return fmt.Errorf("status must be one of: backlog, todo, in_progress, complete, not_doing")
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
			if i+1 >= len(args) {
				return fmt.Errorf("--description requires a value")
			}
			payload["description"] = args[i+1]
			i++
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
			fmt.Println("usage: missionbase-agent task create --title TITLE --box ID (--assign-agent slug | --assign-user ID|@mention) [--description TEXT] [--participant-user ID|@mention] [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]")
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
	contentType := http.DetectContentType(peek[:n])
	if !allowedAttachmentContentType(contentType) {
		return fmt.Errorf("unsupported attachment type %q for %q (allowed: PNG, JPEG, GIF, WEBP)", contentType, path)
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

func allowedAttachmentContentType(contentType string) bool {
	switch contentType {
	case "image/png", "image/jpeg", "image/gif", "image/webp":
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
  work                                Show assigned tasks, unread conversations, and DMs
  listen [--timeout N] [--offset ID] [--once]
                                      Long-poll for agent updates
  dm list [--limit N]                 List agent direct messages
  dm show <chat-id>                   Show an agent DM chat
  dm send --to <handle> --body TEXT   Start/send a DM to a user or agent
  dm send --chat <chat-id> --body TEXT
                                      Reply in an existing DM chat
  agent create --name NAME --slug SLUG [--description TEXT]
                                      Create an agent on the authenticated team
  agent archive <agent-id-or-slug> --yes
                                      Archive/deactivate an agent safely
  agent boxes add <agent-id-or-slug> --box BOX_ID [--box BOX_ID]
                                      Add an agent to one or more boxes
  tasks                               Show assigned tasks
  task create --title TITLE --box ID (--assign-agent slug | --assign-user ID|@mention)
      [--description TEXT] [--attach PATH] [--attach-blob SIGNED_ID_OR_SGID]
                                      Create a task and print the created task JSON
  task assign <task-id> --user ID|@mention
                                      Assign an existing task to a user
  task assign <task-id> --agent slug  Assign an existing task to an agent
  task unassign <task-id> --user ID|@mention
                                      Remove a user assignment from a task
  task unassign <task-id> --agent slug
                                      Remove an agent assignment from a task
  task unassign <task-id> --self      Remove the current agent from a task
  task comment <task-id> --body TEXT [--attach PATH]
      [--attach-blob SIGNED_ID_OR_SGID]
                                      Post a Markdown-capable comment to a task conversation/feed
  task status <task-id> <status>      Set status: backlog, todo, in_progress, complete, not_doing
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
  members [--box ID]                  List group members and mention handles
  boxes tasks <box-id>                Show tasks in an accessible box
      [--status STATUS] [--page N] [--per-page N]
  get /api/path                       GET an API path and print JSON
  update [--check] [--force]          Update this CLI from GitHub Releases
  version                             Show CLI version

Markdown:
  DM bodies and task comment bodies are Markdown-capable by default. Missionbase
  renders headings, emphasis, links, lists, blockquotes, and fenced code blocks
  as sanitized rich text while preserving ordinary plain-text messages.

Directory config:
  missionbase-agent searches the current directory and parents for
  .missionbase-agent.json. This lets each project/worktree use a different
  agent while sharing the global team token in ~/.config/missionbase-agent/credentials.

Default base URL: https://dash.missionbase.app`)
}
