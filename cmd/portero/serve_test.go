package main

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/plavipas/portero/internal/config"
	"github.com/plavipas/portero/internal/launch"
	"github.com/plavipas/portero/internal/rules"
)

func TestListenAddr(t *testing.T) {
	tests := []struct {
		name      string
		port      int
		listen    string
		portSet   bool
		listenSet bool
		want      string
		wantErr   bool
	}{
		{"default", 0, "", false, false, "127.0.0.1:0", false},
		{"port", 8765, "", true, false, "127.0.0.1:8765", false},
		{"port zero", 0, "", true, false, "127.0.0.1:0", false},
		{"port too big", 70000, "", true, false, "", true},
		{"listen loopback", 0, "127.0.0.1:8765", false, true, "127.0.0.1:8765", false},
		{"listen localhost", 0, "localhost:8765", false, true, "127.0.0.1:8765", false},
		{"listen ipv6 loopback", 0, "[::1]:8765", false, true, "[::1]:8765", false},
		{"listen all", 0, "0.0.0.0:8765", false, true, "", true},
		{"listen no host", 0, ":8765", false, true, "", true},
		{"listen lan", 0, "192.168.1.5:8765", false, true, "", true},
		{"listen name", 0, "example.com:8765", false, true, "", true},
		{"listen no port", 0, "127.0.0.1", false, true, "", true},
		{"listen bad port", 0, "127.0.0.1:x", false, true, "", true},
		{"both", 1, "127.0.0.1:2", true, true, "", true},
	}
	for _, tc := range tests {
		got, err := listenAddr(tc.port, tc.listen, tc.portSet, tc.listenSet)
		if (err != nil) != tc.wantErr || got != tc.want {
			t.Errorf("%s: %q, %v", tc.name, got, err)
		}
	}
}

