//go:build unix

package rulesfile

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// lockWait is how long Lock waits for another writer before giving up (a var for tests); lockPoll is the pause
// between attempts. A writer that holds the lock for seconds is stuck, and the link that triggered the write has
// already opened.
var (
	lockWait = 5 * time.Second
	lockPoll = 50 * time.Millisecond
)

// Lock takes an exclusive advisory lock on rules.lock in dir (created if needed), waiting up to lockWait for
// another writer, and returns the function that releases it. The lock file stays; it carries no data.
func Lock(dir string) (unlock func(), err error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("lock: %w", err)
	}
	f, err := os.OpenFile(filepath.Join(dir, "rules.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("lock: %w", err)
	}
	deadline := time.Now().Add(lockWait)
	for {
		err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			break
		}
		if err != syscall.EINTR && err != syscall.EWOULDBLOCK {
			f.Close()
			return nil, fmt.Errorf("lock: %w", err)
		}
		if !time.Now().Before(deadline) {
			f.Close()
			return nil, fmt.Errorf("lock: rules.lock held by another writer (waited %s)", lockWait)
		}
		time.Sleep(lockPoll)
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		f.Close()
	}, nil
}
