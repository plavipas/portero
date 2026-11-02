//go:build darwin

package main

import (
	"errors"
	"net/url"
	"os"
	"time"

	"github.com/plavipas/portero/internal/handler"
	"github.com/plavipas/portero/internal/launch"
	"github.com/plavipas/portero/internal/rules"
	"github.com/plavipas/portero/internal/serve"
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

// serveDefault reads and sets the default browser for the settings page. SetDefault blocks until macOS has
// been answered (or 1.5 s after it), so the server runs it on its own goroutine.
func serveDefault() (func() serve.DefaultInfo, func() ([]serve.SchemeResult, error)) {
	status := func() serve.DefaultInfo {
		info := serve.DefaultInfo{App: appBundle(), BundleID: BundleID}
		for _, s := range handler.Schemes {
			cur := handler.DefaultHandler(s)
			info.Schemes = append(info.Schemes, serve.SchemeHandler{Scheme: s, Handler: cur, Portero: cur == BundleID})
		}
		return info
	}
	set := func() ([]serve.SchemeResult, error) {
		app := appBundle()
		if app == "" {
			return nil, errors.New("not running from Portero.app")
		}
		states, err := ensureDefault(os.Stdout, app, handler.DefaultHandler, handler.SetDefaultHandler, 100*time.Millisecond, 1500*time.Millisecond)
		var out []serve.SchemeResult
		for _, st := range states {
			out = append(out, serve.SchemeResult{Scheme: st.Scheme, Handler: st.Handler, Note: stateNote(st)})
		}
		return out, err
	}
	return status, set
}
