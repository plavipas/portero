package main

import (
	"bytes"
	"errors"
	"net/url"
	"strings"
	"testing"

	"github.com/plavipas/portero/internal/handler"
	"github.com/plavipas/portero/internal/launch"
	"github.com/plavipas/portero/internal/rules"
	"github.com/plavipas/portero/internal/rulesfile"
)

func mustURL(raw string) *url.URL {
	u, err := url.Parse(raw)
	if err != nil {
		panic(err)
	}
	return u
}

func openRecorder(got *[]string) func(*url.URL, rules.Target) (launch.Plan, error) {
	return func(u *url.URL, tg rules.Target) (launch.Plan, error) {
		*got = append(*got, tg.String()+" "+u.String())
		return launch.Plan{}, nil
	}
}

type unmatchedRig struct {
	opened   *[]string
	asked    []handler.AskRequest
	remember []string
	remErr   error
}

// setupUnmatched stubs the installed check and the writer; text is the rules file.
func setupUnmatched(t *testing.T, text string) *unmatchedRig {
	t.Helper()
	opened, _ := askRig(t, text)
	rig := &unmatchedRig{opened: opened}
	oldI, oldR := unmatchedInstalled, rememberUnmatched
	unmatchedInstalled = func(rules.Target) bool { return true }
	rememberUnmatched = func(req rules.Request, chosen rules.Target, follow bool) (string, error) {
		rig.remember = append(rig.remember, req.URL.Hostname()+"|"+chosen.String()+"|"+map[bool]string{true: "follow", false: "target"}[follow])
		return "added", rig.remErr
	}
	t.Cleanup(func() { unmatchedInstalled, rememberUnmatched = oldI, oldR })
	return rig
}

func (r *unmatchedRig) run(t *testing.T, link string, res handler.AskResult, withRemember bool) error {
	t.Helper()
	open := openRecorder(r.opened)
	ask := func(q handler.AskRequest) handler.AskResult { r.asked = append(r.asked, q); return res }
	var rem rememberFunc
	if withRemember {
		rem = func(rules.Request, string, rules.Target) (string, error) { return "", errors.New("must not be used") }
	}
	return routeLinkAsk(link, "com.example.mail", open, ask, rem)
}

func TestRouteUnmatchedPanel(t *testing.T) {
	rig := setupUnmatched(t, askRules)
	if err := rig.run(t, "https://other.example.com/page?q=1", handler.AskResult{Choice: 1}, true); err != nil {
		t.Fatal(err)
	}
	q := rig.asked[0]
	if q.Title != "other.example.com/page" || len(q.Options) != 2 || q.Options[0] != "Keep default · Dia · Personal" ||
		q.Options[1] != "Dia · Work · Team" || !q.CanRemember || q.RememberLabel != "Remember for this host (other.example.com), links from any app" {
		t.Errorf("request %+v", q)
	}
	if len(*rig.opened) != 1 || !strings.HasPrefix((*rig.opened)[0], "Dia/Work:Team ") {
		t.Errorf("opened %v", *rig.opened)
	}
	if c := lastLogLine(t); c[2] != "none" || c[5] != "asked (no rule) -> Dia/Work:Team" {
		t.Errorf("log %q", c)
	}
	if len(rig.remember) != 0 {
		t.Errorf("remembered without the checkbox: %v", rig.remember)
	}
}

func TestRouteUnmatchedKeepDefaultRemember(t *testing.T) {
	rig := setupUnmatched(t, askRules)
	if err := rig.run(t, "https://other.example.com/", handler.AskResult{Choice: 0, Remember: true}, true); err != nil {
		t.Fatal(err)
	}
	if len(rig.remember) != 1 || rig.remember[0] != "other.example.com|Dia/Personal|follow" {
		t.Errorf("remember %v", rig.remember)
	}
	if c := lastLogLine(t); c[5] != `asked (no rule) -> default (kept), remembered "added"` {
		t.Errorf("log %q", c)
	}
}

func TestRouteUnmatchedRememberTarget(t *testing.T) {
	rig := setupUnmatched(t, askRules)
	if err := rig.run(t, "https://other.example.com/", handler.AskResult{Choice: 1, Remember: true}, true); err != nil {
		t.Fatal(err)
	}
	if len(rig.remember) != 1 || rig.remember[0] != "other.example.com|Dia/Work:Team|target" {
		t.Errorf("remember %v", rig.remember)
	}
}

func TestRouteUnmatchedRememberOutcomes(t *testing.T) {
	tests := []struct {
		err      error
		want     string
		notified bool
	}{
		{errors.New("disk full"), "asked (no rule) -> default (kept), remember failed: disk full (notified)", true},
		{rulesfile.ErrNowMatched, "asked (no rule) -> default (kept), remember skipped: a rule matches now", false},
	}
	for _, tt := range tests {
		rig := setupUnmatched(t, askRules)
		rig.remErr = tt.err
		if err := rig.run(t, "https://other.example.com/", handler.AskResult{Choice: 0, Remember: true}, true); err != nil {
			t.Fatal(err)
		}
		if len(*rig.opened) != 1 {
			t.Errorf("the link must open first: %v", *rig.opened)
		}
		if c := lastLogLine(t); c[5] != tt.want {
			t.Errorf("log %q, want %q", c[5], tt.want)
		}
		if tt.notified {
			if len(notifyCalls) != 1 || !strings.Contains(notifyCalls[0], "Could not remember other.example.com: disk full.") || !strings.Contains(notifyCalls[0], "decisions.log") {
				t.Errorf("notifications %q", notifyCalls)
			}
		} else if len(notifyCalls) != 0 {
			t.Errorf("unexpected notifications %q", notifyCalls)
		}
	}
}

