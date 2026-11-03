package main

import (
	"bytes"
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/plavipas/portero/internal/config"
	"github.com/plavipas/portero/internal/handler"
	"github.com/plavipas/portero/internal/launch"
	"github.com/plavipas/portero/internal/rules"
)

const askRules = `version: 1
default: {browser: Dia, profile: Personal}
rules:
  - name: Meetings
    match: {host: [meet.example.com, "*.zoom.example.com"]}
    open:
      ask:
        - {browser: Dia, profile: Personal}
        - {browser: Dia, profile: Work, container: Team}
`

// notifyCalls records the bodies of the fake notifications askRig installs; nothing real is ever posted.
var notifyCalls []string

// askRig points the config at temporary files and returns a recorder for opened targets.
func askRig(t *testing.T, text string) (opened *[]string, open func(*url.URL, rules.Target) (launch.Plan, error)) {
	t.Helper()
	cfg := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", cfg)
	p := filepath.Join(cfg, "rules.yaml")
	t.Setenv("PORTERO_RULES", p)
	if err := os.WriteFile(p, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	notifyCalls = nil
	oldNotify := notifyFunc
	notifyFunc = func(title, body string) string { notifyCalls = append(notifyCalls, title+"|"+body); return "notified" }
	t.Cleanup(func() { notifyFunc = oldNotify })
	var got []string
	return &got, func(u *url.URL, tg rules.Target) (launch.Plan, error) {
		got = append(got, tg.String()+" "+u.String())
		return launch.Plan{}, nil
	}
}

func lastLogLine(t *testing.T) []string {
	t.Helper()
	b, err := os.ReadFile(config.LogFile())
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	return strings.Split(lines[len(lines)-1], "\t")
}

func TestRouteAskedChoice(t *testing.T) {
	opened, open := askRig(t, askRules)
	var gotReq handler.AskRequest
	ask := func(r handler.AskRequest) handler.AskResult { gotReq = r; return handler.AskResult{Choice: 1} }
	link := "https://meet.example.com/abc-defg-hij?authuser=1"
	if err := routeLinkAsk(link, "com.apple.iCal", open, ask, nil); err != nil {
		t.Fatal(err)
	}
	if len(*opened) != 1 || (*opened)[0] != "Dia/Work:Team "+link {
		t.Errorf("opened %v", *opened)
	}
	if gotReq.Title != "meet.example.com/abc-defg-hij" || len(gotReq.Options) != 2 ||
		gotReq.Options[0] != "Dia · Personal" || gotReq.Options[1] != "Dia · Work · Team" || gotReq.CanRemember {
		t.Errorf("request %+v (no remember func: checkbox off)", gotReq)
	}
	c := lastLogLine(t)
	if len(c) != 6 || c[2] != "Meetings" || c[3] != "Dia/Work:Team" || c[5] != "asked -> Dia/Work:Team" {
		t.Errorf("log %q", c)
	}
}

func TestRouteAskedCancelUsesDefault(t *testing.T) {
	opened, open := askRig(t, askRules)
	ask := func(handler.AskRequest) handler.AskResult { return handler.Cancelled }
	remember := func(rules.Request, string, rules.Target) (string, error) {
		t.Error("remember on cancel")
		return "", nil
	}
	if err := routeLinkAsk("https://meet.example.com/abc-defg-hij", "", open, ask, remember); err != nil {
		t.Fatal(err)
	}
	if len(*opened) != 1 || !strings.HasPrefix((*opened)[0], "Dia/Personal ") {
		t.Errorf("opened %v", *opened)
	}
	if c := lastLogLine(t); len(c) != 6 || c[5] != "asked -> default (cancelled)" {
		t.Errorf("log %q", c)
	}
}

func TestRouteAskedWithoutPanelIsCLI(t *testing.T) {
	opened, open := askRig(t, askRules)
	if err := routeLinkWith("https://meet.example.com/abc-defg-hij", "", open); err != nil {
		t.Fatal(err)
	}
	if len(*opened) != 1 || !strings.HasPrefix((*opened)[0], "Dia/Personal ") {
		t.Errorf("opened %v", *opened)
	}
	if c := lastLogLine(t); len(c) != 6 || c[5] != "asked -> default (CLI)" {
		t.Errorf("log %q", c)
	}
}

func TestRouteAskedJoinedDoesNotOpen(t *testing.T) {
	opened, open := askRig(t, askRules)
	ask := func(handler.AskRequest) handler.AskResult { return handler.AskResult{Choice: -1, Joined: true} }
	if err := routeLinkAsk("https://meet.example.com/abc-defg-hij", "", open, ask, nil); err != nil {
		t.Fatal(err)
	}
	if len(*opened) != 0 {
		t.Errorf("a joined link must not open: %v", *opened)
	}
}

func TestRouteAskedRemember(t *testing.T) {
	opened, open := askRig(t, askRules)
	ask := func(r handler.AskRequest) handler.AskResult {
		if !r.CanRemember {
			t.Error("remember should be offered")
		}
		return handler.AskResult{Choice: 0, Remember: true}
	}
	var seen struct {
		label  string
		chosen rules.Target
	}
	remember := func(req rules.Request, label string, chosen rules.Target) (string, error) {
		seen.label, seen.chosen = label, chosen
		return "meet.example.com/abc-defg-hij", nil
	}
	if err := routeLinkAsk("https://meet.example.com/abc-defg-hij", "", open, ask, remember); err != nil {
		t.Fatal(err)
	}
	if seen.label != "Meetings" || seen.chosen.Profile != "Personal" || len(*opened) != 1 {
		t.Errorf("remember call %+v, opened %v", seen, *opened)
	}
	if c := lastLogLine(t); len(c) != 6 || c[5] != `asked -> Dia/Personal, remembered "meet.example.com/abc-defg-hij"` {
		t.Errorf("log %q", c)
	}
}

func TestRouteAskedRememberFailureNeverBlocksTheLink(t *testing.T) {
	opened, open := askRig(t, askRules)
	ask := func(handler.AskRequest) handler.AskResult { return handler.AskResult{Choice: 1, Remember: true} }
	remember := func(rules.Request, string, rules.Target) (string, error) { return "", errors.New("disk full") }
	if err := routeLinkAsk("https://meet.example.com/abc-defg-hij", "", open, ask, remember); err != nil {
		t.Fatal(err)
	}
	if len(*opened) != 1 {
		t.Errorf("opened %v", *opened)
	}
	if c := lastLogLine(t); len(c) != 6 || c[5] != "asked -> Dia/Work:Team, remember failed: disk full (notified)" {
		t.Errorf("log %q", c)
	}
	if len(notifyCalls) != 1 || !strings.Contains(notifyCalls[0], "Portero|Could not remember meet.example.com: disk full.") ||
		!strings.Contains(notifyCalls[0], "decisions.log") {
		t.Errorf("notifications %q", notifyCalls)
	}
}

func TestRouteAskedRememberSuccessDoesNotNotify(t *testing.T) {
	_, open := askRig(t, askRules)
	ask := func(handler.AskRequest) handler.AskResult { return handler.AskResult{Choice: 1, Remember: true} }
	remember := func(rules.Request, string, rules.Target) (string, error) { return "x", nil }
	if err := routeLinkAsk("https://meet.example.com/abc-defg-hij", "", open, ask, remember); err != nil {
		t.Fatal(err)
	}
	if len(notifyCalls) != 0 {
		t.Errorf("notifications %q", notifyCalls)
	}
}

func TestNotifyBodyNamesTheRealLog(t *testing.T) {
	askRig(t, askRules)
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "cfg"))
	notifyRememberFailed("example.com", errors.New("disk full"))
	if len(notifyCalls) != 1 || !strings.HasSuffix(notifyCalls[0], "Details: ~/cfg/portero/decisions.log") {
		t.Errorf("body %q", notifyCalls)
	}
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	notifyCalls = nil
	notifyRememberFailed("example.com", errors.New("disk full"))
	if !strings.HasSuffix(notifyCalls[0], filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "portero", "decisions.log")) || strings.Contains(notifyCalls[0], "~/.config") {
		t.Errorf("body %q", notifyCalls)
	}
}

