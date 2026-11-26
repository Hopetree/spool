package auth

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Hopetree/spool/internal/store"
)

func newTestAuth(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	return NewService(st.DB, time.Hour), st
}

func TestHashAndVerifyPassword(t *testing.T) {
	svc, _ := newTestAuth(t)
	h1, err := svc.HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	h2, err := svc.HashPassword("secret123")
	if err != nil {
		t.Fatal(err)
	}
	if h1 == h2 {
		t.Fatal("bcrypt hashes should differ")
	}
	if !svc.VerifyPassword(h1, "secret123") {
		t.Fatal("verify should succeed")
	}
	if svc.VerifyPassword(h1, "wrong-password1") {
		t.Fatal("verify should fail")
	}
}

func TestValidatePassword(t *testing.T) {
	cases := map[string]bool{
		"short1":     false,
		"abcdefgh":   false,
		"12345678":   false,
		"abc12345":   true,
		"a1b2c3d4e9": true,
	}
	for pw, wantOK := range cases {
		err := ValidatePassword(pw)
		if wantOK && err != nil {
			t.Fatalf("%q: unexpected error %v", pw, err)
		}
		if !wantOK && err == nil {
			t.Fatalf("%q: expected error", pw)
		}
	}
}

func TestBootstrapAdminCreatesOnce(t *testing.T) {
	svc, _ := newTestAuth(t)
	ctx := context.Background()

	pw, created, err := svc.BootstrapAdmin(ctx)
	if err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if !created {
		t.Fatal("expected created=true")
	}
	if len(pw) != 16 {
		t.Fatalf("password len = %d", len(pw))
	}
	u, err := svc.Authenticate(ctx, "admin", pw)
	if err != nil {
		t.Fatalf("authenticate: %v", err)
	}
	if u.Role != "admin" || !u.MustChangePassword {
		t.Fatalf("unexpected bootstrapped user: %+v", u)
	}

	_, created, err = svc.BootstrapAdmin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if created {
		t.Fatal("second bootstrap should not create")
	}
}

func TestAuthenticateRejectsDisabledUser(t *testing.T) {
	svc, st := newTestAuth(t)
	hash, _ := svc.HashPassword("secret123")
	now := time.Now().UnixMilli()
	_, err := st.DB.Exec(`INSERT INTO users (id, username, display_name, password_hash, role, enabled, must_change_password, created_at, updated_at)
		VALUES (?, 'disabled', 'D', ?, 'operator', 0, 0, ?, ?)`, store.NewULID(), hash, now, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(context.Background(), "disabled", "secret123"); !errors.Is(err, ErrUserDisabled) {
		t.Fatalf("err = %v, want ErrUserDisabled", err)
	}
}

func TestAuthenticateUnknownUser(t *testing.T) {
	svc, _ := newTestAuth(t)
	if _, err := svc.Authenticate(context.Background(), "nobody", "secret123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
}

func TestSessionLifecycle(t *testing.T) {
	svc, _ := newTestAuth(t)
	ctx := context.Background()
	pw, _, _ := svc.BootstrapAdmin(ctx)
	u, err := svc.Authenticate(ctx, "admin", pw)
	if err != nil {
		t.Fatal(err)
	}
	token, err := svc.CreateSession(ctx, u.ID, "127.0.0.1", "test")
	if err != nil {
		t.Fatalf("create session: %v", err)
	}
	got, err := svc.UserBySessionToken(ctx, token)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got.ID != u.ID {
		t.Fatalf("user id = %q, want %q", got.ID, u.ID)
	}
	if err := svc.DeleteSession(ctx, token); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UserBySessionToken(ctx, token); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("err = %v, want ErrSessionInvalid", err)
	}
}

func TestSessionExpiry(t *testing.T) {
	svc, _ := newTestAuth(t)
	base := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	svc.SetNowForTest(func() time.Time { return base })
	ctx := context.Background()
	pw, _, _ := svc.BootstrapAdmin(ctx)
	u, err := svc.Authenticate(ctx, "admin", pw)
	if err != nil {
		t.Fatal(err)
	}
	token, err := svc.CreateSession(ctx, u.ID, "", "")
	if err != nil {
		t.Fatal(err)
	}
	svc.SetNowForTest(func() time.Time { return base.Add(2 * time.Hour) })
	if _, err := svc.UserBySessionToken(ctx, token); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("err = %v, want ErrSessionInvalid", err)
	}
}

func TestChangePassword(t *testing.T) {
	svc, _ := newTestAuth(t)
	ctx := context.Background()
	pw, _, _ := svc.BootstrapAdmin(ctx)
	u, err := svc.Authenticate(ctx, "admin", pw)
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.ChangePassword(ctx, u.ID, "wrong-old-1", "newpass123"); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("err = %v, want ErrInvalidCredentials", err)
	}
	if err := svc.ChangePassword(ctx, u.ID, pw, "newpass123"); err != nil {
		t.Fatalf("change: %v", err)
	}
	if _, err := svc.Authenticate(ctx, "admin", pw); !errors.Is(err, ErrInvalidCredentials) {
		t.Fatalf("old password should fail, err = %v", err)
	}
	u2, err := svc.Authenticate(ctx, "admin", "newpass123")
	if err != nil {
		t.Fatalf("new password: %v", err)
	}
	if u2.MustChangePassword {
		t.Fatal("must_change_password should be cleared")
	}
}

func TestDeleteOtherSessions(t *testing.T) {
	svc, _ := newTestAuth(t)
	ctx := context.Background()
	pw, _, _ := svc.BootstrapAdmin(ctx)
	u, _ := svc.Authenticate(ctx, "admin", pw)
	keep, _ := svc.CreateSession(ctx, u.ID, "", "")
	drop, _ := svc.CreateSession(ctx, u.ID, "", "")
	if err := svc.DeleteOtherSessions(ctx, u.ID, keep); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.UserBySessionToken(ctx, keep); err != nil {
		t.Fatalf("kept session should survive: %v", err)
	}
	if _, err := svc.UserBySessionToken(ctx, drop); !errors.Is(err, ErrSessionInvalid) {
		t.Fatalf("dropped session should be invalid, err = %v", err)
	}
}
