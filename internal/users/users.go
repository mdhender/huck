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
		return User{}, classifyInsertErr(conn, in.Handle, in.Email, err)
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

// ListAll returns every user, most recent first. Used by the admin
// users page.
func (s *Store) ListAll(ctx context.Context) ([]User, error) {
	conn, err := s.pool.Take(ctx)
	if err != nil {
		return nil, err
	}
	defer s.pool.Put(conn)

	var out []User
	err = sqlitex.Execute(conn,
		`SELECT id, handle, email, password_hash, is_admin, created_at, updated_at
		   FROM users ORDER BY created_at DESC;`,
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *sqlite.Stmt) error {
				out = append(out, User{
					ID:           stmt.ColumnInt64(0),
					Handle:       stmt.ColumnText(1),
					Email:        stmt.ColumnText(2),
					PasswordHash: stmt.ColumnText(3),
					IsAdmin:      stmt.ColumnInt64(4) != 0,
					CreatedAt:    parseTime(stmt.ColumnText(5)),
					UpdatedAt:    parseTime(stmt.ColumnText(6)),
				})
				return nil
			},
		})
	if err != nil {
		return nil, fmt.Errorf("users: list: %w", err)
	}
	return out, nil
}

// SetAdmin toggles the is_admin flag on the row identified by id and
// bumps updated_at. Returns ErrNotFound when no row matches.
func (s *Store) SetAdmin(ctx context.Context, id int64, isAdmin bool) error {
	conn, err := s.pool.Take(ctx)
	if err != nil {
		return err
	}
	defer s.pool.Put(conn)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := sqlitex.Execute(conn,
		`UPDATE users SET is_admin = ?, updated_at = ? WHERE id = ?;`,
		&sqlitex.ExecOptions{Args: []any{boolToInt(isAdmin), now, id}}); err != nil {
		return fmt.Errorf("users: set admin: %w", err)
	}
	if conn.Changes() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetPassword stores a new bcrypt hash for the user. Returns ErrNotFound
// when no row matches. The caller is responsible for hashing plaintext
// before invoking this method; the users package never sees plaintext.
func (s *Store) SetPassword(ctx context.Context, id int64, passwordHash string) error {
	if passwordHash == "" {
		return errors.New("users: password hash is required")
	}
	conn, err := s.pool.Take(ctx)
	if err != nil {
		return err
	}
	defer s.pool.Put(conn)

	now := time.Now().UTC().Format(time.RFC3339Nano)
	if err := sqlitex.Execute(conn,
		`UPDATE users SET password_hash = ?, updated_at = ? WHERE id = ?;`,
		&sqlitex.ExecOptions{Args: []any{passwordHash, now, id}}); err != nil {
		return fmt.Errorf("users: set password: %w", err)
	}
	if conn.Changes() == 0 {
		return ErrNotFound
	}
	return nil
}

// Delete hard-deletes the user row. Returns ErrNotFound when no row
// matches.
func (s *Store) Delete(ctx context.Context, id int64) error {
	conn, err := s.pool.Take(ctx)
	if err != nil {
		return err
	}
	defer s.pool.Put(conn)

	if err := sqlitex.Execute(conn,
		`DELETE FROM users WHERE id = ?;`,
		&sqlitex.ExecOptions{Args: []any{id}}); err != nil {
		return fmt.Errorf("users: delete: %w", err)
	}
	if conn.Changes() == 0 {
		return ErrNotFound
	}
	return nil
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
//
// On SQLITE_CONSTRAINT_UNIQUE we disambiguate ErrHandleTaken vs.
// ErrEmailTaken by running a same-connection SELECT against the
// normalised input rather than parsing the driver's error text. The
// text form ("UNIQUE constraint failed: users.handle") is a SQLite
// implementation detail, and the previous strings.Contains check was
// silently coupled to both that wording and the column names. The
// SELECT is cheap, runs on the caller's existing conn (so it's inside
// the same transaction when the caller has one open), and survives
// both wording drift and column renames — a rename would surface as a
// loud "no such column" error from classify itself instead of a
// silent fallthrough to the generic wrapper.
func classifyInsertErr(conn *sqlite.Conn, handle, email string, err error) error {
	if sqlite.ErrCode(err) != sqlite.ResultConstraintUnique {
		return fmt.Errorf("users: insert: %w", err)
	}
	if hit, qerr := rowExists(conn, `SELECT 1 FROM users WHERE handle = ?;`, handle); qerr != nil {
		return fmt.Errorf("users: insert: classify: %w", qerr)
	} else if hit {
		return ErrHandleTaken
	}
	if hit, qerr := rowExists(conn, `SELECT 1 FROM users WHERE email = ?;`, email); qerr != nil {
		return fmt.Errorf("users: insert: classify: %w", qerr)
	} else if hit {
		return ErrEmailTaken
	}
	return fmt.Errorf("users: insert: %w", err)
}

func rowExists(conn *sqlite.Conn, query string, arg any) (bool, error) {
	var found bool
	err := sqlitex.Execute(conn, query, &sqlitex.ExecOptions{
		Args: []any{arg},
		ResultFunc: func(stmt *sqlite.Stmt) error {
			found = true
			return nil
		},
	})
	return found, err
}
