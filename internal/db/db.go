// Package db owns SQLite open/create/migrate for huck.
//
// All callers go through this package; nothing else should call
// sqlitex.Open or sqlite.OpenConn directly.
package db

import (
	"errors"
	"fmt"
	"net/url"
	"os"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

// ErrMissing is returned by [Open] when the requested SQLite file does not
// exist on disk. `huck serve` turns it into a fatal error pointing the
// operator at `huck db create`.
var ErrMissing = errors.New("database file does not exist")

// poolSize is intentionally small: the user base is tiny.
const poolSize = 8

// Open opens an existing SQLite file as a connection pool. It refuses to
// create the file if it is missing — that is the job of [Create] alone.
func Open(path string) (*sqlitex.Pool, error) {
	if path == "" {
		return nil, errors.New("db: empty path")
	}
	if _, err := os.Stat(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, fmt.Errorf("%w: %s", ErrMissing, path)
		}
		return nil, fmt.Errorf("db: stat %s: %w", path, err)
	}

	pool, err := sqlitex.NewPool(fileURI(path), sqlitex.PoolOptions{
		Flags:       sqlite.OpenReadWrite | sqlite.OpenWAL | sqlite.OpenURI,
		PoolSize:    poolSize,
		PrepareConn: prepareConn,
	})
	if err != nil {
		return nil, fmt.Errorf("db: open %s: %w", path, err)
	}
	return pool, nil
}

// Create creates a new SQLite file at the given path, applies every
// embedded migration, and closes the pool. It refuses to overwrite an
// existing file.
func Create(path string) (err error) {
	if path == "" {
		return errors.New("db: empty path")
	}
	if _, statErr := os.Stat(path); statErr == nil {
		return fmt.Errorf("db: refuse to overwrite existing file: %s", path)
	} else if !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("db: stat %s: %w", path, statErr)
	}

	pool, err := sqlitex.NewPool(fileURI(path), sqlitex.PoolOptions{
		Flags:       sqlite.OpenReadWrite | sqlite.OpenCreate | sqlite.OpenWAL | sqlite.OpenURI,
		PoolSize:    poolSize,
		PrepareConn: prepareConn,
	})
	if err != nil {
		return fmt.Errorf("db: create %s: %w", path, err)
	}
	defer func() {
		if cerr := pool.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("db: close after create: %w", cerr)
		}
	}()

	if err := Migrate(pool); err != nil {
		return fmt.Errorf("db: initial migrate: %w", err)
	}
	return nil
}

// prepareConn applies the standard PRAGMAs to every connection in the pool,
// per docs/DESIGN.md §7.1.
func prepareConn(conn *sqlite.Conn) error {
	pragmas := []string{
		"PRAGMA journal_mode = WAL;",
		"PRAGMA synchronous  = NORMAL;",
		"PRAGMA foreign_keys = ON;",
		"PRAGMA busy_timeout = 5000;",
	}
	for _, p := range pragmas {
		if err := sqlitex.ExecuteTransient(conn, p, nil); err != nil {
			return fmt.Errorf("db: %s: %w", p, err)
		}
	}
	return nil
}

// fileURI builds a SQLite "file:" URI so OpenURI takes effect. SQLite
// accepts both "file:relative" and "file:/absolute" forms; we percent-encode
// the path so values containing spaces or "?" do not confuse the URI
// parser.
func fileURI(path string) string {
	return "file:" + (&url.URL{Path: path}).EscapedPath()
}
