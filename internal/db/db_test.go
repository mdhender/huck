package db

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"zombiezen.com/go/sqlite"
	"zombiezen.com/go/sqlite/sqlitex"
)

func TestOpenMissingFile(t *testing.T) {
	t.Parallel()
	_, err := Open(filepath.Join(t.TempDir(), "nope.db"))
	if !errors.Is(err, ErrMissing) {
		t.Fatalf("want ErrMissing, got %v", err)
	}
}

func TestCreateThenOpen(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "huck.db")

	if err := Create(path); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// Refusing to overwrite is part of the contract.
	if err := Create(path); err == nil {
		t.Fatal("Create should refuse to overwrite an existing file")
	}

	pool, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })

	// Schema must be in place after Create — including the invites table.
	conn, err := pool.Take(context.Background())
	if err != nil {
		t.Fatalf("Take: %v", err)
	}
	defer pool.Put(conn)

	var tables []string
	err = sqlitex.Execute(conn,
		`SELECT name FROM sqlite_master WHERE type='table' ORDER BY name;`,
		&sqlitex.ExecOptions{
			ResultFunc: func(stmt *sqlite.Stmt) error {
				tables = append(tables, stmt.ColumnText(0))
				return nil
			},
		})
	if err != nil {
		t.Fatalf("list tables: %v", err)
	}
	for _, want := range []string{"users", "invites", "schema_migrations"} {
		if !contains(tables, want) {
			t.Errorf("expected table %q in %v", want, tables)
		}
	}
}

func TestMigrateIdempotentAndBootstrap(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "huck.db")

	// Manually create an empty SQLite file (no schema_migrations yet) so
	// we exercise the bootstrap branch — the very first run must tolerate
	// the table not existing.
	pool, err := sqlitex.NewPool("file:"+path, sqlitex.PoolOptions{
		Flags:       sqlite.OpenReadWrite | sqlite.OpenCreate | sqlite.OpenWAL | sqlite.OpenURI,
		PoolSize:    2,
		PrepareConn: prepareConn,
	})
	if err != nil {
		t.Fatalf("seed pool: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })

	if err := Migrate(pool); err != nil {
		t.Fatalf("first Migrate: %v", err)
	}
	count1, err := AppliedCount(pool)
	if err != nil {
		t.Fatalf("AppliedCount: %v", err)
	}
	if count1 == 0 {
		t.Fatal("expected at least one migration applied")
	}

	// Second run must be a no-op.
	if err := Migrate(pool); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
	count2, err := AppliedCount(pool)
	if err != nil {
		t.Fatalf("AppliedCount: %v", err)
	}
	if count2 != count1 {
		t.Fatalf("expected idempotent migration; got %d then %d", count1, count2)
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
