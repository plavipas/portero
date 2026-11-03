package main

import (
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode"

	"github.com/plavipas/portero/internal/config"
	"github.com/plavipas/portero/internal/decisionlog"
	"github.com/plavipas/portero/internal/handler"
	"github.com/plavipas/portero/internal/launch"
	"github.com/plavipas/portero/internal/rules"
	"github.com/plavipas/portero/internal/rulesfile"
	"github.com/plavipas/portero/internal/sync"
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

// startDownload asks iCloud to fetch an offloaded rules file; the macOS implementation in the app, a fake in tests.
var startDownload = handler.StartDownload

// offloadedFallback handles a link while the rules file is offloaded by iCloud: reading it would block, so the
// link opens in the fallback browser at once, the event is logged and the download is started so the next link
// finds the file. launchURL is what the browser is started with (it may differ from link). It returns the
// error of the browser launch.
func offloadedFallback(link string, launchURL *url.URL, opener, rulesPath string, open func(*url.URL, rules.Target) (launch.Plan, error)) error {
	_, openErr := open(launchURL, fallback)
	cause := fmt.Errorf("rules file offloaded by iCloud, download started; opened in %s", fallback.Browser)
	if err := startDownload(rulesPath); err != nil {
		cause = fmt.Errorf("rules file offloaded by iCloud, download not started (%v); opened in %s", err, fallback.Browser)
	}
	_ = decisionlog.AppendError(config.LogFile(), link, opener, cause)
	return openErr
}

func routeLink(link, opener string) error {
	return routeLinkAsk(link, opener, launch.Open, handler.Ask, rememberRule)
}

// routeLinkWith routes a link without ever asking: a rule that asks opens the default target.
func routeLinkWith(link, opener string, open func(*url.URL, rules.Target) (launch.Plan, error)) error {
	return routeLinkAsk(link, opener, open, nil, nil)
}

// askFunc shows the panel for an asking rule (handler.Ask in the app, a fake in tests); nil means the caller
// cannot ask.
type askFunc func(handler.AskRequest) handler.AskResult

// rememberFunc writes the "Remember" rule: link req, decided by the asking rule label, goes to chosen. It
// returns the name of the new rule.
type rememberFunc func(req rules.Request, label string, chosen rules.Target) (string, error)

// rememberRule is the real rememberFunc: the locked write of internal/rulesfile.
func rememberRule(req rules.Request, label string, chosen rules.Target) (string, error) {
	return rulesfile.Remember(config.RulesFile(), config.Dir(), req, label, chosen, sync.Offloaded, time.Now())
}

// askOption is the button label of a target: "Browser · Profile · Container".
func askOption(t rules.Target) string {
	parts := []string{t.Browser}
	if t.Profile != "" {
		parts = append(parts, t.Profile)
	}
	if t.Container != "" {
		parts = append(parts, t.Container)
	}
	return strings.Join(parts, " · ")
}

// askTitle is what the panel shows about a link: host and path, one line, no control, format or line-separator characters.
func askTitle(u *url.URL) string {
	s := strings.Map(func(r rune) rune {
		if r < 0x20 || r == 0x7f || unicode.Is(unicode.Cf, r) || r == '\u2028' || r == '\u2029' {
			return ' '
		}
		return r
	}, u.Host+u.Path)
	if r := []rune(s); len(r) > 300 {
		s = string(r[:300])
	}
	return s
}

func routeLinkAsk(link, opener string, open func(*url.URL, rules.Target) (launch.Plan, error), ask askFunc, remember rememberFunc) error {
	u, err := parseRoutable(link)
	if err != nil {
		return err
	}
	if p := config.RulesFile(); sync.Offloaded(p) {
		return offloadedFallback(link, u, opener, p, open)
	}
	// The handler has no meaningful cwd (Launch Services starts the app in /), so cwd rules never match here.
	req := rules.Request{URL: u, Opener: opener}
	r, err := rules.Load(config.RulesFile())
	if err != nil {
		// A missing or broken rules file must never swallow a link: Safari is always there.
		_ = decisionlog.AppendError(config.LogFile(), link, opener, fmt.Errorf("%w; opened in %s", err, fallback.Browser))
		_, openErr := open(u, fallback)
		return openErr
	}
	d := decide(r, req)
	if len(d.Ask) == 0 {
		if err := decisionlog.Append(config.LogFile(), d, req); err != nil {
			fmt.Fprintln(os.Stderr, "portero: log:", err)
		}
		_, err = open(u, d.Target)
		return err
	}
	return routeAsked(d, req, open, ask, remember)
}

// routeAsked settles a link whose rule asks: the panel (or, without one, the default target), then opens the
// target, then "Remember". The link opens before anything is written, and a write error never delays it.
func routeAsked(d rules.Decision, req rules.Request, open func(*url.URL, rules.Target) (launch.Plan, error), ask askFunc, remember rememberFunc) error {
	u := req.URL
	target, note := d.Target, "asked -> default (CLI)"
	remembered := false
	if ask != nil {
		opts := make([]string, len(d.Ask))
		for i, t := range d.Ask {
			opts[i] = askOption(t)
		}
		_, _, canRemember := rules.RememberKey(u)
		res := ask(handler.AskRequest{ID: req.Opener + "\x00" + u.String(), Title: askTitle(u), Options: opts, CanRemember: canRemember && remember != nil})
		switch {
		case res.Joined:
			// An identical link is waiting in a panel and that one opens it. If its open later fails, the error is
			// logged for the first link only; this duplicate adds no second tab by design.
			return nil
		case res.Choice >= 0 && res.Choice < len(d.Ask):
			target = d.Ask[res.Choice]
			note = "asked -> " + target.String()
			remembered = res.Remember && canRemember && remember != nil
		default:
			note = "asked -> default (cancelled)"
		}
	}
	_, err := open(u, target)
	if err == nil && remembered {
		if name, rerr := remember(req, d.Rule, target); rerr != nil {
			note += ", remember failed: " + rerr.Error()
		} else {
			note += fmt.Sprintf(", remembered %q", name)
		}
	}
	d.Target, d.Ask = target, nil
	if lerr := decisionlog.AppendNote(config.LogFile(), d, req, note); lerr != nil {
		fmt.Fprintln(os.Stderr, "portero: log:", lerr)
	}
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
		if !printResolution(os.Stdout) {
			return exitCode(2)
		}
	case "set":
		app := appBundle()
		if app == "" {
			return fmt.Errorf("run this from the installed app: ~/Applications/Portero.app/Contents/MacOS/portero default set")
		}
		states, err := ensureDefault(os.Stdout, app, handler.DefaultHandler, handler.SetDefaultHandler, 100*time.Millisecond, 1500*time.Millisecond)
		notes := stateNotes(states)
		printDefaultStatus(os.Stdout, func(s string) string { return notes[s] })
		printResolution(os.Stdout)
		if err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown default command %q", sub)
	}
	return nil
}

