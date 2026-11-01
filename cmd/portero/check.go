package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/plavipas/portero/internal/browsers"
	"github.com/plavipas/portero/internal/rules"
)

// checkEnv is what checkTargets asks the Mac; tests replace it.
type checkEnv struct {
	installed func(browsers.Browser) bool
	// containers returns the display name of the profile (the named one, else the default) and its containers.
	containers func(b browsers.Browser, profile string) (string, []browsers.Container, error)
	// extension returns the same profile name and whether the container extension is active in it.
	extension func(b browsers.Browser, profile string) (string, bool, error)
	// dirExists reports whether a cwd prefix from the rules file is an existing directory on this Mac.
	dirExists func(prefix string) bool
}

func realCheckEnv() checkEnv {
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
	return checkEnv{
		installed: func(b browsers.Browser) bool { return b.Installed() },
		dirExists: func(p string) bool {
			fi, err := os.Stat(rules.ExpandCwd(p))
			return err == nil && fi.IsDir()
		},
		containers: func(b browsers.Browser, name string) (string, []browsers.Container, error) {
			p, err := find(b, name)
			if err != nil {
				return "", nil, err
			}
			list, err := b.Containers(p)
			return p.Name, list, err
		},
		extension: func(b browsers.Browser, name string) (string, bool, error) {
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

// checkTargets validates the targets of the default and every rule against this Mac. A container on a
// browser that cannot take one is an error; everything else is a warning.
func checkTargets(r *rules.Rules, env checkEnv) (warnings []string, err error) {
	type item struct {
		where string // `default` or `rule "x"`
		t     rules.Target
	}
	items := []item{{"default", r.Default}}
	for i, rule := range r.Rules {
		items = append(items, item{"rule " + ruleLabel(i, rule.Name), rule.Open})
	}
	for _, it := range items {
		b, known := browsers.Lookup(it.t.Browser)
		if it.t.Container != "" && b.Kind != browsers.Firefox {
			return warnings, fmt.Errorf("rules: %s: container %q needs a Firefox-family browser (%s); %s does not take one",
				it.where, it.t.Container, firefoxFamily(), b.Name)
		}
		if !known && strings.Contains(b.Name, ".") {
			continue // a bundle id outside the table: nothing to look up without Launch Services
		}
		if !env.installed(b) {
			warnings = append(warnings, fmt.Sprintf("warning: %s: %s is not installed on this Mac", it.where, b.Name))
			continue
		}
		if it.t.Container == "" || !known {
			continue
		}
		if name, list, err := env.containers(b, it.t.Profile); err == nil {
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
		name, active, err := env.extension(b, it.t.Profile)
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

// checkCwd warns about rules whose cwd directories all do not exist on this Mac.
func checkCwd(r *rules.Rules, env checkEnv) (warnings []string) {
	for i, rule := range r.Rules {
		if len(rule.Match.Cwd) == 0 {
			continue
		}
		found := false
		for _, p := range rule.Match.Cwd {
			if env.dirExists(p) {
				found = true
				break
			}
		}
		if !found {
			warnings = append(warnings, fmt.Sprintf("warning: rule %s: none of its cwd directories exist on this Mac (%s)",
				ruleLabel(i, rule.Name), strings.Join(rule.Match.Cwd, ", ")))
		}
	}
	return warnings
}

func ruleLabel(i int, name string) string {
	if name != "" {
		return fmt.Sprintf("%q", name)
	}
	return fmt.Sprintf("#%d", i+1)
}

// containerLines renders one "containers of <profile>" line per Firefox profile with a readable containers.json.
func containerLines(b browsers.Browser) []string {
	if b.Kind != browsers.Firefox {
		return nil
	}
	profiles, err := b.Profiles()
	if err != nil {
		return nil
	}
	var out []string
	for _, p := range profiles {
		list, err := b.Containers(p)
		if err != nil {
			continue
		}
		var names []string
		for _, c := range list {
			names = append(names, c.Name)
		}
		line := fmt.Sprintf("    containers of %s: %s", p.Name, strings.Join(names, ", "))
		if active, err := b.ContainerExtension(p); err == nil {
			if active {
				line += " (extension: active)"
			} else {
				line += " (extension: missing)"
			}
		}
		out = append(out, line)
	}
	return out
}
