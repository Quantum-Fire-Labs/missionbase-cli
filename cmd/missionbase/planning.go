package main

import (
	"fmt"
	"net/url"
	"strings"
	"time"
)

const dayUsage = "usage: missionbase day <show|add TASK_ID|remove PLACEMENT_ID|order PLACEMENT_ID> --date YYYY-MM-DD [--after-task TASK_ID]"

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
	if err := checkPlanningDate(date); err != nil {
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

func checkPlanningDate(value string) error {
	date, err := time.Parse("2006-01-02", value)
	if err != nil || date.Format("2006-01-02") != value {
		return fmt.Errorf("date must be in YYYY-MM-DD format")
	}
	return nil
}

func planningWrite(command, id, path, afterTask string) error {
	switch command {
	case "add":
		return apiPostJSON(path, map[string]string{"task_id": id})
	case "remove", "order":
		memberPath := path + "/" + url.PathEscape(id)
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
