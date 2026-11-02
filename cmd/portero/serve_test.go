package main

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	// The session secret is read from its file and handed to the server only; it must never be printed,
	// so the only place serve.go may name it is the Options field and the loader.
	src, err := os.ReadFile("serve.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Split(string(src), "\n") {
		if strings.Contains(line, "session") && (strings.Contains(line, "Print") || strings.Contains(line, "Fprint") || strings.Contains(line, "log.")) {
			t.Errorf("serve.go prints the session secret: %s", strings.TrimSpace(line))
		}
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
	os.Chmod(path, 0o644) // readable by others: the secret counts as leaked and is replaced
	d, err := sessionSecret(path)
	fi, _ = os.Stat(path)
	if err != nil || d == c || len(d) != 64 || fi.Mode().Perm() != 0o600 {
		t.Errorf("world-readable file kept: %q %v mode %v", d, err, fi.Mode().Perm())
	}
}
