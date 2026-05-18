// Package invites is the invite store and token type. Invites are
// admin-issued, time-limited (7 days), single-use, and consumed inside
// the signup transaction. See docs/DESIGN.md §9.
package invites

import (
	"context"
	"errors"
	"fmt"
	"time"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"

	// invites depends on users for the shared handle/email normaliser
	// (sprint-3 T9). The dependency direction matches signup, where the
	// server consumes an invite *and* creates a user in the same flow,
	// so users never needs to import invites — no cycle is possible
	// without a larger refactor first.
	"github.com/mdhender/huck/internal/users"
)

// ttl is the lifetime applied at Create and refreshed at Resend.
// DESIGN.md §9 fixes this at 7 days; not configurable per-invite.
const ttl = 7 * 24 * time.Hour

// Sentinel errors mapped centrally by internal/server/errors.go.
var (
	ErrNotFound            = errors.New("invite not found")
	ErrExpired             = errors.New("invite expired")
	ErrConsumed            = errors.New("invite already consumed")
	ErrRevoked             = errors.New("invite has been revoked")
	ErrEmailAlreadyInvited = errors.New("email already has an active invite")
)

// Status values returned by Invite.Status. Exported as constants so
// templates and tests share the same vocabulary.
const (
	StatusPending  = "Pending"
	StatusAccepted = "Accepted"
	StatusExpired  = "Expired"
	StatusRevoked  = "Revoked"
)

// Invite is the row shape consumed by handlers.
type Invite struct {
	Token      Token
	Email      string
	InvitedBy  int64
	IsAdmin    bool
	CreatedAt  time.Time
	ExpiresAt  time.Time
	ConsumedAt time.Time // zero value when the invite has not been consumed
	RevokedAt  time.Time // zero value when the invite has not been revoked
}

// Consumed reports whether the invite has been used.
func (i Invite) Consumed() bool { return !i.ConsumedAt.IsZero() }

// Revoked reports whether the invite carries a non-zero revoked_at.
func (i Invite) Revoked() bool { return !i.RevokedAt.IsZero() }

// Expired reports whether the invite's expires_at is in the past
// relative to now. Callers usually pass time.Now().UTC().
func (i Invite) Expired(now time.Time) bool { return now.After(i.ExpiresAt) }

// Status derives the display status used by the admin invites page and
// the per-row partial. Precedence (highest first): Revoked, Accepted
// (consumed), Expired, Pending. See sprint-5 T2.2.
func (i Invite) Status(now time.Time) string {
	switch {
	case i.Revoked():
		return StatusRevoked
	case i.Consumed():
		return StatusAccepted
	case i.Expired(now):
		return StatusExpired
	default:
		return StatusPending
	}
}

// NewInvite is the input to Create / CreateOnConn. IsAdmin records the
// role the invite will grant on signup; the signup flow reads this off
// the row rather than trusting the submitted form (DESIGN.md §9).
type NewInvite struct {
	Email     string
	InvitedBy int64
	IsAdmin   bool
}

// Store is a CRUD facade over the invites table.
type Store struct {
	pool *sqlitex.Pool
}

// NewStore returns a Store backed by the given pool.
func NewStore(pool *sqlitex.Pool) *Store { return &Store{pool: pool} }

// Create generates a token and inserts a new invite. The email is
// lowercased before insert. If an active (unconsumed, non-revoked)
// invite already exists for the same address, the partial unique index
// fires and Create returns ErrEmailAlreadyInvited so the admin endpoint
// can map it to HTTP 409 (DESIGN.md §9 step 1).
func (s *Store) Create(ctx context.Context, in NewInvite) (Invite, error) {
	conn, err := s.pool.Take(ctx)
	if err != nil {
		return Invite{}, err
	}
	defer s.pool.Put(conn)
	return s.CreateOnConn(conn, in)
}

