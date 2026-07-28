package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/config"
)

func connect(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" {
		fmt.Println("usage: missionbase-agent connect TOKEN [--base-url URL] [--name NAME]")
		return nil
	}

	token := strings.TrimSpace(args[0])
	if token == "" {
		return fmt.Errorf("enrollment token is required")
	}

	computerConfig, err := config.LoadComputer()
	if err != nil {
		return err
	}
	hostname, err := os.Hostname()
	if err != nil {
		return fmt.Errorf("determine hostname: %w", err)
	}
	name := hostname
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--base-url":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return fmt.Errorf("--base-url requires a value")
			}
			computerConfig.BaseURL = args[i+1]
			i++
		case "--name":
			if i+1 >= len(args) || strings.TrimSpace(args[i+1]) == "" {
				return fmt.Errorf("--name requires a value")
			}
			name = args[i+1]
			i++
		case "--help", "-h":
			fmt.Println("usage: missionbase-agent connect TOKEN [--base-url URL] [--name NAME]")
			return nil
		default:
			return fmt.Errorf("unknown connect option %q", args[i])
		}
	}

	payload, err := json.Marshal(map[string]string{
		"token":       token,
		"name":        name,
		"hostname":    hostname,
		"platform":    runtime.GOOS,
		"cli_version": Version,
	})
	if err != nil {
		return err
	}

	requestURL := strings.TrimRight(computerConfig.BaseURL, "/") + "/api/v1/computers/enrollment"
	request, err := http.NewRequest(http.MethodPost, requestURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("create enrollment request: %w", err)
	}
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "missionbase-agent/"+Version)

	client := &http.Client{Timeout: 30 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("enroll computer: %w", err)
	}
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		return fmt.Errorf("read enrollment response: %w", err)
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("computer enrollment failed: HTTP %d", response.StatusCode)
	}

	var enrollment struct {
		Computer struct {
			ID   int    `json:"id"`
			Name string `json:"name"`
		} `json:"computer"`
		Credential string `json:"credential"`
	}
	if err := json.Unmarshal(body, &enrollment); err != nil {
		return fmt.Errorf("decode enrollment response: %w", err)
	}
	if enrollment.Computer.ID == 0 || strings.TrimSpace(enrollment.Credential) == "" {
		return fmt.Errorf("Missionbase returned incomplete computer credentials")
	}

	computerConfig.ComputerID = enrollment.Computer.ID
	computerConfig.Credential = enrollment.Credential
	if err := config.SaveComputer(computerConfig); err != nil {
		return fmt.Errorf("save computer credentials: %w", err)
	}
	fmt.Printf("Connected computer %s (ID %d)\nCredentials: %s\n", enrollment.Computer.Name, enrollment.Computer.ID, config.ComputerCredentialsPath())
	return nil
}
