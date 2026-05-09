package users_test

import (
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/mdhender/huck/internal/db"
	"github.com/mdhender/huck/internal/users"
)

func newStore(t *testing.T) *users.Store {
	t.Helper()
	path := filepath.Join(t.TempDir(), "huck.db")
	if err := db.Create(path); err != nil {
		t.Fatalf("db.Create: %v", err)
	}
	pool, err := db.Open(path)
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { _ = pool.Close() })
	return users.NewStore(pool)
}

func TestCreateAndGet(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	got, err := s.Create(ctx, users.NewUser{
		Handle:       "  ALICE ",
		Email:        "Alice@Example.COM",
		PasswordHash: "hash",
		IsAdmin:      true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.Handle != "alice" {
		t.Errorf("handle not lowercased/trimmed: %q", got.Handle)
	}
	if got.Email != "alice@example.com" {
		t.Errorf("email not lowercased: %q", got.Email)
	}
	if !got.IsAdmin {
		t.Error("expected IsAdmin true")
	}

	byHandle, err := s.GetByHandle(ctx, "ALICE")
	if err != nil {
		t.Fatalf("GetByHandle: %v", err)
	}
	if byHandle.ID != got.ID {
		t.Errorf("GetByHandle ID mismatch: %d vs %d", byHandle.ID, got.ID)
	}
}

func TestUniqueConstraints(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	if _, err := s.Create(ctx, users.NewUser{Handle: "alice", Email: "a@example.com", PasswordHash: "h"}); err != nil {
		t.Fatalf("Create alice: %v", err)
	}

	_, err := s.Create(ctx, users.NewUser{Handle: "ALICE", Email: "other@example.com", PasswordHash: "h"})
	if !errors.Is(err, users.ErrHandleTaken) {
		t.Fatalf("want ErrHandleTaken, got %v", err)
	}

	_, err = s.Create(ctx, users.NewUser{Handle: "bob", Email: "A@example.com", PasswordHash: "h"})
	if !errors.Is(err, users.ErrEmailTaken) {
		t.Fatalf("want ErrEmailTaken, got %v", err)
	}
}

func TestAdminExists(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	yes, err := s.AdminExists(ctx)
	if err != nil {
		t.Fatalf("AdminExists: %v", err)
	}
	if yes {
		t.Fatal("fresh DB should not have an admin")
	}

	if _, err := s.Create(ctx, users.NewUser{Handle: "u", Email: "u@x", PasswordHash: "h"}); err != nil {
		t.Fatalf("Create non-admin: %v", err)
	}
	yes, _ = s.AdminExists(ctx)
	if yes {
		t.Fatal("non-admin should not register as admin")
	}

	if _, err := s.Create(ctx, users.NewUser{Handle: "a", Email: "a@x", PasswordHash: "h", IsAdmin: true}); err != nil {
		t.Fatalf("Create admin: %v", err)
	}
	yes, err = s.AdminExists(ctx)
	if err != nil {
		t.Fatalf("AdminExists: %v", err)
	}
	if !yes {
		t.Fatal("expected AdminExists true after creating admin")
	}
}

func TestGetByHandleNotFound(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	_, err := s.GetByHandle(context.Background(), "nope")
	if !errors.Is(err, users.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}
