package main

import (
	"encoding/base64"
	"net/url"
	"strings"
	"testing"

	"github.com/plavipas/portero/internal/handler"
	"github.com/plavipas/portero/internal/rules"
)

const unwrapRules = `version: 1
default: {browser: Dia, profile: Personal}
rules:
  - name: Code
    match: {host: [github.example.com]}
    open: {browser: Dia, profile: Work}
  - name: Meetings
    match: {host: [meet.example.com]}
    open:
      ask:
        - {browser: Dia, profile: Personal}
        - {browser: Dia, profile: Work}
  - name: Google wrapper
    match: {host: [www.google.com]}
    open: {browser: Zen}
  - name: Slack
    match: {host: ["*.slack.com"]}
    open: {browser: Zen}
`

func wrapGoogle(inner string) string { return "https://www.google.com/url?q=" + url.QueryEscape(inner) }

func syntheticSlackLink(target string) string {
	e := base64.RawURLEncoding
	jwt := e.EncodeToString([]byte(`{"alg":"none"}`)) + "." +
		e.EncodeToString([]byte(`{"https://slack.com/target_uri":"`+target+`"}`)) + ".sig"
	return "https://team.slack.com/openid/connect/login_initiate_redirect?login_hint=" + url.QueryEscape(jwt)
}

func TestWrappedLinkRoutesByInner(t *testing.T) {
	cases := []struct {
		name, link, wantTarget, wantRule string
		wantNote                         string
	}{
		{"google to code", wrapGoogle("https://github.example.com/x"), "Dia/Work", "Code", "unwrapped https://github.example.com/x"},
		{"inner no rule -> default, not the wrapper rule", wrapGoogle("https://nothing.example.org/x"), "Dia/Personal", "default", "unwrapped https://nothing.example.org/x"},
		{"unwrap fails -> wrapper rule", "https://www.google.com/url?q=%2Fsearch", "Zen", "Google wrapper", ""},
		{"slack sign-in by target_uri", syntheticSlackLink("https://github.example.com/y"), "Dia/Work", "Code", "unwrapped https://github.example.com/y"},
		{"broken slack JWT -> wrapper rule", "https://team.slack.com/openid/connect/login_initiate_redirect?login_hint=a.b.c", "Zen", "Slack", ""},
		{"plain link", "https://github.example.com/z", "Dia/Work", "Code", ""},
	}
	for _, c := range cases {
		opened, open := askRig(t, unwrapRules)
		if err := routeLinkWith(c.link, "com.example.mail", open); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if len(*opened) != 1 || (*opened)[0] != c.wantTarget+" "+c.link {
			t.Errorf("%s: opened %v, want %s with the original link", c.name, *opened, c.wantTarget)
		}
		l := lastLogLine(t)
		if l[2] != c.wantRule || l[4] != c.link || l[1] != "com.example.mail" {
			t.Errorf("%s: log %q", c.name, l)
		}
		note := ""
		if len(l) == 6 {
			note = l[5]
		}
		if note != c.wantNote {
			t.Errorf("%s: note %q, want %q", c.name, note, c.wantNote)
		}
	}
}

func TestWrappedAskUsesInner(t *testing.T) {
	opened, open := askRig(t, unwrapRules)
	var got handler.AskRequest
	ask := func(r handler.AskRequest) handler.AskResult {
		got = r
		return handler.AskResult{Choice: 1, Remember: true}
	}
	var remReq rules.Request
	remember := func(req rules.Request, _ string, _ rules.Target) (string, error) { remReq = req; return "n", nil }
	link := wrapGoogle("https://meet.example.com/abc-defg-hij")
	if err := routeLinkAsk(link, "com.example.mail", open, ask, remember); err != nil {
		t.Fatal(err)
	}
	if got.Title != "meet.example.com/abc-defg-hij" || !got.CanRemember || got.ID != "com.example.mail\x00"+link {
		t.Errorf("ask request %+v", got)
	}
	if host, path, ok := rules.RememberKey(remReq.URL); !ok || host != "meet.example.com" || path != "/abc-defg-hij" {
		t.Errorf("remember key from %v", remReq.URL)
	}
	if len(*opened) != 1 || (*opened)[0] != "Dia/Work "+link {
		t.Errorf("opened %v", *opened)
	}
	l := lastLogLine(t)
	if len(l) != 6 || l[4] != link || l[5] != `asked -> Dia/Work, remembered "n"; unwrapped https://meet.example.com/abc-defg-hij` {
		t.Errorf("log %q", l)
	}
}

func TestUnwrappedLogIsCut(t *testing.T) {
	_, open := askRig(t, unwrapRules)
	link := wrapGoogle("https://github.example.com/" + strings.Repeat("a", 4000))
	if err := routeLinkWith(link, "", open); err != nil {
		t.Fatal(err)
	}
	l := lastLogLine(t)
	if len(l) != 6 || len(l[5]) != len("unwrapped ")+2048 || l[4] != link {
		t.Errorf("note length %d, columns %d", len(l[5]), len(l))
	}
}
