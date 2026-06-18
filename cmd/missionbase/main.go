package main

import (
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"

	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/config"
	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/httpclient"
	"github.com/Quantum-Fire-Labs/missionbase-cli/internal/resolve"
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
		return apiGet("/api/v1/users/me")
	case "teams":
		return teams(args[1:])
	case "users":
		return users(args[1:])
	case "team":
		return team(args[1:])
	case "boxes":
		return boxes(args[1:])
	case "box":
		return box(args[1:])
	case "tasks":
		return tasks(args[1:])
	case "task":
		return task(args[1:])
	case "conversations":
		return conversations(args[1:])
	case "conversation":
		return conversation(args[1:])
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

func teams(args []string) error {
	if len(args) > 0 {
		if len(args) == 1 && isHelp(args[0]) {
			fmt.Println("usage: missionbase teams")
			return nil
		}
		return fmt.Errorf("usage: missionbase teams")
	}
	return apiGet("/api/v1/teams")
}

func users(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: missionbase users lookup <query-or-mention> [--team <team-id>]")
	}
	if args[0] != "lookup" {
		if isHelp(args[0]) {
			fmt.Println("usage: missionbase users lookup <query-or-mention> [--team <team-id>]")
			return nil
		}
		return fmt.Errorf("unknown users command %q", args[0])
	}
	return usersLookup(args[1:])
}

func usersLookup(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: missionbase users lookup <query-or-mention> [--team <team-id>]")
	}
	query := args[0]
	teamID := ""
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--team":
			if i+1 >= len(args) {
				return fmt.Errorf("--team requires a value")
			}
			teamID = args[i+1]
			i++
		case "--help", "-h":
			fmt.Println("usage: missionbase users lookup <query-or-mention> [--team <team-id>]")
			return nil
		default:
			return fmt.Errorf("unknown users lookup option %q", args[i])
		}
	}

	cfg, err := config.LoadUser()
	if err != nil {
		return err
	}
	client := httpclient.NewUser(cfg)
	if strings.HasPrefix(strings.TrimSpace(query), "@") {
		if teamID == "" {
			return fmt.Errorf("team context is required to resolve %s; pass --team <team-id> or use a numeric user id", query)
		}
		users, err := resolve.TeamMembers(client, teamID)
		if err != nil {
			return err
		}
		id, err := resolve.MatchUserID(users, resolve.StripMention(query), query)
		if err != nil {
			return err
		}
		body, err := json.Marshal(map[string]string{"user_id": id})
		if err != nil {
			return err
		}
		fmt.Println(string(body))
		return nil
	}
	values := url.Values{}
	values.Set("query", query)
	if teamID != "" {
		values.Set("team_id", teamID)
	}
	body, err := client.Get(withQuery("/api/v1/users/lookup", values))
	if err != nil {
		return err
	}
	fmt.Println(string(body))
	return nil
}

func team(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: missionbase team <show|members> <team-id>")
	}
	switch args[0] {
	case "show":
		if len(args) != 2 {
			return fmt.Errorf("usage: missionbase team show <team-id>")
		}
		return apiGet("/api/v1/teams/" + url.PathEscape(args[1]))
	case "members":
		if len(args) != 2 {
			return fmt.Errorf("usage: missionbase team members <team-id>")
		}
		return apiGet("/api/v1/teams/" + url.PathEscape(args[1]) + "/members")
	case "--help", "-h":
		fmt.Println("usage: missionbase team <show|members> <team-id>")
		return nil
	default:
		return fmt.Errorf("unknown team command %q", args[0])
	}
}

func boxes(args []string) error {
	if len(args) == 0 || strings.HasPrefix(args[0], "--") {
		return listBoxes(args)
	}
	switch args[0] {
	case "tasks":
		return boxTasks(args[1:])
	case "discussions":
		return boxDiscussions(args[1:])
	case "statuses", "task-statuses":
		return boxTaskStatuses(args[1:])
	case "--help", "-h":
		fmt.Println("usage: missionbase boxes [--team TEAM_ID]\n       missionbase boxes <tasks|discussions|statuses|task-statuses> <box-id>")
		return nil
	default:
		return fmt.Errorf("unknown boxes command %q", args[0])
	}
}

func listBoxes(args []string) error {
	values := url.Values{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--team":
			if i+1 >= len(args) {
				return fmt.Errorf("--team requires a value")
			}
			values.Set("team_id", args[i+1])
			i++
		case "--help", "-h":
			fmt.Println("usage: missionbase boxes [--team TEAM_ID]")
			return nil
		default:
			return fmt.Errorf("unknown boxes option %q", args[i])
		}
	}
	return apiGet(withQuery("/api/v1/boxes", values))
}

