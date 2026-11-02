package sync

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInICloud(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	dir := ICloudDir()
	for p, want := range map[string]bool{
		filepath.Join(dir, "rules.yaml"): true,
		dir:                              true,
		dir + "X/rules.yaml":             false,
		"/tmp/rules.yaml":                false,
	} {
		if got := InICloud(p); got != want {
			t.Errorf("InICloud(%q) = %v, want %v", p, got, want)
		}
	}
}

func TestCopyFileReplacesAtomically(t *testing.T) {
	d := t.TempDir()
	src, dst, other := filepath.Join(d, "src"), filepath.Join(d, "dst"), filepath.Join(d, "other")
	os.WriteFile(src, []byte("new\n"), 0o600)
	os.WriteFile(other, []byte("keep\n"), 0o600)
	if err := os.Symlink(other, dst); err != nil {
		t.Fatal(err)
	}
	if err := copyFile(src, dst); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(other); string(b) != "keep\n" {
		t.Error("copy wrote through the symlink")
	}
	fi, _ := os.Lstat(dst)
	if b, _ := os.ReadFile(dst); string(b) != "new\n" || fi.Mode()&os.ModeSymlink != 0 || fi.Mode().Perm() != 0o600 {
		t.Errorf("dst = %q mode %v", b, fi.Mode())
	}
	if m, _ := filepath.Glob(filepath.Join(d, ".rules-*")); len(m) != 0 {
		t.Errorf("temp files left: %v", m)
	}
}
