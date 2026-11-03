// Package check validates a parsed rules file against this Mac: missing browsers, containers, extensions, directories.
package check

import (
	"fmt"
	"os"
	"strings"

	"github.com/plavipas/portero/internal/browsers"
	"github.com/plavipas/portero/internal/rules"
)

// Env is what Targets asks the Mac; tests replace it.
type Env struct {
	Installed func(browsers.Browser) bool
	// Containers returns the display name of the profile (the named one, else the default) and its containers.
	Containers func(b browsers.Browser, profile string) (string, []browsers.Container, error)
	// Extension returns the same profile name and whether the container extension is active in it.
	Extension func(b browsers.Browser, profile string) (string, bool, error)
	// DirExists reports whether a cwd prefix from the rules file is an existing directory on this Mac.
	DirExists func(prefix string) bool
}

// RealEnv asks this Mac.
func RealEnv() Env {
	find := func(b browsers.Browser, name string) (browsers.Profile, error) {
		profiles, err := b.Profiles()
		if err != nil {
			return browsers.Profile{}, err
		}
		var p browsers.Profile
		ok := false
		if name == "" {
			p, ok = b.DefaultProfile(profiles)
		} else {
			p, ok = browsers.Resolve(profiles, name)
		}
		if !ok {
			return browsers.Profile{}, fmt.Errorf("no profile")
		}
		return p, nil
	}
	return Env{
		Installed: func(b browsers.Browser) bool { return b.Installed() },
		DirExists: func(p string) bool {
			fi, err := os.Stat(rules.ExpandCwd(p))
			return err == nil && fi.IsDir()
		},
		Containers: func(b browsers.Browser, name string) (string, []browsers.Container, error) {
			p, err := find(b, name)
			if err != nil {
				return "", nil, err
			}
			list, err := b.Containers(p)
			return p.Name, list, err
		},
		Extension: func(b browsers.Browser, name string) (string, bool, error) {
			p, err := find(b, name)
			if err != nil {
				return "", false, err
			}
			ok, err := b.ContainerExtension(p)
			return p.Name, ok, err
		},
	}
}

func firefoxFamily() string {
	var n []string
	for _, b := range browsers.Known {
		if b.Kind == browsers.Firefox {
			n = append(n, b.Name)
		}
	}
	return strings.Join(n, ", ")
}

// Targets validates the targets of the default and every rule against this Mac. A container on a
// browser that cannot take one is an error; everything else is a warning.
func Targets(r *rules.Rules, env Env) (warnings []string, err error) {
	type item struct {
		where string // `default`, `rule "x"` or `group "x"`
		t     rules.Target
	}
	items := []item{{"default", r.Default}}
	for i, rule := range r.Rules {
		where := "rule " + quoted(i, rule.Name)
		if rule.IsGroup() {
			where = fmt.Sprintf("group %q", rule.Name)
		}
		if rule.Open.Asks() {
			for j, t := range rule.Open.Ask {
				items = append(items, item{fmt.Sprintf("%s, ask %d", where, j+1), t})
			}
			continue
		}
		items = append(items, item{where, rule.Open.Target})
	}
	for _, it := range items {
		b, known := browsers.Lookup(it.t.Browser)
		if it.t.Container != "" && b.Kind != browsers.Firefox {
			return warnings, fmt.Errorf("rules: %s: container %q needs a Firefox-family browser (%s); %s does not take one",
				it.where, it.t.Container, firefoxFamily(), b.Name)
		}
		if !known && strings.Contains(b.Name, "/") {
			warnings = append(warnings, fmt.Sprintf("warning: %s: browser %q is a path; Portero opens it with open -a", it.where, b.Name))
			continue
		}
		if !known && strings.Contains(b.Name, ".") {
			continue // a bundle id outside the table: nothing to look up without Launch Services
		}
		if !env.Installed(b) {
			warnings = append(warnings, fmt.Sprintf("warning: %s: %s is not installed on this Mac", it.where, b.Name))
			continue
		}
		if it.t.Container == "" || !known {
			continue
		}
		if name, list, err := env.Containers(b, it.t.Profile); err == nil {
			found := false
			var have []string
			for _, c := range list {
				have = append(have, c.Name)
				if strings.EqualFold(c.Name, it.t.Container) {
					found = true
				}
			}
			if !found {
				warnings = append(warnings, fmt.Sprintf("warning: %s: profile %q of %s has no container %q (have: %s); the extension would create it on first use",
					it.where, name, b.Name, it.t.Container, strings.Join(have, ", ")))
			}
		}
		name, active, err := env.Extension(b, it.t.Profile)
		switch {
		case err != nil:
			warnings = append(warnings, fmt.Sprintf("warning: %s: the state of the %q extension in %s is unknown (profile not found or extensions.json unreadable); links open without a container",
				it.where, browsers.ContainerExtensionName, b.Name))
		case !active:
			warnings = append(warnings, fmt.Sprintf("warning: %s: profile %q of %s has no active %q extension; links open without a container",
				it.where, name, b.Name, browsers.ContainerExtensionName))
		}
	}
	return warnings, nil
}

// Cwd warns about rules whose cwd directories all do not exist on this Mac.
func Cwd(r *rules.Rules, env Env) (warnings []string) {
	warn := func(where string, m rules.Match) {
		if len(m.Cwd) == 0 {
			return
		}
		for _, p := range m.Cwd {
			if env.DirExists(p) {
				return
			}
		}
		warnings = append(warnings, fmt.Sprintf("warning: %s: none of its cwd directories exist on this Mac (%s)",
			where, strings.Join(m.Cwd, ", ")))
	}
	for i, rule := range r.Rules {
		if rule.IsGroup() {
			for j, c := range rule.Rules {
				warn(fmt.Sprintf("group %q, rule %s", rule.Name, quoted(j, c.Name)), c.Match)
			}
			continue
		}
		warn("rule "+quoted(i, rule.Name), rule.Match)
	}
	return warnings
}

// Names warns about names used twice among groups and plain rules: the diagram and the log merge them.
func Names(r *rules.Rules) (warnings []string) {
	seen := map[string]bool{}
	warned := map[string]bool{}
	for i, rule := range r.Rules {
		name := rules.Label(i, rule)
		if seen[name] && !warned[name] {
			warned[name] = true
			warnings = append(warnings, fmt.Sprintf("warning: name %q is used twice; the diagram and the log merge them", name))
		}
		seen[name] = true
		if rule.IsGroup() {
			inner := map[string]bool{}
			for _, c := range rule.Rules {
				if c.Name == "" {
					continue
				}
				if inner[c.Name] && !warned[rule.Name+" / "+c.Name] {
					warned[rule.Name+" / "+c.Name] = true
					warnings = append(warnings, fmt.Sprintf("warning: group %q: rule name %q is used twice; the log merges them", rule.Name, c.Name))
				}
				inner[c.Name] = true
			}
		}
	}
	return warnings
}

// quoted renders a name quoted, or #n (1-based position) for an unnamed rule.
func quoted(i int, name string) string {
	if name != "" {
		return fmt.Sprintf("%q", name)
	}
	return fmt.Sprintf("#%d", i+1)
}

// All is Targets followed by Cwd: every warning `rules check` prints.
func All(r *rules.Rules, env Env) ([]string, error) {
	warnings, err := Targets(r, env)
	if err != nil {
		return warnings, err
	}
	return append(append(warnings, Cwd(r, env)...), Names(r)...), nil
}
