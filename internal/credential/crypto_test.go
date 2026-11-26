package credential

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/Hopetree/spool/internal/store"
)

func testKey(fill byte) []byte {
	return bytes.Repeat([]byte{fill}, 32)
}

func TestCipherRoundTrip(t *testing.T) {
	c, err := NewCipher(testKey(1))
	if err != nil {
		t.Fatal(err)
	}
	plaintext := []byte("super-secret-password")
	ct, err := c.Encrypt(plaintext, "cred-1")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ct, plaintext) {
		t.Fatal("ciphertext contains plaintext")
	}
	got, err := c.Decrypt(ct, "cred-1")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, plaintext) {
		t.Fatalf("decrypt = %q", got)
	}
}

func TestCipherWrongAAD(t *testing.T) {
	c, _ := NewCipher(testKey(1))
	ct, _ := c.Encrypt([]byte("x"), "cred-1")
	if _, err := c.Decrypt(ct, "cred-2"); err == nil {
		t.Fatal("expected error with different AAD")
	}
}

func TestCipherTampered(t *testing.T) {
	c, _ := NewCipher(testKey(1))
	ct, _ := c.Encrypt([]byte("x"), "cred-1")
	ct[len(ct)-1] ^= 0xff
	if _, err := c.Decrypt(ct, "cred-1"); err == nil {
		t.Fatal("expected error with tampered ciphertext")
	}
}

func TestNewCipherRejectsBadKey(t *testing.T) {
	if _, err := NewCipher([]byte("short")); err == nil {
		t.Fatal("expected error for short key")
	}
}

func TestLoadOrCreateMasterKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "master.key")
	k1, err := LoadOrCreateMasterKey(path)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(k1) != 32 {
		t.Fatalf("key len = %d", len(k1))
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("perm = %o", info.Mode().Perm())
	}
	k2, err := LoadOrCreateMasterKey(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !bytes.Equal(k1, k2) {
		t.Fatal("key changed between loads")
	}
}

func TestCheckMasterKeyMismatch(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	if err := st.Migrate(); err != nil {
		t.Fatal(err)
	}
	cipherA, _ := NewCipher(testKey(1))
	cipherB, _ := NewCipher(testKey(2))
	secret, _ := cipherA.Encrypt([]byte("pw"), "cred-1")
	_, err = st.DB.Exec(`INSERT INTO credentials (id, name, auth_type, username, secret_enc, created_at, updated_at)
		VALUES ('cred-1', 'c1', 'password', 'root', ?, 1, 1)`, secret)
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckMasterKey(context.Background(), st.DB, cipherA); err != nil {
		t.Fatalf("matching key: %v", err)
	}
	if err := CheckMasterKey(context.Background(), st.DB, cipherB); !errors.Is(err, ErrMasterKeyMismatch) {
		t.Fatalf("err = %v, want ErrMasterKeyMismatch", err)
	}
}
