package launch

import (
	"github.com/plavipas/portero/internal/browsers"
	"github.com/plavipas/portero/internal/rules"
)

func browsersZen() browsers.Browser {
	b, _ := browsers.Lookup("Zen")
	return b
}

func browsersLookup(s string) (browsers.Browser, bool) { return browsers.Lookup(s) }

func rulesTarget(browser, profile, container string) rules.Target {
	return rules.Target{Browser: browser, Profile: profile, Container: container}
}
