// Package invites is the invite store and token type. Invites are
// admin-issued, time-limited (7 days), single-use, and consumed inside
// the signup transaction. See docs/DESIGN.md §9.
package invites

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// ttl is the lifetime applied at Create and refreshed at Resend.
// DESIGN.md §9 fixes this at 7 days; not configurable per-invite.
const ttl = 7 * 24 * time.Hour

// Sentinel errors mapped centrally by internal/server/errors.go.
var (
	ErrNotFound            = errors.New("invite not found")
	ErrExpired             = errors.New("invite expired")
	ErrConsumed            = errors.New("invite already consumed")
	ErrEmailAlreadyInvited = errors.New("email already has an active invite")
)

// Invite is the row shape consumed by handlers.
type Invite struct {
	Token      Token
	Email      string
	InvitedBy  int64
	CreatedAt  time.Time
	ExpiresAt  time.Time
	ConsumedAt time.Time // zero value when the invite has not been consumed
}

// Consumed reports whether the invite has been used.
func (i Invite) Consumed() bool { return !i.ConsumedAt.IsZero() }

// Expired reports whether the invite's expires_at is in the past
// relative to now. Callers usually pass time.Now().UTC().
func (i Invite) Expired(now time.Time) bool { return now.After(i.ExpiresAt) }

// Store is a CRUD facade over the invites table.
type Store struct {
	pool *sqlitex.Pool
}

// NewStore returns a Store backed by the given pool.
func NewStore(pool *sqlitex.Pool) *Store { return &Store{pool: pool} }

// Create generates a token and inserts a new invite for email. The email
// is lowercased before insert. If an active (unconsumed) invite already
// exists for the same address, the partial unique index fires and Create
// returns ErrEmailAlreadyInvited so the admin endpoint can map it to
// HTTP 409 (DESIGN.md §9 step 1).
func (s *Store) Create(ctx context.Context, email string, invitedBy int64) (Invite, error) {
	conn, err := s.pool.Take(ctx)
	if err != nil {
		return Invite{}, err
	}
	defer s.pool.Put(conn)
	return s.CreateOnConn(conn, email, invitedBy)
}

// CreateOnConn is the connection-scoped sibling of Create. The admin
// invite handler runs Create + Mailgun Send inside one
// sqlitex.Transaction so that a Mailgun failure rolls the row back; the
// caller owns the transaction boundary on conn.
func (s *Store) CreateOnConn(conn *sqlite.Conn, email string, invitedBy int64) (Invite, error) {
	email = normaliseEmail(email)
	if email == "" {
		return Invite{}, errors.New("invites: email is required")
	}
	if invitedBy <= 0 {
		return Invite{}, errors.New("invites: invitedBy must be a valid user id")
	}

	tok, err := Generate()
	if err != nil {
		return Invite{}, err
	}

	now := time.Now().UTC()
	expires := now.Add(ttl)
	err = sqlitex.Execute(conn, `
		INSERT INTO invites (token, email, invited_by, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?);`,
		&sqlitex.ExecOptions{
			Args: []any{
				tok.String(),
				email,
				invitedBy,
				now.Format(time.RFC3339Nano),
				expires.Format(time.RFC3339Nano),
			},
		})
	if err != nil {
		return Invite{}, classifyInsertErr(err)
	}

	return Invite{
		Token:     tok,
		Email:     email,
		InvitedBy: invitedBy,
		CreatedAt: now,
		ExpiresAt: expires,
	}, nil
}

// ListAll returns every invite, most recent first. The admin invite
// page renders the result as a table.
func (s *Store) ListAll(ctx context.Context) ([]Invite, error) {
	conn, err := s.pool.Take(ctx)
	if err != nil {
		return nil, err
	}
	defer s.pool.Put(conn)

	var out []Invite
	err = sqlitex.Execute(conn,
		`SELECT token, email, invited_by, created_at, expires_at, consumed_at
		   FROM invites ORDER BY created_at DESC;`,
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *sqlite.Stmt) error {
				inv := Invite{
					Token:     Token(stmt.ColumnText(0)),
					Email:     stmt.ColumnText(1),
					InvitedBy: stmt.ColumnInt64(2),
					CreatedAt: parseTime(stmt.ColumnText(3)),
					ExpiresAt: parseTime(stmt.ColumnText(4)),
				}
				if stmt.ColumnType(5) != sqlite.TypeNull {
					inv.ConsumedAt = parseTime(stmt.ColumnText(5))
				}
				out = append(out, inv)
				return nil
			},
		})
	if err != nil {
		return nil, fmt.Errorf("invites: list: %w", err)
	}
	return out, nil
}

// GetByToken looks up an invite by its token. Returns ErrNotFound when
// no row matches.
func (s *Store) GetByToken(ctx context.Context, t Token) (Invite, error) {
	conn, err := s.pool.Take(ctx)
	if err != nil {
		return Invite{}, err
	}
	defer s.pool.Put(conn)
	return getByToken(conn, t)
}

