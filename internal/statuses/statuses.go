// Package statuses holds Missionbase's fixed task status keys. Every project
// has the same six statuses; there are no per-project status ids.
//
// The order is the flow: backlog (not for now: not planned yet, or work that
// shouldn't go ahead at the moment), todo (the queue of work to
// plan: task next hands these out and task start moves them to planning),
// planning (being planned with its owner, not built: the agent asks questions
// and writes the plan into the task) and in_progress (being built, once the
// owner approves the plan). task next never hands out a planning task.
// A todo or planning task has a planning mode: questions (the default) or
// session (planned one-on-one with the owner in a Herdr tab; see
// PlanningSession). The owner sets session in todo; task start keeps it.
//
// "Needs you" is not a status: a task needs its owner while it has open
// questions or a live permission prompt (needs_you in task payloads).
// needs_attention was a status until 2026-11 and is refused here.
package statuses

import (
	"fmt"
	"strings"
)

// Keys are the task status keys, in display order.
var Keys = []string{"backlog", "todo", "planning", "in_progress", "complete", "not_doing"}

// NeedsAttention is the removed status key.
const NeedsAttention = "needs_attention"

// RefuseSet rejects setting a removed status, pointing to task ask instead.
// Other keys are passed through for the server to validate.
func RefuseSet(status string) error {
	if strings.TrimSpace(status) != NeedsAttention {
		return nil
	}
	return fmt.Errorf("needs_attention is no longer a status: ask the task's owner with `missionbase-agent task ask <task-id> \"question\"`; the task needs you (needs_you) while questions are open, and its status stays as it is")
}

// RefuseFilter rejects filtering by a removed status in a comma-separated list,
// pointing to the given needs-you flag instead.
func RefuseFilter(statusList, needsYouFlag string) error {
	for _, key := range strings.Split(statusList, ",") {
		if strings.TrimSpace(key) == NeedsAttention {
			return fmt.Errorf("needs_attention is no longer a status: use %s to list tasks waiting on their owner (open questions or a permission prompt)", needsYouFlag)
		}
	}
	return nil
}

// RemovedIDFlag explains that a task status id flag (--task-status-id or
// --task-status-ids) no longer exists and what to use instead.
func RemovedIDFlag(flag string) error {
	replacement := "--status KEY"
	if strings.HasSuffix(flag, "ids") {
		replacement = "--status KEY[,KEY] or --status-category open|done|canceled"
	}
	return fmt.Errorf("%s was removed: task statuses are fixed keys (%s); use %s", flag, strings.Join(Keys, ", "), replacement)
}

// Planning modes: how a task is planned with its owner (#346).
// "questions" (the default) asks the owner Missionbase Questions; "session"
// plans it one-on-one with an agent in a Herdr tab, without Questions.
const (
	PlanningQuestions = "questions"
	PlanningSession   = "session"
)

// StatusArgs parses `task status <task-id> <status> [--session | --questions]`
// and returns the task id and the PATCH payload: {"status": KEY}, plus
// "planning_mode" when --session or --questions is given (the server refuses
// session outside planning). usage is the command's usage line.
func StatusArgs(args []string, usage string) (string, map[string]string, error) {
	var positional []string
	mode := ""
	for _, arg := range args {
		switch arg {
		case "--session", "--questions":
			next := strings.TrimPrefix(arg, "--")
			if mode != "" && mode != next {
				return "", nil, fmt.Errorf("use --session or --questions, not both\n%s", usage)
			}
			mode = next
		default:
			if strings.HasPrefix(arg, "--") {
				return "", nil, fmt.Errorf("unknown task status option %q\n%s", arg, usage)
			}
			positional = append(positional, arg)
		}
	}
	if len(positional) != 2 {
		return "", nil, fmt.Errorf("%s", usage)
	}
	taskID, status := positional[0], strings.TrimSpace(positional[1])
	if status == "" {
		return "", nil, fmt.Errorf("status is required")
	}
	if err := RefuseSet(status); err != nil {
		return "", nil, err
	}
	payload := map[string]string{"status": status}
	if mode != "" {
		payload["planning_mode"] = mode
	}
	return taskID, payload, nil
}
