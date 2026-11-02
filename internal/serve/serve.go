// Package serve is the local web UI of Portero: a diagram of the routing and an editor for the rules file.
//
// It is portable (standard library plus yaml.v3, no cgo, no processes). Everything that touches this Mac comes
// in through Options, so tests run against a temporary directory.
package serve

import (
	"net/http"
	"net/url"
	"sync"

	"github.com/plavipas/portero/internal/browsers"
	"github.com/plavipas/portero/internal/check"
	"github.com/plavipas/portero/internal/rules"
)

// BrowserInfo is one browser of the table with what this Mac has of it.
type BrowserInfo struct {
	Name       string              `json:"name"`
	Kind       string              `json:"kind"`
	BundleID   string              `json:"bundle_id"`
	Installed  bool                `json:"installed"`
	Profiles   []string            `json:"profiles,omitempty"`
	Containers map[string][]string `json:"containers,omitempty"` // profile name -> container names
}

// Options wires the server to the files and the Mac. No path is ever taken from a request.
type Options struct {
	RulesFile  string
	RulesPlace string // "local" or "icloud"
	LogFile    string
	Env        check.Env
	Browsers   func() []BrowserInfo
	// Plan describes what opening u in t would run; nil where launching is not supported.
	Plan func(u *url.URL, t rules.Target) (string, error)
}

// Server serves the UI and its JSON API.
type Server struct {
	opts  Options
	mu    sync.Mutex // serializes Apply
	gmu   sync.RWMutex
	hosts map[string]bool
}

// New builds a server; call SetAddr with the bound address before serving.
func New(opts Options) *Server {
	e := &opts.Env
	if e.Installed == nil {
		e.Installed = func(browsers.Browser) bool { return true }
	}
	if e.Containers == nil {
		e.Containers = func(browsers.Browser, string) (string, []browsers.Container, error) {
			return "", nil, errUnknown
		}
	}
	if e.Extension == nil {
		e.Extension = func(browsers.Browser, string) (string, bool, error) { return "", false, errUnknown }
	}
	if e.DirExists == nil {
		e.DirExists = func(string) bool { return true }
	}
	if opts.Browsers == nil {
		opts.Browsers = func() []BrowserInfo { return nil }
	}
	if opts.RulesPlace == "" {
		opts.RulesPlace = "local"
	}
	return &Server{opts: opts, hosts: map[string]bool{}}
}

// Handler is the whole site: static files and /api/*, behind the guard.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	s.routesAPI(mux)
	s.routesUI(mux)
	return s.guard(mux)
}
