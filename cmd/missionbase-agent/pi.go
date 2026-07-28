package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"text/tabwriter"

	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/config"
	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/httpclient"
)

const piActorMode = "agent"

//go:embed missionbase-bridge.ts
var missionbaseBridgeSource []byte

type piAgent struct {
	ID     int    `json:"id"`
	Slug   string `json:"slug"`
	Name   string `json:"name"`
	Title  string `json:"title"`
	Status string `json:"status"`
}

func pi(args []string) error {
	if len(args) > 0 && args[0] == "agents" {
		return piAgents(args[1:])
	}
	if len(args) > 0 && (args[0] == "--help" || args[0] == "-h") {
		printPiHelp()
		return nil
	}

	slug, piArgs, err := parsePiLaunchArgs(args)
	if err != nil {
		return err
	}

	computer, err := config.LoadComputer()
	if err != nil {
		return err
	}
	if computer.ComputerID == 0 || strings.TrimSpace(computer.Credential) == "" {
		return fmt.Errorf("computer is not connected; run `missionbase-agent connect TOKEN`")
	}

	cfg, err := config.LoadAgent()
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return fmt.Errorf("not authenticated; run `missionbase-agent auth set-token <team-token>`")
	}
	cfg.BaseURL = computer.BaseURL
	cfg.AgentSlug = slug

	agent, err := preflightPiAgent(cfg)
	if err != nil {
		return fmt.Errorf("cannot launch Pi as %q: %w", slug, err)
	}

	extensionPath, err := materializeMissionbaseBridge()
	if err != nil {
		return fmt.Errorf("prepare Missionbase Pi bridge: %w", err)
	}

	systemPrompt := fmt.Sprintf("Your Missionbase identity for this Pi process is agent %q. Use the agent-acting `missionbase-agent` CLI for every Missionbase operation. Do not use the user-acting `missionbase` CLI.", agent.Slug)
	commandArgs := append([]string{"--extension", extensionPath, "--append-system-prompt", systemPrompt}, piArgs...)
	command := exec.Command("pi", commandArgs...)
	command.Env = setEnvironment(os.Environ(), map[string]string{
		"MISSIONBASE_ACTOR_MODE":     piActorMode,
		"MISSIONBASE_COMPUTER_TOKEN": computer.Credential,
		"MISSIONBASE_COMPUTER_ID":    fmt.Sprint(computer.ComputerID),
		"MISSIONBASE_BASE_URL":       computer.BaseURL,
		"MISSIONBASE_AGENT_ID":       fmt.Sprint(agent.ID),
		"MISSIONBASE_AGENT_SLUG":     agent.Slug,
		"MISSIONBASE_CLI_VERSION":    Version,
	})
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr

	fmt.Fprintf(os.Stderr, "Launching Pi as Missionbase agent: %s (%s)\n", agent.Name, agent.Slug)
	return command.Run()
}

func parsePiLaunchArgs(args []string) (string, []string, error) {
	var slug string
	var sessionArgs []string
	var piArgs []string

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--agent":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return "", nil, fmt.Errorf("--agent requires a value")
			}
			slug = args[i+1]
			i++
		case "--continue", "-c":
			if len(sessionArgs) > 0 {
				return "", nil, fmt.Errorf("only one of --continue, --resume, or --session may be used")
			}
			sessionArgs = []string{"--continue"}
		case "--resume", "-r":
			if len(sessionArgs) > 0 {
				return "", nil, fmt.Errorf("only one of --continue, --resume, or --session may be used")
			}
			sessionArgs = []string{"--resume"}
		case "--session", "-s":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" || args[i+1] == "--" {
				return "", nil, fmt.Errorf("--session requires a session path or ID")
			}
			if len(sessionArgs) > 0 {
				return "", nil, fmt.Errorf("only one of --continue, --resume, or --session may be used")
			}
			sessionArgs = []string{"--session", args[i+1]}
			i++
		case "--":
			piArgs = append(piArgs, args[i+1:]...)
			i = len(args)
		case "--help", "-h":
			return "", nil, fmt.Errorf("usage: missionbase-agent pi --agent SLUG [--continue | --resume | --session SESSION] [-- PI_ARGS...]")
		default:
			return "", nil, fmt.Errorf("unknown pi option %q; put Pi arguments after --", args[i])
		}
	}

	if strings.TrimSpace(slug) == "" {
		return "", nil, fmt.Errorf("--agent is required; run `missionbase-agent pi agents` to list agents")
	}
	return slug, append(sessionArgs, piArgs...), nil
}

