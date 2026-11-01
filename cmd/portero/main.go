// Command portero routes links to the browser and profile a rule picks.
package main

import (
	_ "embed"
	"flag"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/plavipas/portero/internal/browsers"
	"github.com/plavipas/portero/internal/config"
	"github.com/plavipas/portero/internal/decisionlog"
	"github.com/plavipas/portero/internal/launch"
	"github.com/plavipas/portero/internal/rules"
	"github.com/plavipas/portero/internal/sync"
)

const usage = `portero - default-browser router

Usage:
  portero open <url> [--opener <bundle id>] [--cwd <dir>] [--dry-run]
                                                          Route a link; --dry-run prints the decision only
  portero rules [path]                                    Print the rules file in use
  portero rules check                                     Parse and validate the rules file; warns about missing
                                                          browsers, containers and the container extension
  portero rules match <url> [--opener <bundle id>] [--cwd <dir>]
                                                          Show which rule a link would hit
                                                          (cwd = current directory unless --cwd)
  portero rules init                                      Write the example rules file if none exists
  portero browsers                                        Installed browsers Portero knows, with their profiles
  portero default [status|set]                            Show or set Portero as the default browser
  portero sync [status|icloud|off]                        Keep the rules file in iCloud Drive
  portero app                                             Run the link handler (what macOS starts)
  portero version
`

var version = "dev"

//go:embed rules.example.yaml
var exampleRules []byte

func main() {
	if len(os.Args) < 2 {
		if appBundle() != "" { // started by macOS (Launch Services) to handle a link
			runApp()
			return
		}
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "app":
		runApp()
	case "default":
		err = defaultCmd(os.Args[2:])
	case "sync":
		err = syncCmd(os.Args[2:])
	case "open":
		err = open(os.Args[2:])
	case "rules":
		err = rulesCmd(os.Args[2:])
	case "browsers":
		err = browsersCmd()
	case "version":
		fmt.Println("portero", version)
	case "-h", "--help", "help":
		fmt.Print(usage)
	default:
		fmt.Fprintf(os.Stderr, "portero: unknown command %q\n%s", os.Args[1], usage)
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "portero:", err)
		os.Exit(1)
	}
}

// linkArgs is the parsed command line of `open` and `rules match`.
type linkArgs struct {
	url    *url.URL
	opener string
	cwd    string // --cwd value, "" when not given
	dry    bool
}

func parseLink(fs *flag.FlagSet, args []string) (linkArgs, error) {
	opener := fs.String("opener", "", "bundle id of the app that opened the link")
	cwd := fs.String("cwd", "", "directory to match cwd rules against (default: the current directory)")
	dry := fs.Bool("dry-run", false, "print the decision only")
	// Accept flags after the URL as well.
	var rest []string
	for i, a := range args {
		if len(a) > 0 && a[0] == '-' {
			if err := fs.Parse(args[i:]); err != nil {
				return linkArgs{}, err
			}
			break
		}
		rest = append(rest, a)
	}
	if len(rest) != 1 {
		return linkArgs{}, fmt.Errorf("expected one URL")
	}
	u, err := url.Parse(rest[0])
	if err != nil || u.Host == "" {
		return linkArgs{}, fmt.Errorf("not a URL: %q", rest[0])
	}
	return linkArgs{url: u, opener: *opener, cwd: *cwd, dry: *dry}, nil
}

// linkCwd returns the physical directory cwd rules are matched against: the --cwd value when given, else the
// current directory. dir is "" and note says why when it cannot be determined.
func linkCwd(flagValue string) (dir, note string) {
	from := flagValue
	if from == "" {
		wd, err := os.Getwd()
		if err != nil {
			return "", fmt.Sprintf("unknown (%v)", err)
		}
		from = wd
	}
	dir, err := rules.ResolveDir(from)
	if err != nil {
		return "", fmt.Sprintf("unknown (%v)", err)
	}
	return dir, ""
}

