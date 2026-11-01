package main

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

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
	u, err := url.Parse(link)
	if err != nil || u.Host == "" {
		return fmt.Errorf("not a URL: %q", link)
	}
	req := rules.Request{URL: u, Opener: opener}
	r, err := rules.Load(config.RulesFile())
	if err != nil {
		// A missing or broken rules file must never swallow a link: Safari is always there.
		_ = decisionlog.AppendError(config.LogFile(), link, opener, fmt.Errorf("%w; opened in %s", err, fallback.Browser))
		_, openErr := launch.Open(u, fallback)
		return openErr
	}
	d := r.Route(req)
	if err := decisionlog.Append(config.LogFile(), d, req); err != nil {
		fmt.Fprintln(os.Stderr, "portero: log:", err)
	}
	_, err = launch.Open(u, d.Target)
	return err
}

func defaultCmd(args []string) error {
	sub := "status"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "status":
		for _, s := range handler.Schemes {
			cur := handler.DefaultHandler(s)
			mark := " "
			if cur == BundleID {
				mark = "*"
			}
			fmt.Printf("%s %-6s %s\n", mark, s, cur)
		}
		if app := appBundle(); app != "" {
			fmt.Println("app:", app)
		} else {
			fmt.Println("app: not running from Portero.app (use `make install`)")
		}
	case "set":
		app := appBundle()
		if app == "" {
			return fmt.Errorf("run this from the installed app: ~/Applications/Portero.app/Contents/MacOS/portero default set")
		}
		// macOS sets http and https together from one dialog; asking again for a scheme that is already
		// ours fails with "The file couldn't be opened", so check before each call.
		for _, s := range handler.Schemes {
			if handler.DefaultHandler(s) == BundleID {
				continue
			}
			if err := handler.SetDefaultHandler(app, s); err != nil && handler.DefaultHandler(s) != BundleID {
				return fmt.Errorf("%s: %w", s, err)
			}
		}
		fmt.Println("Portero is the default browser (confirm the macOS dialog if it is still open)")
		return defaultCmd(nil)
	default:
		return fmt.Errorf("unknown default command %q", sub)
	}
	return nil
}
