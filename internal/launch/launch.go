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
	// Container is the Firefox container the link opens in, "" when none.
	Container string
	Steps     []string
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

// resolveTarget maps the profile and container of a rule to what the browser wants on its command line.
// A container on a browser that cannot take one is dropped with a note; the link still opens.
func resolveTarget(b browsers.Browser, t rules.Target) (profile, container, note string, err error) {
	profile, err = resolveProfile(b, t.Profile)
	if err != nil {
		return "", "", "", err
	}
	if t.Container == "" {
		return profile, "", "", nil
	}
	if b.Kind != browsers.Firefox {
		return profile, "", fmt.Sprintf("container %q ignored: %s does not take one", t.Container, b.Name), nil
	}
	container = t.Container
	if p, ok := firefoxProfile(b, t.Profile); ok {
		if list, err := b.Containers(p); err == nil {
			// The extension compares names case-sensitively: an exact match wins over a case-insensitive one.
			for _, c := range list {
				if c.Name == t.Container {
					container = c.Name
					break
				}
				if strings.EqualFold(c.Name, t.Container) && container == t.Container {
					container = c.Name
				}
			}
		}
	}
	return profile, container, "", nil
}

// firefoxProfile finds the named profile of a Firefox-family browser, else its default one.
func firefoxProfile(b browsers.Browser, name string) (browsers.Profile, bool) {
	profiles, err := b.Profiles()
	if err != nil {
		return browsers.Profile{}, false
	}
	if name == "" {
		return b.DefaultProfile(profiles)
	}
	return browsers.Resolve(profiles, name)
}

// firefoxLink returns the URL to hand to Firefox and the notes for the plan. With a container it is the
// ext+container: form, unless the scheme cannot be opened by the extension or the extension is known to be off.
func firefoxLink(b browsers.Browser, u *url.URL, profileName, container string) (string, []string) {
	link := u.String()
	if container == "" {
		return link, nil
	}
	if s := strings.ToLower(u.Scheme); s != "http" && s != "https" {
		return link, []string{fmt.Sprintf("container skipped: %s links cannot be opened by the extension", u.Scheme)}
	}
	if p, ok := firefoxProfile(b, profileName); ok {
		if active, err := b.ContainerExtension(p); err == nil && !active {
			return link, []string{fmt.Sprintf("container extension not active in profile %q of %s: opening without container", p.Name, b.Name)}
		}
	}
	return ContainerURL(container, u), nil
}

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
