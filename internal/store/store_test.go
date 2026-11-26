package store

import (
	"path/filepath"
	"testing"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	if err := s.Migrate(); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	return s
}

func TestMigrateCreatesCoreTables(t *testing.T) {
	s := openTestStore(t)
	for _, table := range []string{"users", "sessions", "audit_logs", "settings", "goose_db_version"} {
		var n int
		if err := s.DB.Get(&n, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name=?`, table); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Fatalf("table %s missing", table)
		}
	}
}

func TestMigrateIsIdempotent(t *testing.T) {
	s := openTestStore(t)
	if err := s.Migrate(); err != nil {
		t.Fatalf("second Migrate: %v", err)
	}
}

func TestSessionCascadeOnUserDelete(t *testing.T) {
	s := openTestStore(t)
	uid := NewULID()
	now := int64(1700000000000)
	_, err := s.DB.Exec(`INSERT INTO users (id, username, display_name, password_hash, role, enabled, must_change_password, created_at, updated_at)
		VALUES (?, 'u1', 'U1', 'x', 'admin', 1, 0, ?, ?)`, uid, now, now)
	if err != nil {
		t.Fatal(err)
	}
	sid := NewULID()
	_, err = s.DB.Exec(`INSERT INTO sessions (id, user_id, token_hash, expires_at, created_at) VALUES (?, ?, 'h', ?, ?)`, sid, uid, now+1000, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.DB.Exec(`DELETE FROM users WHERE id=?`, uid); err != nil {
		t.Fatal(err)
	}
	var n int
	if err := s.DB.Get(&n, `SELECT count(*) FROM sessions WHERE id=?`, sid); err != nil {
		t.Fatal(err)
	}
	if n != 0 {
		t.Fatal("session not cascaded")
	}
}

func TestULIDIsUniqueAndOrdered(t *testing.T) {
	a := NewULID()
	b := NewULID()
	if a == b {
		t.Fatal("expected distinct ULIDs")
	}
	if len(a) != 26 {
		t.Fatalf("ulid len = %d, want 26", len(a))
	}
	if a >= b {
		t.Fatalf("ulid.Make should be monotonic: %q >= %q", a, b)
	}
}