func box(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: missionbase box show <box-id>")
	}
	switch args[0] {
	case "show":
		if len(args) != 2 {
			return fmt.Errorf("usage: missionbase box show <box-id>")
		}
		return apiGet("/api/v1/boxes/" + url.PathEscape(args[1]))
	case "--help", "-h":
		fmt.Println("usage: missionbase box show <box-id>")
		return nil
	default:
		return fmt.Errorf("unknown box command %q", args[0])
	}
}

func boxTasks(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: missionbase boxes tasks <box-id> [--status STATUS] [--status-category open|done|canceled] [--task-status-ids IDS] [--page N] [--per-page N]")
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
			if !isStatusCategory(args[i+1]) {
				return fmt.Errorf("--status-category must be one of: open, done, canceled")
			}
			values.Set("status_category", args[i+1])
			i++
		case "--task-status-ids":
			if i+1 >= len(args) {
				return fmt.Errorf("--task-status-ids requires a value")
			}
			values.Set("task_status_ids", args[i+1])
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
		case "--help", "-h":
			fmt.Println("usage: missionbase boxes tasks <box-id> [--status STATUS] [--status-category open|done|canceled] [--task-status-ids IDS] [--page N] [--per-page N]")
			return nil
		default:
			return fmt.Errorf("unknown boxes tasks option %q", args[i])
		}
	}
	return apiGet(withQuery("/api/v1/boxes/"+url.PathEscape(boxID)+"/tasks", values))
}

func boxDiscussions(args []string) error {
	if len(args) < 1 {
		return fmt.Errorf("usage: missionbase boxes discussions <box-id> [--page N] [--per-page N]")
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
			fmt.Println("usage: missionbase boxes discussions <box-id> [--page N] [--per-page N]")
			return nil
		default:
			return fmt.Errorf("unknown boxes discussions option %q", args[i])
		}
	}
	return apiGet(withQuery("/api/v1/boxes/"+url.PathEscape(boxID)+"/discussions", values))
}

func boxTaskStatuses(args []string) error {
	if len(args) == 1 && isHelp(args[0]) {
		fmt.Println("usage: missionbase boxes task-statuses <box-id>")
		return nil
	}
	if len(args) != 1 {
		return fmt.Errorf("usage: missionbase boxes task-statuses <box-id>")
	}
	return apiGet("/api/v1/boxes/" + url.PathEscape(args[0]) + "/task_statuses")
}

func tasks(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: missionbase tasks <assigned|visible> [--page N] [--per-page N]")
	}
	command := args[0]
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
			fmt.Println("usage: missionbase tasks <assigned|visible> [--page N] [--per-page N]")
			return nil
		default:
			return fmt.Errorf("unknown tasks option %q", args[i])
		}
	}
	switch command {
	case "assigned":
		return apiGet(withQuery("/api/v1/tasks/assigned", values))
	case "visible":
		return apiGet(withQuery("/api/v1/tasks", values))
	case "--help", "-h":
		fmt.Println("usage: missionbase tasks <assigned|visible> [--page N] [--per-page N]")
		return nil
	default:
		return fmt.Errorf("unknown tasks command %q", command)
	}
}

func task(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: missionbase task <assign|unassign|participants|show|feed|comments> ...")
	}
	switch args[0] {
	case "assign":
		return taskAssign(args[1:])
	case "unassign":
		return taskUnassign(args[1:])
	case "participants":
		return taskParticipants(args[1:])
	case "show":
		if len(args) != 2 {
			return fmt.Errorf("usage: missionbase task show <task-id>")
		}
		return apiGet("/api/v1/tasks/" + url.PathEscape(args[1]))
	case "feed", "comments":
		if len(args) < 2 {
			return fmt.Errorf("usage: missionbase task %s <task-id> [--limit N]", args[0])
		}
		path, err := appendLimit("/api/v1/tasks/"+url.PathEscape(args[1])+"/comments", args[2:])
		if err != nil {
			return err
		}
		return apiGet(path)
	case "--help", "-h":
		fmt.Println("usage: missionbase task <assign|unassign|participants|show|feed|comments> ...")
		return nil
	default:
		return fmt.Errorf("unknown task command %q", args[0])
	}
}

func taskAssign(args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: missionbase task assign <task-id> --user ID|@mention [--team ID]")
	}
	taskID := args[0]
	userValue := ""
	teamID := ""
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--user":
			if i+1 >= len(args) {
				return fmt.Errorf("--user requires a value")
			}
			userValue = args[i+1]
			i++
		case "--team":
			if i+1 >= len(args) {
				return fmt.Errorf("--team requires a value")
			}
			teamID = args[i+1]
			i++
		case "--help", "-h":
			fmt.Println("usage: missionbase task assign <task-id> --user ID|@mention [--team ID]")
			return nil
		default:
			return fmt.Errorf("unknown task assign option %q", args[i])
		}
	}
	if userValue == "" {
		return fmt.Errorf("--user is required")
	}
	return taskUserAssignment(taskID, userValue, teamID, true)
}

