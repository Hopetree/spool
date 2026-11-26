package host

import (
	"bytes"
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/Hopetree/spool/internal/credential"
	"github.com/Hopetree/spool/internal/sshx"
	"github.com/Hopetree/spool/internal/store"
)

type fakeProber struct {
	res  sshx.ProbeResult
	err  error
	last sshx.Target
}

func (f *fakeProber) Probe(ctx context.Context, t sshx.Target) (sshx.ProbeResult, error) {
	f.last = t
	return f.res, f.err
}

func newTestHostService(t *testing.T) (*Service, *credential.Service, *fakeProber, *store.Store) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	if err := st.Migrate(); err != nil {
		t.Fatal(err)
	}
	cipher, err := credential.NewCipher(bytes.Repeat([]byte{1}, 32))
	if err != nil {
		t.Fatal(err)
	}
	creds := credential.NewService(st.DB, cipher)
	prober := &fakeProber{res: sshx.ProbeResult{
		User: "root", OS: "Linux", Kernel: "6.1", Arch: "x86_64", Distro: "Debian",
		BashPath: "/bin/bash", PythonPath: "/usr/bin/python3", SFTP: true,
		Fingerprint: "SHA256:new", KeyType: "ssh-ed25519", LatencyMS: 12,
	}}
	return NewService(st.DB, creds, prober, "tofu"), creds, prober, st
}

