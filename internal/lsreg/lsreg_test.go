package lsreg

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const id = "dev.plavipas.portero"

func TestOthers(t *testing.T) {
	f, err := os.Open("testdata/dump.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err := Others(f, id, "/Users/test/Applications/Portero.app")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/var/folders/xx/T/portero-build.AbC123/Portero.app",
		"/Users/test/Library/Mobile Documents/com~apple~CloudDocs/My Apps/Portero.app",
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestOthersTable(t *testing.T) {
	sep := strings.Repeat("-", 40)
	tests := []struct {
		name, dump, keep string
		want             []string
	}{
		{"empty", "", "/a/P.app", nil},
		{"keep excluded with trailing slash", sep + "\npath:  /a/P.app (0x1)\nidentifier:  " + id + "\n", "/a/P.app/", nil},
		{"no path", sep + "\nidentifier:  " + id + "\n" + sep + "\n", "/a/P.app", nil},
		{"CRLF", sep + "\r\npath:  /b/P.app (0x405c)\r\nidentifier:  " + id + "\r\n" + sep + "\r\n", "/a/P.app", []string{"/b/P.app"}},
		{"duplicates", sep + "\npath:  /b/P.app (0x4)\nidentifier:  " + id + "\n" + sep + "\npath:  /b/P.app (0x4)\nidentifier:  " + id + "\n", "/a/P.app", []string{"/b/P.app"}},
		{"not an app", sep + "\npath:  /b/tool (0x4)\nidentifier:  " + id + "\n", "/a/P.app", nil},
		{"relative", sep + "\npath:  b/P.app (0x4)\nidentifier:  " + id + "\n", "/a/P.app", nil},
		{"other id", sep + "\npath:  /b/P.app (0x4)\nidentifier:  x.y\n", "/a/P.app", nil},
		{"no suffix", sep + "\npath:  /b/P.app\nidentifier:  " + id + "\n", "/a/P.app", []string{"/b/P.app"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, _ := Others(strings.NewReader(tt.dump), id, tt.keep); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestOthersFailsOnAnOversizedLine(t *testing.T) {
	dump := "path: /a/Portero.app\n" + strings.Repeat("x", 5*1024*1024) + "\n"
	if _, err := Others(strings.NewReader(dump), "dev.example.app", "/keep/Portero.app"); err == nil {
		t.Fatal("an oversized line must be an error, not an empty result")
	}
}

func TestOthersStripsBothRecordSuffixForms(t *testing.T) {
	sep := strings.Repeat("-", 30) + "\n"
	dump := sep + "path:  /a/Portero.app (0x405c)\nidentifier:  dev.example.app\n" +
		sep + "path:  /b/Portero.app (0x0-0x1a2b3c)\nidentifier:  dev.example.app\n"
	got, err := Others(strings.NewReader(dump), "dev.example.app", "/keep/Portero.app")
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"/a/Portero.app", "/b/Portero.app"}; !reflect.DeepEqual(got, want) {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestOthersKeepsTheSameAppReachedThroughASymlink(t *testing.T) {
	dir := t.TempDir()
	app := filepath.Join(dir, "Portero.app")
	if err := os.Mkdir(app, 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(dir, "home")
	if err := os.Symlink(dir, link); err != nil {
		t.Fatal(err)
	}
	dump := strings.Repeat("-", 30) + "\npath:  " + app + " (0x405c)\nidentifier:  dev.example.app\n"
	got, err := Others(strings.NewReader(dump), "dev.example.app", filepath.Join(link, "Portero.app"))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("the kept app was listed as another copy: %q", got)
	}
}
