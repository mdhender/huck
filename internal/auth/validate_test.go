package auth

import (
	"errors"
	"strings"
	"testing"
)

func TestValidatePassword(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		pw   string
		want error // nil = valid
	}{
		// Boundaries.
		{"min length, ok", strings.Repeat("a", MinPasswordLen), nil},
		{"one below min", strings.Repeat("a", MinPasswordLen-1), ErrPasswordTooShort},
		{"max length, ok", strings.Repeat("a", MaxPasswordLen), nil},
		{"one above max", strings.Repeat("a", MaxPasswordLen+1), ErrPasswordTooLong},
		{"empty", "", ErrPasswordTooShort},

		// Allowed contents.
		{"spaces allowed", "correct horse battery staple", nil},
		{"punctuation+digits", "P@ssw0rd!1234", nil},
		{"unicode letters allowed", strings.Repeat("ünîcødé", 2), nil}, // 14 runes
		{"emoji allowed", "🐎🐎🐎🐎🐎🐎🐎🐎🐎🐎🐎🐎", nil},                       // 12 runes

		// Rejected contents.
		{"contains tab", "abcdefghijk\tx", ErrPasswordNotPrintable},
		{"contains newline", "abcdefghijkl\n", ErrPasswordNotPrintable},
		{"contains NUL", "abcdefghijkl\x00", ErrPasswordNotPrintable},
		{"contains DEL", "abcdefghijkl\x7f", ErrPasswordNotPrintable},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidatePassword(tc.pw)
			if !errors.Is(err, tc.want) {
				t.Fatalf("ValidatePassword(%q): got %v, want %v", tc.pw, err, tc.want)
			}
		})
	}
}

func TestValidateHandle(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		in   string
		want error // nil = valid
	}{
		// Boundaries.
		{"min length", "abc", nil},
		{"one below min", "ab", ErrHandleTooShort},
		{"max length", "a" + strings.Repeat("b", maxHandleLen-1), nil},
		{"one above max", "a" + strings.Repeat("b", maxHandleLen), ErrHandleTooLong},
		{"empty", "", ErrHandleTooShort},

		// Normalisation: leading/trailing whitespace and uppercase folded
		// before validation.
		{"trims whitespace", "  alice  ", nil},
		{"lowercased input ok", "Alice", nil},
		{"all caps ok after fold", "ALICE", nil},

		// Allowed character classes.
		{"digits ok after first", "alice42", nil},
		{"underscore ok", "a_b_c", nil},
		{"dot ok", "j.doe", nil},
		{"comma ok", "a,b,c", nil},
		{"apostrophe ok", "a'b'c", nil},
		{"hyphen ok", "a-b-c", nil},

		// First-character rule.
		{"digit first", "1alice", ErrHandleBadFirstChar},
		{"underscore first", "_alice", ErrHandleBadFirstChar},
		{"hyphen first", "-alice", ErrHandleBadFirstChar},
		{"unicode first", "élise", ErrHandleBadFirstChar},

		// Disallowed characters elsewhere.
		{"space inside", "a lice", ErrHandleBadChar},
		{"slash inside", "a/lice", ErrHandleBadChar},
		{"@ inside", "a@lice", ErrHandleBadChar},
		{"unicode inside", "alicé", ErrHandleBadChar},
		{"emoji inside", "alice🎉", ErrHandleBadChar},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateHandle(tc.in)
			if !errors.Is(err, tc.want) {
				t.Fatalf("ValidateHandle(%q): got %v, want %v", tc.in, err, tc.want)
			}
		})
	}
}
