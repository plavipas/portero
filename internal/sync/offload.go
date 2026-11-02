package sync

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// isDataless reports whether the file's data is not on this Mac (an iCloud File Provider file that is
// offloaded). Replaced in tests.
var isDataless = platformDataless

// Download asks the system to fetch an offloaded file; it returns at once, the data arrives later.
// cmd/portero sets it to the macOS implementation; nil means "not supported".
var Download func(path string) error

// DownloadTimeout and PollEvery bound the wait for a requested download.
var (
	DownloadTimeout = 30 * time.Second
	PollEvery       = 250 * time.Millisecond
)

// ErrNotDownloaded means an offloaded synced file did not arrive in time; nothing was changed.
var ErrNotDownloaded = errors.New("the synced rules file in iCloud Drive is not downloaded yet (download started); nothing changed, try again in a minute")

// Offloaded reports whether path is a file iCloud Drive removed from this Mac: either the old placeholder
// ".<name>.icloud" stands in its place, or the file is there under its own name but dataless (macOS with the
// File Provider; reading it would block until the download finishes). A regular local file is never offloaded.
func Offloaded(path string) bool {
	fi, err := os.Lstat(path)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return false
		}
		ph, perr := os.Lstat(filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".icloud"))
		return perr == nil && ph.Mode().IsRegular()
	}
	return fi.Mode().IsRegular() && isDataless(fi)
}

// ensureLocal makes sure the data of path is on this Mac: when it is offloaded it starts the download and
// waits up to DownloadTimeout. It changes nothing else.
func ensureLocal(path string) error {
	if !Offloaded(path) {
		return nil
	}
	if Download == nil {
		return fmt.Errorf("%w: iCloud download is not supported on this system: %s", ErrNotDownloaded, path)
	}
	if err := Download(path); err != nil {
		return fmt.Errorf("%w: %v: %s", ErrNotDownloaded, err, path)
	}
	deadline := time.Now().Add(DownloadTimeout)
	for {
		if !Offloaded(path) {
			if _, err := os.Lstat(path); err == nil {
				return nil
			}
		}
		if !time.Now().Before(deadline) {
			return fmt.Errorf("%w: %s", ErrNotDownloaded, path)
		}
		time.Sleep(PollEvery)
	}
}