func TestOpenUILogsNoToken(t *testing.T) {
	const token = "secrettoken"
	link := "http://127.0.0.1:5555/"
	for _, name := range []string{"valid rules", "broken rules"} {
		dir := t.TempDir()
		t.Setenv("XDG_CONFIG_HOME", dir)
		t.Setenv("PORTERO_RULES", "")
		text := "version: 1\ndefault: {browser: Safari}\n"
		if name == "broken rules" {
			text = "version: 9\n"
		}
		if err := os.MkdirAll(filepath.Join(dir, "portero"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(config.RulesFile(), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
		var opened string
		err := openUI(link, link+"auth/"+token, func(u *url.URL, _ rules.Target) (launch.Plan, error) {
			opened = u.String()
			return launch.Plan{}, nil
		})
		if err != nil || opened != link+"auth/"+token {
			t.Errorf("%s: browser got %q, %v", name, opened, err)
		}
		logged, err := os.ReadFile(config.LogFile())
		if err != nil || strings.Contains(string(logged), token) || !strings.Contains(string(logged), link) {
			t.Errorf("%s: log %q, %v", name, logged, err)
		}
	}
}

func TestUILinkCarriesNonceOnly(t *testing.T) {
	bare, auth := uiLinks("127.0.0.1:5555", "nonce123")
	if bare != "http://127.0.0.1:5555/" || auth != "http://127.0.0.1:5555/auth/nonce123" {
		t.Errorf("%q %q", bare, auth)
	}

}

func TestSessionSecretPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "ui-session")
	a, err := sessionSecret(path)
	if err != nil || len(a) != 64 {
		t.Fatalf("first: %q %v", a, err)
	}
	fi, _ := os.Stat(path)
	if fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
	b, err := sessionSecret(path)
	if err != nil || b != a {
		t.Errorf("second: %q %v (want the same secret)", b, err)
	}
	os.WriteFile(path, []byte("short\n"), 0o600)
	c, err := sessionSecret(path)
	if err != nil || c == a || len(c) != 64 {
		t.Errorf("replaced a bad file: %q %v", c, err)
	}
	if err := os.Chmod(path, 0o644); err != nil { // readable by others: the secret counts as leaked
		t.Fatal(err)
	}
	d, err := sessionSecret(path)
	fi, _ = os.Stat(path)
	if err != nil || d == c || len(d) != 64 || fi.Mode().Perm() != 0o600 {
		t.Errorf("world-readable file kept: %q %v mode %v", d, err, fi.Mode().Perm())
	}
	// not hex: replaced
	if err := os.WriteFile(path, []byte(strings.Repeat("z", 64)+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e, err := sessionSecret(path)
	if err != nil || e == d || len(e) != 64 {
		t.Errorf("non-hex file kept: %q %v", e, err)
	}
	// older than the TTL: rotated
	old := time.Now().Add(-sessionTTL - time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	g, err := sessionSecret(path)
	if err != nil || g == e {
		t.Errorf("expired secret kept: %q %v", g, err)
	}
	// a symlink to a private file elsewhere: never followed, replaced by a plain file
	target := filepath.Join(t.TempDir(), "elsewhere")
	if err := os.WriteFile(target, []byte(g+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	os.Remove(path)
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	h, err := sessionSecret(path)
	li, _ := os.Lstat(path)
	if err != nil || h == g || li.Mode()&os.ModeSymlink != 0 {
		t.Errorf("symlink accepted: %q %v mode %v", h, err, li.Mode())
	}
	if b, _ := os.ReadFile(target); strings.TrimSpace(string(b)) != g {
		t.Error("the symlink target was modified")
	}
}

func TestServeAnnouncesLinkOnly(t *testing.T) {
	nonce := strings.Repeat("ab", 32)
	_, auth := uiLinks("127.0.0.1:5555", nonce)
	line := announce(auth)
	if !strings.Contains(line, auth) || !strings.Contains(line, nonce) {
		t.Errorf("announce lacks the link: %q", line)
	}
	// The secret behind the cookie is read into a variable the announce line never sees; make sure the
	// loader itself produces something distinct from the nonce and that the line cannot contain it.
	secret, err := sessionSecret(filepath.Join(t.TempDir(), "ui-session"))
	if err != nil || secret == nonce || strings.Contains(line, secret) {
		t.Errorf("session secret leaks into the announce line: %v", err)
	}
}

func TestSessionDirIsPrivate(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "portero")
	if _, err := sessionSecret(filepath.Join(dir, "ui-session")); err != nil {
		t.Fatal(err)
	}
	fi, _ := os.Stat(dir)
	if fi.Mode().Perm() != 0o700 {
		t.Errorf("session dir mode %v, want 0700", fi.Mode().Perm())
	}
	if _, err := sessionSecret(filepath.Join(t.TempDir(), "no", "such", "dir-is-a-file", "x")); err == nil {
		// a path whose parent cannot be created must fail, not panic
		f := filepath.Join(t.TempDir(), "file")
		os.WriteFile(f, nil, 0o600)
		if _, err := sessionSecret(filepath.Join(f, "ui-session")); err == nil {
			t.Error("expected an error when the parent is a file")
		}
	}
}

func TestSessionSecretConcurrentStarts(t *testing.T) {
	path := filepath.Join(t.TempDir(), "portero", "ui-session")
	const n = 16
	got := make(chan string, n)
	for i := 0; i < n; i++ {
		go func() {
			v, err := sessionSecret(path)
			if err != nil {
				v = "error: " + err.Error()
			}
			got <- v
		}()
	}
	first := <-got
	for i := 1; i < n; i++ {
		if v := <-got; v != first {
			t.Fatalf("servers disagree on the secret: %q vs %q", first, v)
		}
	}
	if strings.HasPrefix(first, "error") || len(first) != 64 {
		t.Fatalf("bad secret %q", first)
	}
}

func TestSessionSecretConcurrentReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "portero", "ui-session")
	if _, err := sessionSecret(path); err != nil {
		t.Fatal(err)
	}
	old := time.Now().Add(-sessionTTL - time.Hour) // expired: every starter wants to replace it
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatal(err)
	}
	const n = 16
	got := make(chan string, n)
	for i := 0; i < n; i++ {
		go func() { v, _ := sessionSecret(path); got <- v }()
	}
	first := <-got
	for i := 1; i < n; i++ {
		if v := <-got; v != first || len(v) != 64 {
			t.Fatalf("servers disagree after replacing an expired secret: %q vs %q", first, v)
		}
	}
}
