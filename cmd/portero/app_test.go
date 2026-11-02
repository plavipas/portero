package main

import (
	"bytes"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/plavipas/portero/internal/handler"
	"github.com/plavipas/portero/internal/rules"
)

type fakeHandlers struct {
	m     map[string]string
	calls []string
	// onSet runs on every set call and may mutate m; its error is returned.
	onSet func(f *fakeHandlers, scheme string) error
}

func (f *fakeHandlers) current(s string) string { return f.m[s] }
func (f *fakeHandlers) set(_, s string) error {
	f.calls = append(f.calls, s)
	return f.onSet(f, s)
}

func setBoth(f *fakeHandlers, _ string) error {
	for _, s := range handler.Schemes {
		f.m[s] = BundleID
	}
	return nil
}

func TestEnsureDefault(t *testing.T) {
	const safari = "com.apple.Safari"
	macErr := errors.New("The file couldn't be opened")
	tests := []struct {
		name      string
		start     map[string]string
		onSet     func(f *fakeHandlers, s string) error
		wantCalls []string
		wantAlr   []bool
		wantErr   []string // substrings; nil means no error
	}{
		{"both already ours", map[string]string{"http": BundleID, "https": BundleID}, nil, nil, []bool{true, true}, nil},
		{"one call sets both", map[string]string{"http": safari, "https": safari}, setBoth, []string{"http"}, []bool{false, false}, nil},
		{"set error but handler ours", map[string]string{"http": safari, "https": safari},
			func(f *fakeHandlers, s string) error { setBoth(f, s); return macErr }, []string{"http"}, []bool{false, false}, nil},
		{"set error and handler unchanged", map[string]string{"http": safari, "https": safari},
			func(*fakeHandlers, string) error { return macErr }, []string{"http"}, nil, []string{"http", safari, "couldn't be opened"}},
		{"no handler at all", map[string]string{"http": "", "https": ""},
			func(*fakeHandlers, string) error { return nil }, []string{"http"}, nil, []string{"http", "without a handler"}},
		{"set ok but handler unchanged", map[string]string{"http": safari, "https": safari},
			func(*fakeHandlers, string) error { return nil }, []string{"http"}, nil, []string{"http", safari}},
		{"only https missing", map[string]string{"http": BundleID, "https": safari},
			func(f *fakeHandlers, s string) error { f.m[s] = BundleID; return nil }, []string{"https"}, []bool{true, false}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeHandlers{m: tt.start, onSet: tt.onSet}
			states, err := ensureDefault(io.Discard, "/x/Portero.app", f.current, f.set, time.Millisecond, 20*time.Millisecond)
			if tt.wantErr == nil && err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("expected an error")
				}
				for _, sub := range tt.wantErr {
					if !strings.Contains(err.Error(), sub) {
						t.Errorf("error %q lacks %q", err, sub)
					}
				}
			}
			if strings.Join(f.calls, ",") != strings.Join(tt.wantCalls, ",") {
				t.Errorf("set calls = %v, want %v", f.calls, tt.wantCalls)
			}
			if tt.wantAlr != nil {
				if len(states) != len(tt.wantAlr) {
					t.Fatalf("states = %v", states)
				}
				for i, st := range states {
					if st.Already != tt.wantAlr[i] {
						t.Errorf("states[%d].Already = %v, want %v", i, st.Already, tt.wantAlr[i])
					}
				}
			}
		})
	}
}

func TestPrintDefaultStatusNotes(t *testing.T) {
	var buf bytes.Buffer
	printDefaultStatus(&buf, func(s string) string { return "already set" })
	out := buf.String()
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if len(lines) != len(handler.Schemes)+1 {
		t.Fatalf("unexpected output:\n%s", out)
	}
	for i := range handler.Schemes {
		if !strings.HasSuffix(lines[i], "already set") {
			t.Errorf("line %q lacks the note", lines[i])
		}
	}
	if !strings.HasPrefix(lines[len(lines)-1], "app:") {
		t.Errorf("last line %q is not the app line", lines[len(lines)-1])
	}
	buf.Reset()
	printDefaultStatus(&buf, nil)
	if strings.Contains(buf.String(), "already set") {
		t.Error("status without a note printed one")
	}
}

