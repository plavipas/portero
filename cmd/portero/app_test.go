package main

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/plavipas/portero/internal/handler"
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
			states, err := ensureDefault("/x/Portero.app", f.current, f.set, time.Millisecond, 20*time.Millisecond)
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
	states, err := ensureDefault("/x/Portero.app", current, set, time.Millisecond, time.Second)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(states) != 2 || states[0].Already {
		t.Errorf("states = %+v", states)
	}

	start := time.Now()
	never := func(string) string { return safari }
	if _, err := ensureDefault("/x/Portero.app", never, set, time.Millisecond, 30*time.Millisecond); err == nil {
		t.Fatal("expected an error when the handler never flips")
	}
	if d := time.Since(start); d > time.Second {
		t.Errorf("polling took %v, want it bounded", d)
	}
}
