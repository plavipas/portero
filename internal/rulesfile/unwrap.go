package rulesfile

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/plavipas/portero/internal/rules"
)

// keyOut collects the lines of the new top-level keys; a trailing comment of an old key moves to the line of the
// same key when it is written again.
type keyOut struct {
	b        strings.Builder
	eol      string
	trailing map[string]string
}

// line writes `key:v`, with the old trailing comment of key if it had one.
func (k *keyOut) line(key, v string) {
	k.b.WriteString(key + ":" + v)
	if c, ok := k.trailing[key]; ok {
		k.b.WriteString("  " + c)
		delete(k.trailing, key)
	}
	k.b.WriteString(k.eol)
}

// item writes one block-list item of the key written last.
func (k *keyOut) item(v string) { k.b.WriteString("  - " + v + k.eol) }

// setTopKeys cuts the given top-level keys (with their values and the comments inside them) out of text and
// writes what render produces where the first of them was, or right after the `version:` line when none was
// there, so a second save changes nothing. Everything else, comments included, is kept byte for byte: comments
// of removed keys stay as lines of their own, in the order of keys. The result is parsed again; it is returned
// with the parse of the old text for the caller's comparison.
func setTopKeys(text string, keys []string, render func(*keyOut)) (out string, old, nw *rules.Rules, err error) {
	old, err = rules.Parse([]byte(text))
	if err != nil {
		return "", nil, nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	eol := "\n"
	if strings.Contains(text, "\r\n") {
		eol = "\r\n"
	}
	lines := strings.SplitAfter(text, "\n")
	firstRemoved := -1       // the new keys go where the old ones were, so a second save changes nothing
	var kept, loose []string // loose: comments of the old keys that stay in the file
	trailing := map[string]string{}
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		key := ""
		for _, k := range keys {
			if isTopKey(l, k) {
				key = k
				break
			}
		}
		if key == "" {
			kept = append(kept, l)
			continue
		}
		if firstRemoved < 0 {
			firstRemoved = len(kept)
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
	ko := &keyOut{eol: eol, trailing: trailing}
	render(ko)
	// the comment of a key that is no longer written stays as a line of its own, in a fixed order
	for _, key := range keys {
		if c, ok := trailing[key]; ok {
			loose = append(loose, c)
		}
	}
	var head strings.Builder
	for _, c := range loose {
		head.WriteString(c + eol)
	}
	at := -1
	for i, l := range kept {
		if isTopKey(l, "version") {
			at = i + 1
			break
		}
	}
	if at < 0 {
		return "", nil, nil, fmt.Errorf("%w: no top-level version key", ErrInvalid)
	}
	if firstRemoved >= 0 {
		at = firstRemoved
	}
	if at > 0 && !strings.HasSuffix(kept[at-1], "\n") {
		kept[at-1] += eol
	}
	out = strings.Join(kept[:at], "") + head.String() + ko.b.String() + strings.Join(kept[at:], "")
	nw, err = rules.Parse([]byte(out))
	if err != nil {
		return "", nil, nil, fmt.Errorf("%w: %v", ErrInvalid, err)
	}
	return out, old, nw, nil
}

// SetUnwrap returns text with the top-level `unwrap` and `unwrap_skip_openers` keys set. Everything else,
// comments included, is kept byte for byte (see setTopKeys). `unwrap: true` and an empty list are written as no
// key at all (the defaults). The result is parsed again and must equal the old file in everything but those two
// settings.
func SetUnwrap(text string, unwrap bool, skip []string) (string, error) {
	out, old, nw, err := setTopKeys(text, []string{"unwrap", "unwrap_skip_openers"}, func(k *keyOut) {
		if !unwrap {
			k.line("unwrap", " false")
		}
		if len(skip) > 0 {
			k.line("unwrap_skip_openers", "")
			for _, o := range skip {
				k.item(strconv.Quote(o))
			}
		}
	})
	if err != nil {
		return "", err
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

// SetAskUnmatched returns text with the top-level `ask_unmatched` key set: on is no key (the default), off is
// `ask_unmatched: false`. The rest of the file is kept as SetUnwrap keeps it.
func SetAskUnmatched(text string, on bool) (string, error) {
	out, old, nw, err := setTopKeys(text, []string{"ask_unmatched"}, func(k *keyOut) {
		if !on {
			k.line("ask_unmatched", " false")
		}
	})
	if err != nil {
		return "", err
	}
	if nw.AsksUnmatched() != on {
		return "", fmt.Errorf("%w: the ask_unmatched key could not be set (edit the file by hand)", ErrInvalid)
	}
	a, b := *old, *nw
	a.AskUnmatched, b.AskUnmatched = nil, nil
	if !reflect.DeepEqual(a, b) {
		return "", fmt.Errorf("%w: the edit would change the rules (edit the file by hand)", ErrInvalid)
	}
	return out, nil
}

func isComment(line string) bool { return strings.HasPrefix(strings.TrimSpace(line), "#") }

// trailingComment returns the `# ...` that ends the rest of a key line, "" when there is none. A `#` inside a
// quoted scalar is not a comment.
func trailingComment(rest string) string {
	var q byte
	for i := 0; i < len(rest); i++ {
		c := rest[i]
		switch {
		case q == '"' && c == '\\':
			i++
		case q != 0 && c == q:
			q = 0
		case q == 0 && (c == '"' || c == '\''):
			q = c
		case q == 0 && c == '#' && (i == 0 || rest[i-1] == ' ' || rest[i-1] == '\t'):
			return strings.TrimSpace(rest[i:])
		}
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
