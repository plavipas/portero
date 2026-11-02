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
