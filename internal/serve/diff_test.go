package serve

import (
	"strings"
	"testing"
)

func render(l []Line) string {
	var b strings.Builder
	for _, x := range l {
		b.WriteString(x.Op + x.Text + "|")
	}
	return b.String()
}

func TestDiff(t *testing.T) {
	tests := []struct{ name, old, new, want string }{
		{"same", "a\nb\n", "a\nb\n", " a| b|"},
		{"added", "a\nc\n", "a\nb\nc\n", " a|+b| c|"},
		{"removed", "a\nb\nc\n", "a\nc\n", " a|-b| c|"},
		{"changed", "a\nb\nc\n", "a\nX\nc\n", " a|-b|+X| c|"},
		{"from empty", "", "a\n", "+a|"},
		{"to empty", "a\n", "", "-a|"},
		{"both empty", "", "", ""},
		{"interleaved", "a\nb\nc\nd\n", "b\nx\nd\ny\n", "-a| b|-c|+x| d|+y|"},
		{"no trailing newline", "a", "a\nb", " a|+b|"},
	}
	for _, tc := range tests {
		got, big := Diff(tc.old, tc.new)
		if big || render(got) != tc.want {
			t.Errorf("%s: %q big=%v, want %q", tc.name, render(got), big, tc.want)
		}
	}
}

func TestDiffTooLarge(t *testing.T) {
	big := strings.Repeat("x\n", maxDiffLines+1)
	if l, tooLarge := Diff(big, "a\n"); !tooLarge || l != nil {
		t.Errorf("tooLarge=%v lines=%d", tooLarge, len(l))
	}
	if _, tooLarge := Diff("a\n", big); !tooLarge {
		t.Error("new side not limited")
	}
	edge := strings.Repeat("x\n", maxDiffLines)
	if _, tooLarge := Diff(edge, edge+""); tooLarge {
		t.Error("limit is inclusive")
	}
}

func TestDiffHugeMiddleFallsBack(t *testing.T) {
	var a, b strings.Builder
	for i := 0; i < 4500; i++ {
		a.WriteString("a" + string(rune('A'+i%26)) + "\n")
		b.WriteString("b" + string(rune('A'+i%26)) + "\n")
	}
	got, big := Diff(a.String(), b.String())
	if big || len(got) != 9000 {
		t.Errorf("len %d big %v", len(got), big)
	}
}
