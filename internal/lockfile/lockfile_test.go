package lockfile

import (
	"path/filepath"
	"testing"
)

func TestAcquireExcludesSecondHolder(t *testing.T) {
	p := filepath.Join(t.TempDir(), "spool.lock")
	release, err := Acquire(p)
	if err != nil {
		t.Fatalf("first acquire: %v", err)
	}
	defer release()
	if _, err := Acquire(p); err == nil {
		t.Fatal("second acquire should fail")
	}
}

func TestReleaseAllowsReacquire(t *testing.T) {
	p := filepath.Join(t.TempDir(), "spool.lock")
	release, err := Acquire(p)
	if err != nil {
		t.Fatalf("acquire: %v", err)
	}
	release()
	again, err := Acquire(p)
	if err != nil {
		t.Fatalf("reacquire: %v", err)
	}
	again()
}
