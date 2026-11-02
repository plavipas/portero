package serve

import (
	"testing"

	"github.com/plavipas/portero/internal/rules"
)

func TestBuildGraph(t *testing.T) {
	r, err := rules.Parse([]byte(goodRules))
	if err != nil {
		t.Fatal(err)
	}
	ds := []Decision{
		mk("com.example.app", "work", "Safari/Work"),
		mk("com.example.app", "work", "Safari/Work"),
		mk("-", "default", "Safari"),
		mk("-", "old rule", "Zen:Work"),
		{Opener: "-", Rule: "ERROR", Kind: "error", Path: [3]string{"o:-", "r:ERROR", "t:Safari"}},
	}
	g := BuildGraph(r, ds)
	if g.Total != 5 {
		t.Errorf("total %d", g.Total)
	}
	if len(g.Openers) != 2 || g.Openers[1].Label != "no opener / CLI" {
		t.Errorf("openers %+v", g.Openers)
	}
	ids := map[string]Node{}
	for _, n := range g.Rules {
		ids[n.ID] = n
	}
	for _, id := range []string{"r:work", "r:default", "r:file", "r:ERROR", "r:old rule"} {
		if _, ok := ids[id]; !ok {
			t.Errorf("missing rule node %s", id)
		}
	}
	if !ids["r:old rule"].Stale || ids["r:work"].Stale {
		t.Errorf("stale flags: %+v", ids)
	}
	if g.Rules[0].ID != "r:work" {
		t.Errorf("file order first: %+v", g.Rules)
	}
	count := map[[2]string]int{}
	for _, e := range g.Edges {
		count[[2]string{e.From, e.To}] = e.Count
	}
	if count[[2]string{"o:com.example.app", "r:work"}] != 2 || count[[2]string{"r:work", "t:Safari/Work"}] != 2 {
		t.Errorf("counts %v", count)
	}
	if c, ok := count[[2]string{"r:file", "t:Safari"}]; !ok || c != 0 {
		t.Errorf("static edge missing: %v", count)
	}
	var zen Node
	for _, n := range g.Targets {
		if n.ID == "t:Zen:Work" {
			zen = n
		}
	}
	if zen.Browser != "Zen" || zen.Container != "Work" {
		t.Errorf("target fields %+v", zen)
	}
	g.markInstalled([]BrowserInfo{{Name: "Safari", BundleID: "com.apple.Safari", Installed: true}})
	for _, n := range g.Targets {
		if want := n.Browser == "Safari"; n.Installed != want {
			t.Errorf("%s installed=%v", n.ID, n.Installed)
		}
	}
}

func TestBuildGraphBrokenFileAndEmptyLog(t *testing.T) {
	g := BuildGraph(nil, []Decision{mk("-", "work", "Safari")})
	for _, n := range g.Rules {
		if n.Stale {
			t.Errorf("no file: nothing is stale (%s)", n.ID)
		}
	}
	g = BuildGraph(nil, nil)
	if g.Total != 0 || len(g.Openers) != 0 || len(g.Edges) != 1 { // only ERROR -> Safari
		t.Errorf("%+v", g)
	}
}

func mk(opener, rule, target string) Decision {
	d := Decision{Opener: opener, Rule: rule, Target: target, Kind: "ok"}
	d.Path = d.path()
	return d
}