func TestEnsureDefaultPollsStaleReads(t *testing.T) {
	const safari = "com.apple.Safari"
	reads := 0
	// http reads foreign for the first 3 reads after the initial snapshot, then ours; https is ours after that.
	current := func(s string) string {
		reads++
		if reads <= 2+3 {
			return safari
		}
		return BundleID
	}
	set := func(string, string) error { return nil }
	states, err := ensureDefault(io.Discard, "/x/Portero.app", current, set, time.Millisecond, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(states) != 2 || states[0].Already {
		t.Errorf("states = %+v", states)
	}

	start := time.Now()
	never := func(string) string { return safari }
	if _, err := ensureDefault(io.Discard, "/x/Portero.app", never, set, time.Millisecond, 30*time.Millisecond); err == nil {
		t.Fatal("expected an error when the handler never flips")
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("polling took %v, want it bounded", d)
	}
}

func TestDecideFileURLsUseDefault(t *testing.T) {
	r := &rules.Rules{Version: 1, Default: rules.Target{Browser: "Dia", Profile: "Personal"},
		Rules: []rules.Rule{{Name: "work", Match: rules.Match{Path: rules.Strings{"/Users"}}, Open: rules.Target{Browser: "Zen"}}}}
	u, err := parseRoutable("file:///Users/someone/page.html")
	if err != nil {
		t.Fatal(err)
	}
	d := decide(r, rules.Request{URL: u})
	if d.Rule != "file" || d.Target != r.Default {
		t.Errorf("file URL: %+v", d)
	}
	for _, ok := range []string{"file:///Users/someone/my%20page.html?x=1#top", "file://localhost/tmp/a.html"} {
		if u, err := parseRoutable(ok); err != nil || u.Scheme != "file" {
			t.Errorf("%q rejected: %v", ok, err)
		}
	}
	for _, bad := range []string{"file://", "file://otherhost/tmp/a.html", "notaurl", "https://", "mailto:a@b.c"} {
		if _, err := parseRoutable(bad); err == nil {
			t.Errorf("%q accepted", bad)
		}
	}
	if u, err := parseRoutable("https://example.com/x"); err != nil || u.Host != "example.com" {
		t.Errorf("https: %v %v", u, err)
	}
}

func TestEnsureDefaultAnnouncesOnTheWriter(t *testing.T) {
	f := &fakeHandlers{m: map[string]string{"http": "com.apple.Safari", "https": "com.apple.Safari"},
		onSet: func(f *fakeHandlers, s string) error { f.m["http"], f.m["https"] = BundleID, BundleID; return nil }}
	var buf bytes.Buffer
	if _, err := ensureDefault(&buf, "/x/Portero.app", f.current, f.set, time.Millisecond, 20*time.Millisecond); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "asking macOS to make Portero the handler of http") {
		t.Errorf("output %q", buf.String())
	}
}

func TestDefaultReport(t *testing.T) {
	ours := map[string]string{"http": BundleID, "https": BundleID}
	other := map[string]string{"http": "com.example.browser", "https": "com.example.browser"}
	const inst = "/h/Applications/Portero.app"
	tests := []struct {
		name                                      string
		cur                                       map[string]string
		resolved, resolvedVer, installed, running string
		want                                      []string
		ok                                        bool
	}{
		{"all match", ours, inst, "1.2.3", inst, "1.2.3", []string{"resolved: " + inst + " (1.2.3)"}, true},
		{"other path", ours, "/tmp/x/Portero.app", "1.2.3", inst, "1.2.3", []string{
			"resolved: /tmp/x/Portero.app (1.2.3)",
			"warning: macOS opens links with /tmp/x/Portero.app, not " + inst + "; run make install"}, false},
		{"other version", ours, inst, "1.0.0", inst, "1.2.3", []string{
			"resolved: " + inst + " (1.0.0)",
			"warning: resolved app is 1.0.0, this binary is 1.2.3"}, false},
		{"empty resolved", ours, "", "", inst, "1.2.3", []string{"resolved: none (macOS has no app for links)"}, false},
		{"bare CLI uses the standard path", ours, inst, "1.2.3", inst, "1.2.3", []string{"resolved: " + inst + " (1.2.3)"}, true},
		{"dev binary", ours, inst, "1.2.3", inst, "dev", []string{"resolved: " + inst + " (1.2.3)", "note: dev binary, version not compared"}, true},
		{"not the default", other, "/Applications/Other.app", "9", inst, "1.2.3", []string{"resolved: /Applications/Other.app (9)"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := defaultReport(tt.cur, tt.resolved, tt.resolvedVer, tt.installed, tt.running)
			if ok != tt.ok || strings.Join(got, "\n") != strings.Join(tt.want, "\n") {
				t.Errorf("got %q ok=%v, want %q ok=%v", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestLsregOthersCmd(t *testing.T) {
	dump := strings.Repeat("-", 30) + "\npath:  /a/Portero.app (0x405c)\nidentifier:  " + BundleID + "\n" +
		strings.Repeat("-", 30) + "\npath:  /b c/Portero.app (0x405c)\nidentifier:  " + BundleID + "\n"
	var out bytes.Buffer
	if err := lsregOthersCmd([]string{BundleID, "/a/Portero.app"}, strings.NewReader(dump), &out); err != nil {
		t.Fatal(err)
	}
	if out.String() != "/b c/Portero.app\n" {
		t.Errorf("got %q", out.String())
	}
	if err := lsregOthersCmd([]string{BundleID}, strings.NewReader(""), &out); err == nil {
		t.Error("missing argument accepted")
	}
}

func TestInstalledAppFallsBackToHomeApplicationsForABareCLI(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	if appBundle() != "" {
		t.Skip("test binary runs inside an app bundle")
	}
	want := filepath.Join(home, "Applications", "Portero.app")
	if got := installedApp(); got != want {
		t.Errorf("installedApp() = %q, want %q", got, want)
	}
}
