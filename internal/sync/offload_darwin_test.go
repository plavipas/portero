//go:build darwin

package sync

import (
	"os"
	"path/filepath"
	"syscall"
	"testing"
)

// These tests use the real platformDataless (SF_DATALESS) check on files in a temporary folder, which is
// never managed by iCloud, so Offloaded must be false for every one of them.

func TestOffloadedFalseForRegularFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "rules.yaml")
	if err := os.WriteFile(p, []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if Offloaded(p) {
		t.Error("regular file reported as offloaded")
	}
}

func TestOffloadedFalseForEmptyFreshFile(t *testing.T) {
	p := filepath.Join(t.TempDir(), "rules.yaml")
	f, err := os.Create(p)
	if err != nil {
		t.Fatal(err)
	}
	f.Close()
	if Offloaded(p) {
		t.Error("fresh empty file reported as offloaded")
	}
}

func TestOffloadedFalseForOtherBSDFlags(t *testing.T) {
	for name, flag := range map[string]int{
		"uchg":   0x00000002, // UF_IMMUTABLE
		"hidden": 0x00008000, // UF_HIDDEN
		"nodump": 0x00000001, // UF_NODUMP
		"both":   0x00008002,
	} {
		t.Run(name, func(t *testing.T) {
			p := filepath.Join(t.TempDir(), "rules.yaml")
			if err := os.WriteFile(p, []byte("version: 1\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if err := syscall.Chflags(p, flag); err != nil {
				t.Skipf("chflags: %v", err)
			}
			t.Cleanup(func() { _ = syscall.Chflags(p, 0) })
			if Offloaded(p) {
				t.Errorf("file with flags %#x reported as offloaded", flag)
			}
		})
	}
}

func TestOffloadedFalseForSymlinkToRegularFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "real.yaml")
	if err := os.WriteFile(target, []byte("version: 1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "rules.yaml")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	if Offloaded(link) {
		t.Error("symlink to a regular file reported as offloaded")
	}
}

func TestOffloadedFalseForDirectory(t *testing.T) {
	p := filepath.Join(t.TempDir(), "rules.yaml")
	if err := os.Mkdir(p, 0o755); err != nil {
		t.Fatal(err)
	}
	if Offloaded(p) {
		t.Error("directory reported as offloaded")
	}
}

func TestOffloadedFalseForMissingFileWithoutPlaceholder(t *testing.T) {
	if Offloaded(filepath.Join(t.TempDir(), "rules.yaml")) {
		t.Error("missing file reported as offloaded")
	}
}

func TestOffloadedFalseWhenPlaceholderIsADirectory(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, ".rules.yaml.icloud"), 0o755); err != nil {
		t.Fatal(err)
	}
	if Offloaded(filepath.Join(dir, "rules.yaml")) {
		t.Error("a directory named like the placeholder reported as offloaded")
	}
}

func TestOffloadedFalseForPlaceholderOfAnotherName(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".other.yaml.icloud"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if Offloaded(filepath.Join(dir, "rules.yaml")) {
		t.Error("placeholder of another file reported as offloaded")
	}
}

func TestPlatformDatalessDetectsFlag(t *testing.T) {
	// SF_DATALESS can only be set by the kernel/File Provider, so the check is exercised on a synthetic Stat_t.
	fi := fakeInfo{st: &syscall.Stat_t{Flags: sfDataless}}
	if !platformDataless(fi) {
		t.Error("SF_DATALESS not detected")
	}
	if platformDataless(fakeInfo{st: &syscall.Stat_t{Flags: 0x00008002}}) {
		t.Error("UF_HIDDEN|UF_IMMUTABLE detected as dataless")
	}
}

type fakeInfo struct {
	os.FileInfo
	st *syscall.Stat_t
}

func (f fakeInfo) Sys() any { return f.st }
