package auth

import (
	"errors"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Password policy sentinels (DESIGN.md §8.7).
var (
	ErrPasswordTooShort     = errors.New("auth: password is shorter than 12 characters")
	ErrPasswordTooLong      = errors.New("auth: password is longer than 128 characters")
	ErrPasswordNotPrintable = errors.New("auth: password contains a non-printable character")
)

// Handle policy sentinels (DESIGN.md §8.8).
var (
	ErrHandleTooShort     = errors.New("auth: handle is shorter than 3 characters")
	ErrHandleTooLong      = errors.New("auth: handle is longer than 32 characters")
	ErrHandleBadFirstChar = errors.New("auth: handle must start with a lowercase ASCII letter")
	ErrHandleBadChar      = errors.New("auth: handle may only contain lowercase ASCII letters, digits, and _ , . ' -")
)

// MinPasswordLen and MaxPasswordLen are the inclusive bounds enforced by
// [ValidatePassword]. Counted in Unicode code points, not bytes.
const (
	MinPasswordLen = 12
	MaxPasswordLen = 128
)

// MinHandleLen and MaxHandleLen are the inclusive bounds enforced by
// [ValidateHandle]. Counted after lowercasing and trimming.
const (
	MinHandleLen = 3
	MaxHandleLen = 32
)

// ValidatePassword enforces the policy from DESIGN.md §8.7:
//
//   - length 12–128 code points,
//   - every rune must be printable (Unicode L/M/N/P/S or ASCII space),
//   - no character-class requirements.
//
// Returns one of the Err* sentinels above so callers can render a message
// naming the rule that failed.
func ValidatePassword(pw string) error {
	n := utf8.RuneCountInString(pw)
	if n < MinPasswordLen {
		return ErrPasswordTooShort
	}
	if n > MaxPasswordLen {
		return ErrPasswordTooLong
	}
	for _, r := range pw {
		if !unicode.IsPrint(r) {
			return ErrPasswordNotPrintable
		}
	}
	return nil
}

// ValidateHandle enforces DESIGN.md §8.8 (regex `^[a-z][a-z0-9_,.'-]{2,31}$`).
// The input is lowercased and trimmed before validation, mirroring
// [users.Normalise] so the value seen by the validator is the value that
// will be stored.
func ValidateHandle(h string) error {
	h = strings.ToLower(strings.TrimSpace(h))
	runes := []rune(h)
	n := len(runes)
	if n < MinHandleLen {
		return ErrHandleTooShort
	}
	if n > MaxHandleLen {
		return ErrHandleTooLong
	}
	if !isHandleStart(runes[0]) {
		return ErrHandleBadFirstChar
	}
	for _, r := range runes[1:] {
		if !isHandleChar(r) {
			return ErrHandleBadChar
		}
	}
	return nil
}

func isHandleStart(r rune) bool {
	return r >= 'a' && r <= 'z'
}

func isHandleChar(r rune) bool {
	switch {
	case r >= 'a' && r <= 'z':
		return true
	case r >= '0' && r <= '9':
		return true
	case r == '_', r == ',', r == '.', r == '\'', r == '-':
		return true
	}
	return false
}
