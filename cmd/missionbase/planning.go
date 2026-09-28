package main

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

const weekUsage = "usage: missionbase week <show|add TASK_ID|remove PLACEMENT_ID|order PLACEMENT_ID> --team TEAM_ID [--starts-on YYYY-MM-DD] [--after-task TASK_ID]"
const dayUsage = "usage: missionbase day <show|add TASK_ID|remove PLACEMENT_ID|order PLACEMENT_ID> --date YYYY-MM-DD [--after-task TASK_ID]"

func week(args []string) error {
	command, id, options, err := planningArgs(args, weekUsage, map[string]bool{"--team": true, "--starts-on": true, "--after-task": true})
	if err != nil {
		return err
	}
	if command == "help" {
		return nil
	}
	if options["--team"] == "" {
		return fmt.Errorf("--team is required")
	}
	date := options["--starts-on"]
	if date != "" {
		if err := checkPlanningDate(date, true); err != nil {
			return err
		}
	}
	if options["--after-task"] != "" && command != "order" {
		return fmt.Errorf("--after-task is only valid with order")
	}
	query := url.Values{"team_id": {options["--team"]}}
	if date != "" {
		query.Set("starts_on", date)
	}
	if command == "show" {
		return apiGet("/api/v1/weeks?" + query.Encode())
	}
	if date == "" {
		return fmt.Errorf("--starts-on is required for week changes")
	}
	path := "/api/v1/weeks/" + url.PathEscape(date) + "/placements"
	query.Del("starts_on")
	path += "?" + query.Encode()
	return planningWrite(command, id, path, options["--after-task"])
}

func day(args []string) error {
	command, id, options, err := planningArgs(args, dayUsage, map[string]bool{"--date": true, "--after-task": true})
	if err != nil {
		return err
	}
	if command == "help" {
		return nil
	}
	date := options["--date"]
	if date == "" {
		return fmt.Errorf("--date is required")
	}
	if err := checkPlanningDate(date, false); err != nil {
		return err
	}
	if options["--after-task"] != "" && command != "order" {
		return fmt.Errorf("--after-task is only valid with order")
	}
	path := "/api/v1/days/" + url.PathEscape(date)
	if command == "show" {
		return apiGet(path)
	}
	return planningWrite(command, id, path+"/placements", options["--after-task"])
}

func planningArgs(args []string, usage string, validOptions map[string]bool) (string, string, map[string]string, error) {
	if len(args) == 0 {
		return "", "", nil, fmt.Errorf("%s", usage)
	}
	command := args[0]
	if command == "--help" || command == "-h" {
		fmt.Println(usage)
		return "help", "", nil, nil
	}
	if command != "show" && command != "add" && command != "remove" && command != "order" {
		return "", "", nil, fmt.Errorf("%s", usage)
	}
	index := 1
	id := ""
	if command != "show" {
		if len(args) < 2 || strings.HasPrefix(args[1], "--") || strings.TrimSpace(args[1]) == "" {
			return "", "", nil, fmt.Errorf("%s", usage)
		}
		id = args[1]
		index = 2
	}
	options := make(map[string]string)
	for index < len(args) {
		key := args[index]
		if !validOptions[key] || index+1 >= len(args) || strings.TrimSpace(args[index+1]) == "" || strings.HasPrefix(args[index+1], "--") || options[key] != "" {
			return "", "", nil, fmt.Errorf("%s", usage)
		}
		options[key] = args[index+1]
		index += 2
	}
	return command, id, options, nil
}

func checkPlanningDate(value string, monday bool) error {
	date, err := time.Parse("2006-01-02", value)
	if err != nil || date.Format("2006-01-02") != value {
		return fmt.Errorf("date must be in YYYY-MM-DD format")
	}
	if monday && date.Weekday() != time.Monday {
		return fmt.Errorf("week date must be a Monday")
	}
	return nil
}

func planningWrite(command, id, path, afterTask string) error {
	switch command {
	case "add":
		return apiPostJSON(path, map[string]string{"task_id": id})
	case "remove", "order":
		base, query, hasQuery := strings.Cut(path, "?")
		memberPath := base + "/" + url.PathEscape(id)
		if hasQuery {
			memberPath += "?" + query
		}
		if command == "remove" {
			return apiDelete(memberPath)
		}
		payload := map[string]string{}
		if afterTask != "" {
			payload["after_task_id"] = afterTask
		}
		return apiPatchJSON(memberPath, payload)
	}
	return fmt.Errorf("unknown planning command %q", command)
}
