package server

import (
	"errors"
	"fmt"
	"testing"

	"github.com/mdhender/huck/internal/auth"
)

func TestPasswordErrMsg(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"too short", auth.ErrPasswordTooShort,
			fmt.Sprintf("Password must be at least %d characters.", auth.MinPasswordLen)},
		{"too long", auth.ErrPasswordTooLong,
			fmt.Sprintf("Password must be at most %d characters.", auth.MaxPasswordLen)},
		{"not printable", auth.ErrPasswordNotPrintable,
			"Password contains a non-printable character."},
		{"nil error", nil, ""},
		{"unrelated error", errors.New("boom"), ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := passwordErrMsg(tt.err); got != tt.want {
				t.Errorf("passwordErrMsg(%v) = %q, want %q", tt.err, got, tt.want)
			}
		})
	}
}

func TestSignupErrorMessageDelegates(t *testing.T) {
	// password sentinels go through passwordErrMsg, the signup-specific
	// switch still handles the other cases.
	if got, want := signupErrorMessage(auth.ErrPasswordNotPrintable),
		"Password contains a non-printable character."; got != want {
		t.Errorf("sentinel: got %q, want %q", got, want)
	}
	if got, want := signupErrorMessage(errEmailMismatch),
		"Submitted email does not match the invite."; got != want {
		t.Errorf("signup-specific: got %q, want %q", got, want)
	}
}
