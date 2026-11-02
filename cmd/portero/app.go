package main

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/plavipas/portero/internal/config"
	"github.com/plavipas/portero/internal/decisionlog"
	"github.com/plavipas/portero/internal/handler"
	"github.com/plavipas/portero/internal/launch"
	"github.com/plavipas/portero/internal/rules"
)

// fallback is used when the rules file cannot be read: the link still opens somewhere.
var fallback = rules.Target{Browser: "Safari"}

// BundleID of the app; must match packaging/Info.plist.
const BundleID = "dev.plavipas.portero"

// appBundle returns the .app this binary runs from ("" when run as a bare CLI binary).
func appBundle() string {
	exe, err := os.Executable()
	if err != nil {
		return ""
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	if i := strings.Index(exe, ".app/Contents/MacOS/"); i >= 0 {
		return exe[:i+4]
	}
	return ""
}

// runApp is the handler app: every link from macOS is routed like `portero open`.
func runApp() {
	handler.Run(func(link, opener string) {
		if err := routeLink(link, opener); err != nil {
			fmt.Fprintln(os.Stderr, "portero:", err)
			_ = decisionlog.AppendError(config.LogFile(), link, opener, err)
		}
	})
}

func routeLink(link, opener string) error {
	u, err := parseRoutable(link)
	if err != nil {
		return err
	}
	// The handler has no meaningful cwd (Launch Services starts the app in /), so cwd rules never match here.
	req := rules.Request{URL: u, Opener: opener}
	r, err := rules.Load(config.RulesFile())
	if err != nil {
		// A missing or broken rules file must never swallow a link: Safari is always there.
		_ = decisionlog.AppendError(config.LogFile(), link, opener, fmt.Errorf("%w; opened in %s", err, fallback.Browser))
		_, openErr := launch.Open(u, fallback)
		return openErr
	}
	d := decide(r, req)
	if err := decisionlog.Append(config.LogFile(), d, req); err != nil {
		fmt.Fprintln(os.Stderr, "portero: log:", err)
	}
	_, err = launch.Open(u, d.Target)
	return err
}

// schemeState is the handler of one URL scheme and whether it was already Portero before `default set`.
type schemeState struct {
	Scheme  string
	Handler string
	Already bool
}

// ensureDefault makes app the handler of every scheme in handler.Schemes.
//
// macOS shows one dialog that sets http and https together, so the call for the second scheme can fail with
// "The file couldn't be opened" although the scheme is already ours. The error of set is therefore not
// informative: only the actual handler, read after each call, is trusted.
//
// After set the handler is polled every `every` for up to `within`, because Launch Services can lag behind the dialog.
func ensureDefault(w io.Writer, app string, current func(scheme string) string, set func(app, scheme string) error, every, within time.Duration) ([]schemeState, error) {
	// Snapshot first: a scheme that the dialog for an earlier one flips to ours is "set now", not "already set".
	before := map[string]string{}
	for _, s := range handler.Schemes {
		before[s] = current(s)
	}
	var states []schemeState
	for _, s := range handler.Schemes {
		already := before[s] == BundleID
		now := current(s)
		if now != BundleID {
			fmt.Fprintf(w, "asking macOS to make Portero the handler of %s (confirm the dialog if one appears)\n", s)
			setErr := set(app, s)
			now = waitHandler(current, s, every, within)
			if now != BundleID {
				who := fmt.Sprintf("handled by %q", now)
				if now == "" {
					who = "without a handler"
				}
				if setErr != nil {
					return states, fmt.Errorf("%s: still %s: %w", s, who, setErr)
				}
				return states, fmt.Errorf("%s: still %s", s, who)
			}
		}
		states = append(states, schemeState{Scheme: s, Handler: now, Already: already})
	}
	return states, nil
}

// stateNote is the remark `default set` and the UI show for one scheme.
func stateNote(st schemeState) string {
	if st.Already {
		return "already set"
	}
	return "set now"
}

// stateNotes maps each scheme to its remark.
func stateNotes(states []schemeState) map[string]string {
	notes := map[string]string{}
	for _, st := range states {
		notes[st.Scheme] = stateNote(st)
	}
	return notes
}

// waitHandler reads the handler of scheme until it is ours or the deadline passes; it always reads at least once.
func waitHandler(current func(string) string, scheme string, every, within time.Duration) string {
	deadline := time.Now().Add(within)
	for {
		now := current(scheme)
		if now == BundleID || !time.Now().Before(deadline) {
			return now
		}
		time.Sleep(every)
	}
}

// printDefaultStatus prints one line per scheme and the app path; note, when not nil, adds a trailing remark.
func printDefaultStatus(w io.Writer, note func(scheme string) string) {
	for _, s := range handler.Schemes {
		cur := handler.DefaultHandler(s)
		mark := " "
		if cur == BundleID {
			mark = "*"
		}
		line := fmt.Sprintf("%s %-6s %s", mark, s, cur)
		if note != nil {
			if n := note(s); n != "" {
				line += "  " + n
			}
		}
		fmt.Fprintln(w, line)
	}
	if app := appBundle(); app != "" {
		fmt.Fprintln(w, "app:", app)
	} else {
		fmt.Fprintln(w, "app: not running from Portero.app (use `make install`)")
	}
}

// parseRoutable accepts http(s) links with a host and local documents (file:// URLs from Open With).
func parseRoutable(link string) (*url.URL, error) {
	u, err := url.Parse(link)
	if err != nil {
		return nil, fmt.Errorf("not a URL: %q", link)
	}
	if u.Scheme == "file" {
		if u.Path == "" || (u.Host != "" && u.Host != "localhost") {
			return nil, fmt.Errorf("not a local file URL: %q", link)
		}
		return u, nil
	}
	if u.Host == "" {
		return nil, fmt.Errorf("not a URL: %q", link)
	}
	return u, nil
}

// decide applies the rules; a local document has no host to match, so it opens in the default target.
func decide(r *rules.Rules, req rules.Request) rules.Decision {
	if req.URL.Scheme == "file" {
		return rules.Decision{Target: r.Default, Rule: "file"}
	}
	return r.Route(req)
}

func defaultCmd(args []string) error {
	sub := "status"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "status":
		printDefaultStatus(os.Stdout, nil)
	case "set":
		app := appBundle()
		if app == "" {
			return fmt.Errorf("run this from the installed app: ~/Applications/Portero.app/Contents/MacOS/portero default set")
		}
		states, err := ensureDefault(os.Stdout, app, handler.DefaultHandler, handler.SetDefaultHandler, 100*time.Millisecond, 1500*time.Millisecond)
		notes := stateNotes(states)
		printDefaultStatus(os.Stdout, func(s string) string { return notes[s] })
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown default command %q", sub)
	}
	return nil
}