// exitCode is an error that makes main exit with that code without printing a message.
type exitCode int

func (e exitCode) Error() string { return fmt.Sprintf("exit status %d", int(e)) }

// installedApp is the canonical install location, whichever copy is running: a stale copy elsewhere that
// Launch Services picked must show up as a mismatch, not as "healthy".
func installedApp() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, "Applications", "Portero.app")
	}
	return ""
}

// printResolution prints which app macOS really uses for links and reports whether that looks healthy.
func printResolution(w io.Writer) bool {
	cur := map[string]string{}
	for _, s := range handler.Schemes {
		cur[s] = handler.DefaultHandler(s)
	}
	resolvedBy := map[string]string{}
	for _, s := range handler.Schemes {
		resolvedBy[s] = handler.DefaultApp(s)
	}
	resolved := resolvedBy[handler.Schemes[0]]
	var ver string
	if resolved != "" {
		short, build := handler.BundleVersion(resolved)
		if ver = short; ver == "" {
			ver = build
		}
	}
	lines, ok := defaultReport(cur, resolved, ver, installedApp(), version)
	if w := schemeMismatch(resolvedBy); w != "" {
		lines, ok = append(lines, w), false
	}
	for _, l := range lines {
		fmt.Fprintln(w, l)
	}
	return ok
}

// defaultReport compares the app Launch Services resolves for links with the installed one and with this binary.
// ok is false when no app resolves, or (Portero being the default) the resolved app is not the installed one
// or has another version than this binary. When Portero is not the default, only the resolved app is shown.
func defaultReport(cur map[string]string, resolved, resolvedVer, installed, running string) (lines []string, ok bool) {
	if resolved == "" {
		return []string{"resolved: none (macOS has no app for links)"}, false
	}
	if resolvedVer != "" {
		lines = append(lines, fmt.Sprintf("resolved: %s (%s)", resolved, resolvedVer))
	} else {
		lines = append(lines, "resolved: "+resolved)
	}
	for _, s := range handler.Schemes {
		if cur[s] != BundleID {
			return lines, true
		}
	}
	ok = true
	if installed != "" && !samePath(resolved, installed) {
		lines = append(lines, fmt.Sprintf("warning: macOS opens links with %s, not %s; run make install", resolved, installed))
		ok = false
	}
	if running == "dev" {
		return append(lines, "note: dev binary, version not compared"), ok
	}
	if resolvedVer != "" && resolvedVer != running {
		lines = append(lines, fmt.Sprintf("warning: resolved app is %s, this binary is %s", resolvedVer, running))
		ok = false
	}
	return lines, ok
}

// schemeMismatch is a warning when the schemes resolve to different apps (an http link and an https link would
// open different copies), else "".
func schemeMismatch(resolvedBy map[string]string) string {
	first := handler.Schemes[0]
	for _, s := range handler.Schemes[1:] {
		if !samePath(resolvedBy[s], resolvedBy[first]) {
			return fmt.Sprintf("warning: %s links open %s, %s links open %s; run make install", first, resolvedBy[first], s, resolvedBy[s])
		}
	}
	return ""
}

// samePath reports whether a and b name the same file: the same file on disk when both exist (symlinked HOME,
// another letter case on a case-insensitive volume), else the same cleaned path.
func samePath(a, b string) bool {
	if a == "" || b == "" {
		return a == b
	}
	if fa, err := os.Stat(a); err == nil {
		if fb, err := os.Stat(b); err == nil {
			return os.SameFile(fa, fb)
		}
	}
	return filepath.Clean(a) == filepath.Clean(b)
}
