//go:build !darwin

package main

import (
	"net/url"

	"github.com/plavipas/portero/internal/rules"
)

// servePlan: launching is not supported here, so the UI shows no plan.
func servePlan() func(*url.URL, rules.Target) (string, error) { return nil }