func TestNotifyBodyIsOneShortLine(t *testing.T) {
	askRig(t, askRules)
	got := notifyRememberFailed("example.com", errors.New("bad\nline\t"+strings.Repeat("x", 500)))
	if got != " (notified)" || len(notifyCalls) != 1 {
		t.Fatalf("%q %q", got, notifyCalls)
	}
	body := strings.TrimPrefix(notifyCalls[0], "Portero|")
	if strings.ContainsAny(body, "\n\t") || len([]rune(body)) > 300 || !strings.HasPrefix(body, "Could not remember example.com: bad line") {
		t.Errorf("body %q", body)
	}
}

func TestRouteAskedRealRememberWritesRule(t *testing.T) {
	opened, open := askRig(t, askRules)
	ask := func(handler.AskRequest) handler.AskResult { return handler.AskResult{Choice: 1, Remember: true} }
	link := "https://meet.example.com/abc-defg-hij?authuser=1"
	if err := routeLinkAsk(link, "", open, ask, rememberRule); err != nil {
		t.Fatal(err)
	}
	r, err := rules.Load(config.RulesFile())
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse(link)
	if d := r.Route(rules.Request{URL: u}); d.Rule != "meet.example.com/abc-defg-hij" || d.Target.Profile != "Work" || len(d.Ask) != 0 {
		t.Errorf("after remember: %+v", d)
	}
	if len(*opened) != 1 {
		t.Errorf("opened %v", *opened)
	}
	if c := lastLogLine(t); !strings.HasSuffix(c[5], `remembered "meet.example.com/abc-defg-hij"`) {
		t.Errorf("log %q", c)
	}
	// the second time the same meeting no longer asks
	*opened = nil
	ask2 := func(handler.AskRequest) handler.AskResult { t.Error("asked twice"); return handler.Cancelled }
	if err := routeLinkAsk(link, "", open, ask2, rememberRule); err != nil || len(*opened) != 1 || !strings.HasPrefix((*opened)[0], "Dia/Work:Team") {
		t.Errorf("second link: %v %v", err, *opened)
	}
}

