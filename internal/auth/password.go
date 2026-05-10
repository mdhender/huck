// Package auth contains huck's bcrypt + JWT primitives, the echo-jwt
// middleware configuration, the cookie helpers, and the login/logout
// handlers.
//
// The package never logs or stores plaintext passwords. The only mention
// of plaintext lives on the wire (during POST /login) and in the bcrypt
// Verify call below.
package auth

import (
	"errors"
	"fmt"

	"golang.org/x/crypto/bcrypt"
)

// bcryptCost matches docs/DESIGN.md and AGENTS.md.
const bcryptCost = 12

// ErrBadPassword is returned by Verify when the supplied password does not
// match the stored hash.
var ErrBadPassword = errors.New("auth: invalid password")

// Hash returns a bcrypt hash of the plaintext password.
func Hash(password string) (string, error) {
	if len(password) == 0 {
		return "", errors.New("auth: empty password")
	}
	out, err := bcrypt.GenerateFromPassword([]byte(password), bcryptCost)
	if err != nil {
		return "", fmt.Errorf("auth: hash: %w", err)
	}
	return string(out), nil
}

// Verify reports whether the bcrypt hash matches the password. It maps a
// mismatch to [ErrBadPassword] and propagates any other bcrypt error
// untouched.
func Verify(hash, password string) error {
	switch err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)); {
	case err == nil:
		return nil
	case errors.Is(err, bcrypt.ErrMismatchedHashAndPassword):
		return ErrBadPassword
	default:
		return fmt.Errorf("auth: verify: %w", err)
	}
}
