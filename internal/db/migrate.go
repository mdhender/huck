package db

import (
	"context"
	"fmt"
	"io/fs"
	"path"
	"regexp"
	"sort"
	"strconv"
	"time"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"

	"github.com/mdhender/huck/migrations"
)

// migrationName matches "NNNN_<name>.sql"; the integer prefix is the version.
var migrationName = regexp.MustCompile(`^(\d+)_[^.]+\.sql$`)

// Migrate applies every embedded migration whose version is not yet
// recorded in schema_migrations. Each migration runs in its own
// transaction.
//
// The migrator tolerates the schema_migrations table not existing on its
// first run — the very first migration is what creates that table.
func Migrate(pool *sqlitex.Pool) error {
	files, err := loadMigrations()
	if err != nil {
		return err
	}

	conn, err := pool.Take(context.Background())
	if err != nil {
		return fmt.Errorf("db: take connection: %w", err)
	}
	defer pool.Put(conn)

	applied, err := appliedVersions(conn)
	if err != nil {
		return err
	}

	for _, m := range files {
		if _, ok := applied[m.version]; ok {
			continue
		}
		if err := applyOne(conn, m); err != nil {
			return fmt.Errorf("db: migration %04d: %w", m.version, err)
		}
	}
	return nil
}

type migration struct {
	version int
	name    string
	body    []byte
}

func loadMigrations() ([]migration, error) {
	entries, err := fs.ReadDir(migrations.FS, ".")
	if err != nil {
		return nil, fmt.Errorf("db: list migrations: %w", err)
	}

	var out []migration
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		match := migrationName.FindStringSubmatch(e.Name())
		if match == nil {
			continue
		}
		v, err := strconv.Atoi(match[1])
		if err != nil {
			return nil, fmt.Errorf("db: bad version %q: %w", e.Name(), err)
		}
		body, err := fs.ReadFile(migrations.FS, e.Name())
		if err != nil {
			return nil, fmt.Errorf("db: read %s: %w", e.Name(), err)
		}
		out = append(out, migration{version: v, name: path.Base(e.Name()), body: body})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].version < out[j].version })
	return out, nil
}

// appliedVersions returns the set of versions already in schema_migrations.
// If the table itself does not yet exist (first run, where the very first
// migration is what creates it), the function returns an empty set instead
// of erroring — that is the bootstrap hazard called out in sprint-1.md.
func appliedVersions(conn *sqlite.Conn) (map[int]struct{}, error) {
	out := make(map[int]struct{})

	var exists bool
	if err := sqlitex.Execute(conn,
		`SELECT 1 FROM sqlite_master WHERE type='table' AND name='schema_migrations';`,
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *sqlite.Stmt) error {
				exists = true
				return nil
			},
		},
	); err != nil {
		return nil, fmt.Errorf("db: probe schema_migrations: %w", err)
	}
	if !exists {
		return out, nil
	}

	if err := sqlitex.Execute(conn, `SELECT version FROM schema_migrations;`, &sqlitex.ExecOptions{
		ResultFunc: func(stmt *sqlite.Stmt) error {
			out[int(stmt.ColumnInt64(0))] = struct{}{}
			return nil
		},
	}); err != nil {
		return nil, fmt.Errorf("db: read schema_migrations: %w", err)
	}
	return out, nil
}

func applyOne(conn *sqlite.Conn, m migration) (err error) {
	endTx, err := sqlitex.ImmediateTransaction(conn)
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer endTx(&err)

	if err = sqlitex.ExecuteScript(conn, string(m.body), nil); err != nil {
		return fmt.Errorf("execute: %w", err)
	}

	if err = sqlitex.Execute(conn,
		`INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?);`,
		&sqlitex.ExecOptions{
			Args: []any{m.version, time.Now().UTC().Format(time.RFC3339Nano)},
		},
	); err != nil {
		return fmt.Errorf("record version: %w", err)
	}
	return nil
}

// AppliedCount reports how many migrations have already been recorded.
// It is used by `huck admin create` to detect a freshly created (but not
// yet migrated) database and emit a friendly error.
func AppliedCount(pool *sqlitex.Pool) (int, error) {
	conn, err := pool.Take(context.Background())
	if err != nil {
		return 0, err
	}
	defer pool.Put(conn)
	applied, err := appliedVersions(conn)
	if err != nil {
		return 0, err
	}
	return len(applied), nil
}
