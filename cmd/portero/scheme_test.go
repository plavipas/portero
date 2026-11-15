package main

import (
	"flag"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/plavipas/portero/internal/config"
	"github.com/plavipas/portero/internal/handler"
)

func porteroLink(inner string) string { return "portero://open?url=" + url.QueryEscape(inner) }

func TestPorteroLinkRoutesByInner(t *testing.T) {
	cases := []struct {
		name, inner, wantTarget, wantRule, wantNote string
	}{
		{"rule", "https://github.example.com/x?a=1&b=2#f", "Dia/Work", "Code", "via portero://"},
		{"default", "https://nothing.example.org/x", "Dia/Personal", "default", "via portero://"},
		{"google wrapper inside", wrapGoogle("https://github.example.com/x"), "Dia/Work", "Code", "via portero://; unwrapped https://github.example.com/x"},
	}
	for _, c := range cases {
		opened, open := askRig(t, unwrapRules)
		if err := routeLinkWith(porteroLink(c.inner), "com.example.browser", open); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if len(*opened) != 1 || (*opened)[0] != c.wantTarget+" "+c.inner {
			t.Errorf("%s: opened %v, want the inner URL in %s", c.name, *opened, c.wantTarget)
		}
		l := lastLogLine(t)
		if len(l) != 6 || l[1] != "com.example.browser" || l[2] != c.wantRule || l[4] != c.inner || l[5] != c.wantNote {
			t.Errorf("%s: log %q", c.name, l)
		}
	}
}

func TestPorteroLinkAsk(t *testing.T) {
	opened, open := askRig(t, unwrapRules)
	var got handler.AskRequest
	ask := func(r handler.AskRequest) handler.AskResult { got = r; return handler.AskResult{Choice: 1} }
	inner := wrapGoogle("https://meet.example.com/room")
	// the wrapper has a rule too, but the innermost URL decides
	if err := routeLinkAsk(porteroLink(inner), "com.example.browser", open, ask, nil); err != nil {
		t.Fatal(err)
	}
	if got.Title != "meet.example.com/room  (via www.google.com)" || len(got.Options) != 2 {
		t.Errorf("ask %+v", got)
	}
	if len(*opened) != 1 || (*opened)[0] != "Dia/Work "+inner {
		t.Errorf("opened %v", *opened)
	}
	l := lastLogLine(t)
	if len(l) != 6 || l[4] != inner || l[5] != "asked -> Dia/Work; via portero://; unwrapped https://meet.example.com/room" {
		t.Errorf("log %q", l)
	}
}

func TestPorteroLinkAskUnmatched(t *testing.T) {
	rig := setupUnmatched(t, askRules)
	inner := "https://other.example.com/p"
	if err := rig.run(t, porteroLink(inner), handler.AskResult{Choice: 1}, true); err != nil {
		t.Fatal(err)
	}
	if len(rig.asked) != 1 || rig.asked[0].Title != "other.example.com/p" {
		t.Errorf("asked %+v", rig.asked)
	}
	if len(*rig.opened) != 1 || !strings.HasSuffix((*rig.opened)[0], " "+inner) {
		t.Errorf("opened %v", *rig.opened)
	}
	l := lastLogLine(t)
	if l[2] != "none" || l[4] != inner || l[5] != "asked (no rule) -> Dia/Work:Team; via portero://" || !strings.HasPrefix(l[5], "asked (no rule)") {
		t.Errorf("log %q", l)
	}
}

func TestPorteroLinkRejected(t *testing.T) {
	bad := []string{
		"portero://open", "portero://open?url=", "portero://open?url=a&url=b",
		"portero://open?url=https%3A%2F%2Fa.example.com&x=1", "portero://open?url=https://a.example.com/b#f",
		"portero://open?url=https://a.example.com/b?x=1&y=2", "portero://settings?url=https%3A%2F%2Fa.example.com",
		"portero:open?url=https%3A%2F%2Fa.example.com", "portero://open:80?url=https%3A%2F%2Fa.example.com",
		"portero://u@open?url=https%3A%2F%2Fa.example.com", porteroLink("portero://open?url=x"),
		porteroLink("file:///etc/hosts"), porteroLink("javascript:alert(1)"), porteroLink("ftp://a.example.com"),
		"portero://open?url=https%253A%252F%252Fa.example.com", porteroLink("https://example.com/" + strings.Repeat("a", 9000)),
	}
	for _, link := range bad {
		opened, open := askRig(t, unwrapRules)
		askCalled := false
		ask := func(handler.AskRequest) handler.AskResult { askCalled = true; return handler.AskResult{} }
		err := routeLinkAsk(link, "com.example.browser", open, ask, nil)
		if err == nil || !strings.HasPrefix(err.Error(), "portero link rejected: ") {
			t.Errorf("%.40q: err %v", link, err)
		}
		if len(*opened) != 0 || askCalled {
			t.Errorf("%.40q: opened %v ask %v", link, *opened, askCalled)
		}
		if b, _ := os.ReadFile(config.LogFile()); len(b) != 0 {
			t.Errorf("%.40q: routeLinkAsk itself must not log, runApp does: %q", link, b)
		}
	}
}

func TestPorteroLinkCLI(t *testing.T) {
	a, err := parseLink(flag.NewFlagSet("open", flag.ContinueOnError), []string{"--dry-run", porteroLink("https://github.example.com/x")})
	if err != nil || !a.via || a.url.String() != "https://github.example.com/x" || !a.dry {
		t.Errorf("%+v %v", a, err)
	}
	a, err = parseLink(flag.NewFlagSet("open", flag.ContinueOnError), []string{"https://github.example.com/x"})
	if err != nil || a.via {
		t.Errorf("plain link: %+v %v", a, err)
	}
	for _, bad := range []string{"portero://open", "portero://open?url=file%3A%2F%2F%2Fetc%2Fhosts", "portero://settings"} {
		if _, err := parseLink(flag.NewFlagSet("open", flag.ContinueOnError), []string{bad}); err == nil || !strings.HasPrefix(err.Error(), "portero link rejected: ") {
			t.Errorf("%q: %v", bad, err)
		}
	}
}

func TestPorteroSchemeRegistration(t *testing.T) {
	b, err := os.ReadFile("../../packaging/Info.plist")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range []string{"<string>http</string>", "<string>https</string>", "<string>portero</string>"} {
		if !strings.Contains(string(b), s) {
			t.Errorf("Info.plist lacks %s", s)
		}
	}
	if len(handler.Schemes) != 2 || handler.Schemes[0] != "http" || handler.Schemes[1] != "https" {
		t.Errorf("handler.Schemes = %v, portero must never be a default-browser scheme", handler.Schemes)
	}
}
