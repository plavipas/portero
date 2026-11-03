package rulesfile

import (
	"errors"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/plavipas/portero/internal/rules"
)

const base = `version: 1
default: {browser: Dia, profile: Personal}
rules:
  - name: Forge
    match: {host: forge.example.com}
    open: {browser: Safari}
`

const askFile = `# top comment
version: 1
default: {browser: Dia, profile: Personal}   # fallback
rules:
  # first rule
  - name: Forge
    match: {host: forge.example.com}
    open: {browser: Safari}

  # Meetings from every calendar
  # (keep this comment)
  - name: Meetings
    match: {host: [meet.google.com, "*.zoom.us"]}
    open:
      ask:
        - {browser: Dia, profile: Personal}
        - {browser: Dia, profile: Work}   # work account
  - name: Docs
    match: {host: docs.example.com}
    open: {browser: Safari}
`

var now = time.Date(2026, 11, 3, 12, 0, 0, 0, time.UTC)

func req(raw string) rules.Request {
	u, _ := url.Parse(raw)
	return rules.Request{URL: u, Opener: "com.apple.iCal"}
}

var work = rules.Target{Browser: "Dia", Profile: "Work"}

func TestWrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.yaml")
	if err := os.WriteFile(path, []byte(base), 0o644); err != nil {
		t.Fatal(err)
	}
	next := strings.Replace(base, "Safari", "Dia", 1)

	if _, err := Write(path, next, "deadbeef", nil); !errors.Is(err, ErrStale) {
		t.Fatalf("wrong base hash: %v", err)
	}
	var se *StaleError
	if _, err := Write(path, next, "x", nil); !errors.As(err, &se) || se.Current != HashOf([]byte(base)) {
		t.Fatalf("stale error carries the current hash: %v", err)
	}
	if _, err := Write(path, "version: 2\n", HashOf([]byte(base)), nil); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid text: %v", err)
	}
	if _, err := Write(path, next, HashOf([]byte(base)), func(string) bool { return true }); !errors.Is(err, ErrOffloaded) {
		t.Fatalf("offloaded: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != base {
		t.Fatal("a refused write changed the file")
	}
	res, err := Write(path, next, HashOf([]byte(base)), nil)
	if err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(path); string(got) != next || res.Hash != HashOf([]byte(next)) {
		t.Fatal("file not replaced")
	}
	if bak, _ := os.ReadFile(res.Backup); string(bak) != base || res.Backup != path+".bak" {
		t.Fatalf("backup %q: %q", res.Backup, bak)
	}
	if fi, _ := os.Stat(path); fi.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", fi.Mode().Perm())
	}
	if m, _ := filepath.Glob(filepath.Join(dir, ".portero-*.tmp")); len(m) != 0 {
		t.Errorf("temp files left: %v", m)
	}
}

func TestWriteNewFileAndSymlink(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "rules.yaml")
	res, err := Write(path, base, "", nil)
	if err != nil || res.Backup != "" {
		t.Fatalf("new file: %+v %v", res, err)
	}
	link := filepath.Join(dir, "link.yaml")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(link, base, HashOf([]byte(base)), nil); !errors.Is(err, ErrSymlink) {
		t.Fatalf("symlink: %v", err)
	}
}

