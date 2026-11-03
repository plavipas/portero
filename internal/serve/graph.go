package serve

import (
	"strings"

	"github.com/plavipas/portero/internal/browsers"
	"github.com/plavipas/portero/internal/rules"
)

// Node ids: "o:<bundle id or ->", "r:<user rule name>", "s:default|file|error" for the built-in nodes,
// "t:<Target.String()>". The built-ins have their own namespace so a user rule named "default" stays apart.
func openerID(o string) string { return "o:" + o }
func ruleID(r string) string   { return "r:" + r }

// specialID is the node of a rule name the log uses for built-in outcomes, "" for any other name.
func specialID(name string) string {
	switch name {
	case "default", "":
		return "s:default"
	case "file":
		return "s:file"
	case "ERROR":
		return "s:error"
	}
	return ""
}

// fileNodes maps every label the log can carry for a rule of the file to its node: a plain rule to its own
// node, a group and each "Group / child" to the group node.
func fileNodes(r *rules.Rules) map[string]string {
	m := map[string]string{}
	if r == nil {
		return m
	}
	for i, rule := range r.Rules {
		if rule.IsGroup() {
			m[rule.Name] = ruleID(rule.Name)
			for _, c := range rule.Rules {
				m[rules.ChildLabel(rule, c)] = ruleID(rule.Name)
			}
			continue
		}
		l := rules.Label(i, rule)
		m[l] = ruleID(l)
	}
	return m
}

// logRuleID maps the rule column of the log to a node. The log cannot tell a user rule named "default" or
// "file" from the built-in outcome of the same name, so when the rules file has such a rule it wins;
// otherwise the built-in node is used. A label the file does not have gets a node of its own (stale).
func logRuleID(r *rules.Rules, name string) string {
	sp := specialID(name)
	if sp == "s:error" {
		return sp
	}
	if id, ok := fileNodes(r)[name]; ok {
		return id
	}
	if sp != "" {
		return sp
	}
	return ruleID(name)
}

// ResolvePaths sets Path[1] of every decision against the rules file (nil when it is broken).
func ResolvePaths(r *rules.Rules, ds []Decision) {
	for i := range ds {
		if ds[i].Kind == "ok" {
			ds[i].Path[1] = logRuleID(r, ds[i].Rule)
		}
	}
}

func targetID(t string) string { return "t:" + t }

// Node is one box of the diagram.
type Node struct {
	ID    string `json:"id"`
	Label string `json:"label"`
	Kind  string `json:"kind"` // opener, rule, default, file, error, target
	// Stale: a rule the log mentions that the file no longer has.
	Stale bool `json:"stale,omitempty"`
	// Ask: the rule or group offers a choice of targets in a panel.
	Ask bool `json:"ask,omitempty"`
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
	// Ask: the edge is one of the choices of an asking rule (or its Esc fallback, the default target).
	Ask bool `json:"ask,omitempty"`
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
			name := rules.Label(i, rule)
			addRule(ruleID(name), name, "rule", false)
			if rule.Open.Asks() {
				for i := range g.Rules {
					if g.Rules[i].ID == ruleID(name) {
						g.Rules[i].Ask = true
					}
				}
				for _, t := range rule.Open.Ask {
					t := t
					addTarget(t.String(), &t)
					addEdge(ruleID(name), targetID(t.String()), 0)
					edges[[2]string{ruleID(name), targetID(t.String())}].Ask = true
				}
				d := r.Default
				addTarget(d.String(), &d)
				addEdge(ruleID(name), targetID(d.String()), 0)
				edges[[2]string{ruleID(name), targetID(d.String())}].Ask = true
				continue
			}
			t := rule.Open.Target
			addTarget(t.String(), &t)
			addEdge(ruleID(name), targetID(t.String()), 0)
		}
	}
	addRule("s:default", "default", "default", false)
	addRule("s:file", "file", "file", false)
	addRule("s:error", "ERROR → Safari fallback", "error", false)
	if r != nil {
		t := r.Default
		addTarget(t.String(), &t)
		addEdge("s:default", targetID(t.String()), 0)
		addEdge("s:file", targetID(t.String()), 0)
	}
	addTarget(fallbackTarget, nil)
	addEdge("s:error", targetID(fallbackTarget), 0)

	// The log.
	ResolvePaths(r, decisions)
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
			addRule(rid, strings.TrimPrefix(rid, "r:"), "rule", r != nil) // only unknown user rules get here
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
