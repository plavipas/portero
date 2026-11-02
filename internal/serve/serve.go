// Package serve is the local web UI of Portero: a diagram of the routing and an editor for the rules file.
//
// It is portable (standard library plus yaml.v3, no cgo, no processes). Everything that touches this Mac comes
// in through Options, so tests run against a temporary directory.
package serve

import (
	"crypto/rand"
	"encoding/hex"
	"io"
	"net/http"
	"net/url"
	"os"
	"sync"
	"time"

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
	// Nonce is the one-time secret in the link that opens the UI (see Nonce()); empty means a fresh random one.
	Nonce string
	// Session is the secret the session cookie carries; it is never in a URL. Empty means a fresh random one.
	Session string
	// Notice receives the warning about a reused link; nil means os.Stdout.
	Notice io.Writer
	// Rules, when set, is asked on every request and wins over RulesFile/RulesPlace, so a `portero sync`
	// while the server runs is picked up without a restart.
	Rules    func() (path, place string)
	LogFile  string
	Env      check.Env
	Browsers func() []BrowserInfo
	// Plan describes what opening u in t would run; nil where launching is not supported.
	Plan func(u *url.URL, t rules.Target) (string, error)
	// ConfigFile is shown on the settings page; it is never read or written by the server.
	ConfigFile string
	// DefaultStatus reads the default browser setting; nil where macOS is not available (no Default block).
	DefaultStatus func() DefaultInfo
	// SetDefault asks macOS to make Portero the default browser; it may block until the user answers the
	// macOS dialog. nil where unsupported.
	SetDefault func() ([]SchemeResult, error)
	// SetDefaultWait is how long a request waits for SetDefault before it answers 202; zero means 60 s.
	SetDefaultWait time.Duration
	// Sync switches the rules file between the local folder and iCloud Drive; nil where unsupported.
	Sync *SyncFuncs
}

// Server serves the UI and its JSON API.
type Server struct {
	opts         Options
	mu           sync.Mutex // serializes Apply
	gmu          sync.RWMutex
	amu          sync.Mutex // guards used and warned
	used, warned bool
	hosts        map[string]bool
	port         string     // from SetAddr; part of the session cookie name
	smu          sync.Mutex // guards pending and last
	pending      bool       // a SetDefault call is running
	last         *defaultResult
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
	if opts.Nonce == "" {
		opts.Nonce = randomHex()
	}
	if opts.Session == "" {
		opts.Session = randomHex()
	}
	if opts.Notice == nil {
		opts.Notice = os.Stdout
	}
	if opts.RulesPlace == "" {
		opts.RulesPlace = "local"
	}
	if opts.SetDefaultWait <= 0 {
		opts.SetDefaultWait = 60 * time.Second
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

func (s *Server) rulesPath() string {
	if s.opts.Rules != nil {
		p, _ := s.opts.Rules()
		return p
	}
	return s.opts.RulesFile
}

func (s *Server) rulesPlace() string {
	if s.opts.Rules != nil {
		if _, pl := s.opts.Rules(); pl != "" {
			return pl
		}
		return "local"
	}
	return s.opts.RulesPlace
}

// Nonce is the one-time secret of the link: `GET /auth/<nonce>` works once and sets the session cookie.
func (s *Server) Nonce() string { return s.opts.Nonce }

func randomHex() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("serve: no randomness for the UI secrets: " + err.Error())
	}
	return hex.EncodeToString(b)
}
