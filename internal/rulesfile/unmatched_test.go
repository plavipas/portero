package rulesfile

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/plavipas/portero/internal/rules"
)

func hostRule(name, host string, t rules.Target) Remembered {
	return Remembered{Name: name, Host: host, Target: t}
}

func TestInsertLast(t *testing.T) {
	r := req("https://new.example.com/page")
	nr := hostRule("new.example.com", "new.example.com", work)
	block := `  # added by Portero (Remember, no rule matched) 2026-11-03
  - name: "new.example.com"
    match:
      host: "new.example.com"
    open:
      browser: "Dia"
      profile: "Work"
`
	tests := []struct {
		name, in, want string
	}{
		{"plain end", base, base + block},
		{"no final newline", strings.TrimSuffix(base, "\n"), base + block},
		{"trailing blanks and comments stay after", base + "\n# footer\n\n", base + block + "\n# footer\n\n"},
		{"next key after rules", base + "\n# about unwrap\nunwrap: false\n", base + block + "\n# about unwrap\nunwrap: false\n"},
		{"indented trailing comment belongs to the last rule", base + "    # note\n", base + "    # note\n" + block},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := InsertLast(tt.in, nr, r, now)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tt.want)
			}
		})
	}
}

func TestInsertLastFollowDefault(t *testing.T) {
	got, err := InsertLast(base, Remembered{Name: "new.example.com", Host: "new.example.com", FollowDefault: true}, req("https://new.example.com/"), now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasSuffix(got, "    open:\n      default: true\n") || strings.Contains(got, "browser: \"Dia\"") {
		t.Errorf("got:\n%s", got)
	}
	nw, err := rules.Parse([]byte(got))
	if err != nil || !nw.Rules[1].Open.Default {
		t.Errorf("%v %+v", err, nw)
	}
}

func TestInsertLastCRLF(t *testing.T) {
	in := strings.ReplaceAll(base, "\n", "\r\n") + "\r\nunwrap: false\r\n"
	got, err := InsertLast(in, hostRule("new.example.com", "new.example.com", work), req("https://new.example.com/"), now)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.ReplaceAll(got, "\r\n", ""), "\n") {
		t.Errorf("bare LF in CRLF file: %q", got)
	}
	if !strings.Contains(got, "added by Portero (Remember, no rule matched) 2026-11-03\r\n") {
		t.Errorf("got %q", got)
	}
}

func TestInsertLastKeepsOldElementsAndSettings(t *testing.T) {
	got, err := InsertLast(askFile+"unwrap: false\n", hostRule("new.example.com", "new.example.com", work), req("https://new.example.com/"), now)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, strings.TrimSuffix(askFile, "\n")) || !strings.HasSuffix(got, "unwrap: false\n") {
		t.Errorf("file changed outside the insertion:\n%s", got)
	}
	nw, _ := rules.Parse([]byte(got))
	if len(nw.Rules) != 4 || nw.Rules[3].Name != "new.example.com" {
		t.Errorf("rules %+v", nw.Rules)
	}
}

func TestInsertLastRefuses(t *testing.T) {
	r := req("https://new.example.com/")
	nr := hostRule("new.example.com", "new.example.com", work)
	tests := []struct{ name, in string }{
		{"flow list", "version: 1\ndefault: {browser: Dia}\nrules: [{name: A, match: {host: a.example.com}, open: {browser: Safari}}]\n"},
		{"empty list", "version: 1\ndefault: {browser: Dia}\nrules: []\n"},
		{"no rules key", "version: 1\ndefault: {browser: Dia}\n"},
		{"invalid", "version: 2\n"},
		{"already matched", base + "  - {name: Own, match: {host: new.example.com}, open: {browser: Safari}}\n"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got, err := InsertLast(tt.in, nr, r, now); err == nil {
				t.Errorf("accepted:\n%s", got)
			}
		})
	}
}

