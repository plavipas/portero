package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/plavipas/portero/internal/config"
	"github.com/plavipas/portero/internal/serve"
)

// syncHome is a temp HOME with an iCloud Drive folder and a local rules file holding text.
func syncHome(t *testing.T, text string) (local, icloud string) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("PORTERO_RULES", "")
	cloud := filepath.Join(home, "Library", "Mobile Documents", "com~apple~CloudDocs")
	if err := os.MkdirAll(cloud, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(config.Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	local = filepath.Join(config.Dir(), "rules.yaml")
	if err := os.WriteFile(local, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return local, filepath.Join(cloud, "Portero", "rules.yaml")
}

func settingsServer() *serve.Server {
	s := serve.New(serve.Options{Rules: rulesNow, LogFile: config.LogFile(), ConfigFile: config.File(),
		Session: "sess", Nonce: "n", Notice: io.Discard, Sync: serveSync()})
	s.SetAddr("127.0.0.1:7777")
	return s
}

const qaRules = "version: 1\ndefault: {browser: Safari}\n"

// A double click on the switch sends two requests: one moves the file, the other is a no-op, and the
// .local-backup keeps the original text.
func TestServeSettingsDoubleSyncClick(t *testing.T) {
	local, icloud := syncHome(t, qaRules)
	h := settingsServer().Handler()
	type res struct {
		code    int
		changed any
	}
	out := make(chan res, 2)
	for i := 0; i < 2; i++ {
		go func() {
			code, m := settingsCall(t, h, "POST", "/api/settings/sync", `{"mode":"icloud"}`)
			out <- res{code, m["changed"]}
		}()
	}
	changed := 0
	for i := 0; i < 2; i++ {
		r := <-out
		if r.code != 200 {
			t.Errorf("status %d", r.code)
		}
		if r.changed == true {
			changed++
		}
	}
	if changed != 1 {
		t.Errorf("%d requests reported a change, want 1", changed)
	}
	if b, err := os.ReadFile(icloud); err != nil || string(b) != qaRules {
		t.Errorf("icloud file: %q %v", b, err)
	}
	if b, err := os.ReadFile(local + ".local-backup"); err != nil || string(b) != qaRules {
		t.Errorf("backup: %q %v", b, err)
	}
}

// An Apply and a sync switch racing each other: whichever runs first, the new text ends up in iCloud Drive
// and no local rules.yaml is written after the move.
func TestServeSettingsSyncRacesApply(t *testing.T) {
	for i := 0; i < 20; i++ {
		local, icloud := syncHome(t, qaRules)
		h := settingsServer().Handler()
		_, st := settingsCall(t, h, "GET", "/api/state", "")
		const next = "version: 1\ndefault: {browser: Firefox}\n"
		body, _ := json.Marshal(map[string]string{"yaml": next, "base_hash": st["hash"].(string)})
		done := make(chan int, 2)
		go func() { c, _ := settingsCall(t, h, "POST", "/api/apply", string(body)); done <- c }()
		go func() { c, _ := settingsCall(t, h, "POST", "/api/settings/sync", `{"mode":"icloud"}`); done <- c }()
		for j := 0; j < 2; j++ {
			if c := <-done; c != 200 {
				t.Fatalf("run %d: status %d", i, c)
			}
		}
		if b, err := os.ReadFile(icloud); err != nil || string(b) != next {
			t.Errorf("run %d: icloud file %q %v", i, b, err)
		}
		if _, err := os.Stat(local); !os.IsNotExist(err) {
			t.Errorf("run %d: a local rules.yaml exists after the move: %v", i, err)
		}
	}
}

// An existing synced file (another Mac) is adopted; the local file is kept as the backup, not lost.
func TestServeSettingsSyncAdoptsExistingICloudFile(t *testing.T) {
	local, icloud := syncHome(t, qaRules)
	const other = "version: 1\ndefault: {browser: Firefox}\n"
	if err := os.MkdirAll(filepath.Dir(icloud), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(icloud, []byte(other), 0o600); err != nil {
		t.Fatal(err)
	}
	h := settingsServer().Handler()
	if code, m := settingsCall(t, h, "POST", "/api/settings/sync", `{"mode":"icloud"}`); code != 200 || m["changed"] != true {
		t.Fatalf("%d %v", code, m)
	}
	if b, _ := os.ReadFile(icloud); string(b) != other {
		t.Errorf("icloud file was overwritten: %q", b)
	}
	if b, err := os.ReadFile(local + ".local-backup"); err != nil || string(b) != qaRules {
		t.Errorf("backup: %q %v", b, err)
	}
}

// With no rules file at all, switching on is refused (422) and nothing is written.
func TestServeSettingsSyncWithoutRulesFile(t *testing.T) {
	local, _ := syncHome(t, qaRules)
	if err := os.Remove(local); err != nil {
		t.Fatal(err)
	}
	h := settingsServer().Handler()
	if code, m := settingsCall(t, h, "POST", "/api/settings/sync", `{"mode":"icloud"}`); code != 422 {
		t.Errorf("%d %v", code, m)
	}
	if _, err := os.Stat(config.File()); !os.IsNotExist(err) {
		t.Errorf("config.yaml written by a failed enable: %v", err)
	}
}
