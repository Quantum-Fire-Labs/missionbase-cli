// Package statuses holds Missionbase's fixed task status keys. Every project
// has the same six statuses; there are no per-project status ids.
package statuses

import (
	"fmt"
	"strings"
)

// Keys are the task status keys, in display order.
var Keys = []string{"backlog", "todo", "in_progress", "needs_attention", "complete", "not_doing"}

// RemovedIDFlag explains that a task status id flag (--task-status-id or
// --task-status-ids) no longer exists and what to use instead.
func RemovedIDFlag(flag string) error {
	replacement := "--status KEY"
	if strings.HasSuffix(flag, "ids") {
		replacement = "--status KEY[,KEY] or --status-category open|done|canceled"
	}
	return fmt.Errorf("%s was removed: task statuses are fixed keys (%s); use %s", flag, strings.Join(Keys, ", "), replacement)
}