func preflightPiAgent(cfg config.Config) (piAgent, error) {
	body, err := httpclient.NewAgent(cfg).Get("/api/v1/agent/me")
	if err != nil {
		return piAgent{}, err
	}
	var response struct {
		Agent piAgent `json:"agent"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return piAgent{}, fmt.Errorf("decode agent response: %w", err)
	}
	if response.Agent.Slug == "" {
		return piAgent{}, fmt.Errorf("Missionbase returned no agent identity")
	}
	if response.Agent.Slug != cfg.AgentSlug {
		return piAgent{}, fmt.Errorf("Missionbase returned agent %q instead of %q", response.Agent.Slug, cfg.AgentSlug)
	}
	return response.Agent, nil
}

func piAgents(args []string) error {
	jsonOutput := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOutput = true
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent pi agents [--json]")
			return nil
		default:
			return fmt.Errorf("unknown pi agents option %q", arg)
		}
	}

	cfg, err := config.LoadAgent()
	if err != nil {
		return err
	}
	if strings.TrimSpace(cfg.Token) == "" {
		return fmt.Errorf("not authenticated; run `missionbase-agent auth set-token <team-token>`")
	}
	cfg.AgentSlug = ""
	body, err := httpclient.NewAgent(cfg).Get("/api/v1/agents")
	if err != nil {
		return err
	}
	if jsonOutput {
		fmt.Println(string(body))
		return nil
	}

	var response struct {
		Agents []piAgent `json:"agents"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("decode agents response: %w", err)
	}
	if len(response.Agents) == 0 {
		fmt.Println("No active agents found.")
		return nil
	}

	writer := tabwriter.NewWriter(os.Stdout, 0, 4, 2, ' ', 0)
	fmt.Fprintln(writer, "SLUG\tNAME\tTITLE")
	for _, agent := range response.Agents {
		fmt.Fprintf(writer, "%s\t%s\t%s\n", agent.Slug, agent.Name, agent.Title)
	}
	return writer.Flush()
}

func materializeMissionbaseBridge() (string, error) {
	directory := config.ComputerConfigDir()
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(directory, "missionbase-bridge.ts")
	if err := os.WriteFile(path, missionbaseBridgeSource, 0o600); err != nil {
		return "", err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func setEnvironment(environment []string, values map[string]string) []string {
	filtered := make([]string, 0, len(environment)+len(values))
	for _, entry := range environment {
		key, _, found := strings.Cut(entry, "=")
		if found {
			if _, replaced := values[key]; replaced {
				continue
			}
		}
		filtered = append(filtered, entry)
	}
	for key, value := range values {
		filtered = append(filtered, key+"="+value)
	}
	return filtered
}

func printPiHelp() {
	fmt.Println(`Usage:
  missionbase-agent pi agents [--json]
  missionbase-agent pi --agent SLUG [--continue | --resume | --session SESSION] [-- PI_ARGS...]

Launches a local Pi process with Missionbase agent identity fixed to SLUG.
-c, --continue         Open the most recent session for the current working directory.
-r, --resume           Open Pi's interactive session chooser.
-s, --session SESSION  Open a specific session path or ID.
Arguments after -- are forwarded to Pi unchanged.`)
}