// GetByTokenOnConn is the connection-scoped sibling of GetByToken. The
// signup handler reads the invite inside the same transaction it later
// uses for the user-insert + Consume, so no extra pool round-trips
// happen across the boundary.
func (s *Store) GetByTokenOnConn(conn *sqlite.Conn, t Token) (Invite, error) {
	return getByToken(conn, t)
}

// Resend refreshes an existing invite's expires_at to now+7d and returns
// the updated row. Resend rejects consumed invites with ErrConsumed; an
// expired-but-not-consumed invite is allowed (DESIGN.md §9 step 2 says
// resend is permitted "regardless of prior expiry").
func (s *Store) Resend(ctx context.Context, t Token) (Invite, error) {
	conn, err := s.pool.Take(ctx)
	if err != nil {
		return Invite{}, err
	}
	defer s.pool.Put(conn)

	inv, err := getByToken(conn, t)
	if err != nil {
		return Invite{}, err
	}
	if inv.Consumed() {
		return Invite{}, ErrConsumed
	}

	now := time.Now().UTC()
	expires := now.Add(ttl)
	if err := sqlitex.Execute(conn,
		`UPDATE invites SET expires_at = ? WHERE token = ?;`,
		&sqlitex.ExecOptions{
			Args: []any{expires.Format(time.RFC3339Nano), t.String()},
		}); err != nil {
		return Invite{}, fmt.Errorf("invites: resend: %w", err)
	}

	inv.ExpiresAt = expires
	return inv, nil
}

// Revoke deletes the invite row. Returns ErrNotFound when no row matches.
func (s *Store) Revoke(ctx context.Context, t Token) error {
	conn, err := s.pool.Take(ctx)
	if err != nil {
		return err
	}
	defer s.pool.Put(conn)

	if err := sqlitex.Execute(conn,
		`DELETE FROM invites WHERE token = ?;`,
		&sqlitex.ExecOptions{Args: []any{t.String()}}); err != nil {
		return fmt.Errorf("invites: revoke: %w", err)
	}
	if conn.Changes() == 0 {
		return ErrNotFound
	}
	return nil
}

// Consume marks an invite as used. It runs against the caller-supplied
// connection so the signup handler can include it in the same
// transaction as the user-insert (DESIGN.md §9 step 5). Returns
// ErrNotFound, ErrExpired, or ErrConsumed if the invite is in any state
// other than active.
func (s *Store) Consume(ctx context.Context, conn *sqlite.Conn, t Token) error {
	_ = ctx // present for API symmetry; sqlitex.Execute ignores ctx
	inv, err := getByToken(conn, t)
	if err != nil {
		return err
	}
	if inv.Consumed() {
		return ErrConsumed
	}
	if inv.Expired(time.Now().UTC()) {
		return ErrExpired
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := sqlitex.Execute(conn,
		`UPDATE invites SET consumed_at = ? WHERE token = ? AND consumed_at IS NULL;`,
		&sqlitex.ExecOptions{Args: []any{now, t.String()}}); err != nil {
		return fmt.Errorf("invites: consume: %w", err)
	}
	if conn.Changes() == 0 {
		// Lost a race with another consumer between getByToken and the
		// UPDATE. From the caller's perspective the invite is consumed.
		return ErrConsumed
	}
	return nil
}

// getByToken reads a single invite using the supplied connection. Used
// by both pool-backed methods and Consume (which runs on the caller's
// transaction connection).
func getByToken(conn *sqlite.Conn, t Token) (Invite, error) {
	var (
		inv   Invite
		found bool
	)
	err := sqlitex.Execute(conn,
		`SELECT token, email, invited_by, created_at, expires_at, consumed_at
		   FROM invites WHERE token = ?;`,
		&sqlitex.ExecOptions{
			Args: []any{t.String()},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				found = true
				inv.Token = Token(stmt.ColumnText(0))
				inv.Email = stmt.ColumnText(1)
				inv.InvitedBy = stmt.ColumnInt64(2)
				inv.CreatedAt = parseTime(stmt.ColumnText(3))
				inv.ExpiresAt = parseTime(stmt.ColumnText(4))
				if stmt.ColumnType(5) != sqlite.TypeNull {
					inv.ConsumedAt = parseTime(stmt.ColumnText(5))
				}
				return nil
			},
		})
	if err != nil {
		return Invite{}, fmt.Errorf("invites: get: %w", err)
	}
	if !found {
		return Invite{}, ErrNotFound
	}
	return inv, nil
}

// normaliseEmail mirrors users.Normalise. The invites package keeps its
// own copy to avoid importing users (and accidentally creating a cycle
// if users ever needs invites).
func normaliseEmail(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

// classifyInsertErr maps the SQLite UNIQUE failure on the
// invites_email_active partial index to ErrEmailAlreadyInvited. Any
// other constraint or driver error is wrapped and returned as-is.
func classifyInsertErr(err error) error {
	if sqlite.ErrCode(err) != sqlite.ResultConstraintUnique {
		return fmt.Errorf("invites: insert: %w", err)
	}
	// SQLite reports partial-index UNIQUE violations as
	// "UNIQUE constraint failed: invites.email".
	if strings.Contains(err.Error(), "invites.email") {
		return ErrEmailAlreadyInvited
	}
	return fmt.Errorf("invites: insert: %w", err)
}