func TestRouteUnmatchedCancelKeepsDefault(t *testing.T) {
	rig := setupUnmatched(t, askRules)
	if err := rig.run(t, "https://other.example.com/", handler.Cancelled, true); err != nil {
		t.Fatal(err)
	}
	if len(*rig.opened) != 1 || !strings.HasPrefix((*rig.opened)[0], "Dia/Personal ") || len(rig.remember) != 0 {
		t.Errorf("opened %v remember %v", *rig.opened, rig.remember)
	}
	if c := lastLogLine(t); c[5] != "asked (no rule) -> default (cancelled)" {
		t.Errorf("log %q", c)
	}
}

func TestRouteUnmatchedJoined(t *testing.T) {
	rig := setupUnmatched(t, askRules)
	if err := rig.run(t, "https://other.example.com/", handler.AskResult{Choice: -1, Joined: true}, true); err != nil {
		t.Fatal(err)
	}
	if len(*rig.opened) != 0 {
		t.Errorf("a joined duplicate opened %v", *rig.opened)
	}
}

func TestRouteUnmatchedNoRememberFunc(t *testing.T) {
	rig := setupUnmatched(t, askRules)
	if err := rig.run(t, "https://other.example.com/", handler.AskResult{Choice: 1, Remember: true}, false); err != nil {
		t.Fatal(err)
	}
	if rig.asked[0].CanRemember || rig.asked[0].RememberLabel != "" || len(rig.remember) != 0 {
		t.Errorf("%+v %v", rig.asked[0], rig.remember)
	}
}

func TestRouteUnmatchedNotAsked(t *testing.T) {
	const one = "version: 1\ndefault: {browser: Dia, profile: Personal}\nrules:\n  - {name: A, match: {host: a.example.com}, open: {browser: Dia, profile: Personal}}\n  - {name: S, match: {host: s.example.com}, open: {default: true}}\n"
	const off = askRules + "ask_unmatched: false\n"
	tests := []struct {
		name, rules, link string
	}{
		{"one choice", one, "https://other.example.com/"},
		{"switched off", off, "https://other.example.com/"},
		{"a rule matches", askRules + "  - {name: P, match: {host: p.example.com}, open: {browser: Safari}}\n", "https://p.example.com/"},
		{"file url", askRules, "file:///tmp/doc.html"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rig := setupUnmatched(t, tt.rules)
			if err := rig.run(t, tt.link, handler.AskResult{Choice: 1}, true); err != nil {
				t.Fatal(err)
			}
			if len(rig.asked) != 0 || len(*rig.opened) != 1 {
				t.Errorf("asked %v opened %v", rig.asked, *rig.opened)
			}
		})
	}
}

func TestRouteUnmatchedCLINeverAsks(t *testing.T) {
	rig := setupUnmatched(t, askRules)
	if err := routeLinkWith("https://other.example.com/", "", openRecorder(rig.opened)); err != nil {
		t.Fatal(err)
	}
	if len(*rig.opened) != 1 || !strings.HasPrefix((*rig.opened)[0], "Dia/Personal ") {
		t.Errorf("opened %v", *rig.opened)
	}
	if c := lastLogLine(t); len(c) > 5 && c[5] != "" {
		t.Errorf("log %q", c)
	}
}

func TestRouteUnmatchedWrapped(t *testing.T) {
	rig := setupUnmatched(t, askRules)
	link := "https://www.google.com/url?q=https%3A%2F%2Finner.example.com%2Fp"
	if err := rig.run(t, link, handler.AskResult{Choice: 0, Remember: true}, true); err != nil {
		t.Fatal(err)
	}
	if rig.asked[0].Title != "inner.example.com/p  (via www.google.com)" || len(rig.remember) != 1 || !strings.HasPrefix(rig.remember[0], "inner.example.com|") {
		t.Errorf("%+v %v", rig.asked[0], rig.remember)
	}
	if !strings.HasSuffix((*rig.opened)[0], link) {
		t.Errorf("the browser must get the original link: %v", *rig.opened)
	}
}

func TestPrintUnmatchedLines(t *testing.T) {
	old := unmatchedInstalled
	unmatchedInstalled = func(rules.Target) bool { return true }
	defer func() { unmatchedInstalled = old }()
	r, _ := rules.Parse([]byte(askRules))
	off, _ := rules.Parse([]byte(askRules + "ask_unmatched: false\n"))
	req := func(raw string) rules.Request { return rules.Request{URL: mustURL(raw)} }
	tests := []struct {
		name string
		r    *rules.Rules
		link string
		want string
		n    int
	}{
		{"ask", r, "https://other.example.com/x", "unmatched: for links from an app, Portero.app would ask: [default, Dia/Work:Team]\nremember: host other.example.com\n", 2},
		{"idn host shows both forms", r, "https://bücher.example/", "unmatched: for links from an app, Portero.app would ask: [default, Dia/Work:Team]\nremember: host bücher.example = xn--bcher-kva.example\n", 2},
		{"punycode host shows both forms", r, "https://xn--80ak6aa92e.com/", "unmatched: for links from an app, Portero.app would ask: [default, Dia/Work:Team]\nremember: host xn--80ak6aa92e.com = аррӏе.com\n", 2},
		{"off", off, "https://other.example.com/", "ask_unmatched: off\n", 0},
		{"matched", r, "https://meet.example.com/x", "", 0},
		{"file", r, "file:///tmp/x.html", "", 0},
	}
	for _, tt := range tests {
		var b bytes.Buffer
		rq := req(tt.link)
		got := printUnmatchedLines(&b, tt.r, tt.r.Route(rq), rq.URL)
		if b.String() != tt.want || len(got) != tt.n {
			t.Errorf("%s: %q (%d)", tt.name, b.String(), len(got))
		}
	}
}
