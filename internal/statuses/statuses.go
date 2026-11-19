// Package statuses holds Missionbase's fixed task status keys. Every project
// has the same six statuses; there are no per-project status ids.
//
// The order is the flow: backlog (not for now: not planned yet, or work that
// shouldn't go ahead at the moment), todo (the queue of work to
// plan: task next hands these out and task start moves them to planning),
// planning (being planned with its owner, not built: the agent asks questions
// and writes the plan into the task) and in_progress (being built, once the
// owner approves the plan). task next never hands out a planning task.
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
