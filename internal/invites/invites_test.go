package invites_test

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"zombiezen.com/go/sqlite/sqlitex"

	"github.com/mdhender/huck/internal/db"
	"github.com/mdhender/huck/internal/invites"
	"github.com/mdhender/huck/internal/users"
)

// fixture wires a fresh DB, a Store under test, and a seeded admin
// user whose id is the value tests pass to Create as invitedBy. The
// pool is exposed so tests that need to call Consume (which takes an
// explicit *sqlite.Conn) can do the take/put dance.
type fixture struct {
	pool   *sqlitex.Pool
	store  *invites.Store
	admin  users.User
}

func newFixture(t *testing.T) *fixture {
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

	us := users.NewStore(pool)
	admin, err := us.Create(context.Background(), users.NewUser{
		Handle:       "admin",
		Email:        "admin@example.com",
		PasswordHash: "x",
		IsAdmin:      true,
	})
	if err != nil {
		t.Fatalf("seed admin: %v", err)
	}

	return &fixture{
		pool:  pool,
		store: invites.NewStore(pool),
		admin: admin,
	}
}

// backdateExpiry rewrites the stored expires_at for token to a past
// timestamp so the "expired" branches can be exercised without sleep.
func (f *fixture) backdateExpiry(t *testing.T, tok invites.Token) {
	t.Helper()
	conn, err := f.pool.Take(context.Background())
	if err != nil {
		t.Fatalf("pool.Take: %v", err)
	}
	defer f.pool.Put(conn)
	past := time.Now().UTC().Add(-time.Hour).Format(time.RFC3339Nano)
	if err := sqlitex.Execute(conn,
		`UPDATE invites SET expires_at = ? WHERE token = ?;`,
		&sqlitex.ExecOptions{Args: []any{past, tok.String()}}); err != nil {
		t.Fatalf("backdate: %v", err)
	}
	if conn.Changes() != 1 {
		t.Fatalf("backdate: expected 1 row changed, got %d", conn.Changes())
	}
}

func TestGenerate(t *testing.T) {
	t.Parallel()

	a, err := invites.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	b, err := invites.Generate()
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if a == b {
		t.Fatal("Generate returned identical tokens twice")
	}
	// 32 bytes base64.RawURLEncoding → 43 characters, no padding.
	if got := len(a.String()); got != 43 {
		t.Errorf("token length = %d, want 43", got)
	}
	if strings.ContainsAny(a.String(), "=+/") {
		t.Errorf("token %q is not base64url (contains pad/non-url chars)", a)
	}
}

func TestTokenLogValueRedacts(t *testing.T) {
	t.Parallel()
	tok := invites.Token("super-secret")
	if got := tok.LogValue().String(); !strings.Contains(got, "REDACTED") {
		t.Errorf("LogValue = %q, want a redacted form", got)
	}
}

