package main

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/plavipas/portero/internal/config"
	"github.com/plavipas/portero/internal/launch"
	"github.com/plavipas/portero/internal/rules"
	"github.com/plavipas/portero/internal/sync"
)

// offloadRig points the rules file at a fake iCloud folder with only the old placeholder in it.
func offloadRig(t *testing.T) (rulesPath string) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	rulesPath = filepath.Join(sync.ICloudDir(), "rules.yaml")
	if err := os.MkdirAll(sync.ICloudDir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sync.ICloudDir(), ".rules.yaml.icloud"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PORTERO_RULES", rulesPath)
	return rulesPath
}

func fakeDownload(t *testing.T, err error) *[]string {
	t.Helper()
	var calls []string
	old := startDownload
	startDownload = func(p string) error { calls = append(calls, p); return err }
	t.Cleanup(func() { startDownload = old })
	return &calls
}

func TestRouteLinkOffloadedOpensSafariFirst(t *testing.T) {
	p := offloadRig(t)
	calls := fakeDownload(t, nil)
	var order []string
	open := func(u *url.URL, tg rules.Target) (launch.Plan, error) {
		order = append(order, "open:"+tg.Browser+":"+u.String())
		return launch.Plan{}, nil
	}
	if err := routeLinkWith("https://example.com/a", "com.example.app", open); err != nil {
		t.Fatal(err)
	}
	if len(order) != 1 || order[0] != "open:Safari:https://example.com/a" {
		t.Errorf("open = %v", order)
	}
	if len(*calls) != 1 || (*calls)[0] != p {
		t.Errorf("download calls = %v", *calls)
	}
	b, _ := os.ReadFile(config.LogFile())
	if !strings.Contains(string(b), "ERROR") || !strings.Contains(string(b), "offloaded by iCloud, download started; opened in Safari") {
		t.Errorf("log = %q", b)
	}
}

func TestRouteLinkOffloadedDownloadError(t *testing.T) {
	offloadRig(t)
	fakeDownload(t, errors.New("no iCloud"))
	open := func(*url.URL, rules.Target) (launch.Plan, error) { return launch.Plan{}, nil }
	if err := routeLinkWith("https://example.com/", "", open); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(config.LogFile())
	if !strings.Contains(string(b), "download not started (no iCloud)") {
		t.Errorf("log = %q", b)
	}
}

func TestRouteLinkLocalFileIsNotOffloaded(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	t.Setenv("PORTERO_RULES", "")
	os.MkdirAll(filepath.Dir(config.RulesFile()), 0o755)
	os.WriteFile(config.RulesFile(), []byte("version: 1\ndefault: {browser: Firefox}\n"), 0o600)
	calls := fakeDownload(t, nil)
	var got string
	open := func(_ *url.URL, tg rules.Target) (launch.Plan, error) { got = tg.Browser; return launch.Plan{}, nil }
	if err := routeLinkWith("https://example.com/", "", open); err != nil {
		t.Fatal(err)
	}
	if got != "Firefox" || len(*calls) != 0 {
		t.Errorf("browser %q, downloads %v", got, *calls)
	}
}

func TestOpenUIOffloaded(t *testing.T) {
	p := offloadRig(t)
	calls := fakeDownload(t, nil)
	link := "http://127.0.0.1:5555/"
	var opened string
	err := openUI(link, link+"auth/tok", func(u *url.URL, tg rules.Target) (launch.Plan, error) {
		opened = tg.Browser + " " + u.String()
		return launch.Plan{}, nil
	})
	if err != nil || opened != "Safari "+link+"auth/tok" {
		t.Errorf("opened %q, %v", opened, err)
	}
	if len(*calls) != 1 || (*calls)[0] != p {
		t.Errorf("downloads %v", *calls)
	}
	if b, _ := os.ReadFile(config.LogFile()); strings.Contains(string(b), "tok") {
		t.Errorf("nonce in log: %q", b)
	}
}

func TestRulesCheckOffloaded(t *testing.T) {
	p := offloadRig(t)
	calls := fakeDownload(t, nil)
	err := rulesCmd([]string{"check"})
	if err == nil || !strings.Contains(err.Error(), "offloaded by iCloud, download started") || !strings.Contains(err.Error(), p) {
		t.Errorf("err = %v", err)
	}
	if len(*calls) != 1 {
		t.Errorf("downloads %v", *calls)
	}
}

func TestServeSyncOffloaded(t *testing.T) {
	offloadRig(t)
	f := serveSync()
	if !f.ICloudExists() || !f.ICloudOffloaded() {
		t.Error("placeholder: want exists and offloaded")
	}
	os.Remove(filepath.Join(sync.ICloudDir(), ".rules.yaml.icloud"))
	if f.ICloudExists() || f.ICloudOffloaded() {
		t.Error("nothing there: want neither")
	}
	os.WriteFile(filepath.Join(sync.ICloudDir(), "rules.yaml"), []byte("x"), 0o600)
	if !f.ICloudExists() || f.ICloudOffloaded() {
		t.Error("regular file: want exists, not offloaded")
	}
}
