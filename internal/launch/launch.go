// Package launch opens a link in a browser and, where the browser has them, in a profile.
package launch

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/plavipas/portero/internal/browsers"
	"github.com/plavipas/portero/internal/rules"
)

// Plan is what Open would run, for --dry-run and the log.
type Plan struct {
	Browser browsers.Browser
	Profile string // resolved profile directory / name, "" when none
	Steps   []string
}

func (p Plan) String() string { return strings.Join(p.Steps, "; then ") }

// Open opens u in the target's browser and profile.
func Open(u *url.URL, t rules.Target) (Plan, error) {
	return open(u, t, false)
}

// DryRun returns the plan without running anything.
func DryRun(u *url.URL, t rules.Target) (Plan, error) {
	return open(u, t, true)
}

// resolveProfile maps the profile name of a rule to what the browser wants on its command line.
func resolveProfile(b browsers.Browser, want string) (string, error) {
	if want == "" {
		return "", nil
	}
	switch b.Kind {
	case browsers.Chromium, browsers.Firefox:
		profiles, err := b.Profiles()
		if err != nil {
			return "", fmt.Errorf("profiles of %s: %w", b.Name, err)
		}
		p, ok := browsers.Resolve(profiles, want)
		if !ok {
			return "", fmt.Errorf("%s has no profile %q (have: %s)", b.Name, want, browsers.Names(profiles))
		}
		if b.Kind == browsers.Chromium {
			return p.Dir, nil
		}
		return p.Name, nil
	case browsers.Dia, browsers.Arc:
		return want, nil
	}
	return "", fmt.Errorf("%s does not take a profile (rule asks for %q)", b.Name, want)
}