func TestCreateAndGet(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()

	got, err := f.store.Create(ctx, invites.NewInvite{
		Email:     "  NewUser@Example.COM ",
		InvitedBy: f.admin.ID,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if got.Email != "newuser@example.com" {
		t.Errorf("email not normalised: %q", got.Email)
	}
	if got.Token == "" {
		t.Error("expected non-empty token")
	}
	if got.InvitedBy != f.admin.ID {
		t.Errorf("InvitedBy = %d, want %d", got.InvitedBy, f.admin.ID)
	}
	if got.IsAdmin {
		t.Error("default invite should not be admin")
	}
	if got.Consumed() {
		t.Error("freshly created invite should not be consumed")
	}
	if got.Revoked() {
		t.Error("freshly created invite should not be revoked")
	}
	want := got.CreatedAt.Add(7 * 24 * time.Hour)
	if delta := got.ExpiresAt.Sub(want); delta < -time.Second || delta > time.Second {
		t.Errorf("ExpiresAt = %v, want ~created_at+7d (%v)", got.ExpiresAt, want)
	}

	round, err := f.store.GetByToken(ctx, got.Token)
	if err != nil {
		t.Fatalf("GetByToken: %v", err)
	}
	if round.Token != got.Token || round.Email != got.Email || round.IsAdmin != got.IsAdmin {
		t.Errorf("round trip mismatch: %+v vs %+v", round, got)
	}
}

func TestCreateAdminRoundTrip(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()

	got, err := f.store.Create(ctx, invites.NewInvite{
		Email:     "boss@example.com",
		InvitedBy: f.admin.ID,
		IsAdmin:   true,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if !got.IsAdmin {
		t.Error("Create returned IsAdmin = false for an admin invite")
	}
	round, err := f.store.GetByToken(ctx, got.Token)
	if err != nil {
		t.Fatalf("GetByToken: %v", err)
	}
	if !round.IsAdmin {
		t.Errorf("round-tripped invite lost IsAdmin: %+v", round)
	}
}

func TestCreateRequiresEmail(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if _, err := f.store.Create(context.Background(), invites.NewInvite{
		Email: "   ", InvitedBy: f.admin.ID,
	}); err == nil {
		t.Fatal("expected error for empty email")
	}
}

func TestCreateRequiresInvitedBy(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if _, err := f.store.Create(context.Background(), invites.NewInvite{
		Email: "x@example.com",
	}); err == nil {
		t.Fatal("expected error for zero invitedBy")
	}
}

func TestCreateDuplicateActive(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()

	if _, err := f.store.Create(ctx, invites.NewInvite{
		Email: "dup@example.com", InvitedBy: f.admin.ID,
	}); err != nil {
		t.Fatalf("first Create: %v", err)
	}
	_, err := f.store.Create(ctx, invites.NewInvite{
		Email: "DUP@example.com", InvitedBy: f.admin.ID,
	})
	if !errors.Is(err, invites.ErrEmailAlreadyInvited) {
		t.Fatalf("want ErrEmailAlreadyInvited, got %v", err)
	}
}

// TestCreateAfterRevoke pins the partial-index predicate: the index now
// excludes both consumed and revoked rows, so a fresh Create for the
// same email must succeed after Revoke (sprint-5 T2.1 + T2.2).
func TestCreateAfterRevoke(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()

	first, err := f.store.Create(ctx, invites.NewInvite{
		Email: "again@example.com", InvitedBy: f.admin.ID,
	})
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}
	if err := f.store.Revoke(ctx, first.Token); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := f.store.Create(ctx, invites.NewInvite{
		Email: "again@example.com", InvitedBy: f.admin.ID,
	}); err != nil {
		t.Fatalf("second Create after Revoke: %v", err)
	}
}

func TestCreateAfterConsume(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()

	first, err := f.store.Create(ctx, invites.NewInvite{
		Email: "again@example.com", InvitedBy: f.admin.ID,
	})
	if err != nil {
		t.Fatalf("first Create: %v", err)
	}

	conn, err := f.pool.Take(ctx)
	if err != nil {
		t.Fatalf("pool.Take: %v", err)
	}
	if err := f.store.Consume(ctx, conn, first.Token); err != nil {
		f.pool.Put(conn)
		t.Fatalf("Consume: %v", err)
	}
	f.pool.Put(conn)

	if _, err := f.store.Create(ctx, invites.NewInvite{
		Email: "again@example.com", InvitedBy: f.admin.ID,
	}); err != nil {
		t.Fatalf("second Create after Consume: %v", err)
	}
}

func TestGetByTokenNotFound(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	_, err := f.store.GetByToken(context.Background(), invites.Token("nope"))
	if !errors.Is(err, invites.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestResendRefreshesExpiry(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()

	inv, err := f.store.Create(ctx, invites.NewInvite{
		Email: "re@example.com", InvitedBy: f.admin.ID,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	f.backdateExpiry(t, inv.Token)

	got, err := f.store.Resend(ctx, inv.Token)
	if err != nil {
		t.Fatalf("Resend: %v", err)
	}
	if got.Expired(time.Now().UTC()) {
		t.Errorf("Resend left expires_at in the past: %v", got.ExpiresAt)
	}
	want := time.Now().UTC().Add(7 * 24 * time.Hour)
	if delta := got.ExpiresAt.Sub(want); delta < -time.Second || delta > time.Second {
		t.Errorf("ExpiresAt = %v, want ~now+7d (%v)", got.ExpiresAt, want)
	}
}

func TestResendConsumed(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()

	inv, err := f.store.Create(ctx, invites.NewInvite{
		Email: "rc@example.com", InvitedBy: f.admin.ID,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	conn, err := f.pool.Take(ctx)
	if err != nil {
		t.Fatalf("pool.Take: %v", err)
	}
	if err := f.store.Consume(ctx, conn, inv.Token); err != nil {
		f.pool.Put(conn)
		t.Fatalf("Consume: %v", err)
	}
	f.pool.Put(conn)

	if _, err := f.store.Resend(ctx, inv.Token); !errors.Is(err, invites.ErrConsumed) {
		t.Fatalf("want ErrConsumed, got %v", err)
	}
}

func TestResendRevoked(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()

	inv, err := f.store.Create(ctx, invites.NewInvite{
		Email: "rvres@example.com", InvitedBy: f.admin.ID,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := f.store.Revoke(ctx, inv.Token); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	if _, err := f.store.Resend(ctx, inv.Token); !errors.Is(err, invites.ErrRevoked) {
		t.Fatalf("want ErrRevoked, got %v", err)
	}
}

func TestResendNotFound(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	if _, err := f.store.Resend(context.Background(), invites.Token("nope")); !errors.Is(err, invites.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

// TestRevoke covers the soft-revoke contract: the row stays in the
// table with revoked_at set, GetByToken still returns it, and a second
// Revoke is idempotent at the caller surface (returns ErrNotFound).
func TestRevoke(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()

	inv, err := f.store.Create(ctx, invites.NewInvite{
		Email: "rv@example.com", InvitedBy: f.admin.ID,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := f.store.Revoke(ctx, inv.Token); err != nil {
		t.Fatalf("Revoke: %v", err)
	}
	got, err := f.store.GetByToken(ctx, inv.Token)
	if err != nil {
		t.Fatalf("after Revoke, GetByToken: %v", err)
	}
	if !got.Revoked() {
		t.Errorf("expected revoked_at to be set, got %+v", got)
	}
	if err := f.store.Revoke(ctx, inv.Token); !errors.Is(err, invites.ErrNotFound) {
		t.Fatalf("second Revoke want ErrNotFound, got %v", err)
	}
}

func TestStatus(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		inv  invites.Invite
		want string
	}{
		{
			name: "pending",
			inv: invites.Invite{
				CreatedAt: now.Add(-time.Hour),
				ExpiresAt: now.Add(time.Hour),
			},
			want: invites.StatusPending,
		},
		{
			name: "accepted",
			inv: invites.Invite{
				CreatedAt:  now.Add(-time.Hour),
				ExpiresAt:  now.Add(time.Hour),
				ConsumedAt: now.Add(-30 * time.Minute),
			},
			want: invites.StatusAccepted,
		},
		{
			name: "expired",
			inv: invites.Invite{
				CreatedAt: now.Add(-2 * time.Hour),
				ExpiresAt: now.Add(-time.Hour),
			},
			want: invites.StatusExpired,
		},
		{
			name: "revoked wins over consumed",
			inv: invites.Invite{
				CreatedAt:  now.Add(-time.Hour),
				ExpiresAt:  now.Add(time.Hour),
				ConsumedAt: now.Add(-30 * time.Minute),
				RevokedAt:  now.Add(-15 * time.Minute),
			},
			want: invites.StatusRevoked,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := tc.inv.Status(now); got != tc.want {
				t.Errorf("Status = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestConsume(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()

	inv, err := f.store.Create(ctx, invites.NewInvite{
		Email: "cn@example.com", InvitedBy: f.admin.ID,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	conn, err := f.pool.Take(ctx)
	if err != nil {
		t.Fatalf("pool.Take: %v", err)
	}
	defer f.pool.Put(conn)

	if err := f.store.Consume(ctx, conn, inv.Token); err != nil {
		t.Fatalf("Consume: %v", err)
	}
	got, err := f.store.GetByToken(ctx, inv.Token)
	if err != nil {
		t.Fatalf("GetByToken: %v", err)
	}
	if !got.Consumed() {
		t.Error("expected consumed_at to be set")
	}
	if err := f.store.Consume(ctx, conn, inv.Token); !errors.Is(err, invites.ErrConsumed) {
		t.Fatalf("second Consume want ErrConsumed, got %v", err)
	}
}

func TestConsumeExpired(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()

	inv, err := f.store.Create(ctx, invites.NewInvite{
		Email: "ex@example.com", InvitedBy: f.admin.ID,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	f.backdateExpiry(t, inv.Token)

	conn, err := f.pool.Take(ctx)
	if err != nil {
		t.Fatalf("pool.Take: %v", err)
	}
	defer f.pool.Put(conn)
	if err := f.store.Consume(ctx, conn, inv.Token); !errors.Is(err, invites.ErrExpired) {
		t.Fatalf("want ErrExpired, got %v", err)
	}
}

func TestConsumeRevoked(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()

	inv, err := f.store.Create(ctx, invites.NewInvite{
		Email: "cnrev@example.com", InvitedBy: f.admin.ID,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := f.store.Revoke(ctx, inv.Token); err != nil {
		t.Fatalf("Revoke: %v", err)
	}

	conn, err := f.pool.Take(ctx)
	if err != nil {
		t.Fatalf("pool.Take: %v", err)
	}
	defer f.pool.Put(conn)
	if err := f.store.Consume(ctx, conn, inv.Token); !errors.Is(err, invites.ErrRevoked) {
		t.Fatalf("want ErrRevoked, got %v", err)
	}
}

func TestConsumeNotFound(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx := context.Background()
	conn, err := f.pool.Take(ctx)
	if err != nil {
		t.Fatalf("pool.Take: %v", err)
	}
	defer f.pool.Put(conn)
	if err := f.store.Consume(ctx, conn, invites.Token("nope")); !errors.Is(err, invites.ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

// TestConsumeCancelled asserts that Consume short-circuits on a
// cancelled ctx instead of issuing the UPDATE. It locks in the fail-fast
// half of the ctx contract documented on Consume; the other half
// (SQLITE_INTERRUPT via pool.Take's SetInterrupt wiring) is a property
// of the zombiezen driver and is exercised by its own tests.
func TestConsumeCancelled(t *testing.T) {
	t.Parallel()
	f := newFixture(t)

	inv, err := f.store.Create(context.Background(), invites.NewInvite{
		Email: "cn@example.com", InvitedBy: f.admin.ID,
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	conn, err := f.pool.Take(context.Background())
	if err != nil {
		t.Fatalf("pool.Take: %v", err)
	}
	defer f.pool.Put(conn)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := f.store.Consume(ctx, conn, inv.Token); !errors.Is(err, context.Canceled) {
		t.Fatalf("want context.Canceled, got %v", err)
	}

	// The invite must still be active — Consume must not have run the
	// UPDATE when ctx was already cancelled.
	got, err := f.store.GetByToken(context.Background(), inv.Token)
	if err != nil {
		t.Fatalf("GetByToken: %v", err)
	}
	if got.Consumed() {
		t.Fatal("invite was consumed despite cancelled ctx")
	}
}
