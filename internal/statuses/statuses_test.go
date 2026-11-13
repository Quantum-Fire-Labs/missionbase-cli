package statuses

import (
	"strings"
	"testing"
)

func TestKeysPutPlanningBetweenBacklogAndTodo(t *testing.T) {
	want := "backlog planning todo in_progress complete not_doing"
	if got := strings.Join(Keys, " "); got != want {
		t.Fatalf("Keys = %q, want %q", got, want)
	}
}

func TestRemovedIDFlagPointsToStatusKeys(t *testing.T) {
	single := RemovedIDFlag("--task-status-id").Error()
	if !strings.Contains(single, "--task-status-id was removed") || !strings.Contains(single, "use --status KEY") || strings.Contains(single, "--status-category") {
		t.Fatalf("single = %q", single)
	}
	plural := RemovedIDFlag("--task-status-ids").Error()
	if !strings.Contains(plural, "use --status KEY[,KEY] or --status-category open|done|canceled") {
		t.Fatalf("plural = %q", plural)
	}
	if !strings.Contains(plural, "backlog, planning, todo, in_progress, complete, not_doing") {
		t.Fatalf("plural = %q, want the fixed keys", plural)
	}
}

func TestNeedsAttentionIsRefusedToSetAndFilter(t *testing.T) {
	if err := RefuseSet("needs_attention"); err == nil || !strings.Contains(err.Error(), "task ask") {
		t.Fatalf("RefuseSet = %v", err)
	}
	for _, key := range []string{"planning", "todo", "complete", "bogus"} {
		if err := RefuseSet(key); err != nil {
			t.Fatalf("RefuseSet(%q) = %v, want nil (the server validates other keys)", key, err)
		}
	}
	if err := RefuseFilter("todo, needs_attention", "--needs-you"); err == nil || !strings.Contains(err.Error(), "use --needs-you") {
		t.Fatalf("RefuseFilter = %v", err)
	}
	if err := RefuseFilter("planning,todo", "--needs-you"); err != nil {
		t.Fatalf("RefuseFilter(planning,todo) = %v", err)
	}
}
