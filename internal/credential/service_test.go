package credential

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"

	"github.com/Hopetree/spool/internal/store"
)

func newTestService(t *testing.T) (*Service, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(); err != nil {
		t.Fatal(err)
	}
	cipher, err := NewCipher(testKey(1))
	if err != nil {
		t.Fatal(err)
	}
	return NewService(st.DB, cipher), st
}

func TestCreateGetListOpen(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	c, err := svc.Create(ctx, Input{
		Name: "prod-root", AuthType: "password", Username: "root", Secret: "s3cret", Description: "prod",
	}, "admin-id")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if c.ID == "" || c.Name != "prod-root" || c.AuthType != "password" || c.HasPassphrase {
		t.Fatalf("unexpected credential: %+v", c)
	}
	got, err := svc.Get(ctx, c.ID)
	if err != nil || got.Name != "prod-root" {
		t.Fatalf("get: %+v %v", got, err)
	}
	list, err := svc.List(ctx)
	if err != nil || len(list) != 1 {
		t.Fatalf("list: %v %v", list, err)
	}
	authType, username, secret, passphrase, err := svc.Open(ctx, c.ID)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if authType != "password" || username != "root" || secret != "s3cret" || passphrase != "" {
		t.Fatalf("open = %q %q %q %q", authType, username, secret, passphrase)
	}
}

func TestCreateValidation(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	cases := []Input{
		{AuthType: "password", Username: "root", Secret: "x"},
		{Name: "n", AuthType: "password", Secret: "x"},
		{Name: "n", AuthType: "password", Username: "root"},
		{Name: "n", AuthType: "weird", Username: "root", Secret: "x"},
		{Name: "n", AuthType: "password", Username: "root", Secret: "x", Passphrase: "p"},
		{Name: "n", AuthType: "private_key", Username: "root"},
	}
	for i, in := range cases {
		if _, err := svc.Create(ctx, in, ""); err == nil {
			t.Fatalf("case %d: expected validation error", i)
		}
	}
}

func TestUpdateKeepsSecretWhenEmpty(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	c, _ := svc.Create(ctx, Input{Name: "c1", AuthType: "password", Username: "root", Secret: "old-secret"}, "")
	updated, err := svc.Update(ctx, c.ID, Input{Name: "c1-renamed", AuthType: "password", Username: "root", Description: "d"})
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Name != "c1-renamed" || updated.Description != "d" {
		t.Fatalf("updated = %+v", updated)
	}
	_, _, secret, _, _ := svc.Open(ctx, c.ID)
	if secret != "old-secret" {
		t.Fatalf("secret = %q, want unchanged", secret)
	}
}

func TestUpdateReplacesSecret(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	c, _ := svc.Create(ctx, Input{Name: "c1", AuthType: "password", Username: "root", Secret: "old"}, "")
	if _, err := svc.Update(ctx, c.ID, Input{Name: "c1", AuthType: "password", Username: "root", Secret: "new"}); err != nil {
		t.Fatal(err)
	}
	_, _, secret, _, _ := svc.Open(ctx, c.ID)
	if secret != "new" {
		t.Fatalf("secret = %q", secret)
	}
}

func TestUpdateAuthTypeRequiresSecret(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	c, _ := svc.Create(ctx, Input{Name: "c1", AuthType: "password", Username: "root", Secret: "old"}, "")
	if _, err := svc.Update(ctx, c.ID, Input{Name: "c1", AuthType: "private_key", Username: "root"}); err == nil {
		t.Fatal("expected error when switching auth type without secret")
	}
}

func TestPrivateKeyWithPassphrase(t *testing.T) {
	svc, _ := newTestService(t)
	ctx := context.Background()
	c, err := svc.Create(ctx, Input{
		Name: "key1", AuthType: "private_key", Username: "ubuntu",
		Secret: "-----BEGIN KEY-----\nabc\n-----END KEY-----", Passphrase: "pp",
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if !c.HasPassphrase {
		t.Fatal("HasPassphrase should be true")
	}
	_, _, secret, passphrase, _ := svc.Open(ctx, c.ID)
	if !bytes.Contains([]byte(secret), []byte("BEGIN KEY")) || passphrase != "pp" {
		t.Fatalf("secret/passphrase mismatch")
	}
}

func TestDeleteInUse(t *testing.T) {
	svc, st := newTestService(t)
	ctx := context.Background()
	c, _ := svc.Create(ctx, Input{Name: "c1", AuthType: "password", Username: "root", Secret: "x"}, "")
	_, err := st.DB.Exec(`INSERT INTO hosts (id, name, address, port, credential_id, created_at, updated_at)
		VALUES ('h1', 'web-1', '10.0.0.1', 22, ?, 1, 1)`, c.ID)
	if err != nil {
		t.Fatal(err)
	}
	err = svc.Delete(ctx, c.ID)
	var inUse *ErrInUse
	if !errors.As(err, &inUse) || inUse.Hosts != 1 {
		t.Fatalf("err = %v", err)
	}
	if _, err := st.DB.Exec(`DELETE FROM hosts WHERE id = 'h1'`); err != nil {
		t.Fatal(err)
	}
	if err := svc.Delete(ctx, c.ID); err != nil {
		t.Fatalf("delete after host removal: %v", err)
	}
	if _, err := svc.Get(ctx, c.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get after delete = %v", err)
	}
}

func TestOpenWrongKey(t *testing.T) {
	svc, st := newTestService(t)
	ctx := context.Background()
	c, _ := svc.Create(ctx, Input{Name: "c1", AuthType: "password", Username: "root", Secret: "x"}, "")
	other, _ := NewCipher(testKey(9))
	wrong := NewService(st.DB, other)
	if _, _, _, _, err := wrong.Open(ctx, c.ID); err == nil {
		t.Fatal("expected decryption failure with wrong key")
	}
}
