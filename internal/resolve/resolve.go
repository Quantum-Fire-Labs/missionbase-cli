package resolve

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type Getter interface {
	Get(path string) ([]byte, error)
}

type User struct {
	ID       int    `json:"id"`
	UserID   int    `json:"user_id"`
	Name     string `json:"name"`
	Email    string `json:"email"`
	Mention  string `json:"mention"`
	Handle   string `json:"handle"`
	Username string `json:"username"`
}

type Options struct {
	TeamID string
}

func StripMention(value string) string {
	return strings.TrimPrefix(strings.TrimSpace(value), "@")
}

func NumericUserID(value string) (string, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", false
	}
	if _, err := strconv.Atoi(value); err != nil {
		return "", false
	}
	return value, true
}

func ResolveUserID(client Getter, value string, opts Options) (string, error) {
	if id, ok := NumericUserID(value); ok {
		return id, nil
	}
	mention := StripMention(value)
	if mention == "" || mention == strings.TrimSpace(value) {
		return "", fmt.Errorf("--user requires a numeric user id or @mention")
	}
	if strings.TrimSpace(opts.TeamID) == "" {
		return "", fmt.Errorf("team context is required to resolve %s; pass --team <team-id> or use a numeric user id", value)
	}
	users, err := TeamMembers(client, opts.TeamID)
	if err != nil {
		return "", err
	}
	return MatchUserID(users, mention, value)
}

func TeamMembers(client Getter, teamID string) ([]User, error) {
	body, err := client.Get("/api/v1/teams/" + url.PathEscape(teamID) + "/members")
	if err != nil {
		return nil, err
	}
	return ParseUsers(body)
}

func MatchUserID(users []User, mention, original string) (string, error) {
	normalized := NormalizeMention(mention)
	var matches []string
	seen := map[string]bool{}
	for _, user := range users {
		id := user.UserID
		if id == 0 {
			id = user.ID
		}
		if id == 0 {
			continue
		}
		candidates := []string{user.Mention, user.Handle, user.Username, user.Name, strings.Split(user.Email, "@")[0]}
		for _, candidate := range candidates {
			if NormalizeMention(StripMention(candidate)) == normalized {
				idString := strconv.Itoa(id)
				if !seen[idString] {
					matches = append(matches, idString)
					seen[idString] = true
				}
				break
			}
		}
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("no team member found for %s", original)
	}
	if len(matches) > 1 {
		return "", fmt.Errorf("multiple team members match %s; pass a numeric user id", original)
	}
	return matches[0], nil
}

func ParseUsers(body []byte) ([]User, error) {
	var response struct {
		Users   []User `json:"users"`
		Members []User `json:"members"`
		User    User   `json:"user"`
		Member  User   `json:"member"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, err
	}
	if len(response.Users) > 0 {
		return response.Users, nil
	}
	if len(response.Members) > 0 {
		return response.Members, nil
	}
	if response.User.ID != 0 || response.User.UserID != 0 {
		return []User{response.User}, nil
	}
	if response.Member.ID != 0 || response.Member.UserID != 0 {
		return []User{response.Member}, nil
	}
	return nil, nil
}

func NormalizeMention(value string) string {
	value = strings.ToLower(value)
	var b strings.Builder
	for _, r := range value {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		}
	}
	return b.String()
}
