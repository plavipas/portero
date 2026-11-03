// Package rulesfile is the one way Portero changes the rules file: the web UI (portero serve) and the link
// handler ("Remember") both write through Write, under the same lock. It is portable.
package rulesfile

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/plavipas/portero/internal/rules"
)

var (
	// ErrSymlink: the rules file is a symlink; writing would replace it, so the user edits the target by hand.
	ErrSymlink = errors.New("rules file is a symlink; edit the target by hand")
	// ErrOffloaded: the rules file is in iCloud Drive but not on this Mac; reading it would block.
	ErrOffloaded = errors.New("the rules file is offloaded by iCloud; its download was requested, reload in a few seconds")
	// ErrStale: the file differs from the content the new text was built from.
	ErrStale = errors.New("the rules file changed on disk; reload it before applying")
	// ErrInvalid: the new text does not parse as a rules file.
	ErrInvalid = errors.New("the new rules text is not valid")
)

// StaleError carries the hash of the file as it is now.
type StaleError struct{ Current string }

func (e *StaleError) Error() string   { return ErrStale.Error() }
func (e *StaleError) Is(t error) bool { return t == ErrStale }

// HashOf is the SHA-256 of b in hex; "" is the hash of a file that does not exist.
func HashOf(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// Result describes a finished write.
type Result struct {
	Hash   string // of the new content
	Backup string // path of the copy of the previous content; "" when there was no file
}

// Read returns the content of path and its hash; a missing file is empty content with hash "".
func Read(path string) (data []byte, hash string, err error) {
	data, err = os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, "", nil
	}
	if err != nil {
		return nil, "", err
	}
	return data, HashOf(data), nil
}

// Write replaces the rules file with text. The caller holds Lock. baseHash is the hash of the content text was
// built from ("" for a file that does not exist); the file is compared with it before anything is written and
// again right before the rename. offloaded (may be nil) reports a file iCloud removed from this Mac.
//
// Order: symlink check, offloaded check, parse of text, hash check, `.bak` of the old content, temp file in the
// same directory (mode 0600, fsync), hash check, rename.
func Write(path, text, baseHash string, offloaded func(path string) bool) (Result, error) {
	if fi, err := os.Lstat(path); err == nil && fi.Mode()&fs.ModeSymlink != 0 {
		return Result{}, ErrSymlink
	}
	if offloaded != nil && offloaded(path) {
		return Result{}, ErrOffloaded
	}
	if _, err := rules.Parse([]byte(text)); err != nil {
		return Result{}, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	old, cur, err := Read(path)
	if err != nil {
		return Result{}, fmt.Errorf("read rules file: %w", err)
	}
	if cur != baseHash {
		return Result{}, &StaleError{Current: cur}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return Result{}, fmt.Errorf("create the rules directory: %w", err)
	}
	var res Result
	if old != nil || cur != "" {
		res.Backup = path + ".bak"
		if err := writeAtomic(res.Backup, old, nil); err != nil {
			return Result{}, fmt.Errorf("write the backup: %w", err)
		}
	}
	recheck := func() error {
		_, now, err := Read(path)
		if err != nil {
			return fmt.Errorf("re-read rules file: %w", err)
		}
		if now != baseHash {
			return &StaleError{Current: now}
		}
		return nil
	}
	if err := writeAtomic(path, []byte(text), recheck); err != nil {
		return Result{}, err
	}
	res.Hash = HashOf([]byte(text))
	return res, nil
}

// writeAtomic writes data with mode 0600 through a temporary file in the same directory and a rename, so a
// reader never sees half a file and a symlink at path is replaced, not followed. beforeRename, when set, runs
// after the data is synced and may veto the rename.
func writeAtomic(path string, data []byte, beforeRename func() error) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".portero-*.tmp") // 0600
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, werr := f.Write(data)
	if werr == nil {
		werr = f.Sync()
	}
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr == nil && beforeRename != nil {
		werr = beforeRename()
	}
	if werr != nil {
		os.Remove(tmp)
		return werr
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}