// CreateOnConn is the connection-scoped sibling of Create. The admin
// invite handler runs Create + Mailgun Send inside one
// sqlitex.Transaction so that a Mailgun failure rolls the row back; the
// caller owns the transaction boundary on conn.
func (s *Store) CreateOnConn(conn *sqlite.Conn, in NewInvite) (Invite, error) {
	in.Email = users.Normalise(in.Email)
	if in.Email == "" {
		return Invite{}, errors.New("invites: email is required")
	}
	if in.InvitedBy <= 0 {
		return Invite{}, errors.New("invites: invitedBy must be a valid user id")
	}

	tok, err := Generate()
	if err != nil {
		return Invite{}, err
	}

	now := time.Now().UTC()
	expires := now.Add(ttl)
	err = sqlitex.Execute(conn, `
		INSERT INTO invites (token, email, invited_by, is_admin, created_at, expires_at)
		VALUES (?, ?, ?, ?, ?, ?);`,
		&sqlitex.ExecOptions{
			Args: []any{
				tok.String(),
				in.Email,
				in.InvitedBy,
				boolToInt(in.IsAdmin),
				now.Format(time.RFC3339Nano),
				expires.Format(time.RFC3339Nano),
			},
		})
	if err != nil {
		return Invite{}, classifyInsertErr(conn, in.Email, err)
	}

	return Invite{
		Token:     tok,
		Email:     in.Email,
		InvitedBy: in.InvitedBy,
		IsAdmin:   in.IsAdmin,
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
		`SELECT token, email, invited_by, is_admin, created_at, expires_at, consumed_at, revoked_at
		   FROM invites ORDER BY created_at DESC;`,
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *sqlite.Stmt) error {
				inv := Invite{
					Token:     Token(stmt.ColumnText(0)),
					Email:     stmt.ColumnText(1),
					InvitedBy: stmt.ColumnInt64(2),
					IsAdmin:   stmt.ColumnInt(3) != 0,
					CreatedAt: parseTime(stmt.ColumnText(4)),
					ExpiresAt: parseTime(stmt.ColumnText(5)),
				}
				if stmt.ColumnType(6) != sqlite.TypeNull {
					inv.ConsumedAt = parseTime(stmt.ColumnText(6))
				}
				if stmt.ColumnType(7) != sqlite.TypeNull {
					inv.RevokedAt = parseTime(stmt.ColumnText(7))
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
// the updated row. Resend rejects consumed invites with ErrConsumed and
// revoked invites with ErrRevoked; an expired-but-not-consumed invite
// is allowed (DESIGN.md §9 step 2 says resend is permitted "regardless
// of prior expiry").
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
	if inv.Revoked() {
		return Invite{}, ErrRevoked
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

// Revoke soft-deletes the invite by stamping revoked_at. The row stays
// for audit; the partial unique index excludes revoked rows so a fresh
// Create for the same email succeeds afterwards.
//
// Idempotent: calling Revoke on an already-revoked token returns nil.
// ErrNotFound is reserved for tokens that do not exist at all. This
// mirrors users.Store.Suspend and matches the soft-delete idempotency
// rule shared with future store methods (e.g. games.Store.Archive).
func (s *Store) Revoke(ctx context.Context, t Token) error {
	conn, err := s.pool.Take(ctx)
	if err != nil {
		return err
	}
	defer s.pool.Put(conn)

	exists, err := tokenExists(conn, t)
	if err != nil {
		return fmt.Errorf("invites: revoke: %w", err)
	}
	if !exists {
		return ErrNotFound
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := sqlitex.Execute(conn,
		`UPDATE invites SET revoked_at = ? WHERE token = ? AND revoked_at IS NULL;`,
		&sqlitex.ExecOptions{Args: []any{now, t.String()}}); err != nil {
		return fmt.Errorf("invites: revoke: %w", err)
	}
	return nil
}

// tokenExists reports whether a row exists for the given token, regardless
// of its revoked/consumed state.
func tokenExists(conn *sqlite.Conn, t Token) (bool, error) {
	var found bool
	err := sqlitex.Execute(conn,
		`SELECT 1 FROM invites WHERE token = ?;`,
		&sqlitex.ExecOptions{
			Args: []any{t.String()},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				found = true
				return nil
			},
		})
	return found, err
}

// Consume marks an invite as used. It runs against the caller-supplied
// connection so the signup handler can include it in the same
// transaction as the user-insert (DESIGN.md §9 step 5).
//
// ctx is honoured two ways:
//  1. A fail-fast ctx.Err() check at entry, so an already-cancelled
//     request never starts a write.
//  2. The connection's interrupt channel, which sqlitex.Pool.Take wires
//     to the take-time ctx.Done() — every sqlitex.Execute below
//     therefore respects request cancellation. Consume does not call
//     conn.SetInterrupt itself; callers that obtain conn via
//     pool.Take(reqCtx) and pass the same reqCtx here get cancellation
//     for free.
//
// Returns ErrNotFound, ErrExpired, ErrConsumed, or ErrRevoked if the
// invite is in any state other than active.
func (s *Store) Consume(ctx context.Context, conn *sqlite.Conn, t Token) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	inv, err := getByToken(conn, t)
	if err != nil {
		return err
	}
	if inv.Revoked() {
		return ErrRevoked
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
		`SELECT token, email, invited_by, is_admin, created_at, expires_at, consumed_at, revoked_at
		   FROM invites WHERE token = ?;`,
		&sqlitex.ExecOptions{
			Args: []any{t.String()},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				found = true
				inv.Token = Token(stmt.ColumnText(0))
				inv.Email = stmt.ColumnText(1)
				inv.InvitedBy = stmt.ColumnInt64(2)
				inv.IsAdmin = stmt.ColumnInt(3) != 0
				inv.CreatedAt = parseTime(stmt.ColumnText(4))
				inv.ExpiresAt = parseTime(stmt.ColumnText(5))
				if stmt.ColumnType(6) != sqlite.TypeNull {
					inv.ConsumedAt = parseTime(stmt.ColumnText(6))
				}
				if stmt.ColumnType(7) != sqlite.TypeNull {
					inv.RevokedAt = parseTime(stmt.ColumnText(7))
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

func parseTime(s string) time.Time {
	t, _ := time.Parse(time.RFC3339Nano, s)
	return t
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// classifyInsertErr maps the SQLite UNIQUE failure on the
// invites_email_active partial index to ErrEmailAlreadyInvited.
//
// On SQLITE_CONSTRAINT_UNIQUE we confirm the conflict by querying for
// an active (unconsumed, non-revoked) invite at the same email on the
// caller's connection, rather than parsing the driver's error text. The
// previous strings.Contains check was silently coupled to SQLite's
// "UNIQUE constraint failed: invites.email" wording and to the column
// name; the SELECT is cheap, survives both wording drift and a column
// rename, and runs inside whatever transaction the caller has open on
// conn. The predicate mirrors the partial index (sprint-5 T2.2) so a
// revoke-then-recreate cycle is not misclassified. Any other constraint
// or driver error is wrapped and returned as-is.
func classifyInsertErr(conn *sqlite.Conn, email string, err error) error {
	if sqlite.ErrCode(err) != sqlite.ResultConstraintUnique {
		return fmt.Errorf("invites: insert: %w", err)
	}
	hit, qerr := activeInviteExists(conn, email)
	if qerr != nil {
		return fmt.Errorf("invites: insert: classify: %w", qerr)
	}
	if hit {
		return ErrEmailAlreadyInvited
	}
	return fmt.Errorf("invites: insert: %w", err)
}

func activeInviteExists(conn *sqlite.Conn, email string) (bool, error) {
	var found bool
	err := sqlitex.Execute(conn,
		`SELECT 1 FROM invites
		   WHERE email = ? AND consumed_at IS NULL AND revoked_at IS NULL;`,
		&sqlitex.ExecOptions{
			Args: []any{email},
			ResultFunc: func(stmt *sqlite.Stmt) error {
				found = true
				return nil
			},
		})
	return found, err
}
