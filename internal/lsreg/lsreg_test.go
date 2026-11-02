package lsreg

import (
	"os"
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
	got := Others(f, id, "/Users/test/Applications/Portero.app")
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
			if got := Others(strings.NewReader(tt.dump), id, tt.keep); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}
