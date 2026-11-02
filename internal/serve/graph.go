package serve

import (
	"fmt"
	"strings"

	"github.com/plavipas/portero/internal/browsers"
	"github.com/plavipas/portero/internal/rules"
)

// Node ids: "o:<bundle id or ->", "r:<rule>", "t:<Target.String()>".
func openerID(o string) string { return "o:" + o }
func ruleID(r string) string {
	if r == "" {
		r = "default"
	}
	return "r:" + r
}
func targetID(t string) string { return "t:" + t }

// Node is one box of the diagram.
type Node struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"` // opener, rule, default, file, error, target
	// Stale: a rule the log mentions that the file no longer has.
	Stale bool `json:"stale,omitempty"`
	// Targets only.
	Browser   string `json:"browser,omitempty"`
	Profile   string `json:"profile,omitempty"`
	Container string `json:"container,omitempty"`
	Installed bool   `json:"installed"`
}

// Edge joins two nodes; Count is how many logged links took it (0: it exists in the file only).
type Edge struct {
	From  string `json:"from"`
	To    string `json:"to"`
	Count int    `json:"count"`
}

// Graph is the diagram: openers -> rules -> targets.
type Graph struct {
	Openers []Node `json:"openers"`
	Rules   []Node `json:"rules"`
	Targets []Node `json:"targets"`
	Edges   []Edge `json:"edges"`
	Total   int    `json:"total"` // decisions counted
}

// BuildGraph draws the rules file (nil when it is broken) and the decisions of the log.
func BuildGraph(r *rules.Rules, decisions []Decision) Graph {
	var g Graph
	seen := map[string]bool{}
	edges := map[[2]string]*Edge{}
	var edgeOrder [][2]string
	addEdge := func(from, to string, n int) {
		k := [2]string{from, to}
		if e, ok := edges[k]; ok {
			e.Count += n
			return
		}
		edges[k] = &Edge{From: from, To: to, Count: n}
		edgeOrder = append(edgeOrder, k)
	}
	addTarget := func(t string, rt *rules.Target) {
		id := targetID(t)
		if seen[id] {
			return
		}
		seen[id] = true
		n := Node{ID: id, Label: t, Kind: "target"}
		if rt != nil {
			n.Browser, n.Profile, n.Container = rt.Browser, rt.Profile, rt.Container
		} else {
			n.Browser, n.Profile, n.Container = splitTarget(t)
		}
		g.Targets = append(g.Targets, n)
	}
	addRule := func(id, label, kind string, stale bool) {
		if seen[id] {
			return
		}
		seen[id] = true
		g.Rules = append(g.Rules, Node{ID: id, Label: label, Kind: kind, Stale: stale})
	}

	// The file: rules in order, then the special ones.
	if r != nil {
		for i, rule := range r.Rules {
			name := rule.Name
			if name == "" {
				name = fmt.Sprintf("#%d", i+1)
			}
			addRule(ruleID(name), name, "rule", false)
			t := rule.Open
			addTarget(t.String(), &t)
			addEdge(ruleID(name), targetID(t.String()), 0)
		}
	}
	addRule(ruleID("default"), "default", "default", false)
	addRule(ruleID("file"), "file", "file", false)
	addRule(ruleID("ERROR"), "ERROR → Safari fallback", "error", false)
	if r != nil {
		t := r.Default
		addTarget(t.String(), &t)
		addEdge(ruleID("default"), targetID(t.String()), 0)
		addEdge(ruleID("file"), targetID(t.String()), 0)
	}
	addTarget(fallbackTarget, nil)
	addEdge(ruleID("ERROR"), targetID(fallbackTarget), 0)

	// The log.
	for _, d := range decisions {
		g.Total++
		oid := d.Path[0]
		if !seen[oid] {
			seen[oid] = true
			label := d.Opener
			kind := "opener"
			if d.Opener == "-" {
				label = "no opener / CLI"
			}
			g.Openers = append(g.Openers, Node{ID: oid, Label: label, Kind: kind})
		}
		rid, tid := d.Path[1], d.Path[2]
		if !seen[rid] {
			addRule(rid, strings.TrimPrefix(rid, "r:"), "rule", r != nil)
		}
		if !seen[tid] {
			addTarget(strings.TrimPrefix(tid, "t:"), nil)
		}
		addEdge(oid, rid, 1)
		addEdge(rid, tid, 1)
	}
	for _, k := range edgeOrder {
		g.Edges = append(g.Edges, *edges[k])
	}
	return g
}

// splitTarget parses "Browser/Profile:Container" as Target.String renders it.
func splitTarget(s string) (browser, profile, container string) {
	if i := strings.Index(s, ":"); i >= 0 {
		s, container = s[:i], s[i+1:]
	}
	if i := strings.Index(s, "/"); i >= 0 {
		s, profile = s[:i], s[i+1:]
	}
	return s, profile, container
}

// markInstalled sets Installed on every target node from the browser list.
func (g *Graph) markInstalled(list []BrowserInfo) {
	for i, n := range g.Targets {
		want, _ := browsers.Lookup(n.Browser) // aliases and bundle ids resolve to the table name
		for _, b := range list {
			if b.Installed && (strings.EqualFold(b.Name, want.Name) || strings.EqualFold(b.BundleID, n.Browser)) {
				g.Targets[i].Installed = true
			}
		}
	}
}