func rememberRig(t *testing.T, text string) (path, dir string) {
	t.Helper()
	dir = t.TempDir()
	path = filepath.Join(dir, "rules.yaml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path, dir
}

func TestRememberUnmatched(t *testing.T) {
	path, dir := rememberRig(t, base)
	name, err := RememberUnmatched(path, dir, req("https://New.Example.com/a?b=1"), work, false, nil, now)
	if err != nil || name != "new.example.com" {
		t.Fatalf("%q %v", name, err)
	}
	r, err := rules.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	u := req("https://new.example.com/other").URL
	if d := r.Route(rules.Request{URL: u}); d.Rule != name || d.Target != work {
		t.Errorf("decision %+v", d)
	}
	if _, err := os.Stat(path + ".bak"); err != nil {
		t.Errorf("no backup: %v", err)
	}
	// The same host again: a rule matches now, nothing is written.
	before, _ := os.ReadFile(path)
	if _, err := RememberUnmatched(path, dir, req("https://new.example.com/"), work, true, nil, now); !errors.Is(err, ErrNowMatched) {
		t.Errorf("second remember: %v", err)
	}
	if after, _ := os.ReadFile(path); string(after) != string(before) {
		t.Error("ErrNowMatched wrote the file")
	}
}

func TestRememberUnmatchedUniqueNameAndFollow(t *testing.T) {
	in := base + "  - {name: new.example.com, match: {host: other.example.com}, open: {browser: Safari}}\n"
	path, dir := rememberRig(t, in)
	name, err := RememberUnmatched(path, dir, req("https://new.example.com/"), rules.Target{}, true, nil, now)
	if err != nil || name != "new.example.com (2)" {
		t.Fatalf("%q %v", name, err)
	}
	r, _ := rules.Load(path)
	if !r.Rules[2].Open.Default {
		t.Errorf("not a follow rule: %+v", r.Rules[2])
	}
}

func TestRememberUnmatchedGuards(t *testing.T) {
	path, dir := rememberRig(t, base)
	r := req("https://new.example.com/")
	if _, err := RememberUnmatched(path, dir, r, work, false, func(string) bool { return true }, now); !errors.Is(err, ErrOffloaded) {
		t.Errorf("offloaded: %v", err)
	}
	if got, _ := os.ReadFile(path); string(got) != base {
		t.Error("offloaded write changed the file")
	}
	link := filepath.Join(dir, "link.yaml")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	if _, err := RememberUnmatched(link, dir, r, work, false, nil, now); !errors.Is(err, ErrSymlink) {
		t.Errorf("symlink: %v", err)
	}
	if _, err := RememberUnmatched(path, dir, req("file:///tmp/x.html"), work, false, nil, now); err == nil {
		t.Error("file URL accepted")
	}
	bad, bdir := rememberRig(t, "version: 9\n")
	if _, err := RememberUnmatched(bad, bdir, r, work, false, nil, now); err == nil {
		t.Error("invalid file accepted")
	}
}

const askUnmatchedFile = `# top
version: 1   # v
default: {browser: Dia}
ask_unmatched: false   # quiet please
unwrap: false
rules:
  - {name: A, match: {host: a.example.com}, open: {browser: Safari}}
`

func TestSetAskUnmatched(t *testing.T) {
	off, err := SetAskUnmatched(base, false)
	if err != nil {
		t.Fatal(err)
	}
	if want := strings.Replace(base, "rules:\n", "ask_unmatched: false\nrules:\n", 1); !strings.Contains(off, "ask_unmatched: false\n") || len(off) != len(base)+len("ask_unmatched: false\n") {
		t.Errorf("off:\n%s\nwant like:\n%s", off, want)
	}
	if r, _ := rules.Parse([]byte(off)); r.AsksUnmatched() {
		t.Error("still on")
	}
	again, err := SetAskUnmatched(off, false)
	if err != nil || again != off {
		t.Errorf("not idempotent: %v\n%s", err, again)
	}
	on, err := SetAskUnmatched(off, true)
	if err != nil || on != base {
		t.Errorf("on:\n%s\n%v", on, err)
	}
	same, err := SetAskUnmatched(base, true)
	if err != nil || same != base {
		t.Errorf("no-op on: %v", err)
	}
}

func TestSetAskUnmatchedKeepsCommentsAndOtherKeys(t *testing.T) {
	on, err := SetAskUnmatched(askUnmatchedFile, true)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(askUnmatchedFile, "ask_unmatched: false   # quiet please\n", "# quiet please\n", 1)
	if on != want {
		t.Errorf("got:\n%s\nwant:\n%s", on, want)
	}
	back, err := SetAskUnmatched(askUnmatchedFile, false)
	if want := strings.Replace(askUnmatchedFile, "false   # quiet", "false  # quiet", 1); err != nil || back != want {
		t.Errorf("same value: only the comment gap is normalised: %v\n%s", err, back)
	}
	if again, err := SetAskUnmatched(back, false); err != nil || again != back {
		t.Errorf("second save changed the file: %v", err)
	}
	crlf := strings.ReplaceAll(base, "\n", "\r\n")
	out, err := SetAskUnmatched(crlf, false)
	if err != nil || strings.Contains(strings.ReplaceAll(out, "\r\n", ""), "\n") {
		t.Errorf("CRLF: %v %q", err, out)
	}
	if _, err := SetAskUnmatched("version: 2\n", false); !errors.Is(err, ErrInvalid) {
		t.Errorf("invalid: %v", err)
	}
}

func TestInsertLastCRLFTrailingCommentAfterLastRule(t *testing.T) {
	in := "version: 1\r\ndefault: {browser: Dia, profile: Personal}\r\nrules:\r\n  - name: Forge\r\n    match: {host: forge.example.com}\r\n    open: {browser: Safari}\r\n    # kept with the last rule\r\n"
	want := in + "  # added by Portero (Remember, no rule matched) 2026-11-03\r\n" +
		"  - name: \"new.example.com\"\r\n    match:\r\n      host: \"new.example.com\"\r\n    open:\r\n      browser: \"Dia\"\r\n      profile: \"Work\"\r\n"
	got, err := InsertLast(in, hostRule("new.example.com", "new.example.com", work), req("https://new.example.com/"), now)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Errorf("got %q\nwant %q", got, want)
	}
}

func TestRememberUnmatchedGroupFollowingDefault(t *testing.T) {
	in := `version: 1
default: {browser: Dia, profile: Personal}
rules:
  - name: Team
    open: {default: true}
    rules:
      - name: forge
        match: {host: git.example.com}
`
	path, dir := rememberRig(t, in)
	name, err := RememberUnmatched(path, dir, req("https://new.example.com/"), work, false, nil, now)
	if err != nil || name != "new.example.com" {
		t.Fatalf("%q %v", name, err)
	}
	r, err := rules.Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if d := r.Route(req("https://new.example.com/x")); d.Rule != name || d.Target != work {
		t.Errorf("new rule: %+v", d)
	}
	if d := r.Route(req("https://git.example.com/x")); d.Rule == "" || d.Target != r.Default {
		t.Errorf("group child must still follow the default: %+v", d)
	}
	got, _ := os.ReadFile(path)
	if !strings.HasPrefix(string(got), in) {
		t.Errorf("existing text changed:\n%s", got)
	}
}
