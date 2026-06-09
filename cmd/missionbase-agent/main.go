package main

import (
	"encoding/json"
	"fmt"
	"os"
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

	body, err := apiGetBody(path)
	if err == nil {
		fmt.Println(string(body))
		return nil
	}
	if filtered || !strings.Contains(err.Error(), "404") {
		return err
	}

	// Backward-compatible fallback for Missionbase deployments that expose team
	// members before the agent-specific members endpoint is available.
	meBody, meErr := apiGetBody("/api/v1/agent/me")
	if meErr != nil {
		return err
	}
	var me struct {
		Agent struct {
			Team struct {
				ID int `json:"id"`
			} `json:"team"`
		} `json:"agent"`
	}
	if jsonErr := json.Unmarshal(meBody, &me); jsonErr != nil || me.Agent.Team.ID == 0 {
		return err
	}
	body, fallbackErr := apiGetBody(fmt.Sprintf("/api/v1/teams/%d/members", me.Agent.Team.ID))
	if fallbackErr != nil {
		return err
	}
	fmt.Println(string(body))
	return nil
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

func apiGet(path string) error {
	body, err := apiGetBody(path)
	if err != nil {
		return err
	}
	fmt.Println(string(body))
	return nil
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
