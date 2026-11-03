//go:build unix

package rulesfile

import (
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

// A second process holds the lock through its own descriptor (flock on a separate open file description).
func TestLockTimesOutAgainstForeignHolder(t *testing.T) {
	old := lockWait
	lockWait = 200 * time.Millisecond
	defer func() { lockWait = old }()
	dir := t.TempDir()
	f, err := os.OpenFile(filepath.Join(dir, "rules.lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		t.Fatal(err)
	}
	_, err = Lock(dir)
	if err == nil || !strings.Contains(err.Error(), "held by another writer") {
		t.Fatalf("err %v", err)
	}
	// Once the holder lets go, Lock succeeds.
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_UN); err != nil {
		t.Fatal(err)
	}
	unlock, err := Lock(dir)
	if err != nil {
		t.Fatalf("after release: %v", err)
	}
	unlock()
}
