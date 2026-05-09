package auth_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/mdhender/huck/internal/auth"
)

func TestValidatePassword(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		pw   string
		want error // nil = valid
	}{
		// Boundaries.
		{"min length, ok", strings.Repeat("a", auth.MinPasswordLen), nil},
		{"one below min", strings.Repeat("a", auth.MinPasswordLen-1), auth.ErrPasswordTooShort},
		{"max length, ok", strings.Repeat("a", auth.MaxPasswordLen), nil},
		{"one above max", strings.Repeat("a", auth.MaxPasswordLen+1), auth.ErrPasswordTooLong},
		{"empty", "", auth.ErrPasswordTooShort},

		// Allowed contents.
		{"spaces allowed", "correct horse battery staple", nil},
		{"punctuation+digits", "P@ssw0rd!1234", nil},
		{"unicode letters allowed", strings.Repeat("ünîcødé", 2), nil}, // 14 runes
		{"emoji allowed", "🐎🐎🐎🐎🐎🐎🐎🐎🐎🐎🐎🐎", nil},                       // 12 runes

		// Rejected contents.
		{"contains tab", "abcdefghijk\tx", auth.ErrPasswordNotPrintable},
		{"contains newline", "abcdefghijkl\n", auth.ErrPasswordNotPrintable},
		{"contains NUL", "abcdefghijkl\x00", auth.ErrPasswordNotPrintable},
		{"contains DEL", "abcdefghijkl\x7f", auth.ErrPasswordNotPrintable},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := auth.ValidatePassword(tc.pw)
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
		{"one below min", "ab", auth.ErrHandleTooShort},
		{"max length", "a" + strings.Repeat("b", auth.MaxHandleLen-1), nil},
		{"one above max", "a" + strings.Repeat("b", auth.MaxHandleLen), auth.ErrHandleTooLong},
		{"empty", "", auth.ErrHandleTooShort},

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
		{"digit first", "1alice", auth.ErrHandleBadFirstChar},
		{"underscore first", "_alice", auth.ErrHandleBadFirstChar},
		{"hyphen first", "-alice", auth.ErrHandleBadFirstChar},
		{"unicode first", "élise", auth.ErrHandleBadFirstChar},

		// Disallowed characters elsewhere.
		{"space inside", "a lice", auth.ErrHandleBadChar},
		{"slash inside", "a/lice", auth.ErrHandleBadChar},
		{"@ inside", "a@lice", auth.ErrHandleBadChar},
		{"unicode inside", "alicé", auth.ErrHandleBadChar},
		{"emoji inside", "alice🎉", auth.ErrHandleBadChar},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := auth.ValidateHandle(tc.in)
			if !errors.Is(err, tc.want) {
				t.Fatalf("ValidateHandle(%q): got %v, want %v", tc.in, err, tc.want)
			}
		})
	}
}