func taskUnassign(args []string) error {
	if len(args) < 3 {
		return fmt.Errorf("usage: missionbase task unassign <task-id> --user ID|@mention [--team ID]")
	}
	taskID := args[0]
	userValue := ""
	teamID := ""
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--user":
			if i+1 >= len(args) {
				return fmt.Errorf("--user requires a value")
			}
			userValue = args[i+1]
			i++
		case "--team":
			if i+1 >= len(args) {
				return fmt.Errorf("--team requires a value")
			}
			teamID = args[i+1]
			i++
		case "--help", "-h":
			fmt.Println("usage: missionbase task unassign <task-id> --user ID|@mention [--team ID]")
			return nil
		default:
			return fmt.Errorf("unknown task unassign option %q", args[i])
		}
	}
	if userValue == "" {
		return fmt.Errorf("--user is required")
	}
	return taskUserAssignment(taskID, userValue, teamID, false)
}

func taskUserAssignment(taskID, userValue, teamID string, assign bool) error {
	cfg, err := config.LoadUser()
	if err != nil {
		return err
	}
	client := httpclient.NewUser(cfg)
	if _, ok := resolve.NumericUserID(userValue); !ok && teamID == "" {
		teamID, _ = taskTeamID(client, taskID)
	}
	userID, err := resolve.ResolveUserID(client, userValue, resolve.Options{TeamID: teamID})
	if err != nil {
		return err
	}
	if assign {
		body, err := json.Marshal(map[string]string{"user_id": userID})
		if err != nil {
			return err
		}
		return apiPost("/api/v1/tasks/"+url.PathEscape(taskID)+"/assignments", body)
	}
	return apiDelete("/api/v1/tasks/" + url.PathEscape(taskID) + "/assignments/" + url.PathEscape(userID))
}

func taskParticipants(args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: missionbase task participants <list|add> <task-id> [--user ID|@mention] [--team ID]")
	}
	command := args[0]
	taskID := args[1]
	switch command {
	case "list":
		if len(args) != 2 {
			return fmt.Errorf("usage: missionbase task participants list <task-id>")
		}
		return apiGet("/api/v1/tasks/" + url.PathEscape(taskID) + "/participants")
	case "add":
		return taskParticipantsAdd(taskID, args[2:])
	default:
		return fmt.Errorf("unknown task participants command %q", command)
	}
}

func taskParticipantsAdd(taskID string, args []string) error {
	if len(args) < 2 {
		return fmt.Errorf("usage: missionbase task participants add <task-id> --user ID|@mention [--team ID]")
	}
	userValue := ""
	teamID := ""
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--user":
			if i+1 >= len(args) {
				return fmt.Errorf("--user requires a value")
			}
			userValue = args[i+1]
			i++
		case "--team":
			if i+1 >= len(args) {
				return fmt.Errorf("--team requires a value")
			}
			teamID = args[i+1]
			i++
		case "--help", "-h":
			fmt.Println("usage: missionbase task participants add <task-id> --user ID|@mention [--team ID]")
			return nil
		default:
			return fmt.Errorf("unknown task participants add option %q", args[i])
		}
	}
	if userValue == "" {
		return fmt.Errorf("--user is required")
	}
	cfg, err := config.LoadUser()
	if err != nil {
		return err
	}
	client := httpclient.NewUser(cfg)
	if _, ok := resolve.NumericUserID(userValue); !ok && teamID == "" {
		teamID, _ = taskTeamID(client, taskID)
	}
	userID, err := resolve.ResolveUserID(client, userValue, resolve.Options{TeamID: teamID})
	if err != nil {
		return err
	}
	body, err := json.Marshal(map[string]string{"user_id": userID})
	if err != nil {
		return err
	}
	return apiPost("/api/v1/tasks/"+url.PathEscape(taskID)+"/participants", body)
}

