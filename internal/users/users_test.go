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
	if !got.LastLoginAt.IsZero() {
		t.Errorf("LastLoginAt: want zero on fresh row, got %v", got.LastLoginAt)
	}
	if !got.SuspendedAt.IsZero() {
		t.Errorf("SuspendedAt: want zero on fresh row, got %v", got.SuspendedAt)
	}
	if got.IsSuspended() {
		t.Error("IsSuspended: want false on fresh row")
	}

	byHandle, err := s.GetByHandle(ctx, "ALICE")
	if err != nil {
		t.Fatalf("GetByHandle: %v", err)
	}
	if byHandle.ID != got.ID {
		t.Errorf("GetByHandle ID mismatch: %d vs %d", byHandle.ID, got.ID)
	}
}

// TestUniqueConstraints is the regression check for the T10
// classifier: it must distinguish handle-vs-email conflicts without
// depending on the SQLite driver's error wording. The third case
// pins the both-columns-conflict ordering — when an insert would
// violate both the handle and the email UNIQUE constraints, the
// classifier reports the handle hit first, matching the order
// handlers expect for "handle already in use" precedence.
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

	_, err = s.Create(ctx, users.NewUser{Handle: "alice", Email: "a@example.com", PasswordHash: "h"})
	if !errors.Is(err, users.ErrHandleTaken) {
		t.Fatalf("both-columns conflict: want ErrHandleTaken (handle checked first), got %v", err)
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

func TestListAll(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	if _, err := s.Create(ctx, users.NewUser{Handle: "alice", Email: "a@x", PasswordHash: "h", IsAdmin: true}); err != nil {
		t.Fatalf("Create alice: %v", err)
	}
	if _, err := s.Create(ctx, users.NewUser{Handle: "bob", Email: "b@x", PasswordHash: "h"}); err != nil {
		t.Fatalf("Create bob: %v", err)
	}

	all, err := s.ListAll(ctx)
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("len = %d, want 2", len(all))
	}
}

func TestSetAdmin(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	u, err := s.Create(ctx, users.NewUser{Handle: "alice", Email: "a@x", PasswordHash: "h", IsAdmin: true})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.SetAdmin(ctx, u.ID, false); err != nil {
		t.Fatalf("SetAdmin false: %v", err)
	}
	got, err := s.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.IsAdmin {
		t.Errorf("IsAdmin = true, want false after SetAdmin(false)")
	}
	if !got.UpdatedAt.After(u.UpdatedAt) {
		t.Errorf("updated_at not bumped: %v vs %v", got.UpdatedAt, u.UpdatedAt)
	}

	if err := s.SetAdmin(ctx, 9999, true); !errors.Is(err, users.ErrNotFound) {
		t.Fatalf("SetAdmin missing id: want ErrNotFound, got %v", err)
	}
}

func TestSetPassword(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	u, err := s.Create(ctx, users.NewUser{Handle: "alice", Email: "a@x", PasswordHash: "old", IsAdmin: false})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := s.SetPassword(ctx, u.ID, "new"); err != nil {
		t.Fatalf("SetPassword: %v", err)
	}
	got, err := s.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.PasswordHash != "new" {
		t.Errorf("PasswordHash = %q, want %q", got.PasswordHash, "new")
	}

	if err := s.SetPassword(ctx, u.ID, ""); err == nil {
		t.Error("SetPassword empty: expected error")
	}
	if err := s.SetPassword(ctx, 9999, "x"); !errors.Is(err, users.ErrNotFound) {
		t.Fatalf("SetPassword missing id: want ErrNotFound, got %v", err)
	}
}

func TestRecordLogin(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	u, err := s.Create(ctx, users.NewUser{Handle: "alice", Email: "a@x", PasswordHash: "h"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !u.LastLoginAt.IsZero() {
		t.Fatalf("fresh user LastLoginAt = %v, want zero", u.LastLoginAt)
	}

	if err := s.RecordLogin(ctx, u.ID); err != nil {
		t.Fatalf("RecordLogin: %v", err)
	}
	got, err := s.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got.LastLoginAt.IsZero() {
		t.Error("LastLoginAt: want non-zero after RecordLogin")
	}
	if !got.UpdatedAt.After(u.UpdatedAt) {
		t.Errorf("updated_at not bumped: %v vs %v", got.UpdatedAt, u.UpdatedAt)
	}

	if err := s.RecordLogin(ctx, 9999); !errors.Is(err, users.ErrNotFound) {
		t.Fatalf("RecordLogin missing id: want ErrNotFound, got %v", err)
	}
}

func TestSuspendAndReactivate(t *testing.T) {
	t.Parallel()
	s := newStore(t)
	ctx := context.Background()

	u, err := s.Create(ctx, users.NewUser{Handle: "alice", Email: "a@x", PasswordHash: "h"})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := s.Suspend(ctx, u.ID); err != nil {
		t.Fatalf("Suspend: %v", err)
	}
	got, err := s.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !got.IsSuspended() {
		t.Error("IsSuspended: want true after Suspend")
	}
	firstSuspendedAt := got.SuspendedAt

	// Suspending an already-suspended user is a no-op (still nil).
	if err := s.Suspend(ctx, u.ID); err != nil {
		t.Fatalf("Suspend (idempotent): %v", err)
	}
	got2, err := s.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !got2.SuspendedAt.Equal(firstSuspendedAt) {
		t.Errorf("Suspend re-stamped suspended_at: %v vs %v", got2.SuspendedAt, firstSuspendedAt)
	}

	if err := s.Reactivate(ctx, u.ID); err != nil {
		t.Fatalf("Reactivate: %v", err)
	}
	got3, err := s.GetByID(ctx, u.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if got3.IsSuspended() {
		t.Error("IsSuspended: want false after Reactivate")
	}
	if !got3.SuspendedAt.IsZero() {
		t.Errorf("SuspendedAt: want zero after Reactivate, got %v", got3.SuspendedAt)
	}

	if err := s.Suspend(ctx, 9999); !errors.Is(err, users.ErrNotFound) {
		t.Fatalf("Suspend missing id: want ErrNotFound, got %v", err)
	}
	if err := s.Reactivate(ctx, 9999); !errors.Is(err, users.ErrNotFound) {
		t.Fatalf("Reactivate missing id: want ErrNotFound, got %v", err)
	}
}
