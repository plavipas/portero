package sync

import (
	"errors"
	"time"

	"github.com/plavipas/portero/internal/config"
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

// rig sets up a fake HOME with an iCloud folder and an XDG config dir; returns the iCloud rules path.
func rig(t *testing.T) (icloud string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PORTERO_RULES", "")
	if err := os.MkdirAll(ICloudDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		Download, isDataless = nil, platformDataless
		DownloadTimeout, PollEvery = 30*time.Second, 250*time.Millisecond
	})
	DownloadTimeout, PollEvery = 200*time.Millisecond, 10*time.Millisecond
	return filepath.Join(ICloudDir(), "rules.yaml")
}

func placeholder(t *testing.T, icloud string) string {
	t.Helper()
	ph := filepath.Join(filepath.Dir(icloud), ".rules.yaml.icloud")
	if err := os.WriteFile(ph, []byte("plist"), 0o600); err != nil {
		t.Fatal(err)
	}
	return ph
}

func TestOffloaded(t *testing.T) {
	icloud := rig(t)
	if Offloaded(icloud) {
		t.Error("missing file without placeholder is not offloaded")
	}
	ph := placeholder(t, icloud)
	if !Offloaded(icloud) {
		t.Error("placeholder not detected")
	}
	os.Remove(ph)
	if err := os.WriteFile(icloud, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if Offloaded(icloud) {
		t.Error("a regular local file must not be offloaded")
	}
	isDataless = func(os.FileInfo) bool { return true }
	if !Offloaded(icloud) {
		t.Error("dataless file not detected")
	}
	if Offloaded(filepath.Dir(icloud)) {
		t.Error("a directory is never offloaded")
	}
}

func TestEnableOffloaded(t *testing.T) {
	setup := func(t *testing.T) (icloud, local string) {
		icloud = rig(t)
		placeholder(t, icloud)
		local = config.RulesFile()
		os.MkdirAll(filepath.Dir(local), 0o755)
		os.WriteFile(local, []byte("local\n"), 0o600)
		return
	}
	t.Run("download arrives", func(t *testing.T) {
		icloud, local := setup(t)
		calls := 0
		Download = func(p string) error {
			calls++
			os.Remove(filepath.Join(filepath.Dir(p), ".rules.yaml.icloud"))
			return os.WriteFile(p, []byte("cloud\n"), 0o600)
		}
		got, err := EnableICloud()
		if err != nil || got != icloud || calls != 1 {
			t.Fatalf("%q %v calls=%d", got, err, calls)
		}
		if b, _ := os.ReadFile(icloud); string(b) != "cloud\n" {
			t.Errorf("icloud = %q", b)
		}
		if b, _ := os.ReadFile(local + ".local-backup"); string(b) != "local\n" {
			t.Errorf("backup = %q", b)
		}
		if _, err := os.Stat(config.File()); err != nil {
			t.Error("config not written")
		}
	})
	for name, dl := range map[string]func(string) error{
		"timeout": func(string) error { return nil },
		"error":   func(string) error { return errors.New("boom") },
		"nil":     nil,
	} {
		t.Run(name, func(t *testing.T) {
			icloud, local := setup(t)
			Download = dl
			_, err := EnableICloud()
			if !errors.Is(err, ErrNotDownloaded) {
				t.Fatalf("err = %v", err)
			}
			if b, _ := os.ReadFile(local); string(b) != "local\n" {
				t.Errorf("local changed: %q", b)
			}
			if _, err := os.Stat(local + ".local-backup"); err == nil {
				t.Error("local moved to backup")
			}
			if _, err := os.Stat(config.File()); err == nil {
				t.Error("config written")
			}
			if _, err := os.Lstat(icloud); err == nil {
				t.Error("icloud file created")
			}
		})
	}
	t.Run("dataless", func(t *testing.T) {
		icloud := rig(t)
		os.WriteFile(icloud, []byte("cloud\n"), 0o600)
		local := config.RulesFile()
		os.MkdirAll(filepath.Dir(local), 0o755)
		os.WriteFile(local, []byte("local\n"), 0o600)
		on := true
		isDataless = func(os.FileInfo) bool { return on }
		Download = func(string) error { on = false; return nil }
		if _, err := EnableICloud(); err != nil {
			t.Fatal(err)
		}
	})
}

func TestDisableBackupAndOffloaded(t *testing.T) {
	icloud := rig(t)
	local := filepath.Join(config.Dir(), "rules.yaml")
	os.MkdirAll(config.Dir(), 0o755)
	os.WriteFile(local, []byte("local1\n"), 0o640)
	os.WriteFile(icloud, []byte("cloud1\n"), 0o600)
	os.WriteFile(config.File(), []byte("rules: "+icloud+"\n"), 0o600)
	if _, err := Disable(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(local + ".icloud-backup"); string(b) != "local1\n" {
		t.Errorf("backup = %q", b)
	}
	if b, _ := os.ReadFile(local); string(b) != "cloud1\n" {
		t.Errorf("local = %q", b)
	}
	// second round overwrites the backup
	os.WriteFile(local, []byte("local2\n"), 0o600)
	os.WriteFile(icloud, []byte("cloud2\n"), 0o600)
	os.WriteFile(config.File(), []byte("rules: "+icloud+"\n"), 0o600)
	if _, err := Disable(); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(local + ".icloud-backup"); string(b) != "local2\n" {
		t.Errorf("second backup = %q", b)
	}
	// offloaded and never arriving: nothing changes
	os.Remove(icloud)
	placeholder(t, icloud)
	os.WriteFile(local, []byte("local3\n"), 0o600)
	os.WriteFile(config.File(), []byte("rules: "+icloud+"\n"), 0o600)
	Download = func(string) error { return nil }
	if _, err := Disable(); !errors.Is(err, ErrNotDownloaded) {
		t.Fatalf("err = %v", err)
	}
	if b, _ := os.ReadFile(local); string(b) != "local3\n" {
		t.Errorf("local changed: %q", b)
	}
	if b, _ := os.ReadFile(local + ".icloud-backup"); string(b) != "local2\n" {
		t.Errorf("backup changed: %q", b)
	}
	if _, err := os.Stat(config.File()); err != nil {
		t.Error("config removed")
	}
}
