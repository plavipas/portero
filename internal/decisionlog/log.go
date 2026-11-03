// Package decisionlog appends one line per routed link.
package decisionlog

import (
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/plavipas/portero/internal/rules"
)

// Append writes "time opener rule target url" (tab-separated) to path, creating it with mode 0600.
func Append(path string, d rules.Decision, req rules.Request) error {
	return AppendNote(path, d, req, "")
}

// AppendNote is Append with an optional sixth column that says how an asking rule was settled:
// `asked -> Dia/Work`, `asked -> Dia/Work, remembered "<name>"`, `asked -> default (cancelled)`. d.Target is
// the target the link was opened in. An empty note writes the usual five columns.
func AppendNote(path string, d rules.Decision, req rules.Request, note string) error {
	return AppendLink(path, d, req, req.URL, note)
}

// maxUnwrappedLog caps the inner URL written to the log.
const maxUnwrappedLog = 2048

// AppendLink is AppendNote for a link that was unwrapped: req.URL is the inner URL the rules decided on, link is
// the original link (column 5, what the browser was given). A req.URL other than link adds
// `unwrapped <inner URL>` as the last part of the note, after the ask note, cut at 2048 characters.
func AppendLink(path string, d rules.Decision, req rules.Request, link *url.URL, note string) error {
	inner := ""
	if req.URL != nil && link != nil && req.URL != link {
		inner = req.URL.String()
	}
	note = cleanNote(note)
	if inner != "" {
		inner = cleanNote(inner)
		if r := []rune(inner); len(r) > maxUnwrappedLog {
			inner = string(r[:maxUnwrappedLog])
		}
		if note != "" {
			note += "; "
		}
		note += "unwrapped " + inner
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	opener, rule := req.Opener, d.Rule
	if opener == "" {
		opener = "-"
	}
	if rule == "" {
		rule = "default"
	}
	line := fmt.Sprintf("%s\t%s\t%s\t%s\t%s", time.Now().UTC().Format(time.RFC3339), opener, rule, d.Target, link)
	if note != "" {
		line += "\t" + note
	}
	_, err = fmt.Fprintln(f, line)
	return err
}

// cleanNote keeps the note on one column of one line.
func cleanNote(s string) string {
	return strings.TrimSpace(strings.Map(func(r rune) rune {
		if r == '\t' || r == '\n' || r == '\r' {
			return ' '
		}
		return r
	}, s))
}

// AppendError records a link that could not be routed or opened.
func AppendError(path, link, opener string, cause error) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if opener == "" {
		opener = "-"
	}
	// %q: a link that failed to parse may carry tabs or newlines and must not forge log lines.
	_, err = fmt.Fprintf(f, "%s\t%s\tERROR\t%q\t%q\n", time.Now().UTC().Format(time.RFC3339), opener, cause.Error(), link)
	return err
}
