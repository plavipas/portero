// Package rules holds the rules file: where a link should open and the conditions that decide it.
package rules

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// CurrentVersion is the only rules file version this build reads.
const CurrentVersion = 1

// Target is where a link opens: a browser (app name or bundle id) and, optionally, a profile.
type Target struct {
	Browser string `yaml:"browser"`
	Profile string `yaml:"profile,omitempty"`
	// Container is a Firefox-family container name (needs the "Open external links in a container" extension).
	Container string `yaml:"container,omitempty"`
}

// String renders "Browser", "Browser/Profile", "Browser:Container" or "Browser/Profile:Container".
func (t Target) String() string {
	s := t.Browser
	if t.Profile != "" {
		s += "/" + t.Profile
	}
	if t.Container != "" {
		s += ":" + t.Container
	}
	return s
}

// Strings accepts one string or a list in YAML: `host: a.com` or `host: [a.com, b.com]`.
type Strings []string

// UnmarshalYAML implements yaml.Unmarshaler.
func (s *Strings) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		var one string
		if err := n.Decode(&one); err != nil {
			return err
		}
		*s = Strings{one}
		return nil
	}
	var many []string
	if err := n.Decode(&many); err != nil {
		return err
	}
	*s = many
	return nil
}

// Match lists the conditions of a rule; every present key must match.
type Match struct {
	// Host patterns: exact (`example.com`), any subdomain (`*.example.com`, also the bare domain),
	// or a glob with `*` anywhere.
	Host Strings `yaml:"host,omitempty"`
	// Path prefixes on segments: `/org` matches `/org` and `/org/...`, not `/organic`.
	Path Strings `yaml:"path,omitempty"`
	// Bundle ids of the app that opened the link.
	Opener Strings `yaml:"opener,omitempty"`
	// Directory prefixes (absolute or `~/...`) of the working directory of the process that ran
	// `portero open`; `/a/b` matches `/a/b` and `/a/b/c`, not `/a/bc`. Never matches links from the handler app.
	Cwd Strings `yaml:"cwd,omitempty"`
}

func (m Match) empty() bool {
	return len(m.Host) == 0 && len(m.Path) == 0 && len(m.Opener) == 0 && len(m.Cwd) == 0
}

func checkCwd(where string, prefixes []string) error {
	for _, p := range prefixes {
		if p == "" || strings.ContainsAny(p, "\t\n\r") {
			return fmt.Errorf("rules: %s: cwd %q is empty or has a control character", where, p)
		}
		if !strings.HasPrefix(p, "/") && p != "~" && !strings.HasPrefix(p, "~/") {
			return fmt.Errorf("rules: %s: cwd %q must be an absolute path or start with ~/", where, p)
		}
	}
	return nil
}

// UsesCwd reports whether any rule, including the rules inside groups, has a cwd condition.
func (r *Rules) UsesCwd() bool {
	for _, rule := range r.Rules {
		if len(rule.Match.Cwd) > 0 {
			return true
		}
		for _, c := range rule.Rules {
			if len(c.Match.Cwd) > 0 {
				return true
			}
		}
	}
	return false
}

// Rule is one element of the table. A plain rule has match and open. A group (an element with a `rules` key)
// has a name, one open target and a list of child rules that carry only a match.
type Rule struct {
	Name  string `yaml:"name,omitempty"`
	Match Match  `yaml:"match"`
	Open  Target `yaml:"open"`
	// Rules makes the element a group; the children have a match and no open.
	Rules []Rule `yaml:"rules,omitempty"`
}

// IsGroup reports whether the element has a `rules` key (also an empty list, which Validate rejects).
func (r Rule) IsGroup() bool { return r.Rules != nil }

// Rules is the whole file.
type Rules struct {
	Version int    `yaml:"version"`
	Default Target `yaml:"default"`
	Rules   []Rule `yaml:"rules"`
}

// Parse decodes and validates a rules file.
func Parse(text []byte) (*Rules, error) {
	var r Rules
	dec := yaml.NewDecoder(bytesReader(text))
	dec.KnownFields(true)
	if err := dec.Decode(&r); err != nil {
		return nil, fmt.Errorf("rules: %w", err)
	}
	if err := r.Validate(); err != nil {
		return nil, err
	}
	return &r, nil
}

// Load reads and parses the file at path.
func Load(path string) (*Rules, error) {
	text, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return Parse(text)
}

