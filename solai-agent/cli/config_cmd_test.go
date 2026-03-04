package cli

import (
	"strings"
	"testing"
)

// ---- redact -----------------------------------------------------------------

func TestRedact_EmptyString(t *testing.T) {
	if got := redact(""); got != "" {
		t.Errorf("redact(%q): got %q, want empty", "", got)
	}
}

func TestRedact_ShortString_AllStars(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"a", "*"},
		{"ab", "**"},
		{"abcdefgh", "********"}, // exactly 8 chars → all stars (len <= 8)
	}
	for _, tc := range tests {
		got := redact(tc.input)
		if got != tc.want {
			t.Errorf("redact(%q): got %q, want %q", tc.input, got, tc.want)
		}
	}
}

func TestRedact_LongString_KeepsFourCharPrefix(t *testing.T) {
	input := "sk-ant-abcdefghijklmno"
	got := redact(input)

	if !strings.HasPrefix(got, "sk-a") {
		t.Errorf("redact(%q): expected prefix sk-a, got %q", input, got)
	}
	suffix := got[4:]
	for _, ch := range suffix {
		if ch != '*' {
			t.Errorf("redact(%q): expected all stars after prefix, got %q", input, got)
			break
		}
	}
	if len(got) != len(input) {
		t.Errorf("redact(%q): length changed: got %d, want %d", input, len(got), len(input))
	}
}

func TestRedact_NineChars_KeepsFourCharPrefix(t *testing.T) {
	input := "123456789" // 9 chars, just over the threshold
	got := redact(input)
	if !strings.HasPrefix(got, "1234") {
		t.Errorf("expected prefix 1234, got %q", got)
	}
	if len(got) != 9 {
		t.Errorf("expected length 9, got %d", len(got))
	}
}

func TestRedact_PreservesLength(t *testing.T) {
	cases := []string{
		"short",
		"exactly8",
		"longerapikey1234567890",
	}
	for _, input := range cases {
		got := redact(input)
		if got == "" && input != "" {
			t.Errorf("redact(%q): got empty, want non-empty", input)
		}
		if len(got) != len(input) {
			t.Errorf("redact(%q): length changed from %d to %d", input, len(input), len(got))
		}
	}
}