// cwdLine is the "cwd:" line printed before the decision, or "" when the output stays as it was.
func cwdLine(r *rules.Rules, a linkArgs, dir, note string) string {
	if !r.UsesCwd() && a.cwd == "" {
		return ""
	}
	if note != "" {
		return "cwd: " + note + "; cwd rules skipped"
	}
	return "cwd: " + dir
}

func open(args []string) error {
	a, err := parseLink(flag.NewFlagSet("open", flag.ContinueOnError), args)
	if err != nil {
		return err
	}
	r, err := rules.Load(config.RulesFile())
	if err != nil {
		return err
	}
	cwd, note := linkCwd(a.cwd)
	if a.dry {
		if line := cwdLine(r, a, cwd, note); line != "" {
			fmt.Println(line)
		}
	} else if note != "" {
		fmt.Fprintf(os.Stderr, "portero: cwd: %s; cwd rules skipped\n", note)
	}
	u := a.url
	req := rules.Request{URL: u, Opener: a.opener, Cwd: cwd}
	d := r.Route(req)
	fmt.Println(d)
	if a.dry {
		plan, err := launch.DryRun(u, d.Target)
		if err != nil {
			return err
		}
		fmt.Println("would run:", plan)
		return nil
	}
	if err := decisionlog.Append(config.LogFile(), d, req); err != nil {
		fmt.Fprintln(os.Stderr, "portero: log:", err)
	}
	_, err = launch.Open(u, d.Target)
	return err
}

func browsersCmd() error {
	for _, b := range browsers.Known {
		if !b.Installed() {
			continue
		}
		line := fmt.Sprintf("%-28s %-9s %s", b.Name, b.Kind, b.BundleID)
		if profiles, err := b.Profiles(); err == nil && len(profiles) > 0 {
			line += "\n" + "    profiles: " + browsers.Names(profiles)
		}
		for _, l := range containerLines(b) {
			line += "\n" + l
		}
		fmt.Println(line)
	}
	return nil
}

func rulesCmd(args []string) error {
	sub := "path"
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	switch sub {
	case "path":
		fmt.Println(config.RulesFile())
	case "init":
		p := config.RulesFile()
		if _, err := os.Stat(p); err == nil {
			fmt.Println("exists:", p)
			return nil
		}
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, exampleRules, 0o600); err != nil {
			return err
		}
		fmt.Println("written:", p)
	case "check":
		p := config.RulesFile()
		r, err := rules.Load(p)
		if err != nil {
			return err
		}
		warnings, err := checkTargets(r, realCheckEnv())
		if err != nil {
			return err
		}
		warnings = append(warnings, checkCwd(r, realCheckEnv())...)
		for _, w := range warnings {
			fmt.Println(w)
		}
		fmt.Printf("%s: %d rule(s), default %s\n", p, len(r.Rules), r.Default)
	case "match":
		a, err := parseLink(flag.NewFlagSet("match", flag.ContinueOnError), args)
		if err != nil {
			return err
		}
		r, err := rules.Load(config.RulesFile())
		if err != nil {
			return err
		}
		cwd, note := linkCwd(a.cwd)
		if line := cwdLine(r, a, cwd, note); line != "" {
			fmt.Println(line)
		}
		fmt.Println(r.Route(rules.Request{URL: a.url, Opener: a.opener, Cwd: cwd}))
	default:
		return fmt.Errorf("unknown rules command %q", sub)
	}
	return nil
}

func syncCmd(args []string) error {
	sub := "status"
	if len(args) > 0 {
		sub = args[0]
	}
	switch sub {
	case "status":
		p := config.RulesFile()
		where := "local"
		if strings.HasPrefix(p, sync.ICloudDir()) {
			where = "iCloud Drive"
		}
		fmt.Printf("rules: %s (%s)\n", p, where)
	case "icloud":
		p, err := sync.EnableICloud()
		if err != nil {
			return err
		}
		fmt.Println("rules now in iCloud Drive:", p)
	case "off":
		p, err := sync.Disable()
		if err != nil {
			return err
		}
		fmt.Println("rules back to the local file:", p)
	default:
		return fmt.Errorf("unknown sync command %q", sub)
	}
	return nil
}
