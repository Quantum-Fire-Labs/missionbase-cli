package textbody

import "testing"

func TestNormalizeConvertsEscapedNewlines(t *testing.T) {
	got := Normalize(`first\nsecond\r\nthird`)
	want := "first\nsecond\nthird"
	if got != want {
		t.Fatalf("Normalize() = %q, want %q", got, want)
	}
}

func TestNormalizePreservesCodeAndQuotedContexts(t *testing.T) {
	input := "before\\nafter `code\\nvalue` \"quoted\\nvalue\"\n```\nfenced\\nvalue\n```"
	got := Normalize(input)
	want := "before\nafter `code\\nvalue` \"quoted\\nvalue\"\n```\nfenced\\nvalue\n```"
	if got != want {
		t.Fatalf("Normalize() = %q, want %q", got, want)
	}
}
