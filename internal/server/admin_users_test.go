package server_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"github.com/mdhender/huck/internal/auth"
	"github.com/mdhender/huck/internal/users"
)

func TestAdminUsersAnonymousRedirected(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)

	client := &http.Client{
		CheckRedirect: func(*http.Request, []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}
	resp, err := client.Get(f.ts.URL + "/admin/users")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status: got %d, want 303", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/login" {
		t.Errorf("Location: got %q, want /login", loc)
	}
}

func TestAdminUsersNonAdminForbidden(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	client, _ := f.userClient(t)

	resp, err := client.Get(f.ts.URL + "/admin/users")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403", resp.StatusCode)
	}
}

func TestAdminUsersList(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	client, _ := f.adminClient(t)

	body := getBody(t, client, f.ts.URL+"/admin/users", http.StatusOK)
	if !strings.Contains(body, "admin@example.com") {
		t.Errorf("list missing admin email; body=%s", trim(body))
	}
	if !strings.Contains(body, "alice@example.com") {
		t.Errorf("list missing alice email; body=%s", trim(body))
	}
	// The current admin row should be marked "(you)".
	if !strings.Contains(body, "(you)") {
		t.Errorf("list missing self marker; body=%s", trim(body))
	}
}

func TestAdminUsersView(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	client, _ := f.adminClient(t)

	body := getBody(t, client,
		f.ts.URL+"/admin/users/"+strconv.FormatInt(f.user.ID, 10), http.StatusOK)
	if !strings.Contains(body, "alice@example.com") {
		t.Errorf("view missing email; body=%s", trim(body))
	}
}

func TestAdminUsersViewMissing(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	client, _ := f.adminClient(t)

	resp, err := client.Get(f.ts.URL + "/admin/users/9999")
	if err != nil {
		t.Fatalf("GET: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", resp.StatusCode)
	}
}

func TestAdminUsersEditForm(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	client, _ := f.adminClient(t)

	body := getBody(t, client,
		f.ts.URL+"/admin/users/"+strconv.FormatInt(f.user.ID, 10)+"/edit",
		http.StatusOK)
	if !strings.Contains(body, `name="is_admin"`) {
		t.Errorf("edit form missing is_admin checkbox; body=%s", trim(body))
	}
	if !strings.Contains(body, `name="password"`) {
		t.Errorf("edit form missing password input; body=%s", trim(body))
	}
}

func TestAdminUsersEditPromote(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	client, jar := f.adminClient(t)
	mustGet(t, client, f.ts.URL+"/admin/users", http.StatusOK)
	csrf := jar.value("_csrf")

	resp := mustPost(t, client,
		f.ts.URL+"/admin/users/"+strconv.FormatInt(f.user.ID, 10)+"/edit",
		url.Values{"_csrf": {csrf}, "is_admin": {"1"}})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status: got %d, want 303", resp.StatusCode)
	}

	got, err := f.usersStore.GetByID(context.Background(), f.user.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !got.IsAdmin {
		t.Errorf("IsAdmin = false, want true after promote")
	}
}

func TestAdminUsersEditResetPassword(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	client, jar := f.adminClient(t)
	mustGet(t, client, f.ts.URL+"/admin/users", http.StatusOK)
	csrf := jar.value("_csrf")

	const newPassword = "rotateMyKeysNow"
	resp := mustPost(t, client,
		f.ts.URL+"/admin/users/"+strconv.FormatInt(f.user.ID, 10)+"/edit",
		url.Values{"_csrf": {csrf}, "password": {newPassword}})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status: got %d, want 303", resp.StatusCode)
	}

	got, err := f.usersStore.GetByID(context.Background(), f.user.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if err := auth.Verify(got.PasswordHash, newPassword); err != nil {
		t.Errorf("new password does not verify: %v", err)
	}
	if err := auth.Verify(got.PasswordHash, adminPassword); !errors.Is(err, auth.ErrBadPassword) {
		t.Errorf("old password should no longer verify, got %v", err)
	}
}

func TestAdminUsersEditWeakPassword(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	client, jar := f.adminClient(t)
	mustGet(t, client, f.ts.URL+"/admin/users", http.StatusOK)
	csrf := jar.value("_csrf")

	resp := mustPost(t, client,
		f.ts.URL+"/admin/users/"+strconv.FormatInt(f.user.ID, 10)+"/edit",
		url.Values{"_csrf": {csrf}, "password": {"short"}})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d, want 422; body=%s", resp.StatusCode, trim(string(body)))
	}

	// Password unchanged: original still verifies.
	got, err := f.usersStore.GetByID(context.Background(), f.user.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if err := auth.Verify(got.PasswordHash, adminPassword); err != nil {
		t.Errorf("password should be unchanged, original no longer verifies: %v", err)
	}
}

func TestAdminUsersEditRefusesSelfDemote(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	client, jar := f.adminClient(t)
	mustGet(t, client, f.ts.URL+"/admin/users", http.StatusOK)
	csrf := jar.value("_csrf")

	// is_admin omitted → wantAdmin=false → demote-self.
	resp := mustPost(t, client,
		f.ts.URL+"/admin/users/"+strconv.FormatInt(f.admin.ID, 10)+"/edit",
		url.Values{"_csrf": {csrf}})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status: got %d, want 403; body=%s", resp.StatusCode, trim(string(body)))
	}

	// The admin's is_admin flag must still be true.
	got, err := f.usersStore.GetByID(context.Background(), f.admin.ID)
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if !got.IsAdmin {
		t.Errorf("self-demote leaked through: IsAdmin=false")
	}
}

func TestAdminUsersDelete(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	client, jar := f.adminClient(t)
	mustGet(t, client, f.ts.URL+"/admin/users", http.StatusOK)
	csrf := jar.value("_csrf")

	resp := mustPost(t, client,
		f.ts.URL+"/admin/users/"+strconv.FormatInt(f.user.ID, 10)+"/delete",
		url.Values{"_csrf": {csrf}})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("status: got %d, want 303", resp.StatusCode)
	}

	if _, err := f.usersStore.GetByID(context.Background(), f.user.ID); !errors.Is(err, users.ErrNotFound) {
		t.Errorf("after delete: want ErrNotFound, got %v", err)
	}
}

func TestAdminUsersDeleteRefusesSelf(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	client, jar := f.adminClient(t)
	mustGet(t, client, f.ts.URL+"/admin/users", http.StatusOK)
	csrf := jar.value("_csrf")

	resp := mustPost(t, client,
		f.ts.URL+"/admin/users/"+strconv.FormatInt(f.admin.ID, 10)+"/delete",
		url.Values{"_csrf": {csrf}})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("status: got %d, want 403", resp.StatusCode)
	}

	if _, err := f.usersStore.GetByID(context.Background(), f.admin.ID); err != nil {
		t.Errorf("admin should still exist after self-delete attempt: %v", err)
	}
}

func TestAdminUsersDeleteMissing(t *testing.T) {
	t.Parallel()
	f := newAdminFixture(t)
	client, jar := f.adminClient(t)
	mustGet(t, client, f.ts.URL+"/admin/users", http.StatusOK)
	csrf := jar.value("_csrf")

	resp := mustPost(t, client,
		f.ts.URL+"/admin/users/9999/delete",
		url.Values{"_csrf": {csrf}})
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("status: got %d, want 404", resp.StatusCode)
	}
}
