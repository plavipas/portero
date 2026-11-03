package rulesfile

import (
	"errors"
	"fmt"
	"reflect"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/plavipas/portero/internal/rules"
)

// Remembered is the plain rule "Remember" adds: a host plus a path prefix, opened in one target. For an
// unmatched link Path is "" (a host rule) and FollowDefault writes `open: {default: true}` instead of Target.
type Remembered struct {
	Name          string
	Host          string
	Path          string
	Target        rules.Target
	FollowDefault bool
}

// UniqueName returns base, or "base (2)", "base (3)" ... when a top-level rule or group already has the name.
func UniqueName(r *rules.Rules, base string) string {
	used := map[string]bool{}
	for i, rule := range r.Rules {
		used[rules.Label(i, rule)] = true
	}
	name := base
	for n := 2; used[name]; n++ {
		name = fmt.Sprintf("%s (%d)", base, n)
	}
	return name
}

// scalar renders v as a double-quoted YAML scalar, which escapes control characters and quotes.
func scalar(v string) (string, error) {
	out, err := yaml.Marshal(&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: v, Style: yaml.DoubleQuotedStyle})
	if err != nil {
		return "", err
	}
	return strings.TrimRight(string(out), "\r\n"), nil
}

// InsertBefore returns text with a new plain rule inserted as a top-level element directly before the element
// topIndex of `rules` (and above the comment block attached to it). The insertion is textual: the rest of the
// file, comments included, is kept byte for byte. req is the link being remembered; the result is verified to
// parse, to route req to the new rule and its target, and to keep every other element unchanged.
func InsertBefore(text string, topIndex int, nr Remembered, req rules.Request, now time.Time) (string, error) {
	old, err := rules.Parse([]byte(text))
	if err != nil {
		return "", fmt.Errorf("current rules file: %w", err)
	}
	if topIndex < 0 || topIndex >= len(old.Rules) {
		return "", fmt.Errorf("no rule at position %d", topIndex+1)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return "", errors.New("cannot read the rules file structure")
	}
	var seq *yaml.Node
	m := doc.Content[0]
	for j := 0; j+1 < len(m.Content); j += 2 {
		if m.Content[j].Value == "rules" {
			seq = m.Content[j+1]
		}
	}
	if seq == nil || seq.Kind != yaml.SequenceNode || topIndex >= len(seq.Content) {
		return "", errors.New("the rules list is not a block list")
	}
	if seq.Style&yaml.FlowStyle != 0 {
		return "", errors.New("the rules list is written in flow style ([...]); edit the file by hand")
	}
	nl := "\n"
	if strings.Contains(text, "\r\n") {
		nl = "\r\n"
	}
	lines := strings.SplitAfter(text, "\n")
	at := seq.Content[topIndex].Line - 1
	if at < 0 || at >= len(lines) {
		return "", errors.New("cannot locate the rule in the file")
	}
	item := strings.TrimRight(lines[at], "\r\n")
	trim := strings.TrimLeft(item, " ")
	if trim != "-" && !strings.HasPrefix(trim, "- ") {
		return "", errors.New("the rule does not start with a dash on its own line; edit the file by hand")
	}
	indent := item[:len(item)-len(trim)]
	for at > 0 && strings.HasPrefix(strings.TrimLeft(lines[at-1], " \t"), "#") {
		at-- // the comment block attached to the rule moves with it
	}

	q := func(v string) string { s, _ := scalar(v); return s }
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, indent+format+nl, a...) }
	w("# added by Portero (Remember) %s", now.Format("2006-01-02"))
	w("- name: %s", q(nr.Name))
	w("  match:")
	w("    host: %s", q(nr.Host))
	w("    path: %s", q(nr.Path))
	w("  open:")
	w("    browser: %s", q(nr.Target.Browser))
	if nr.Target.Profile != "" {
		w("    profile: %s", q(nr.Target.Profile))
	}
	if nr.Target.Container != "" {
		w("    container: %s", q(nr.Target.Container))
	}

	// The text before the insertion must end with a line break, or the comment would join the previous line.
	var out strings.Builder
	out.WriteString(strings.Join(lines[:at], ""))
	if at > 0 && !strings.HasSuffix(lines[at-1], "\n") {
		out.WriteString(nl)
	}
	out.WriteString(b.String())
	out.WriteString(strings.Join(lines[at:], ""))
	res := out.String()
	if err := verifyInsert(old, res, topIndex, nr, req); err != nil {
		return "", fmt.Errorf("inserted rule failed verification: %w", err)
	}
	return res, nil
}

// verifyInsert checks the new text: it parses, the link goes to the new rule and target, and every other
// top-level element equals the old one.
func verifyInsert(old *rules.Rules, text string, topIndex int, nr Remembered, req rules.Request) error {
	nw, err := rules.Parse([]byte(text))
	if err != nil {
		return err
	}
	if len(nw.Rules) != len(old.Rules)+1 {
		return fmt.Errorf("expected %d rules, found %d", len(old.Rules)+1, len(nw.Rules))
	}
	if !reflect.DeepEqual(nw.Default, old.Default) {
		return errors.New("default changed")
	}
	for i, r := range old.Rules {
		j := i
		if i >= topIndex {
			j = i + 1
		}
		if !reflect.DeepEqual(r, nw.Rules[j]) {
			return fmt.Errorf("element %d changed", i+1)
		}
	}
	d := nw.Route(req)
	if d.Rule != nr.Name || len(d.Ask) != 0 || d.Target != nr.Target {
		return fmt.Errorf("the link routes to %q (%s), not to the new rule", d.Rule, d)
	}
	return nil
}