func createTestCredential(t *testing.T, creds *credential.Service) *credential.Credential {
	t.Helper()
	c, err := creds.Create(context.Background(), credential.Input{
		Name: "cred", AuthType: "password", Username: "root", Secret: "pw",
	}, "admin")
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func baseInput(credID string) Input {
	return Input{Name: "web-1", Address: "10.0.0.1", Port: 22, CredentialID: credID, Enabled: true, Tags: []string{"prod"}}
}

func TestHostCRUDAndTags(t *testing.T) {
	svc, creds, _, _ := newTestHostService(t)
	ctx := context.Background()
	cred := createTestCredential(t, creds)

	in := baseInput(cred.ID)
	in.Tags = []string{"prod", "b", "a", "a", " "}
	h, err := svc.Create(ctx, in)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(h.Tags) != 3 || h.Tags[0] != "a" || h.Tags[2] != "prod" {
		t.Fatalf("tags = %v", h.Tags)
	}
	if h.CredentialName != "cred" || !h.Enabled {
		t.Fatalf("host = %+v", h)
	}

	in.Tags = []string{"staging"}
	in.Note = "updated"
	if _, err := svc.Update(ctx, h.ID, in); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, _ := svc.Get(ctx, h.ID)
	if len(got.Tags) != 1 || got.Tags[0] != "staging" || got.Note != "updated" {
		t.Fatalf("host after update = %+v", got)
	}

	items, total, err := svc.List(ctx, Filter{Tag: "staging", Page: 1, PageSize: 10})
	if err != nil || total != 1 || len(items) != 1 {
		t.Fatalf("list by tag: %v total=%d items=%d", err, total, len(items))
	}
	items, total, _ = svc.List(ctx, Filter{Tag: "prod", Page: 1, PageSize: 10})
	if total != 0 || len(items) != 0 {
		t.Fatalf("stale tag still matches: total=%d", total)
	}
	enabled := false
	_, total, _ = svc.List(ctx, Filter{Enabled: &enabled, Page: 1, PageSize: 10})
	if total != 0 {
		t.Fatalf("disabled filter total = %d", total)
	}
}

func TestHostValidation(t *testing.T) {
	svc, creds, _, _ := newTestHostService(t)
	ctx := context.Background()
	cred := createTestCredential(t, creds)
	cases := []Input{
		{Name: "", Address: "1.1.1.1", Port: 22, CredentialID: cred.ID},
		{Name: "h", Address: "", Port: 22, CredentialID: cred.ID},
		{Name: "h", Address: "1.1.1.1", Port: -1, CredentialID: cred.ID},
		{Name: "h", Address: "1.1.1.1", Port: 70000, CredentialID: cred.ID},
		{Name: "h", Address: "1.1.1.1", Port: 22, CredentialID: "nope"},
		{Name: "h", Address: "1.1.1.1", Port: 22, CredentialID: cred.ID, GroupID: "nope"},
		{Name: "h", Address: "1.1.1.1", Port: 22, CredentialID: cred.ID, Tags: []string{"a,b"}},
	}
	for i, in := range cases {
		if _, err := svc.Create(ctx, in); err == nil {
			t.Fatalf("case %d: expected error", i)
		}
	}
}

func TestGroupCRUDAndHostCount(t *testing.T) {
	svc, creds, _, _ := newTestHostService(t)
	ctx := context.Background()
	cred := createTestCredential(t, creds)

	g, err := svc.CreateGroup(ctx, GroupInput{Name: "prod", Description: "production"})
	if err != nil {
		t.Fatalf("create group: %v", err)
	}
	if _, err := svc.CreateGroup(ctx, GroupInput{Name: "prod"}); err == nil {
		t.Fatal("duplicate group name should fail")
	}
	in := baseInput(cred.ID)
	in.GroupID = g.ID
	h, err := svc.Create(ctx, in)
	if err != nil {
		t.Fatal(err)
	}
	if h.GroupName != "prod" {
		t.Fatalf("group name = %q", h.GroupName)
	}
	groups, _ := svc.ListGroups(ctx)
	if len(groups) != 1 || groups[0].HostCount != 1 {
		t.Fatalf("groups = %+v", groups)
	}
	if err := svc.DeleteGroup(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	got, _ := svc.Get(ctx, h.ID)
	if got.GroupID != nil || got.GroupName != "" {
		t.Fatalf("host should be ungrouped: %+v", got)
	}
}

func TestTestTOFURecordsFingerprint(t *testing.T) {
	svc, creds, prober, _ := newTestHostService(t)
	ctx := context.Background()
	cred := createTestCredential(t, creds)
	h, _ := svc.Create(ctx, baseInput(cred.ID))

	res, err := svc.Test(ctx, h.ID)
	if err != nil {
		t.Fatalf("test: %v", err)
	}
	if !res.OK || !res.FingerprintRecorded || res.Fingerprint != "SHA256:new" {
		t.Fatalf("result = %+v", res)
	}
	if prober.last.ExpectedFingerprint != "" || prober.last.Policy != "tofu" {
		t.Fatalf("probe target = %+v", prober.last)
	}
	got, _ := svc.Get(ctx, h.ID)
	if got.Fingerprint != "SHA256:new" || got.FingerprintType != "ssh-ed25519" {
		t.Fatalf("stored fingerprint = %+v", got)
	}
	if got.Capability.User != "root" || !got.Capability.SFTP {
		t.Fatalf("capability = %+v", got.Capability)
	}
	if got.LastTestOK == nil || !*got.LastTestOK || got.LastTestAt == nil {
		t.Fatalf("last test fields = %+v", got)
	}

	// Second test must present the stored fingerprint.
	if _, err := svc.Test(ctx, h.ID); err != nil {
		t.Fatal(err)
	}
	if prober.last.ExpectedFingerprint != "SHA256:new" {
		t.Fatalf("expected fingerprint not passed: %+v", prober.last)
	}
}

func TestTestStrictEmptyFingerprint(t *testing.T) {
	svc, creds, prober, _ := newTestHostService(t)
	svc.policy = "strict"
	ctx := context.Background()
	cred := createTestCredential(t, creds)
	h, _ := svc.Create(ctx, baseInput(cred.ID))

	prober.err = &sshx.HostKeyError{Actual: "SHA256:new", KeyType: "ssh-ed25519"}
	_, err := svc.Test(ctx, h.ID)
	var keyErr *sshx.HostKeyError
	if !errors.As(err, &keyErr) || keyErr.Changed {
		t.Fatalf("err = %v", err)
	}
	got, _ := svc.Get(ctx, h.ID)
	if got.Fingerprint != "" {
		t.Fatalf("fingerprint should not be stored: %q", got.Fingerprint)
	}
	if got.LastTestOK == nil || *got.LastTestOK {
		t.Fatalf("last_test_ok should be false: %+v", got.LastTestOK)
	}
}

func TestTestFingerprintChanged(t *testing.T) {
	svc, creds, prober, _ := newTestHostService(t)
	ctx := context.Background()
	cred := createTestCredential(t, creds)
	h, _ := svc.Create(ctx, baseInput(cred.ID))
	if _, err := svc.SetFingerprint(ctx, h.ID, "SHA256:old", "ssh-ed25519"); err != nil {
		t.Fatal(err)
	}
	prober.err = &sshx.HostKeyError{Expected: "SHA256:old", Actual: "SHA256:new", KeyType: "ssh-ed25519", Changed: true}
	_, err := svc.Test(ctx, h.ID)
	var keyErr *sshx.HostKeyError
	if !errors.As(err, &keyErr) || !keyErr.Changed {
		t.Fatalf("err = %v", err)
	}
	got, _ := svc.Get(ctx, h.ID)
	if got.Fingerprint != "SHA256:old" {
		t.Fatalf("fingerprint should be unchanged: %q", got.Fingerprint)
	}
}

func TestHostNotFound(t *testing.T) {
	svc, _, _, _ := newTestHostService(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if _, err := svc.Get(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("get = %v", err)
	}
	if err := svc.Delete(ctx, "nope"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("delete = %v", err)
	}
	if _, err := svc.SetFingerprint(ctx, "nope", "SHA256:x", "ssh-ed25519"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("fingerprint = %v", err)
	}
}
