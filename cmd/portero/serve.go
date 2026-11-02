package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/signal"
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

	rulesNow := func() (string, string) {
		p := config.RulesFile()
		if strings.HasPrefix(p, sync.ICloudDir()) {
			return p, "icloud"
		}
		return p, "local"
	}
	s := serve.New(serve.Options{
		Rules: rulesNow, LogFile: config.LogFile(),
		Env: check.RealEnv(), Browsers: browserList, Plan: servePlan(),
	})
	s.SetAddr(ln.Addr().String())
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 10 * time.Second}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() { done <- srv.Serve(ln) }()

	link := "http://" + ln.Addr().String() + "/"
	fmt.Printf("Portero UI: %s  (Ctrl-C stops it)\n", link)
	if !*noOpen {
		if err := openUI(link); err != nil {
			fmt.Fprintln(os.Stderr, "portero: could not open the UI:", err)
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

// openUI opens the UI address like `portero open` would: by the user's rules, with the shell's working
// directory. A rules file that cannot be read sends it to the fallback browser and logs an ERROR line.
func openUI(link string) error {
	u, err := url.Parse(link)
	if err != nil {
		return err
	}
	cwd, _ := linkCwd("")
	req := rules.Request{URL: u, Cwd: cwd}
	r, err := rules.Load(config.RulesFile())
	if err != nil {
		_ = decisionlog.AppendError(config.LogFile(), link, "", fmt.Errorf("%w; opened in %s", err, fallback.Browser))
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
