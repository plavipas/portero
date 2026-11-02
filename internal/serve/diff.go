package serve

import "strings"

// Line is one line of a diff: Op is " " (same), "-" (only in old) or "+" (only in new).
type Line struct {
	Op   string `json:"op"`
	Text string `json:"text"`
}

const (
	maxDiffLines = 5000
	maxDiffCells = 16 << 20
)

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(strings.TrimSuffix(s, "\n"), "\n")
}

// Diff is a line diff of old and new. Over maxDiffLines per side it returns tooLarge and no lines.
func Diff(old, new string) (lines []Line, tooLarge bool) {
	a, b := splitLines(old), splitLines(new)
	if len(a) > maxDiffLines || len(b) > maxDiffLines {
		return nil, true
	}
	// Common head and tail first: an edit usually touches a few lines.
	p := 0
	for p < len(a) && p < len(b) && a[p] == b[p] {
		p++
	}
	s := 0
	for s < len(a)-p && s < len(b)-p && a[len(a)-1-s] == b[len(b)-1-s] {
		s++
	}
	for _, t := range a[:p] {
		lines = append(lines, Line{" ", t})
	}
	lines = append(lines, lcsDiff(a[p:len(a)-s], b[p:len(b)-s])...)
	for _, t := range a[len(a)-s:] {
		lines = append(lines, Line{" ", t})
	}
	return lines, false
}

func lcsDiff(a, b []string) []Line {
	var out []Line
	n, m := len(a), len(b)
	if n == 0 || m == 0 || (n+1)*(m+1) > maxDiffCells {
		// Nothing to align, or too big to align: show the middle as replaced.
		for _, t := range a {
			out = append(out, Line{"-", t})
		}
		for _, t := range b {
			out = append(out, Line{"+", t})
		}
		return out
	}
	// t[i][j] = length of the LCS of a[i:] and b[j:].
	w := m + 1
	t := make([]uint16, (n+1)*w)
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				t[i*w+j] = t[(i+1)*w+j+1] + 1
			} else if t[(i+1)*w+j] >= t[i*w+j+1] {
				t[i*w+j] = t[(i+1)*w+j]
			} else {
				t[i*w+j] = t[i*w+j+1]
			}
		}
	}
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			out = append(out, Line{" ", a[i]})
			i++
			j++
		case t[(i+1)*w+j] >= t[i*w+j+1]:
			out = append(out, Line{"-", a[i]})
			i++
		default:
			out = append(out, Line{"+", b[j]})
			j++
		}
	}
	for ; i < n; i++ {
		out = append(out, Line{"-", a[i]})
	}
	for ; j < m; j++ {
		out = append(out, Line{"+", b[j]})
	}
	return out
}
