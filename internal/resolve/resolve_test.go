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
	id, err := ResolveUserID(getter, "42")
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

func TestResolveUserIDRejectsBareName(t *testing.T) {
	_, err := ResolveUserID(&fakeGetter{}, "DanielLemky")
	if err == nil || !strings.Contains(err.Error(), "numeric user id or @mention") {
		t.Fatalf("err = %v, want numeric/mention error", err)
	}
}

func TestResolveUserIDLooksUpHandle(t *testing.T) {
	getter := &fakeGetter{body: []byte(`{"user":{"id":7,"name":"Daniel Lemky","username":"DanielLemky","handle":"DanielLemky"}}`)}
	id, err := ResolveUserID(getter, "@DanielLemky")
	if err != nil {
		t.Fatalf("ResolveUserID: %v", err)
	}
	if id != "7" {
		t.Fatalf("id = %q", id)
	}
	if len(getter.paths) != 1 || getter.paths[0] != "/api/v1/users/lookup?handle=%40DanielLemky" {
		t.Fatalf("paths = %#v, want only the users lookup by handle", getter.paths)
	}
	for _, path := range getter.paths {
		if strings.Contains(path, "team") {
			t.Fatalf("called a team endpoint: %s", path)
		}
	}
}

func TestResolveUserIDNoMatch(t *testing.T) {
	getter := &fakeGetter{body: []byte(`{}`)}
	_, err := ResolveUserID(getter, "@nobody")
	if err == nil || !strings.Contains(err.Error(), "no person found") {
		t.Fatalf("err = %v, want no-match error", err)
	}
}
