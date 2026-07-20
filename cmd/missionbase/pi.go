package main

import (
	"fmt"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/config"
)

var safePiValue = regexp.MustCompile(`^[A-Za-z0-9._@:-]+$`)

func piCommand(args []string) error {
	cfg, err := config.LoadUser()
	if err != nil {
		return err
	}
	if len(args) > 0 && args[0] == "configure" {
		return piConfigure(cfg, args[1:])
	}

	host, remote, showHelp, err := piInvocation(cfg.PiHost, args)
	if err != nil {
		return err
	}
	if showHelp {
		fmt.Println("usage: missionbase pi [--team TEAM ...] [--agent SLUG] [--task ID|--discussion ID] [--one-shot] [--host SSH_HOST]\n       missionbase pi configure --host SSH_HOST")
		return nil
	}
	if strings.TrimSpace(host) == "" {
		return fmt.Errorf("Pi SSH host is not configured; run missionbase pi configure --host SSH_HOST")
	}
	if !safePiValue.MatchString(host) {
		return fmt.Errorf("unsafe Pi SSH host")
	}

	sshArgs := append([]string{"-t", host}, remote...)
	command := exec.Command("ssh", sshArgs...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = os.Stderr
	if err := command.Run(); err != nil {
		if exitError, ok := err.(*exec.ExitError); ok {
			return fmt.Errorf("remote Pi exited with status %d", exitError.ExitCode())
		}
		return err
	}
	return nil
}

func piInvocation(defaultHost string, args []string) (string, []string, bool, error) {
	host := defaultHost
	remote := []string{"sudo", "-n", "/opt/missionbase-runner/bin/mb-pi"}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--host":
			if i+1 >= len(args) {
				return "", nil, false, fmt.Errorf("--host requires a value")
			}
			host = args[i+1]
			i++
		case "--agent", "--task", "--discussion":
			if i+1 >= len(args) {
				return "", nil, false, fmt.Errorf("%s requires a value", args[i])
			}
			if !safePiValue.MatchString(args[i+1]) {
				return "", nil, false, fmt.Errorf("unsafe %s value", strings.TrimPrefix(args[i], "--"))
			}
			remote = append(remote, args[i], args[i+1])
			i++
		case "--team":
			remote = append(remote, "--team")
			teamWords := 0
			for i+1 < len(args) && !strings.HasPrefix(args[i+1], "--") {
				words := strings.Fields(args[i+1])
				for _, word := range words {
					if !safePiValue.MatchString(word) {
						return "", nil, false, fmt.Errorf("unsafe team value")
					}
					remote = append(remote, word)
					teamWords++
				}
				i++
			}
			if teamWords == 0 {
				return "", nil, false, fmt.Errorf("--team requires a value")
			}
		case "--one-shot":
			remote = append(remote, "--one-shot")
		case "--help", "-h":
			return host, remote, true, nil
		default:
			return "", nil, false, fmt.Errorf("unknown pi option %q", args[i])
		}
	}
	return host, remote, false, nil
}

func piConfigure(cfg config.Config, args []string) error {
	if len(args) != 2 || args[0] != "--host" || strings.TrimSpace(args[1]) == "" {
		return fmt.Errorf("usage: missionbase pi configure --host SSH_HOST")
	}
	if !safePiValue.MatchString(args[1]) {
		return fmt.Errorf("unsafe Pi SSH host")
	}
	cfg.PiHost = args[1]
	if err := config.SaveUser(cfg); err != nil {
		return err
	}
	fmt.Printf("Configured Pi SSH host: %s\n", cfg.PiHost)
	return nil
}
