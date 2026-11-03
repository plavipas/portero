//go:build unix

package rulesfile

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// Lock takes an exclusive advisory lock on rules.lock in dir (created if needed), waiting for another writer,
// and returns the function that releases it. The lock file stays; it carries no data.
func Lock(dir string) (unlock func(), err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("lock: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "rules.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("lock: %w", err)
	}
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX)
		if err != syscall.EINTR {
			break
		}
	}
	if err != nil {
		f.Close()
		return nil, fmt.Errorf("lock: %w", err)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
