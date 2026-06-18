package resolve

import (
	"strings"
	"testing"
)

type fakeGetter struct {
	paths []string
	body  []byte
}

func (f *fakeGetter) Get(path string) ([]byte, error) {
	f.paths = append(f.paths, path)
	return f.body, nil
}

func TestStripMentionAndNormalize(t *testing.T) {
	if got := StripMention("@DanielLemky"); got != "DanielLemky" {
		t.Fatalf("StripMention = %q", got)
	}
	if got := NormalizeMention("Daniel-Lemky_42"); got != "daniellemky42" {
		t.Fatalf("NormalizeMention = %q", got)
	}
}

func TestResolveUserIDNumericPassthrough(t *testing.T) {
	getter := &fakeGetter{}
	id, err := ResolveUserID(getter, "42", Options{})
	if err != nil {
		t.Fatalf("ResolveUserID: %v", err)
	}
	if id != "42" {
		t.Fatalf("id = %q", id)
	}
	if len(getter.paths) != 0 {
		t.Fatalf("unexpected HTTP calls: %#v", getter.paths)
	}
}

func TestResolveUserIDRequiresTeamForMention(t *testing.T) {
	_, err := ResolveUserID(&fakeGetter{}, "@DanielLemky", Options{})
	if err == nil || !strings.Contains(err.Error(), "--team") || !strings.Contains(err.Error(), "numeric user id") {
		t.Fatalf("err = %v, want helpful team/numeric error", err)
	}
}

func TestResolveUserIDUsesTeamMembersNotAgentMembers(t *testing.T) {
	getter := &fakeGetter{body: []byte(`{"members":[{"user_id":7,"mention":"DanielLemky"}]}`)}
	id, err := ResolveUserID(getter, "@daniel-lemky", Options{TeamID: "2"})
	if err != nil {
		t.Fatalf("ResolveUserID: %v", err)
	}
	if id != "7" {
		t.Fatalf("id = %q", id)
	}
	if len(getter.paths) != 1 || getter.paths[0] != "/api/v1/teams/2/members" {
		t.Fatalf("paths = %#v, want only team members endpoint", getter.paths)
	}
	for _, path := range getter.paths {
		if strings.Contains(path, "/api/v1/agent/members") {
			t.Fatalf("called agent-only endpoint: %s", path)
		}
	}
}

func TestResolveUserIDAmbiguousMention(t *testing.T) {
	getter := &fakeGetter{body: []byte(`{"members":[{"user_id":7,"mention":"sam"},{"user_id":8,"handle":"sam"}]}`)}
	_, err := ResolveUserID(getter, "@sam", Options{TeamID: "2"})
	if err == nil || !strings.Contains(err.Error(), "multiple team members match") {
		t.Fatalf("err = %v, want ambiguity error", err)
	}
}