func TestInsertBefore(t *testing.T) {
	r := req("https://meet.google.com/abc-defg-hij?authuser=1")
	nr := Remembered{Name: "meet.google.com/abc-defg-hij", Host: "meet.google.com", Path: "/abc-defg-hij", Target: work}
	got, err := InsertBefore(askFile, 1, nr, r, now)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(askFile, "  # Meetings from every calendar\n", `  # added by Portero (Remember) 2026-11-03
  - name: "meet.google.com/abc-defg-hij"
    match:
      host: "meet.google.com"
      path: "/abc-defg-hij"
    open:
      browser: "Dia"
      profile: "Work"
  # Meetings from every calendar
`, 1)
	if got != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

func TestInsertBeforeFirstAndGroup(t *testing.T) {
	r := req("https://example.zoom.us/j/123?pwd=x")
	nr := Remembered{Name: "example.zoom.us/j/123", Host: "example.zoom.us", Path: "/j/123", Target: work}
	got, err := InsertBefore(base, 0, nr, r, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "version: 1\ndefault: {browser: Dia, profile: Personal}\nrules:\n  # added by Portero (Remember) 2026-11-03\n  - name: ") {
		t.Errorf("first position:\n%s", got)
	}
	group := `version: 1
default: {browser: Safari}
rules:
- name: Calls
  open:
    ask: [{browser: Dia, profile: Personal}, {browser: Dia, profile: Work}]
  rules:
  - match: {host: "*.zoom.us"}
`
	got, err = InsertBefore(group, 0, nr, r, now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(got, "rules:\n# added by Portero (Remember) 2026-11-03\n- name: \"example.zoom.us/j/123\"\n  match:\n    host: ") {
		t.Errorf("unindented list:\n%s", got)
	}
	if _, err := rules.Parse([]byte(got)); err != nil {
		t.Error(err)
	}
}

func TestInsertBeforeCRLFAndRefusals(t *testing.T) {
	crlf := strings.ReplaceAll(askFile, "\n", "\r\n")
	nr := Remembered{Name: "meet.google.com/abc-defg-hij", Host: "meet.google.com", Path: "/abc-defg-hij", Target: work}
	got, err := InsertBefore(crlf, 1, nr, req("https://meet.google.com/abc-defg-hij"), now)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") {
		t.Error("a bare LF in a CRLF file")
	}
	flow := "version: 1\ndefault: {browser: Safari}\nrules: [{name: A, match: {host: a.example.com}, open: {browser: Dia}}]\n"
	if _, err := InsertBefore(flow, 0, nr, req("https://a.example.com/x"), now); err == nil || !strings.Contains(err.Error(), "flow style") {
		t.Errorf("flow rules list: %v", err)
	}
	if _, err := InsertBefore(base, 5, nr, req("https://forge.example.com/x"), now); err == nil {
		t.Error("position out of range")
	}
	// a name that cannot be written as a rule name fails verification instead of corrupting the file
	bad := nr
	bad.Name = "a / b"
	if _, err := InsertBefore(askFile, 1, bad, req("https://meet.google.com/abc-defg-hij"), now); err == nil {
		t.Error("invalid name accepted")
	}
	// a target the link does not reach (wrong host) fails verification
	if _, err := InsertBefore(askFile, 1, nr, req("https://other.example.com/"), now); err == nil {
		t.Error("link that does not route to the new rule accepted")
	}
}

func TestUniqueName(t *testing.T) {
	r, _ := rules.Parse([]byte(askFile))
	if got := UniqueName(r, "Docs"); got != "Docs (2)" {
		t.Error(got)
	}
	if got := UniqueName(r, "new"); got != "new" {
		t.Error(got)
	}
}

func TestRemember(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.yaml")
	if err := os.WriteFile(path, []byte(askFile), 0o600); err != nil {
		t.Fatal(err)
	}
	r := req("https://meet.google.com/abc-defg-hij?authuser=1")
	name, err := Remember(path, dir, r, "Meetings", work, nil, now)
	if err != nil || name != "meet.google.com/abc-defg-hij" {
		t.Fatalf("%q %v", name, err)
	}
	rs, err := rules.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if d := rs.Route(r); d.Rule != name || d.Target != work || len(d.Ask) != 0 {
		t.Errorf("after remember: %+v", d)
	}
	// another meeting still asks
	if d := rs.Route(req("https://meet.google.com/zzz-yyyy-xxx")); len(d.Ask) != 2 {
		t.Errorf("other meeting: %+v", d)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Error("no backup")
	}
	if _, err := os.Stat(filepath.Join(dir, "rules.lock")); err != nil {
		t.Error("no lock file")
	}
	// the same link again: the rules no longer ask for it
	if _, err := Remember(path, dir, r, "Meetings", work, nil, now); !errors.Is(err, ErrNoLongerAsks) {
		t.Errorf("second remember: %v", err)
	}
	// a target that is not one of the choices
	if _, err := Remember(path, dir, req("https://meet.google.com/zzz-yyyy-xxx"), "Meetings", rules.Target{Browser: "Safari"}, nil, now); !errors.Is(err, ErrNoLongerAsks) {
		t.Errorf("foreign target: %v", err)
	}
	if _, err := Remember(path, dir, req("https://meet.google.com/"), "Meetings", work, nil, now); err == nil {
		t.Error("no path to remember")
	}
}

func TestRememberGroupAndClash(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.yaml")
	text := `version: 1
default: {browser: Safari}
rules:
  - name: example.zoom.us/j/123
    match: {host: unrelated.example.com}
    open: {browser: Safari}
  - name: Calls
    open:
      ask: [{browser: Dia, profile: Personal}, {browser: Dia, profile: Work}]
    rules:
      - name: zoom
        match: {host: "*.zoom.us"}
`
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	name, err := Remember(path, "", req("https://example.zoom.us/j/123?pwd=x"), "Calls / zoom", work, nil, now)
	if err != nil || name != "example.zoom.us/j/123 (2)" {
		t.Fatalf("%q %v", name, err)
	}
	data, _ := os.ReadFile(path)
	if i, j := strings.Index(string(data), "(2)"), strings.Index(string(data), "- name: Calls"); i < 0 || i > j {
		t.Errorf("not inserted before the group:\n%s", data)
	}
}

func TestRememberOffloaded(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rules.yaml")
	os.WriteFile(path, []byte(askFile), 0o600)
	if _, err := Remember(path, dir, req("https://meet.google.com/abc-defg-hij"), "Meetings", work, func(string) bool { return true }, now); !errors.Is(err, ErrOffloaded) {
		t.Errorf("%v", err)
	}
}

func TestLockSerializes(t *testing.T) {
	dir := t.TempDir()
	unlock, err := Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan struct{})
	go func() {
		u2, err := Lock(dir)
		if err == nil {
			u2()
		}
		close(got)
	}()
	select {
	case <-got:
		t.Fatal("second Lock did not wait")
	case <-time.After(100 * time.Millisecond):
	}
	unlock()
	select {
	case <-got:
	case <-time.After(2 * time.Second):
		t.Fatal("second Lock never got the lock")
	}
}

func TestLockTimesOut(t *testing.T) {
	old := lockWait
	lockWait = 200 * time.Millisecond
	defer func() { lockWait = old }()
	dir := t.TempDir()
	unlock, err := Lock(dir)
	if err != nil {
		t.Fatal(err)
	}
	defer unlock()
	start := time.Now()
	_, err = Lock(dir)
	if err == nil || !strings.Contains(err.Error(), "held by another writer") {
		t.Fatalf("err %v", err)
	}
	if d := time.Since(start); d < 150*time.Millisecond || d > time.Second {
		t.Errorf("waited %v", d)
	}
}
