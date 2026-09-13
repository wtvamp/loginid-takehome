package api

import (
	"encoding/base64"
	"testing"
)

func TestCursor_RoundTrip(t *testing.T) {
	for _, offset := range []int{0, 1, 20, 12345} {
		c := encodeCursor(offset)
		got, err := decodeCursor(c)
		if err != nil {
			t.Fatalf("decodeCursor(%q): %v", c, err)
		}
		if got != offset {
			t.Errorf("round trip of offset %d = %d", offset, got)
		}
	}
}

func TestCursor_EmptyIsOffsetZero(t *testing.T) {
	got, err := decodeCursor("")
	if err != nil {
		t.Fatalf("decodeCursor(\"\"): %v", err)
	}
	if got != 0 {
		t.Errorf("empty cursor decoded to %d, want 0", got)
	}
}

func TestCursor_OpaqueAtAPIBoundary(t *testing.T) {
	// A caller cannot construct or walk a cursor by hand — the encoded
	// form must not be a bare decimal offset a caller could increment by
	// eye (handoff-03-auth.md v3's opaque-cursor requirement).
	c := encodeCursor(20)
	if c == "20" {
		t.Error("cursor must not be the bare decimal offset")
	}
}

func TestCursor_MalformedRejected(t *testing.T) {
	cases := []string{"not-base64!!!", "AAAA", "0"}
	for _, c := range cases {
		if _, err := decodeCursor(c); err == nil {
			t.Errorf("decodeCursor(%q) = nil error, want a malformed-cursor error", c)
		}
	}
}

// TestCursor_NegativeOffsetRejected constructs a tampered cursor directly
// (base64 of "v1:-5") — encodeCursor never produces a negative offset
// itself, so this proves decodeCursor's own guard against untrusted
// caller input, not encodeCursor's behavior.
func TestCursor_NegativeOffsetRejected(t *testing.T) {
	tampered := base64.RawURLEncoding.EncodeToString([]byte("v1:-5"))
	neg, err := decodeCursor(tampered)
	if err == nil {
		t.Errorf("decodeCursor of a negative-offset payload = %d, nil error, want rejected", neg)
	}
}
