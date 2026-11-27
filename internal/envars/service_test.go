package envars

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Hopetree/spool/internal/credential"
	"github.com/Hopetree/spool/internal/store"
)

func newService(t *testing.T) *Service {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(); err != nil {
		t.Fatal(err)
	}
	cipher, err := credential.NewCipher([]byte(strings.Repeat("k", 32)))
	if err != nil {
		t.Fatal(err)
	}
	return NewService(st.DB, cipher)
}

func TestEnvVarCRUDAndValues(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	created, err := svc.Create(ctx, Input{Name: "SP_APP_URL", Value: "http://demo", Description: "demo", Sensitive: false}, "admin")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if created.UpdatedByName != "admin" {
		t.Fatalf("created = %+v", created)
	}
	values, err := svc.Values(ctx)
	if err != nil || values["SP_APP_URL"] != "http://demo" {
		t.Fatalf("values = %v err=%v", values, err)
	}

	updated, err := svc.Update(ctx, created.ID, Input{Name: "SP_APP_URL", Value: "", Description: "updated", Sensitive: true}, "admin")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if !updated.Sensitive || updated.Description != "updated" {
		t.Fatalf("updated = %+v", updated)
	}
	values, err = svc.Values(ctx)
	if err != nil || values["SP_APP_URL"] != "http://demo" {
		t.Fatalf("values after blank update = %v err=%v", values, err)
	}

	list, err := svc.List(ctx)
	if err != nil || len(list) != 1 || list[0].Sensitive != true {
		t.Fatalf("list = %+v err=%v", list, err)
	}
	if err := svc.Delete(ctx, created.ID); err != nil {
		t.Fatal(err)
	}
	values, err = svc.Values(ctx)
	if err != nil || len(values) != 0 {
		t.Fatalf("values after delete = %v err=%v", values, err)
	}
}

func TestRevealValue(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	created, err := svc.Create(ctx, Input{Name: "SP_SECRET", Value: "hidden", Sensitive: true}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	list, err := svc.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if list[0].Value != "" {
		t.Fatalf("sensitive value leaked in list: %+v", list[0])
	}
	value, err := svc.RevealValue(ctx, created.ID)
	if err != nil || value != "hidden" {
		t.Fatalf("reveal = %q err=%v", value, err)
	}
	if _, err := svc.RevealValue(ctx, "nope"); err == nil {
		t.Fatal("expected missing env var to fail")
	}
}

func TestEnvVarValidation(t *testing.T) {
	svc := newService(t)
	ctx := context.Background()
	cases := []Input{
		{Name: "lower", Value: "x"},
		{Name: "SP_WITH_1", Value: "x"},
		{Name: "APP_URL", Value: "x"},
		{Name: "SP_", Value: "x"},
		{Name: "SP_TOO_LONG", Value: strings.Repeat("x", 4097)},
		{Name: "SP_OK", Value: ""},
	}
	for _, in := range cases {
		if _, err := svc.Create(ctx, in, "admin"); err == nil {
			t.Fatalf("%+v: expected error", in)
		}
	}
	in := Input{Name: "SP_OK", Value: "x"}
	created, err := svc.Create(ctx, in, "admin")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Create(ctx, in, "admin"); err == nil {
		t.Fatal("expected duplicate name to fail")
	}
	if _, err := svc.Get(ctx, "nope"); err == nil {
		t.Fatal("expected missing env var to fail")
	}
	_ = created
}