// InsertLast returns text with a new host rule appended as the last element of `rules`: after the last element
// and its comments, before the next top-level key (or the end of the file) and before the blank lines and
// column-0 comments that lead to it. The rest of the file is kept byte for byte. The result is verified to parse,
// to keep the default and every old element, to have the new rule last and to route req to it.
func InsertLast(text string, nr Remembered, req rules.Request, now time.Time) (string, error) {
	old, err := rules.Parse([]byte(text))
	if err != nil {
		return "", fmt.Errorf("current rules file: %w", err)
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(text), &doc); err != nil || len(doc.Content) == 0 || doc.Content[0].Kind != yaml.MappingNode {
		return "", errors.New("cannot read the rules file structure")
	}
	var seq *yaml.Node
	m := doc.Content[0]
	next := -1 // 1-based line of the next top-level key after rules
	for j := 0; j+1 < len(m.Content); j += 2 {
		if m.Content[j].Value == "rules" {
			seq = m.Content[j+1]
			if j+2 < len(m.Content) {
				next = m.Content[j+2].Line
			}
		}
	}
	if seq == nil || seq.Kind != yaml.SequenceNode {
		return "", errors.New("the rules list is not a block list")
	}
	if seq.Style&yaml.FlowStyle != 0 {
		return "", errors.New("the rules list is written in flow style ([...]); edit the file by hand")
	}
	if len(seq.Content) == 0 {
		return "", errors.New("the rules list is empty; add the first rule by hand")
	}
	nl := "\n"
	if strings.Contains(text, "\r\n") {
		nl = "\r\n"
	}
	lines := strings.SplitAfter(text, "\n")
	if n := len(lines); n > 0 && lines[n-1] == "" {
		lines = lines[:n-1]
	}
	first := seq.Content[0].Line - 1
	if first < 0 || first >= len(lines) {
		return "", errors.New("cannot locate the first rule in the file")
	}
	item := strings.TrimRight(lines[first], "\r\n")
	trim := strings.TrimLeft(item, " ")
	if trim != "-" && !strings.HasPrefix(trim, "- ") {
		return "", errors.New("the first rule does not start with a dash on its own line; edit the file by hand")
	}
	indent := item[:len(item)-len(trim)]
	at := len(lines)
	if next > 0 {
		at = next - 1
		if at > len(lines) {
			at = len(lines)
		}
	}
	for at > 0 {
		l := lines[at-1]
		if strings.TrimSpace(l) == "" || strings.HasPrefix(l, "#") {
			at--
			continue
		}
		break
	}
	if at <= first {
		return "", errors.New("cannot locate the end of the rules list")
	}

	q := func(v string) string { s, _ := scalar(v); return s }
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, indent+format+nl, a...) }
	w("# added by Portero (Remember, no rule matched) %s", now.Format("2006-01-02"))
	w("- name: %s", q(nr.Name))
	w("  match:")
	w("    host: %s", q(nr.Host))
	w("  open:")
	if nr.FollowDefault {
		w("    default: true")
	} else {
		w("    browser: %s", q(nr.Target.Browser))
		if nr.Target.Profile != "" {
			w("    profile: %s", q(nr.Target.Profile))
		}
		if nr.Target.Container != "" {
			w("    container: %s", q(nr.Target.Container))
		}
	}

	var out strings.Builder
	out.WriteString(strings.Join(lines[:at], ""))
	if at > 0 && !strings.HasSuffix(lines[at-1], "\n") {
		out.WriteString(nl)
	}
	out.WriteString(b.String())
	out.WriteString(strings.Join(lines[at:], ""))
	res := out.String()
	if err := verifyAppend(old, res, nr, req); err != nil {
		return "", fmt.Errorf("appended rule failed verification: %w", err)
	}
	return res, nil
}

// verifyAppend checks the new text: it parses, nothing but the rules list changed, every old element is
// unchanged, the new rule is last and the link routes to it.
func verifyAppend(old *rules.Rules, text string, nr Remembered, req rules.Request) error {
	nw, err := rules.Parse([]byte(text))
	if err != nil {
		return err
	}
	n := len(old.Rules)
	if len(nw.Rules) != n+1 {
		return fmt.Errorf("expected %d rules, found %d", n+1, len(nw.Rules))
	}
	if !reflect.DeepEqual(nw.Default, old.Default) {
		return errors.New("default changed")
	}
	for i, r := range old.Rules {
		if !reflect.DeepEqual(r, nw.Rules[i]) {
			return fmt.Errorf("element %d changed", i+1)
		}
	}
	a, b := *old, *nw
	a.Rules, b.Rules = nil, nil
	if !reflect.DeepEqual(a, b) {
		return errors.New("a setting changed")
	}
	if nw.Rules[n].Name != nr.Name {
		return errors.New("the new rule is not the last element")
	}
	d := nw.Route(req)
	switch {
	case d.Rule != nr.Name || len(d.Ask) != 0:
		return fmt.Errorf("the link routes to %q (%s), not to the new rule", d.Rule, d)
	case nr.FollowDefault && (!d.FollowsDefault || d.Target != nw.Default):
		return fmt.Errorf("the new rule does not follow the default: %s", d)
	case !nr.FollowDefault && (d.FollowsDefault || d.Target != nr.Target):
		return fmt.Errorf("the new rule opens %s, not %s", d.Target, nr.Target)
	}
	return nil
}
