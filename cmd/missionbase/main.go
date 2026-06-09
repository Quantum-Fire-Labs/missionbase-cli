package main

import (
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
		fmt.Fprintf(os.Stderr, "missionbase: %v\n", err)
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
		fmt.Printf("Missionbase CLI %s\nCommit: %s\n", Version, Commit)
	case "update":
		return update.Run(update.Options{CurrentVersion: Version, Repo: Repo}, args[1:])
	case "auth":
		return auth(args[1:])
	case "agent":
		return agent(args[1:])
	case "me":
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		if cfg.AgentSlug != "" {
			return apiGet("/api/v1/agent/me")
		}
		return apiGetFirst([]string{"/api/v1/users/me", "/api/v1/agent/me"})
	case "get":
		if len(args) < 2 {
			return fmt.Errorf("usage: missionbase get /api/path")
		}
		return apiGet(args[1])
	default:
		return fmt.Errorf("unknown command %q", args[0])
	}

	return nil
}

func auth(args []string) error {
	if len(args) == 0 {
		fmt.Println("usage: missionbase auth <status|set-token|set-agent|clear-agent>")
		return nil
	}

	switch args[0] {
	case "status":
		cfg, err := config.Load()
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
		}
		fmt.Printf("Credentials: %s\n", config.CredentialsPath())
	case "set-token":
		if len(args) < 2 {
			return fmt.Errorf("usage: missionbase auth set-token <token> [--base-url URL]")
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		cfg.Token = args[1]
		for i := 2; i < len(args); i++ {
			if args[i] == "--base-url" && i+1 < len(args) {
				cfg.BaseURL = args[i+1]
				i++
			}
		}
		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Printf("Saved credentials to %s\n", config.CredentialsPath())
	case "set-agent":
		if len(args) < 2 {
			return fmt.Errorf("usage: missionbase auth set-agent <agent-slug>")
		}
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		cfg.AgentSlug = args[1]
		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Printf("Saved agent slug %q to %s\n", cfg.AgentSlug, config.CredentialsPath())
	case "clear-agent":
		cfg, err := config.Load()
		if err != nil {
			return err
		}
		cfg.AgentSlug = ""
		if err := config.Save(cfg); err != nil {
			return err
		}
		fmt.Printf("Cleared agent slug in %s\n", config.CredentialsPath())
	default:
		return fmt.Errorf("unknown auth command %q", args[0])
	}

	return nil
}

func agent(args []string) error {
	if len(args) == 0 {
		fmt.Println("usage: missionbase agent <me|work|tasks>")
		return nil
	}

	switch args[0] {
	case "me":
		return apiGet("/api/v1/agent/me")
	case "work":
		return apiGet("/api/v1/agent/work")
	case "tasks":
		return apiGet("/api/v1/agent/tasks")
	default:
		return fmt.Errorf("unknown agent command %q", args[0])
	}
}

func apiGet(path string) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	client := httpclient.New(cfg)
	body, err := client.Get(path)
	if err != nil {
		return err
	}
	fmt.Println(string(body))
	return nil
}

func apiGetFirst(paths []string) error {
	var last error
	for _, path := range paths {
		if err := apiGet(path); err != nil {
			last = err
			if strings.Contains(err.Error(), "404") {
				continue
			}
			return err
		}
		return nil
	}
	return last
}

func printHelp() {
	fmt.Println(`Missionbase CLI

Usage:
  missionbase <command> [args]

Commands:
  auth status                         Show auth status
  auth set-token <token> [--base-url URL]
                                      Save an API token
  auth set-agent <agent-slug>         Act as an agent for team API keys
  auth clear-agent                    Stop acting as an agent
  agent me                            Show the current agent
  agent work                          Show assigned tasks and unread conversations
  agent tasks                         Show assigned tasks
  me                                  Show the current user/agent
  get /api/path                       GET an API path and print JSON
  update [--check] [--force]          Update this CLI from GitHub Releases
  version                             Show CLI version

Default base URL: https://dash.missionbase.app`)
}
