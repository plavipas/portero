package serve

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

func okLine(i int) string {
	return fmt.Sprintf("2026-01-01T00:00:%02dZ\tcom.example.app\twork\tSafari/Work\thttps://example.com/%d\n", i%60, i)
}

func writeLog(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "decisions.log")
	if err := os.WriteFile(p, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTailBigFile(t *testing.T) {
	var b strings.Builder
	for i := 0; i < 20000; i++ {
		b.WriteString(okLine(i))
	}
	p := writeLog(t, b.String())
	ds, off, err := Tail(p, 200)
	if err != nil || len(ds) != 200 || off != int64(b.Len()) {
		t.Fatalf("len %d off %d/%d err %v", len(ds), off, b.Len(), err)
	}
	if ds[0].URL != "https://example.com/19800" || ds[199].URL != "https://example.com/19999" {
		t.Errorf("window %s .. %s", ds[0].URL, ds[199].URL)
	}
	if ds[0].Kind != "ok" || ds[0].Path != [3]string{"o:com.example.app", "r:work", "t:Safari/Work"} {
		t.Errorf("decision %+v", ds[0])
	}
}

func TestTailFewLinesAndMissing(t *testing.T) {
	ds, off, err := Tail(filepath.Join(t.TempDir(), "none.log"), 10)
	if err != nil || len(ds) != 0 || off != 0 {
		t.Errorf("missing: %v %d %v", ds, off, err)
	}
	p := writeLog(t, okLine(1)+okLine(2))
	ds, _, _ = Tail(p, 10)
	if len(ds) != 2 || ds[0].URL != "https://example.com/1" {
		t.Errorf("few: %+v", ds)
	}
	ds, _, _ = Tail(writeLog(t, ""), 10)
	if len(ds) != 0 {
		t.Errorf("empty: %+v", ds)
	}
}

func TestTailLongLinesAcrossBlocks(t *testing.T) {
	long := "2026-01-01T00:00:00Z\t-\tdefault\tSafari\thttps://example.com/" + strings.Repeat("a", 100000) + "\n"
	p := writeLog(t, okLine(1)+long+okLine(2))
	ds, _, err := Tail(p, 2)
	if err != nil || len(ds) != 2 || len(ds[0].URL) < 100000 || ds[1].URL != "https://example.com/2" {
		t.Errorf("%d %v", len(ds), err)
	}
}

func TestTailErrorGarbageAndPartial(t *testing.T) {
	errLine := "2026-01-01T00:00:00Z\tcom.example.app\tERROR\t" + strconv.Quote("rules: broken; opened in Safari") + "\t" + strconv.Quote("https://example.com/a\tb") + "\n"
	content := "garbage without tabs\n" + "a\tb\tc\n" + okLine(1) + errLine + "2026-01-01T00:00:00Z\t-\tdefault\tSa" // partial last line
	p := writeLog(t, content)
	page, err := TailPage(p, 10)
	if err != nil || len(page.Decisions) != 2 || page.Skipped != 2 {
		t.Fatalf("%+v %v", page, err)
	}
	e := page.Decisions[1]
	if e.Kind != "error" || e.Target != "rules: broken; opened in Safari" || e.URL != "https://example.com/a\tb" ||
		e.Path != [3]string{"o:com.example.app", "s:error", "t:Safari"} {
		t.Errorf("error line: %+v", e)
	}
	if want := int64(len(content) - len("2026-01-01T00:00:00Z\t-\tdefault\tSa")); page.Offset != want {
		t.Errorf("offset %d, want %d (before the partial line)", page.Offset, want)
	}
	bad := writeLog(t, "t\to\tERROR\tnot quoted\talso not\n")
	if pg, _ := TailPage(bad, 5); len(pg.Decisions) != 0 || pg.Skipped != 1 {
		t.Errorf("unquoted error line: %+v", pg)
	}
}

func TestAfter(t *testing.T) {
	p := writeLog(t, okLine(1)+okLine(2))
	_, off, _ := Tail(p, 10)
	ds, off2, trunc, err := After(p, off, 10)
	if err != nil || len(ds) != 0 || trunc || off2 != off {
		t.Fatalf("nothing new: %v %d %v %v", ds, off2, trunc, err)
	}
	f, _ := os.OpenFile(p, os.O_APPEND|os.O_WRONLY, 0o600)
	f.WriteString(okLine(3) + okLine(4) + "partial")
	f.Close()
	ds, off2, trunc, _ = After(p, off, 1)
	if len(ds) != 1 || ds[0].URL != "https://example.com/3" || trunc {
		t.Fatalf("limit 1: %+v", ds)
	}
	ds, off3, _, _ := After(p, off2, 10)
	if len(ds) != 1 || ds[0].URL != "https://example.com/4" {
		t.Fatalf("rest: %+v", ds)
	}
	if st, _ := os.Stat(p); off3 != st.Size()-int64(len("partial")) {
		t.Errorf("offset %d stops before the partial line", off3)
	}
}

func TestAfterTruncated(t *testing.T) {
	p := writeLog(t, okLine(1)+okLine(2)+okLine(3))
	_, off, _ := Tail(p, 10)
	if err := os.WriteFile(p, []byte(okLine(9)), 0o600); err != nil {
		t.Fatal(err)
	}
	ds, off2, trunc, err := After(p, off, 10)
	if err != nil || !trunc || len(ds) != 1 || ds[0].URL != "https://example.com/9" || off2 != int64(len(okLine(9))) {
		t.Errorf("%+v %d %v %v", ds, off2, trunc, err)
	}
	os.Remove(p)
	if _, _, trunc, _ := After(p, off, 10); !trunc {
		t.Error("removed file is a truncation")
	}
}

func TestParseLineSixColumns(t *testing.T) {
	d, ok := parseLine("2026-01-01T00:00:00Z\tcom.apple.iCal\tMeetings\tDia/Work\thttps://meet.example.com/abc-defg-hij\tasked -> Dia/Work")
	if !ok || d.Ask != "asked -> Dia/Work" || d.Target != "Dia/Work" || d.Kind != "ok" {
		t.Fatalf("%+v %v", d, ok)
	}
	if d.Path[2] != "t:Dia/Work" {
		t.Errorf("path %v", d.Path)
	}
	if d5, ok := parseLine("2026-01-01T00:00:00Z\t-\tdefault\tSafari\thttps://example.com/"); !ok || d5.Ask != "" {
		t.Errorf("five columns: %+v %v", d5, ok)
	}
	if _, ok := parseLine("a\tb\tc\td\te\tf\tg"); ok {
		t.Error("seven columns must be skipped")
	}
}
