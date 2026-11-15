package rules

import (
	"net/url"
	"strings"
	"testing"
)

func TestPorteroLink(t *testing.T) {
	long := "https://example.com/" + strings.Repeat("a", 9000)
	cases := []struct {
		name, link string
		wantInner  string // "" = rejected
		wantIs     bool
	}{
		{"plain", "portero://open?url=https%3A%2F%2Fexample.com%2Fx", "https://example.com/x", true},
		{"upper case", "PORTERO://OPEN?url=https%3A%2F%2Fexample.com%2Fx", "https://example.com/x", true},
		{"slash path", "portero://open/?url=https%3A%2F%2Fexample.com%2Fx", "https://example.com/x", true},
		{"inner query and fragment", "portero://open?url=https%3A%2F%2Fexample.com%2Fx%3Fa%3D1%26b%3D2%23f", "https://example.com/x?a=1&b=2#f", true},
		{"http", "portero://open?url=http%3A%2F%2Fexample.com", "http://example.com", true},
		{"other scheme", "https://example.com/?url=x", "", false},
		{"no action", "portero://open", "", true},
		{"empty url", "portero://open?url=", "", true},
		{"repeated", "portero://open?url=https%3A%2F%2Fa.example.com&url=https%3A%2F%2Fb.example.com", "", true},
		{"extra parameter", "portero://open?url=https%3A%2F%2Fa.example.com&x=1", "", true},
		{"only other parameter", "portero://open?x=1", "", true},
		{"unencoded fragment", "portero://open?url=https://a.example.com/b#f", "", true},
		{"unencoded query", "portero://open?url=https://a.example.com/b?x=1&y=2", "", true},
		{"semicolon", "portero://open?url=https%3A%2F%2Fa.example.com;x=1", "", true},
		{"other action", "portero://settings?url=https%3A%2F%2Fa.example.com", "", true},
		{"opaque", "portero:open?url=https%3A%2F%2Fa.example.com", "", true},
		{"port", "portero://open:80?url=https%3A%2F%2Fa.example.com", "", true},
		{"user info", "portero://u@open?url=https%3A%2F%2Fa.example.com", "", true},
		{"path", "portero://open/x?url=https%3A%2F%2Fa.example.com", "", true},
		{"inner portero", "portero://open?url=portero%3A%2F%2Fopen%3Furl%3Dhttps%253A%252F%252Fa.example.com", "", true},
		{"inner file", "portero://open?url=file%3A%2F%2F%2Fetc%2Fhosts", "", true},
		{"inner javascript", "portero://open?url=javascript%3Aalert(1)", "", true},
		{"inner ftp", "portero://open?url=ftp%3A%2F%2Fa.example.com", "", true},
		{"double encoded", "portero://open?url=https%253A%252F%252Fa.example.com", "", true},
		{"no host", "portero://open?url=https%3A%2F%2F", "", true},
		{"too long", "portero://open?url=" + url.QueryEscape(long), "", true},
	}
	for _, c := range cases {
		u, err := url.Parse(c.link)
		if err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		inner, is, err := PorteroLink(u)
		if is != c.wantIs {
			t.Errorf("%s: is = %v", c.name, is)
		}
		if !c.wantIs {
			continue
		}
		if c.wantInner == "" {
			if err == nil || inner != nil || !strings.HasPrefix(err.Error(), "portero link rejected: ") {
				t.Errorf("%s: want rejection, got %v %v", c.name, inner, err)
			} else if strings.Contains(err.Error(), "example.com") || strings.Contains(err.Error(), "etc") {
				t.Errorf("%s: link content in the reason: %q", c.name, err)
			}
			continue
		}
		if err != nil || inner == nil || inner.String() != c.wantInner {
			t.Errorf("%s: got %v %v, want %s", c.name, inner, err, c.wantInner)
		}
	}
	if _, is, err := PorteroLink(nil); is || err != nil {
		t.Error("nil URL")
	}
}
