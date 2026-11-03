package rules

import (
	"net/url"
	"strings"
	"testing"
)

const followFile = `version: 1
default: {browser: Dia, profile: Personal}
rules:
  - name: Same
    match: {host: same.example.com}
    open: {default: true}
  - name: Work
    match: {host: work.example.com}
    open: {browser: Dia, profile: Work}
`

func TestFollowsDefault(t *testing.T) {
	r, err := Parse([]byte(followFile))
	if err != nil {
		t.Fatal(err)
	}
	u, _ := url.Parse("https://same.example.com/x")
	d := r.Route(Request{URL: u})
	if !d.FollowsDefault || d.Target != r.Default || d.Rule != "Same" {
		t.Errorf("decision %+v", d)
	}
	if got := d.String(); got != "Dia/Personal (follows default)  (rule: Same)" {
		t.Errorf("String %q", got)
	}
	if r.Rules[0].Open.String() != "default" || r.Rules[0].Open.IsZero() {
		t.Errorf("Open %q zero=%v", r.Rules[0].Open, r.Rules[0].Open.IsZero())
	}
	u, _ = url.Parse("https://work.example.com/")
	if d := r.Route(Request{URL: u}); d.FollowsDefault {
		t.Errorf("work follows default: %+v", d)
	}
}

func TestOpenDefaultInvalid(t *testing.T) {
	head := "version: 1\ndefault: {browser: Dia}\nrules:\n"
	tests := []struct{ name, body, want string }{
		{"with browser", "  - {name: A, match: {host: a.example.com}, open: {default: true, browser: Safari}}\n", "use one of them"},
		{"with profile", "  - {name: A, match: {host: a.example.com}, open: {default: true, profile: Work}}\n", "use one of them"},
		{"with ask", "  - {name: A, match: {host: a.example.com}, open: {default: true, ask: [{browser: Safari}]}}\n", "use one of them"},
		{"false", "  - {name: A, match: {host: a.example.com}, open: {default: false}}\n", "remove it"},
		{"false group", "  - name: G\n    open: {default: false}\n    rules:\n      - match: {host: a.example.com}\n", "remove it"},
		{"scalar", "  - {name: A, match: {host: a.example.com}, open: default}\n", ""},
		{"child", "  - name: G\n    open: {browser: Safari}\n    rules:\n      - match: {host: a.example.com}\n        open: {default: true}\n", "open is set on the group only"},
		{"in ask item", "  - {name: A, match: {host: a.example.com}, open: {ask: [{default: true}]}}\n", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Parse([]byte(head + tt.body))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Errorf("err %v, want %q", err, tt.want)
			}
		})
	}
	if _, err := Parse([]byte("version: 1\ndefault: {browser: Dia, default: true}\nrules: []\n")); err == nil {
		t.Error("default: inside default accepted")
	}
	ok := head + "  - name: G\n    open: {default: true}\n    rules:\n      - match: {host: a.example.com}\n"
	if _, err := Parse([]byte(ok)); err != nil {
		t.Errorf("group that follows default: %v", err)
	}
}

func TestAsksUnmatched(t *testing.T) {
	off, on := false, true
	tests := []struct {
		name string
		r    *Rules
		want bool
	}{
		{"nil rules", nil, true},
		{"no key", &Rules{}, true},
		{"true", &Rules{AskUnmatched: &on}, true},
		{"false", &Rules{AskUnmatched: &off}, false},
	}
	for _, tt := range tests {
		if got := tt.r.AsksUnmatched(); got != tt.want {
			t.Errorf("%s: %v", tt.name, got)
		}
	}
	r, err := Parse([]byte("version: 1\ndefault: {browser: Dia}\nask_unmatched: false\nrules: []\n"))
	if err != nil || r.AsksUnmatched() {
		t.Errorf("parse: %v %v", r, err)
	}
}

func TestUnmatchedChoices(t *testing.T) {
	def := Target{Browser: "Dia", Profile: "Personal"}
	work := Target{Browser: "Dia", Profile: "Work"}
	saf := Target{Browser: "Safari"}
	ff := Target{Browser: "Zen", Container: "Team"}
	mk := func(open ...Open) *Rules {
		r := &Rules{Default: def}
		for _, o := range open {
			r.Rules = append(r.Rules, Rule{Open: o})
		}
		return r
	}
	var many []Open
	for i := 0; i < 12; i++ {
		many = append(many, Open{Target: Target{Browser: "B" + string(rune('a'+i))}})
	}
	tests := []struct {
		name      string
		r         *Rules
		installed func(Target) bool
		want      []Target
	}{
		{"empty", mk(), nil, []Target{def}},
		{"file order, ask lists and dupes", mk(Open{Target: work}, Open{Ask: []Target{saf, work, ff}}, Open{Target: saf}), nil, []Target{def, work, saf, ff}},
		{"default and follows-default skipped", mk(Open{Target: def}, Open{Default: true}, Open{Ask: []Target{def, saf}}), nil, []Target{def, saf}},
		{"same target in an ask list and a plain rule counts once", mk(Open{Ask: []Target{saf, work}}, Open{Target: saf}, Open{Target: work}), nil, []Target{def, saf, work}},
		{"not installed", mk(Open{Target: work}, Open{Target: saf}), func(t Target) bool { return t != work }, []Target{def, saf}},
		{"limit", mk(many...), nil, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := UnmatchedChoices(tt.r, tt.installed)
			if tt.want == nil {
				if len(got) != MaxAsk || got[0] != def || got[1].Browser != "Ba" || got[MaxAsk-1].Browser != "Bh" {
					t.Errorf("limit: %v", got)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("got %v want %v", got, tt.want)
			}
			for i := range got {
				if got[i] != tt.want[i] {
					t.Errorf("#%d: %v, want %v", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestRememberHost(t *testing.T) {
	tests := []struct {
		raw, host string
		ok        bool
	}{
		{"https://Example.COM/a/b?x=1", "example.com", true},
		{"https://example.com", "example.com", true},
		{"http://sub.example.com:8080/", "sub.example.com", true},
		{"https://[::1]/", "::1", true},
		{"file:///tmp/x.html", "", false},
		{"https://exa%2Ampl.com/", "", false},
		{"https://example.com​/", "", false},
		{"https://exa\u0085mple.com/", "", false},
		{"https://exa\u009bmple.com/", "", false},
	}
	for _, tt := range tests {
		u, err := url.Parse(tt.raw)
		if err != nil {
			continue
		}
		host, ok := RememberHost(u)
		if host != tt.host || ok != tt.ok {
			t.Errorf("%s: %q %v", tt.raw, host, ok)
		}
	}
	if _, ok := RememberHost(nil); ok {
		t.Error("nil")
	}
	if _, ok := RememberHost(&url.URL{Host: "a*.example.com"}); ok {
		t.Error("star accepted")
	}
}
