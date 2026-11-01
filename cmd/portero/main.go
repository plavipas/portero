// Command portero routes links to the browser and profile a rule picks.
package main

import (
	"flag"
	"fmt"
	"net/url"
	"os"

	"github.com/plavipas/portero/internal/config"
	"github.com/plavipas/portero/internal/decisionlog"
	"github.com/plavipas/portero/internal/launch"
	"github.com/plavipas/portero/internal/rules"
)

const usage = `portero - default-browser router

Usage:
  portero open <url> [--opener <bundle id>] [--dry-run]   Route a link; --dry-run prints the decision only
  portero rules [path]                                    Print the rules file in use
  portero rules check                                     Parse and validate the rules file
  portero rules match <url> [--opener <bundle id>]        Show which rule a link would hit
  portero version
`

var version = "dev"

func main() {
	if len(os.Args) < 2 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "open":
		err = open(os.Args[2:])
	case "rules":
		err = rulesCmd(os.Args[2:])
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

func parseLink(fs *flag.FlagSet, args []string) (*url.URL, string, bool, error) {
	opener := fs.String("opener", "", "bundle id of the app that opened the link")
	dry := fs.Bool("dry-run", false, "print the decision only")
	// Accept flags after the URL as well.
	var rest []string
	for i, a := range args {
		if len(a) > 0 && a[0] == '-' {
			if err := fs.Parse(args[i:]); err != nil {
				return nil, "", false, err
			}
			break
		}
		rest = append(rest, a)
	}
	if len(rest) != 1 {
		return nil, "", false, fmt.Errorf("expected one URL")
	}
	u, err := url.Parse(rest[0])
	if err != nil || u.Host == "" {
		return nil, "", false, fmt.Errorf("not a URL: %q", rest[0])
	}
	return u, *opener, *dry, nil
}

func open(args []string) error {
	u, opener, dry, err := parseLink(flag.NewFlagSet("open", flag.ContinueOnError), args)
	if err != nil {
		return err
	}
	r, err := rules.Load(config.RulesFile())
	if err != nil {
		return err
	}
	req := rules.Request{URL: u, Opener: opener}
	d := r.Route(req)
	fmt.Println(d)
	if dry {
		return nil
	}
	if err := decisionlog.Append(config.LogFile(), d, req); err != nil {
		fmt.Fprintln(os.Stderr, "portero: log:", err)
	}
	return launch.Open(u, d.Target)
}

func rulesCmd(args []string) error {
	sub := "path"
	if len(args) > 0 {
		sub, args = args[0], args[1:]
	}
	switch sub {
	case "path":
		fmt.Println(config.RulesFile())
	case "check":
		p := config.RulesFile()
		r, err := rules.Load(p)
		if err != nil {
			return err
		}
		fmt.Printf("%s: %d rule(s), default %s\n", p, len(r.Rules), r.Default)
	case "match":
		u, opener, _, err := parseLink(flag.NewFlagSet("match", flag.ContinueOnError), args)
		if err != nil {
			return err
		}
		r, err := rules.Load(config.RulesFile())
		if err != nil {
			return err
		}
		fmt.Println(r.Route(rules.Request{URL: u, Opener: opener}))
	default:
		return fmt.Errorf("unknown rules command %q", sub)
	}
	return nil
}
