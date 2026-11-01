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
}

func (m Match) empty() bool { return len(m.Host) == 0 && len(m.Path) == 0 && len(m.Opener) == 0 }

// Rule is one line of the table: conditions and the target.
type Rule struct {
	Name  string `yaml:"name,omitempty"`
	Match Match  `yaml:"match"`
	Open  Target `yaml:"open"`
}

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

// Validate checks the version, that every rule has a condition and a browser.
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
		if err := checkControl("rule "+label(i, rule.Name), rule.Open); err != nil {
			return err
		}
		if rule.Match.empty() {
			return fmt.Errorf("rules: rule %s has no match conditions (host, path or opener)", label(i, rule.Name))
		}
		if rule.Open.Browser == "" {
			return fmt.Errorf("rules: rule %s has an empty browser", label(i, rule.Name))
		}
	}
	return nil
}

// checkControl rejects tabs and line breaks in the fields that reach the log and the command line.
func checkControl(where string, t Target) error {
	for field, v := range map[string]string{"profile": t.Profile, "container": t.Container} {
		if strings.ContainsAny(v, "\t\n\r") {
			return fmt.Errorf("rules: %s: %s contains a control character", where, field)
		}
	}
	return nil
}

func label(i int, name string) string {
	if name != "" {
		return fmt.Sprintf("%q", name)
	}
	return fmt.Sprintf("#%d", i+1)
}
