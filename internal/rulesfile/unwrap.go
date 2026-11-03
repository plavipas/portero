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
	lines := strings.SplitAfter(text, "\n")
	var kept []string
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if isTopKey(l, "unwrap") || isTopKey(l, "unwrap_skip_openers") {
			for i+1 < len(lines) && continuesKey(lines[i+1]) {
				i++
			}
			continue
		}
		kept = append(kept, l)
	}
	var add strings.Builder
	if !unwrap {
		add.WriteString("unwrap: false\n")
	}
	if len(skip) > 0 {
		add.WriteString("unwrap_skip_openers:\n")
		for _, o := range skip {
			add.WriteString("  - " + strconv.Quote(o) + "\n")
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
		kept[at-1] += "\n"
	}
	out := strings.Join(kept[:at], "") + add.String() + strings.Join(kept[at:], "")
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
