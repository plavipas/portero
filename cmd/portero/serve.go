package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/plavipas/portero/internal/browsers"
	"github.com/plavipas/portero/internal/check"
	"github.com/plavipas/portero/internal/config"
	"github.com/plavipas/portero/internal/decisionlog"
	"github.com/plavipas/portero/internal/launch"
	"github.com/plavipas/portero/internal/rules"
	"github.com/plavipas/portero/internal/serve"
	"github.com/plavipas/portero/internal/sync"
)

// listenAddr turns the flags of `serve` into the address to listen on. Only loopback is allowed.
func listenAddr(port int, listen string, portSet, listenSet bool) (string, error) {
	switch {
	case portSet && listenSet:
		return "", fmt.Errorf("use either --port or --listen, not both")
	case portSet:
		if port < 0 || port > 65535 {
			return "", fmt.Errorf("--port %d is not a port number", port)
		}
		return net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), nil
	case listenSet:
		host, p, err := net.SplitHostPort(listen)
		if err != nil {
			return "", fmt.Errorf("--listen %q: %w (expected 127.0.0.1:<port>)", listen, err)
		}
		if n, err := strconv.Atoi(p); err != nil || n < 0 || n > 65535 {
			return "", fmt.Errorf("--listen %q: not a port number", listen)
		}
		if host == "localhost" {
			host = "127.0.0.1"
		}
		if ip := net.ParseIP(host); ip == nil || !ip.IsLoopback() {
			return "", fmt.Errorf("--listen %q: the UI edits your rules and only listens on a loopback address", listen)
		}
		return net.JoinHostPort(host, p), nil
	}
	return "127.0.0.1:0", nil
}

func browserList() []serve.BrowserInfo {
	var out []serve.BrowserInfo
	for _, b := range browsers.Known {
		info := serve.BrowserInfo{Name: b.Name, Kind: string(b.Kind), BundleID: b.BundleID, Installed: b.Installed()}
		if info.Installed {
			if profiles, err := b.Profiles(); err == nil {
				for _, p := range profiles {
					info.Profiles = append(info.Profiles, p.Name)
					if list, err := b.Containers(p); err == nil && len(list) > 0 {
						if info.Containers == nil {
							info.Containers = map[string][]string{}
						}
						for _, c := range list {
							info.Containers[p.Name] = append(info.Containers[p.Name], c.Name)
						}
					}
				}
			}
		}
		out = append(out, info)
	}
	return out
}

func serveCmd(args []string) error {
	fs := flag.NewFlagSet("serve", flag.ContinueOnError)
	port := fs.Int("port", 0, "port on 127.0.0.1 (default: a free one)")
	listen := fs.String("listen", "", "loopback address to listen on, e.g. 127.0.0.1:8765")
	noOpen := fs.Bool("no-open", false, "only print the URL")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return fmt.Errorf("unexpected argument %q", fs.Arg(0))
	}
	set := map[string]bool{}
	fs.Visit(func(f *flag.Flag) { set[f.Name] = true })
	addr, err := listenAddr(*port, *listen, set["port"], set["listen"])
	if err != nil {
		return err
	}
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("serve: %w", err)
	}
	defer ln.Close()
	if tcp, ok := ln.Addr().(*net.TCPAddr); !ok || !tcp.IP.IsLoopback() {
		return fmt.Errorf("serve: refusing to listen on %s", ln.Addr())
	}

	defStatus, setDef := serveDefault()
	session := "" // empty: serve.New makes a per-run secret
	if f := sessionFile(); f != "" {
		var err error
		if session, err = sessionSecret(f); err != nil {
			return err
		}
	}
	s := serve.New(serve.Options{
		Rules: rulesNow, LogFile: config.LogFile(), Session: session,
		Env: check.RealEnv(), Browsers: browserList, Plan: servePlan(), ConfigFile: config.File(),
		DefaultStatus: defStatus, SetDefault: setDef, Sync: serveSync(),
	})
	s.SetAddr(ln.Addr().String())
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()

	// The nonce in the link works once (see internal/serve/guard.go): it is printed and handed to the browser,
	// never logged. The session secret behind the cookie is never printed.
	link, authLink := uiLinks(ln.Addr().String(), s.Nonce())
	fmt.Print(announce(authLink))
	if !*noOpen {
		if err := openUI(link, authLink, launch.Open); err != nil {
			fmt.Fprintln(os.Stderr, "portero: could not open the UI:", strings.ReplaceAll(err.Error(), s.Nonce(), "<nonce>"))
		}
	}
	select {
	case err := <-done:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve: %w", err)
		}
	case <-ctx.Done():
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	return srv.Shutdown(shutdown)
}

