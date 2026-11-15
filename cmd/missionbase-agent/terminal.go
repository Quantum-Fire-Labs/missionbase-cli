package main

// The agent's registered Herdr terminal (#218): register, clear and show.
// The Herdr bridge on the terminal's machine types new DMs into it. See
// README "Agent terminal".

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

const terminalPath = "/api/v1/agent/terminal"

const terminalUsage = `usage: missionbase-agent terminal <register|clear|show> [options]

  register [--terminal [machine/]tN] [--json]
      Record where this agent runs, so the Herdr bridge on that machine can type
      new DMs into it. Default: $HC_TERMINAL_ID on this machine (hc's name for it).
  clear [--json]   Forget the registered terminal.
  show [--json]    Show the registered terminal.`

type agentTerminal struct {
	Address      string `json:"address"`
	Machine      string `json:"machine"`
	ID           string `json:"id"`
	RegisteredAt string `json:"registered_at"`
}

type terminalResponse struct {
	Agent struct {
		ID   int64  `json:"id"`
		Slug string `json:"slug"`
	} `json:"agent"`
	Terminal *agentTerminal `json:"terminal"`
}

func terminalCommand(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		fmt.Println(terminalUsage)
		return nil
	}
	switch args[0] {
	case "register":
		return terminalRegister(args[1:])
	case "clear":
		return terminalSimple("clear", http.MethodDelete, args[1:])
	case "show":
		return terminalSimple("show", http.MethodGet, args[1:])
	default:
		return fmt.Errorf("unknown terminal command %q\n%s", args[0], terminalUsage)
	}
}

func terminalRegister(args []string) error {
	spec, jsonOut := "", false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--terminal":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return fmt.Errorf("--terminal requires a value like t7 or machine/t7")
			}
			spec = args[i+1]
			i++
		case "--json":
			jsonOut = true
		case "--help", "-h":
			fmt.Println(terminalUsage)
			return nil
		default:
			return fmt.Errorf("unknown terminal register option %q\n%s", args[i], terminalUsage)
		}
	}

	machine, terminal, err := resolveWorkTerminal(spec)
	if err != nil {
		return err
	}
	name := machine.Name
	if name == "" {
		name = machine.ID
	}
	body, err := agentDo(http.MethodPut, terminalPath, map[string]string{"terminal": name + "/" + terminal})
	if err != nil {
		return err
	}
	return printTerminal("register", body, jsonOut)
}

func terminalSimple(command, method string, args []string) error {
	jsonOut := false
	for _, arg := range args {
		switch arg {
		case "--json":
			jsonOut = true
		case "--help", "-h":
			fmt.Println(terminalUsage)
			return nil
		default:
			return fmt.Errorf("unknown terminal %s option %q\n%s", command, arg, terminalUsage)
		}
	}
	body, err := agentDo(method, terminalPath, nil)
	if err != nil {
		return err
	}
	return printTerminal(command, body, jsonOut)
}

func printTerminal(command string, body []byte, jsonOut bool) error {
	if jsonOut {
		fmt.Println(strings.TrimSpace(string(body)))
		return nil
	}
	var response terminalResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("unexpected terminal response: %w", err)
	}
	slug, t := response.Agent.Slug, response.Terminal
	switch {
	case command == "register" && t != nil:
		fmt.Printf("Registered %s as %s's terminal\n", t.Address, slug)
	case command == "clear":
		fmt.Printf("Cleared %s's terminal\n", slug)
	case t != nil:
		line := t.Address
		if t.RegisteredAt != "" {
			line += " (registered " + t.RegisteredAt + ")"
		}
		fmt.Println(line)
	default:
		fmt.Printf("No terminal registered for %s\n", slug)
	}
	return nil
}
