package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
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
	case "tasks":
		return apiGet("/api/v1/agent/tasks")
	case "task":
		return task(args[1:])
	case "conversation":
		return conversation(args[1:])
	case "members":
		return members(args[1:])
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
		return fmt.Errorf("usage: missionbase-agent task create --title TITLE --box ID (--assign-agent slug | --assign-user ID|@mention) [--description TEXT] [--participant-user ID|@mention] OR missionbase-agent task <feed|comments> <task-id> [--limit N] OR missionbase-agent task participants <list|add> <task-id> [--user ID|@mention | --agent slug]")
	}

	switch args[0] {
	case "create":
		return taskCreate(args[1:])
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

func taskCreate(args []string) error {
	payload := map[string]string{}
	var participantUsers []string

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
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent task create --title TITLE --box ID (--assign-agent slug | --assign-user ID|@mention) [--description TEXT] [--participant-user ID|@mention]")
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

	requestBody, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	responseBody, err := apiPostBody("/api/v1/tasks", requestBody)
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

func apiGet(path string) error {
	body, err := apiGetBody(path)
	if err != nil {
		return err
	}
	fmt.Println(string(body))
	return nil
}

func apiPostBody(path string, requestBody []byte) ([]byte, error) {
	cfg, err := config.LoadAgent()
	if err != nil {
		return nil, err
	}
	if cfg.AgentSlug == "" {
		return nil, fmt.Errorf("agent slug is not set; run `missionbase-agent use <slug>` in this directory or set MISSIONBASE_AGENT_SLUG")
	}
	client := httpclient.New(cfg)
	return client.Post(path, requestBody)
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
  work                                Show assigned tasks and unread conversations
  tasks                               Show assigned tasks
  task create --title TITLE --box ID (--assign-agent slug | --assign-user ID|@mention)
                                      Create a task and print the created task JSON
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
  get /api/path                       GET an API path and print JSON
  update [--check] [--force]          Update this CLI from GitHub Releases
  version                             Show CLI version

Directory config:
  missionbase-agent searches the current directory and parents for
  .missionbase-agent.json. This lets each project/worktree use a different
  agent while sharing the global team token in ~/.config/missionbase-agent/credentials.

Default base URL: https://dash.missionbase.app`)
}