// Validate checks the version, that every rule has a condition and a browser, and the shape of groups.
func (r *Rules) Validate() error {
	if r.Version != CurrentVersion {
		return fmt.Errorf("rules: version %d is not supported (expected %d)", r.Version, CurrentVersion)
	}
	if r.Default.Browser == "" {
		return fmt.Errorf("rules: default has an empty browser")
	}
	if err := checkControl("default", r.Default); err != nil {
		return err
	}
	for i, rule := range r.Rules {
		if rule.IsGroup() {
			if err := validateGroup(i, rule); err != nil {
				return err
			}
			continue
		}
		where := "rule " + quoted(i, rule.Name)
		if err := checkName(where, rule.Name); err != nil {
			return err
		}
		if err := checkControl(where, rule.Open); err != nil {
			return err
		}
		if rule.Match.empty() {
			return fmt.Errorf("rules: %s has no match conditions (host, path, opener or cwd)", where)
		}
		if err := checkCwd(where, rule.Match.Cwd); err != nil {
			return err
		}
		if rule.Open.Browser == "" {
			return fmt.Errorf("rules: %s has an empty browser", where)
		}
	}
	return nil
}

func validateGroup(i int, g Rule) error {
	if g.Name == "" {
		return fmt.Errorf("rules: group #%d has no name", i+1)
	}
	gw := fmt.Sprintf("group %q", g.Name)
	if err := checkName(gw, g.Name); err != nil {
		return err
	}
	if g.Open.Browser == "" {
		return fmt.Errorf("rules: %s has an empty browser", gw)
	}
	if err := checkControl(gw, g.Open); err != nil {
		return err
	}
	if !g.Match.empty() {
		return fmt.Errorf("rules: %s has match; conditions go into its rules", gw)
	}
	if len(g.Rules) == 0 {
		return fmt.Errorf("rules: %s has no rules", gw)
	}
	for j, c := range g.Rules {
		where := gw + ", rule " + quoted(j, c.Name)
		if err := checkName(where, c.Name); err != nil {
			return err
		}
		if c.IsGroup() {
			return fmt.Errorf("rules: %s: groups cannot contain groups", where)
		}
		if c.Open != (Target{}) {
			return fmt.Errorf("rules: %s: open is set on the group only", where)
		}
		if c.Match.empty() {
			return fmt.Errorf("rules: %s has no match conditions (host, path, opener or cwd)", where)
		}
		if err := checkCwd(where, c.Match.Cwd); err != nil {
			return err
		}
	}
	return nil
}

// checkName rejects names that would break the tab-separated log or the "Group / rule" label.
func checkName(where, name string) error {
	if strings.ContainsAny(name, "\t\n\r") {
		return fmt.Errorf("rules: %s: name contains a control character", where)
	}
	if name != strings.TrimSpace(name) {
		return fmt.Errorf("rules: %s: name has leading or trailing spaces", where)
	}
	if strings.Contains(name, " / ") {
		return fmt.Errorf("rules: %s: name must not contain \" / \" (it separates a group from its rule in the log)", where)
	}
	return nil
}

// checkControl rejects tabs, line breaks and surrounding spaces in the fields that reach the log and the
// command line: the container extension matches names exactly, so " Work" would create a second container.
func checkControl(where string, t Target) error {
	for _, f := range []struct{ field, v string }{{"browser", t.Browser}, {"profile", t.Profile}, {"container", t.Container}} {
		field, v := f.field, f.v
		if strings.ContainsAny(v, "\t\n\r") {
			return fmt.Errorf("rules: %s: %s contains a control character", where, field)
		}
		if v != strings.TrimSpace(v) {
			return fmt.Errorf("rules: %s: %s has leading or trailing spaces", where, field)
		}
	}
	return nil
}

// quoted renders a name in quotes for errors, or #n (1-based position) when there is none.
func quoted(i int, name string) string {
	if name != "" {
		return fmt.Sprintf("%q", name)
	}
	return fmt.Sprintf("#%d", i+1)
}

// Label is the name a plain rule or a group goes by in the log, the diagram and warnings: its name, or "#n"
// where n is its 1-based position in the top-level list (groups counted).
func Label(i int, rule Rule) string {
	if rule.Name != "" {
		return rule.Name
	}
	return fmt.Sprintf("#%d", i+1)
}

// ChildLabel is "Group / child"; a child without a name goes by the group name alone.
func ChildLabel(group, child Rule) string {
	if child.Name == "" {
		return group.Name
	}
	return group.Name + " / " + child.Name
}
