//go:build !darwin

package main

import (
	"net/url"

	"github.com/plavipas/portero/internal/rules"
	"github.com/plavipas/portero/internal/serve"
)

// servePlan: launching is not supported here, so the UI shows no plan.
func servePlan() func(*url.URL, rules.Target) (string, error) { return nil }

// serveDefault: the default browser is a macOS setting, so the settings page has no such block here.
func serveDefault() (func() serve.DefaultInfo, func() ([]serve.SchemeResult, error)) { return nil, nil }
