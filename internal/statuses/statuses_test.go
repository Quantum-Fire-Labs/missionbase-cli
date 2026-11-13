package statuses

import (
	"strings"
	"testing"
)

func TestRemovedIDFlagPointsToStatusKeys(t *testing.T) {
	single := RemovedIDFlag("--task-status-id").Error()
	if !strings.Contains(single, "--task-status-id was removed") || !strings.Contains(single, "use --status KEY") || strings.Contains(single, "--status-category") {
		t.Fatalf("single = %q", single)
	}
	plural := RemovedIDFlag("--task-status-ids").Error()
	if !strings.Contains(plural, "use --status KEY[,KEY] or --status-category open|done|canceled") {
		t.Fatalf("plural = %q", plural)
	}
	if !strings.Contains(plural, "backlog, todo, in_progress, needs_attention, complete, not_doing") {
		t.Fatalf("plural = %q, want the fixed keys", plural)
	}
}