func taskTeamID(client httpclient.Client, taskID string) (string, error) {
	body, err := client.Get("/api/v1/tasks/" + url.PathEscape(taskID))
	if err != nil {
		return "", err
	}
	var response struct {
		Task struct {
			TeamID int `json:"team_id"`
			Box    struct {
				TeamID int `json:"team_id"`
				Team   struct {
					ID int `json:"id"`
				} `json:"team"`
			} `json:"box"`
		} `json:"task"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", err
	}
	id := response.Task.TeamID
	if id == 0 {
		id = response.Task.Box.TeamID
	}
	if id == 0 {
		id = response.Task.Box.Team.ID
	}
	if id == 0 {
		return "", fmt.Errorf("team context is required; pass --team <team-id> or use a numeric user id")
	}
	return fmt.Sprintf("%d", id), nil
}

func conversations(args []string) error {
	values := url.Values{}
	for i := 0; i < len(args); i++ {
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
			fmt.Println("usage: missionbase conversations [--page N] [--per-page N]")
			return nil
		default:
			return fmt.Errorf("unknown conversations option %q", args[i])
		}
	}
	return apiGet(withQuery("/api/v1/conversations", values))
}

func conversation(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: missionbase conversation show <feed-id> [--limit N]")
	}
	switch args[0] {
	case "show":
		if len(args) < 2 {
			return fmt.Errorf("usage: missionbase conversation show <feed-id> [--limit N]")
		}
		path, err := appendLimit("/api/v1/conversations/"+url.PathEscape(args[1]), args[2:])
		if err != nil {
			return err
		}
		return apiGet(path)
	case "--help", "-h":
		fmt.Println("usage: missionbase conversation show <feed-id> [--limit N]")
		return nil
	default:
		return fmt.Errorf("unknown conversation command %q", args[0])
	}
}

func appendLimit(path string, args []string) (string, error) {
	values := url.Values{}
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--limit":
			if i+1 >= len(args) {
				return "", fmt.Errorf("--limit requires a value")
			}
			values.Set("limit", args[i+1])
			i++
		case "--help", "-h":
			return "", fmt.Errorf("usage includes optional [--limit N]")
		default:
			return "", fmt.Errorf("unknown option %q", args[i])
		}
	}
	return withQuery(path, values), nil
}

func apiPost(path string, requestBody []byte) error {
	cfg, err := config.LoadUser()
	if err != nil {
		return err
	}
	client := httpclient.NewUser(cfg)
	body, err := client.Post(path, requestBody)
	if err != nil {
		return err
	}
	fmt.Println(string(body))
	return nil
}

func apiDelete(path string) error {
	cfg, err := config.LoadUser()
	if err != nil {
		return err
	}
	client := httpclient.NewUser(cfg)
	body, err := client.Delete(path)
	if err != nil {
		return err
	}
	fmt.Println(string(body))
	return nil
}

func apiGet(path string) error {
	cfg, err := config.LoadUser()
	if err != nil {
		return err
	}
	client := httpclient.NewUser(cfg)
	body, err := client.Get(path)
	if err != nil {
		return err
	}
	fmt.Println(string(body))
	return nil
}

func withQuery(path string, values url.Values) string {
	if encoded := values.Encode(); encoded != "" {
		return path + "?" + encoded
	}
	return path
}

func isStatusCategory(value string) bool {
	return value == "open" || value == "done" || value == "canceled"
}

func isHelp(value string) bool {
	return value == "--help" || value == "-h"
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
  teams                               List teams visible to the current user
  users lookup <query-or-mention> [--team <team-id>]
                                      Look up users or resolve a team @mention
  team show <team-id>                 Show a team
  team members <team-id>              List team members
  boxes [--team TEAM_ID]              List boxes visible to the current user
  box show <box-id>                   Show a box
  boxes tasks <box-id>                List tasks in a box
      [--status STATUS] [--status-category open|done|canceled] [--task-status-ids IDS]
      [--page N] [--per-page N]
  boxes discussions <box-id>          List standalone box discussions
      [--page N] [--per-page N]
  boxes statuses <box-id>             Alias for boxes task-statuses
  boxes task-statuses <box-id>        List configured task statuses for a box
  tasks assigned                      List tasks assigned to the current user
      [--page N] [--per-page N]
  tasks visible                       List tasks visible to the current user
      [--page N] [--per-page N]
  task assign <task-id> --user ID|@mention [--team ID]
                                      Assign a task to a user
  task unassign <task-id> --user ID|@mention [--team ID]
                                      Remove a user assignment from a task
  task participants list <task-id>    List task participants
  task participants add <task-id> --user ID|@mention [--team ID]
                                      Add a user task participant
  task show <task-id>                 Show a task
  task feed <task-id> [--limit N]     Show a task feed and comments
  task comments <task-id> [--limit N] Alias for task feed
  conversations [--page N] [--per-page N]
                                      List conversations visible to the current user
  conversation show <feed-id> [--limit N]
                                      Show a conversation/feed
  get /api/path                       GET an API path and print JSON
  update [--check] [--force]          Update this CLI from GitHub Releases
  version                             Show CLI version

For agent acting, use missionbase-agent.
Default base URL: https://dash.missionbase.app`)
}