// openUI routes the UI address like `portero open` would: by the user's rules, with the shell's working
// directory. link is the bare address (what rules match and the log records); authLink, which carries the
// token, is only what the browser is started with. A rules file that cannot be read sends it to the fallback
// browser and logs an ERROR line.
func openUI(link, authLink string, open func(*url.URL, rules.Target) (launch.Plan, error)) error {
	u, err := url.Parse(link)
	if err != nil {
		return err
	}
	launchURL, err := url.Parse(authLink)
	if err != nil {
		return err
	}
	cwd, _ := linkCwd("")
	req := rules.Request{URL: u, Cwd: cwd}
	r, err := rules.Load(config.RulesFile())
	if err != nil {
		_ = decisionlog.AppendError(config.LogFile(), link, "", fmt.Errorf("%w; opened in %s", err, fallback.Browser))
		_, openErr := open(launchURL, fallback)
		return openErr
	}
	d := r.Route(req)
	if err := decisionlog.Append(config.LogFile(), d, req); err != nil {
		fmt.Fprintln(os.Stderr, "portero: log:", err)
	}
	_, err = open(launchURL, d.Target)
	return err
}

// uiLinks returns the bare address and the one-time link that carries the nonce. The session secret behind the
// cookie is not known here and never appears in a URL, a log line or argv.
func uiLinks(addr, nonce string) (bare, auth string) {
	bare = "http://" + addr + "/"
	return bare, bare + "auth/" + nonce
}

// sessionSecret returns the per-user secret behind the UI session cookie: 32 random bytes, hex, in a 0600 file
// under the config directory. Keeping it across runs lets a page opened earlier survive a server restart on the
// same port with a plain reload; the one-time nonce in the link stays per run. The secret is reused only when
// the file is a plain file (no symlink), readable by nobody else, well-formed and younger than sessionTTL;
// otherwise a fresh one replaces it. Deleting the file resets every session.
const sessionTTL = 7 * 24 * time.Hour

// sessionSecret holds an exclusive lock on a sibling lock file while it checks and, when needed, replaces the
// secret, so two portero serve processes starting at once always end up with the same secret.
func sessionSecret(path string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil { // an existing directory keeps its mode
		return "", err
	}
	lock, err := os.OpenFile(path+".lock", os.O_RDWR|os.O_CREATE, 0o600)
	if err != nil {
		return "", err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return "", err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)
	if v, ok := validSession(path); ok {
		return v, nil
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", err
	}
	v := hex.EncodeToString(raw)
	// A complete private temp file renamed over the name: no reader sees half a secret, and rename replaces a
	// symlink or a bad file instead of following or keeping it.
	tmp, err := os.CreateTemp(filepath.Dir(path), ".ui-session-*") // 0600
	if err != nil {
		return "", err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.WriteString(v + "\n"); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		return "", err
	}
	return v, nil
}

// validSession returns the secret in path when the file is a plain file (no symlink), readable by nobody
// else, 64 hex characters and younger than sessionTTL.
func validSession(path string) (string, bool) {
	fi, err := os.Lstat(path)
	if err != nil || !fi.Mode().IsRegular() || fi.Mode().Perm()&0o077 != 0 {
		return "", false
	}
	if age := time.Since(fi.ModTime()); age < -time.Minute || age >= sessionTTL { // future mtime counts as invalid
		return "", false
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	v := strings.TrimSpace(string(b))
	if _, err := hex.DecodeString(v); err != nil || len(v) != 64 {
		return "", false
	}
	return v, true
}

// sessionFile is where the session secret lives: the user's cache directory, which no dotfiles repo or
// folder sync is expected to carry (the config directory often is). Losing it only means a new link.
func sessionFile() string {
	dir, err := os.UserCacheDir()
	if err != nil || dir == "" {
		return "" // no cache directory: a per-run secret, never one under the config directory
	}
	return filepath.Join(dir, "portero", "ui-session")
}

// announce is the one line serve prints: the single-use link. It takes the link only, so the session secret
// cannot end up in it (TestServeAnnouncesLinkOnly).
func announce(authLink string) string { return "Portero UI: " + authLink + "  (Ctrl-C stops it)\n" }

// rulesNow is the active rules file and where it lives, asked on every request.
func rulesNow() (string, string) {
	p := config.RulesFile()
	if strings.HasPrefix(p, sync.ICloudDir()) {
		return p, "icloud"
	}
	return p, "local"
}

// serveSync wires the settings page to `portero sync`. The rules file is pinned when PORTERO_RULES names it.
func serveSync() *serve.SyncFuncs {
	return &serve.SyncFuncs{
		Enable: sync.EnableICloud, Disable: sync.Disable, ICloudDir: sync.ICloudDir,
		ICloudAvailable: func() bool { _, err := os.Stat(filepath.Dir(sync.ICloudDir())); return err == nil },
		ICloudExists:    func() bool { _, err := os.Stat(filepath.Join(sync.ICloudDir(), "rules.yaml")); return err == nil },
		LocalPath:       func() string { return filepath.Join(config.Dir(), "rules.yaml") },
		Pinned:          func() bool { return os.Getenv("PORTERO_RULES") != "" },
	}
}
