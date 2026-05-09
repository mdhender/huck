// Package users is the user store. Handles and emails are normalised
// (lower-cased, trimmed) in Go before insert; uniqueness is enforced by
// the schema.
package users

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// Sentinel errors mapped centrally by internal/server/errors.go.
var (
	ErrNotFound    = errors.New("user not found")
	ErrHandleTaken = errors.New("handle already in use")
	ErrEmailTaken  = errors.New("email already in use")
)

// User is the row shape consumed by handlers.
type User struct {
	ID           int64
	Handle       string
	Email        string
	PasswordHash string
	IsAdmin      bool
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// NewUser is the input to Create.
type NewUser struct {
	Handle       string
	Email        string
	PasswordHash string // already-bcrypted; users package never sees plaintext
	IsAdmin      bool
}

// Store is a CRUD facade over the users table.
type Store struct {
	pool *sqlitex.Pool
}

// NewStore returns a Store backed by the given pool.
func NewStore(pool *sqlitex.Pool) *Store { return &Store{pool: pool} }

// Normalise lower-cases and trims a handle or email value. Exported so the
// `admin create` subcommand can present the lower-cased value back to the
// operator.
func Normalise(s string) string { return strings.ToLower(strings.TrimSpace(s)) }

// Create inserts a new user. Returns ErrHandleTaken / ErrEmailTaken when
// the unique constraints fire.
func (s *Store) Create(ctx context.Context, in NewUser) (User, error) {
	conn, err := s.pool.Take(ctx)
	if err != nil {
		return User{}, err
	}
	defer s.pool.Put(conn)
	return s.CreateOnConn(conn, in)
}

// CreateOnConn inserts a new user using the supplied connection. Used by
// callers (notably the signup handler in internal/server) that need to
// include the insert in their own transaction. The same uniqueness
// errors are surfaced as Create.
func (s *Store) CreateOnConn(conn *sqlite.Conn, in NewUser) (User, error) {
	in.Handle = Normalise(in.Handle)
	in.Email = Normalise(in.Email)
	if in.Handle == "" || in.Email == "" || in.PasswordHash == "" {
		return User{}, errors.New("users: handle, email, and password hash are required")
	}

	now := time.Now().UTC().Format(time.RFC3339Nano)
	err := sqlitex.Execute(conn, `
		INSERT INTO users (handle, email, password_hash, is_admin, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?);`,
		&sqlitex.ExecOptions{
			Args: []any{in.Handle, in.Email, in.PasswordHash, boolToInt(in.IsAdmin), now, now},
		})
	if err != nil {
		return User{}, classifyInsertErr(err)
	}

	return getOneOnConn(conn,
		`SELECT id, handle, email, password_hash, is_admin, created_at, updated_at
		   FROM users WHERE id = ?;`, conn.LastInsertRowID())
}

// GetByHandle looks up a user by handle. The lookup lowercases the input
// to mirror what is stored.
func (s *Store) GetByHandle(ctx context.Context, handle string) (User, error) {
	return s.getOne(ctx, `SELECT id, handle, email, password_hash, is_admin, created_at, updated_at
		FROM users WHERE handle = ?;`, Normalise(handle))
}

// GetByID looks up a user by primary key.
func (s *Store) GetByID(ctx context.Context, id int64) (User, error) {
	return s.getOne(ctx, `SELECT id, handle, email, password_hash, is_admin, created_at, updated_at
		FROM users WHERE id = ?;`, id)
}

// AdminExists reports whether at least one row has is_admin = 1.
func (s *Store) AdminExists(ctx context.Context) (bool, error) {
	conn, err := s.pool.Take(ctx)
	if err != nil {
		return false, err
	}
	defer s.pool.Put(conn)

	var found bool
	err = sqlitex.Execute(conn, `SELECT 1 FROM users WHERE is_admin = 1 LIMIT 1;`,
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *sqlite.Stmt) error {
				found = true
				return nil
			},
		})
	if err != nil {
		return false, fmt.Errorf("users: admin exists: %w", err)
	}
	return found, nil
}

func (s *Store) getOne(ctx context.Context, query string, arg any) (User, error) {
	conn, err := s.pool.Take(ctx)
	if err != nil {
		return User{}, err
	}
	defer s.pool.Put(conn)
	return getOneOnConn(conn, query, arg)
}

// getOneOnConn runs a single-row SELECT against the supplied connection
// and decodes a User. Shared by getOne (pool-acquired) and CreateOnConn
// (caller-supplied transaction connection).
func getOneOnConn(conn *sqlite.Conn, query string, arg any) (User, error) {
	var (
		u     User
		found bool
	)
	err := sqlitex.Execute(conn, query, &sqlitex.ExecOptions{
		Args: []any{arg},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			found = true
			u.ID = stmt.ColumnInt64(0)
			u.Handle = stmt.ColumnText(1)
			u.Email = stmt.ColumnText(2)
			u.PasswordHash = stmt.ColumnText(3)
			u.IsAdmin = stmt.ColumnInt64(4) != 0
			u.CreatedAt = parseTime(stmt.ColumnText(5))
			u.UpdatedAt = parseTime(stmt.ColumnText(6))
			return nil
		},
	})
	if err != nil {
		return User{}, fmt.Errorf("users: get: %w", err)
	}
	if !found {
		return User{}, ErrNotFound
	}
	return u, nil
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

// classifyInsertErr maps SQLite UNIQUE failures to our sentinel errors.
func classifyInsertErr(err error) error {
	code := sqlite.ErrCode(err)
	if code != sqlite.ResultConstraintUnique {
		return fmt.Errorf("users: insert: %w", err)
	}
	msg := err.Error()
	switch {
	case strings.Contains(msg, "users.handle"):
		return ErrHandleTaken
	case strings.Contains(msg, "users.email"):
		return ErrEmailTaken
	default:
		return fmt.Errorf("users: insert: %w", err)
	}
}
