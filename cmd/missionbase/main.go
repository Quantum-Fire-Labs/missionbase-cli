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
		return update.Run(update.Options{CurrentVersion: Version, Repo: Repo, BinaryName: "missionbase"}, args[1:])
	case "auth":
		return auth(args[1:])
	case "me":
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
		fmt.Println("usage: missionbase auth <status|set-token>")
		return nil
	}

	switch args[0] {
	case "status":
		cfg, err := config.LoadUser()
		if err != nil {
			return err
		}
		if cfg.Token == "" {
			fmt.Println("Not authenticated")
			return nil
		}
		fmt.Printf("Authenticated\nBase URL: %s\nCredentials: %s\n", cfg.BaseURL, config.CredentialsPath("missionbase"))
	case "set-token":
		if len(args) < 2 {
			return fmt.Errorf("usage: missionbase auth set-token <token> [--base-url URL]")
		}
		cfg, err := config.LoadUser()
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
		if err := config.SaveUser(cfg); err != nil {
			return err
		}
		fmt.Printf("Saved credentials to %s\n", config.CredentialsPath("missionbase"))
	default:
		return fmt.Errorf("unknown auth command %q", args[0])
	}

	return nil
}

func apiGet(path string) error {
	cfg, err := config.LoadUser()
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

User-acting Missionbase command-line client.

Usage:
  missionbase <command> [args]

Commands:
  auth status                         Show auth status
  auth set-token <token> [--base-url URL]
                                      Save a personal/user API token
  me                                  Show the current user
  get /api/path                       GET an API path and print JSON
  update [--check] [--force]          Update this CLI from GitHub Releases
  version                             Show CLI version

For agent acting, use missionbase-agent.
Default base URL: https://dash.missionbase.app`)
}