func TestRouteAskedRulesChangedSkipsRemember(t *testing.T) {
	opened, open := askRig(t, askRules)
	ask := func(handler.AskRequest) handler.AskResult {
		// the rules file stops asking while the panel is open
		if err := os.WriteFile(config.RulesFile(), []byte("version: 1\ndefault: {browser: Safari}\nrules: []\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return handler.AskResult{Choice: 1, Remember: true}
	}
	if err := routeLinkAsk("https://meet.example.com/abc-defg-hij", "", open, ask, rememberRule); err != nil {
		t.Fatal(err)
	}
	if len(*opened) != 1 {
		t.Errorf("opened %v", *opened)
	}
	c := lastLogLine(t)
	if len(c) != 6 || !strings.Contains(c[5], "remember failed: ") {
		t.Errorf("log %q", c)
	}
	b, _ := os.ReadFile(config.RulesFile())
	if strings.Contains(string(b), "added by Portero") {
		t.Error("the file was written although it no longer asks")
	}
}

func TestAskTitle(t *testing.T) {
	cases := []struct{ url, want string }{
		{"https://meet.example.com/a%0Ab?x=1", "meet.example.com/a b"},
		{"https://meet.example.com/a%E2%80%AEb", "meet.example.com/a b"},
		{"https://meet.example.com/a%C2%85b%C2%9Bc", "meet.example.com/a b c"},
		{"https://meet.example.com/a%E2%80%8Bb", "meet.example.com/a b"},
		{"https://meet.example.com/a%E2%80%A8b%E2%80%A9c", "meet.example.com/a b c"},
	}
	for _, c := range cases {
		u, _ := url.Parse(c.url)
		if got := askTitle(u); got != c.want {
			t.Errorf("askTitle(%s) = %q, want %q", c.url, got, c.want)
		}
	}
}

func TestPrintAskLines(t *testing.T) {
	d := rules.Decision{Target: rules.Target{Browser: "Dia", Profile: "Personal"}}
	var b bytes.Buffer
	u, _ := url.Parse("https://meet.google.com/abc-defg-hij?authuser=1")
	printAskLines(&b, d, u)
	if b.String() != "cancel: Dia/Personal (default)\nremember: meet.google.com /abc-defg-hij\n" {
		t.Errorf("%q", b.String())
	}
	b.Reset()
	u, _ = url.Parse("https://meet.google.com/")
	printAskLines(&b, d, u)
	if !strings.HasSuffix(b.String(), "remember: not offered\n") {
		t.Errorf("%q", b.String())
	}
}

func TestOpenCLIAskedUsesDefault(t *testing.T) {
	opened, open := askRig(t, askRules)
	u, _ := url.Parse("https://meet.example.com/abc-defg-hij")
	req := rules.Request{URL: u}
	r, _ := rules.Load(config.RulesFile())
	d := r.Route(req)
	if err := openCLIAsked(d, req, u, open); err != nil {
		t.Fatal(err)
	}
	if len(*opened) != 1 || !strings.HasPrefix((*opened)[0], "Dia/Personal ") {
		t.Errorf("opened %v", *opened)
	}
	if c := lastLogLine(t); len(c) != 6 || c[3] != "Dia/Personal" || c[5] != "asked -> default (CLI)" {
		t.Errorf("log %q", c)
	}
}
