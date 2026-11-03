package rulesfile

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/plavipas/portero/internal/rules"
)

// SetUnwrap returns text with the top-level `unwrap` and `unwrap_skip_openers` keys set. Everything else,
// comments included, is kept byte for byte: the old keys are cut out line by line and the new ones are put
// right after the `version:` line. `unwrap: true` and an empty list are written as no key at all (the defaults).
// The result is parsed again and must equal the old file in everything but those two settings.
func SetUnwrap(text string, unwrap bool, skip []string) (string, error) {
	old, err := rules.Parse([]byte(text))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	eol := "\n"
	if strings.Contains(text, "\r\n") {
		eol = "\r\n"
	}
	lines := strings.SplitAfter(text, "\n")
	var kept, loose []string // loose: comments of the old keys that stay in the file
	trailing := map[string]string{}
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		key := ""
		switch {
		case isTopKey(l, "unwrap"):
			key = "unwrap"
		case isTopKey(l, "unwrap_skip_openers"):
			key = "unwrap_skip_openers"
		default:
			kept = append(kept, l)
			continue
		}
		if c := trailingComment(strings.TrimRight(l, "\r\n")[len(key)+1:]); c != "" {
			trailing[key] = c
		}
		for i+1 < len(lines) {
			n := lines[i+1]
			if continuesKey(n) {
				if isComment(n) {
					loose = append(loose, strings.TrimSpace(n))
				}
				i++
				continue
			}
			// a column-0 comment inside the list: only when an item follows it
			j := i + 1
			for j < len(lines) && isComment(lines[j]) {
				j++
			}
			if j > i+1 && j < len(lines) && continuesKey(lines[j]) {
				for ; i+1 < j; i++ {
					loose = append(loose, strings.TrimSpace(lines[i+1]))
				}
				continue
			}
			break
		}
	}
	var add strings.Builder
	line := func(key, v string) {
		add.WriteString(key + ":" + v)
		if c, ok := trailing[key]; ok {
			add.WriteString("  " + c)
			delete(trailing, key)
		}
		add.WriteString(eol)
	}
	if !unwrap {
		line("unwrap", " false")
	}
	for _, c := range trailing { // a default key that is no longer written keeps its comment
		loose = append(loose, c)
	}
	var head strings.Builder
	for _, c := range loose {
		head.WriteString(c + eol)
	}
	if len(skip) > 0 {
		line("unwrap_skip_openers", "")
		for _, o := range skip {
			add.WriteString("  - " + strconv.Quote(o) + eol)
		}
	}
	at := -1
	for i, l := range kept {
		if isTopKey(l, "version") {
			at = i + 1
			break
		}
	}
	if at < 0 {
		return "", fmt.Errorf("%w: no top-level version key", ErrInvalid)
	}
	if at > 0 && !strings.HasSuffix(kept[at-1], "\n") {
		kept[at-1] += eol
	}
	out := strings.Join(kept[:at], "") + head.String() + add.String() + strings.Join(kept[at:], "")
	nw, err := rules.Parse([]byte(out))
	if err != nil {
		return "", fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	wantOn := unwrap
	if nw.Unwrap != nil && *nw.Unwrap != wantOn || nw.Unwrap == nil && !wantOn {
		return "", fmt.Errorf("%w: the unwrap key could not be set (edit the file by hand)", ErrInvalid)
	}
	if len(nw.UnwrapSkipOpeners) != len(skip) || (len(skip) > 0 && !reflect.DeepEqual(nw.UnwrapSkipOpeners, skip)) {
		return "", fmt.Errorf("%w: the skipped apps could not be set (edit the file by hand)", ErrInvalid)
	}
	a, b := *old, *nw
	a.Unwrap, b.Unwrap, a.UnwrapSkipOpeners, b.UnwrapSkipOpeners = nil, nil, nil, nil
	if !reflect.DeepEqual(a, b) {
		return "", fmt.Errorf("%w: the edit would change the rules (edit the file by hand)", ErrInvalid)
	}
	return out, nil
}

func isComment(line string) bool { return strings.HasPrefix(strings.TrimSpace(line), "#") }

// trailingComment returns the `# ...` that ends the rest of a key line, "" when there is none.
func trailingComment(rest string) string {
	if i := strings.Index(rest, "#"); i >= 0 && (i == 0 || rest[i-1] == ' ' || rest[i-1] == '\t') && !strings.ContainsAny(rest[:i], "\"'") {
		return strings.TrimSpace(rest[i:])
	}
	return ""
}

func isTopKey(line, key string) bool {
	return strings.HasPrefix(line, key+":")
}

// continuesKey: the line belongs to the previous top-level key (indented, a block-sequence item at column 0, or blank).
func continuesKey(line string) bool {
	if strings.TrimSpace(line) == "" {
		return false // a blank line ends the value and stays
	}
	return line[0] == ' ' || line[0] == '\t' || strings.HasPrefix(line, "- ")
}
