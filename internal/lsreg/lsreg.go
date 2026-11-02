// Package lsreg reads the output of `lsregister -dump`. It is portable; it only stats paths to recognize the kept app.
package lsreg

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

var hexSuffix = regexp.MustCompile(`(?i) \(0x[0-9a-f]+(-0x[0-9a-f]+)?\)$`) // "(0x405c)" or "(0x0-0x1a2b3c)"

// Others returns the app paths registered under bundle id id, except keep. Records are separated by lines of
// dashes; only the first "path:" and "identifier:" of a record count, so "all roles:" lines of other records
// never match. Paths are returned in dump order without duplicates.
func Others(dump io.Reader, id, keep string) ([]string, error) {
	keep = filepath.Clean(keep)
	var out []string
	seen := map[string]bool{}
	var path, ident string
	flush := func() {
		defer func() { path, ident = "", "" }()
		if ident != id || !strings.HasSuffix(path, ".app") || !filepath.IsAbs(path) {
			return
		}
		p := filepath.Clean(path)
		if p == keep || sameFile(p, keep) || seen[p] {
			return
		}
		seen[p] = true
		out = append(out, p)
	}
	sc := bufio.NewScanner(dump)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if isSeparator(line) {
			flush()
			continue
		}
		if v, ok := strings.CutPrefix(line, "path:"); ok && path == "" {
			path = strings.TrimSpace(hexSuffix.ReplaceAllString(strings.TrimSpace(v), ""))
		} else if v, ok := strings.CutPrefix(line, "identifier:"); ok && ident == "" {
			ident = strings.TrimSpace(v)
		}
	}
	flush()
	if err := sc.Err(); err != nil {
		return nil, err // a truncated dump must not look like "no other copies"
	}
	return out, nil
}

func isSeparator(line string) bool {
	return len(line) >= 20 && strings.Trim(line, "-") == ""
}

// sameFile reports whether a and b are the same existing file (symlinked HOME, another letter case).
func sameFile(a, b string) bool {
	fa, err := os.Stat(a)
	if err != nil {
		return false
	}
	fb, err := os.Stat(b)
	return err == nil && os.SameFile(fa, fb)
}
