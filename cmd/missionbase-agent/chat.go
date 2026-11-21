package main

// Project chat (Missionbase #368): the owner's one chat per project with the
// project's orchestrator, the agent watching it (`project watch`). The Herdr
// bridge types the owner's messages into the watching terminal; the agent
// reads the chat with `project chat show` and answers with `project chat
// reply`, as its own identity.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const projectChatUsage = `usage: missionbase-agent project chat show [ID] [--limit N] [--json]
       missionbase-agent project chat reply [ID] --body-file PATH [--json]

  show    The chat's latest messages (default 20, at most 100), oldest first,
          and who's listening. Every file sent with a message (any type) is saved under
          ~/.cache/missionbase/chats/<project>/ and listed under the message with its
          name, type and path.
  reply   Post a Markdown reply as this agent. The owner gets a push notification.

The project is ID, else --project ID, --cwd DIR or the linked project here.`

type chatSender struct {
	Type   string `json:"type"`
	ID     int64  `json:"id"`
	Name   string `json:"name"`
	Handle string `json:"handle"`
}

type chatMessage struct {
	ID               int64              `json:"id"`
	Body             string             `json:"body"`
	Sender           chatSender         `json:"sender"`
	Files            []answerAttachment `json:"files"`
	CreatedAt        string             `json:"created_at"`
	DeliveredAt      *string            `json:"delivered_at"`
	AwaitingDelivery bool               `json:"awaiting_delivery"`
}

type chatWatch struct {
	Label     string `json:"label"`
	Terminal  string `json:"terminal"`
	Listening bool   `json:"listening"`
	Machine   struct {
		Name string `json:"name"`
	} `json:"machine"`
}

type chatResponse struct {
	Chat struct {
		ID      *int64 `json:"id"`
		Project struct {
			ID   int64  `json:"id"`
			Name string `json:"name"`
		} `json:"project"`
		Listening bool       `json:"listening"`
		Watch     *chatWatch `json:"watch"`
	} `json:"chat"`
	Messages []chatMessage `json:"messages"`
	HasMore  bool          `json:"has_more"`
}

func projectChatCommand(args []string) error {
	if len(args) == 0 || args[0] == "--help" || args[0] == "-h" || args[0] == "help" {
		fmt.Println(projectChatUsage)
		return nil
	}
	switch args[0] {
	case "show":
		return projectChatShow(args[1:])
	case "reply":
		return projectChatReply(args[1:])
	default:
		return fmt.Errorf("unknown project chat command %q\n%s", args[0], projectChatUsage)
	}
}

// chatProject reads [ID] and the project flags, passing every other argument to option.
func chatProject(args []string, option func(args []string, i *int) (bool, error)) (string, error) {
	var pf projectFlags
	id := ""
	for i := 0; i < len(args); i++ {
		if handled, err := pf.parse(args, &i); handled {
			if err != nil {
				return "", err
			}
			continue
		}
		if handled, err := option(args, &i); handled {
			if err != nil {
				return "", err
			}
			continue
		}
		switch {
		case args[i] == "--help" || args[i] == "-h":
			return "", errShowUsage
		case !strings.HasPrefix(args[i], "-") && id == "":
			if _, err := strconv.ParseInt(args[i], 10, 64); err != nil {
				return "", fmt.Errorf("project id must be a number, got %q", args[i])
			}
			id = args[i]
		default:
			return "", fmt.Errorf("unknown project chat option %q\n%s", args[i], projectChatUsage)
		}
	}
	if id != "" {
		if pf.project != "" || pf.cwd != "" {
			return "", fmt.Errorf("give the project once: ID, --project or --cwd")
		}
		return id, nil
	}
	if err := pf.validate(); err != nil {
		return "", err
	}
	project, ok, err := resolveProject(pf.project, pf.cwd)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", errNoProject()
	}
	return project.ID, nil
}

var errShowUsage = fmt.Errorf("usage")

func chatPath(projectID string) string {
	return "/api/v1/projects/" + url.PathEscape(projectID) + "/chat"
}

func projectChatShow(args []string) error {
	limit, jsonOut := "", false
	id, err := chatProject(args, func(args []string, i *int) (bool, error) {
		switch args[*i] {
		case "--json":
			jsonOut = true
			return true, nil
		case "--limit":
			if *i+1 >= len(args) {
				return true, fmt.Errorf("--limit requires a number from 1 to 100")
			}
			n, err := strconv.Atoi(args[*i+1])
			if err != nil || n < 1 || n > 100 {
				return true, fmt.Errorf("--limit requires a number from 1 to 100, got %q", args[*i+1])
			}
			limit = args[*i+1]
			*i++
			return true, nil
		}
		return false, nil
	})
	if err == errShowUsage {
		fmt.Println(projectChatUsage)
		return nil
	}
	if err != nil {
		return err
	}

	path := chatPath(id)
	if limit != "" {
		path += "?" + url.Values{"limit": {limit}}.Encode()
	}
	body, err := agentDo(http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	if jsonOut {
		fmt.Println(strings.TrimSpace(string(body)))
		return nil
	}
	var response chatResponse
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("unexpected project chat response: %w", err)
	}
	downloadChatFiles(id, response.Messages)
	printChat(id, response)
	return nil
}

func printChat(id string, response chatResponse) {
	chat := response.Chat
	name := chat.Project.Name
	if name == "" {
		name = "project " + id
	}
	listening := "No orchestrator listening"
	if w := chat.Watch; w != nil {
		where := w.Label
		if w.Machine.Name != "" {
			where += " on " + w.Machine.Name
		}
		if chat.Listening {
			listening = "Listening: " + where
		} else {
			listening = "No orchestrator listening (" + where + " isn't heard from)"
		}
	}
	fmt.Printf("%s chat · %s\n", name, listening)
	if len(response.Messages) == 0 {
		fmt.Println("No messages yet.")
		return
	}
	if response.HasMore {
		fmt.Println("(older messages not shown; use --limit up to 100)")
	}
	for _, m := range response.Messages {
		who := m.Sender.Name
		if m.Sender.Type == "Agent" {
			who += " (agent)"
		}
		state := ""
		if m.AwaitingDelivery {
			state = " · not yet delivered"
		}
		fmt.Printf("\n[%s] %s, message %d%s:\n", chatTime(m.CreatedAt), who, m.ID, state)
		if body := strings.TrimRight(m.Body, "\n"); body != "" {
			for _, line := range strings.Split(body, "\n") {
				fmt.Println("  " + line)
			}
		}
		printChatFiles(m.Files)
	}
	fmt.Printf("\nReply with: missionbase-agent project chat reply %s --body-file PATH\n", id)
}

func chatTime(value string) string {
	t, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return value
	}
	return t.Local().Format("2006-01-02 15:04")
}

func projectChatReply(args []string) error {
	body, haveBody, jsonOut := "", false, false
	id, err := chatProject(args, func(args []string, i *int) (bool, error) {
		switch args[*i] {
		case "--json":
			jsonOut = true
			return true, nil
		case "--body", "--message", "--body-stdin":
			return true, fmt.Errorf("%s is not supported; use --body-file PATH", args[*i])
		case "--body-file":
			if *i+1 >= len(args) {
				return true, fmt.Errorf("--body-file requires a file path")
			}
			text, err := readBodyFile(args[*i+1])
			if err != nil {
				return true, err
			}
			body, haveBody = text, true
			*i++
			return true, nil
		}
		return false, nil
	})
	if err == errShowUsage {
		fmt.Println(projectChatUsage)
		return nil
	}
	if err != nil {
		return err
	}
	if !haveBody {
		return fmt.Errorf("--body-file PATH is required")
	}
	body = strings.TrimSpace(normalizeAgentAuthoredBody(body))
	if body == "" {
		return fmt.Errorf("the reply is empty")
	}

	response, err := agentDo(http.MethodPost, chatPath(id)+"/messages", map[string]string{"body": body})
	if err != nil {
		return err
	}
	if jsonOut {
		fmt.Println(strings.TrimSpace(string(response)))
		return nil
	}
	var posted struct {
		Chat struct {
			Project struct {
				Name string `json:"name"`
			} `json:"project"`
		} `json:"chat"`
		Message struct {
			ID int64 `json:"id"`
		} `json:"message"`
	}
	if err := json.Unmarshal(response, &posted); err != nil {
		return fmt.Errorf("unexpected project chat response: %w", err)
	}
	name := posted.Chat.Project.Name
	if name == "" {
		name = "project " + id
	}
	fmt.Printf("Replied in the %s chat (message %d)\n", name, posted.Message.ID)
	return nil
}
