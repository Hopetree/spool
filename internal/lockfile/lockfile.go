package lockfile

import (
	"fmt"
	"os"
	"syscall"
)

// Acquire takes an exclusive non-blocking flock on path and returns a release
// function. Only one process can hold the lock at a time.
func Acquire(path string) (func(), error) {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock file: %w", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("another spool instance is running: %w", err)
	}
	if err := f.Truncate(0); err == nil {
		_, _ = f.WriteString(fmt.Sprintf("%d", os.Getpid()))
	}
	release := func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}
	return release, nil
}
