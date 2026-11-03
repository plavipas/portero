package serve

import (
	"bytes"
	"errors"
	"io"
	"io/fs"
	"os"
	"strconv"
	"strings"
)

// Decision is one line of the decision log.
type Decision struct {
	Time   string `json:"time"`
	Opener string `json:"opener"` // bundle id, "-" when unknown
	Rule   string `json:"rule"`   // rule name, "default", "none", "file" or "ERROR"
	Target string `json:"target"` // "Browser/Profile:Container"; the message for an ERROR line
	URL    string `json:"url"`
	// Ask is the optional sixth column: how an asking rule was settled ("asked -> Dia/Work").
	Ask string `json:"ask,omitempty"`
	// Unwrapped is the inner URL the rules decided on when URL was a redirect wrapper.
	Unwrapped string `json:"unwrapped,omitempty"`
	Kind      string `json:"kind"` // "ok" or "error"
	// Path is [opener node, rule node, target node] of the graph.
	Path [3]string `json:"path"`
}

const (
	blockSize   = 64 << 10
	maxTailRead = 8 << 20 // a tail never reads more than this, however long the lines are
	maxAfter    = 4 << 20
	// fallbackTarget is where an ERROR line's link went (the handler's Safari fallback).
	fallbackTarget = "Safari"
)

// Page is the result of reading the log.
type Page struct {
	Decisions []Decision
	Offset    int64 // where to continue: the end of the last complete line read
	Truncated bool  // the file is shorter than the offset the caller had
	Skipped   int   // lines that are not decisions
}

// Tail returns the last n decisions and the offset at the end of the last complete line.
// It reads the file from its end in 64 KiB blocks, never the whole file. A missing file is an empty log.
func Tail(path string, n int) (decisions []Decision, offset int64, err error) {
	p, err := TailPage(path, n)
	return p.Decisions, p.Offset, err
}

// After returns up to limit decisions that follow offset. When the file is shorter than offset (it was
// truncated) it returns the tail instead and Truncated.
func After(path string, offset int64, limit int) (decisions []Decision, newOffset int64, truncated bool, err error) {
	p, err := AfterPage(path, offset, limit)
	return p.Decisions, p.Offset, p.Truncated, err
}

// TailPage is Tail with the skipped-line count.
func TailPage(path string, n int) (Page, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Page{}, nil
	}
	if err != nil {
		return Page{}, err
	}
	defer f.Close()
	return tailFile(f, n)
}

func tailFile(f *os.File, n int) (Page, error) {
	fi, err := f.Stat()
	if err != nil {
		return Page{}, err
	}
	size := fi.Size()
	var buf []byte
	pos := size
	for pos > 0 && len(buf) < maxTailRead && bytes.Count(buf, []byte{'\n'}) <= n+1 {
		step := int64(blockSize)
		if step > pos {
			step = pos
		}
		pos -= step
		blk := make([]byte, step)
		if _, err := f.ReadAt(blk, pos); err != nil && !errors.Is(err, io.EOF) {
			return Page{}, err
		}
		buf = append(blk, buf...)
	}
	// The file may have grown meanwhile; what was read ends at `size`.
	end := bytes.LastIndexByte(buf, '\n') + 1 // a trailing partial line is left for the next read
	offset := pos + int64(end)
	buf = buf[:end]
	if pos > 0 { // the first line is cut at the block boundary
		if i := bytes.IndexByte(buf, '\n'); i >= 0 {
			buf = buf[i+1:]
		} else {
			buf = nil
		}
	}
	lines := strings.Split(strings.TrimSuffix(string(buf), "\n"), "\n")
	if len(buf) == 0 {
		lines = nil
	}
	if len(lines) > n {
		lines = lines[len(lines)-n:]
	}
	p := parseLines(lines)
	p.Offset = offset
	return p, nil
}

// AfterPage is After with the skipped-line count.
func AfterPage(path string, offset int64, limit int) (Page, error) {
	f, err := os.Open(path)
	if errors.Is(err, fs.ErrNotExist) {
		return Page{Truncated: offset > 0}, nil
	}
	if err != nil {
		return Page{}, err
	}
	defer f.Close()
	fi, err := f.Stat()
	if err != nil {
		return Page{}, err
	}
	if offset < 0 || offset > fi.Size() {
		p, err := tailFile(f, limit)
		p.Truncated = true
		return p, err
	}
	buf := make([]byte, min(fi.Size()-offset, maxAfter))
	nr, err := f.ReadAt(buf, offset)
	if err != nil && !errors.Is(err, io.EOF) {
		return Page{}, err
	}
	buf = buf[:nr]
	var lines []string
	consumed := 0
	for len(lines) < limit {
		i := bytes.IndexByte(buf[consumed:], '\n')
		if i < 0 {
			break
		}
		lines = append(lines, string(buf[consumed:consumed+i]))
		consumed += i + 1
	}
	p := parseLines(lines)
	if consumed == 0 && len(buf) == maxAfter { // one line longer than the window: step over it
		consumed = len(buf)
		p.Skipped++
	}
	p.Offset = offset + int64(consumed)
	return p, nil
}

func parseLines(lines []string) Page {
	var p Page
	for _, l := range lines {
		if l == "" {
			continue
		}
		d, ok := parseLine(l)
		if !ok {
			p.Skipped++
			continue
		}
		p.Decisions = append(p.Decisions, d)
	}
	return p
}

func parseLine(l string) (Decision, bool) {
	c := strings.Split(l, "\t")
	if len(c) != 5 && len(c) != 6 {
		return Decision{}, false
	}
	d := Decision{Time: c[0], Opener: c[1], Rule: c[2], Target: c[3], URL: c[4], Kind: "ok"}
	if len(c) == 6 {
		d.Ask, d.Unwrapped = splitNote(c[5])
	}
	if d.Rule == "ERROR" {
		msg, err1 := strconv.Unquote(c[3])
		link, err2 := strconv.Unquote(c[4])
		if err1 != nil || err2 != nil {
			return Decision{}, false
		}
		d.Kind, d.Target, d.URL = "error", msg, link
	}
	d.Path = d.path()
	return d, true
}

// path is the decision's route through the graph.
func (d Decision) path() [3]string {
	if d.Kind == "error" {
		return [3]string{openerID(d.Opener), specialID("ERROR"), targetID(fallbackTarget)}
	}
	return [3]string{openerID(d.Opener), logRuleID(nil, d.Rule), targetID(d.Target)}
}

// splitNote separates the sixth column into the ask note and the inner URL: `[ask note; ]unwrapped <url>`.
// The URL comes last, so the first "unwrapped " that starts the note or follows "; " is the marker. Splitting
// from the left is deliberate: an inner URL can contain "; unwrapped " in its query (url.Parse keeps it raw), so
// splitting from the right would let a link choose where the cut falls.
func splitNote(n string) (ask, unwrapped string) {
	const mark = "unwrapped "
	if strings.HasPrefix(n, mark) {
		return "", n[len(mark):]
	}
	if i := strings.Index(n, "; "+mark); i >= 0 {
		return n[:i], n[i+2+len(mark):]
	}
	return n, ""
}
