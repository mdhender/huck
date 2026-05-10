package auth

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/mdhender/huck/internal/users"
)

// CookieName is the cookie under which we store the signed JWT.
const CookieName = "auth"

// DefaultTokenTTL is the validity window of an issued token, matching
// docs/DESIGN.md §8.1.
const DefaultTokenTTL = 24 * time.Hour

// Claims is the typed JWT payload. echo-jwt's NewClaimsFunc is wired to
// return *Claims so handlers can do a typed assertion in O(1) without
// silently downgrading to jwt.MapClaims.
type Claims struct {
	jwt.RegisteredClaims
	Handle string `json:"handle"`
	Admin  bool   `json:"admin"`
}

// UserID returns the user id encoded in the Subject claim. Returns 0
// when Subject is missing or unparseable; callers that need a non-zero
// id should treat that as an error.
func (c *Claims) UserID() int64 {
	if c == nil {
		return 0
	}
	id, err := strconv.ParseInt(c.Subject, 10, 64)
	if err != nil {
		return 0
	}
	return id
}

// Issue signs a new JWT for the given user.
func Issue(u users.User, key []byte, ttl time.Duration) (string, error) {
	if len(key) == 0 {
		return "", errors.New("auth: empty signing key")
	}
	if ttl <= 0 {
		ttl = DefaultTokenTTL
	}
	now := time.Now().UTC()
	claims := &Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   strconv.FormatInt(u.ID, 10),
			IssuedAt:  jwt.NewNumericDate(now),
			ExpiresAt: jwt.NewNumericDate(now.Add(ttl)),
		},
		Handle: u.Handle,
		Admin:  u.IsAdmin,
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := tok.SignedString(key)
	if err != nil {
		return "", fmt.Errorf("auth: sign: %w", err)
	}
	return signed, nil
}

// Parse parses and validates a token, returning the typed claims.
// Used by best-effort auth on GET / and by tests.
func Parse(tokenString string, key []byte) (*Claims, error) {
	tok, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", t.Header["alg"])
		}
		return key, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := tok.Claims.(*Claims)
	if !ok || !tok.Valid {
		return nil, errors.New("auth: invalid token")
	}
	return claims, nil
}
