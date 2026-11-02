//go:build darwin

package main

import (
	"net/url"

	"github.com/plavipas/portero/internal/launch"
	"github.com/plavipas/portero/internal/rules"
)

// servePlan shows what a link would run, without running it.
func servePlan() func(*url.URL, rules.Target) (string, error) {
	return func(u *url.URL, t rules.Target) (string, error) {
		p, err := launch.DryRun(u, t)
		if err != nil {
			return "", err
		}
		return p.String(), nil
	}
}
