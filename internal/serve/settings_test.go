package serve

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// settingsRig is a rig whose server has the settings hooks of o.
func settingsRig(t *testing.T, o Options) *rig {
	t.Helper()
	r := newRig(t, goodRules)
	o.RulesFile, o.LogFile, o.Nonce, o.Session, o.Notice = r.rules, r.log, testNonce, testSession, &r.notice
	r.srv = New(o)
	r.srv.SetAddr(testAddr)
	return r
}

func okStatus(app string) func() DefaultInfo {
	return func() DefaultInfo {
		return DefaultInfo{App: app, BundleID: "dev.example.app", Schemes: []SchemeHandler{
			{Scheme: "http", Handler: "com.example.other"}, {Scheme: "https", Handler: "dev.example.app", Portero: true}}}
	}
}

func TestSettingsGuard(t *testing.T) {
	r := settingsRig(t, Options{DefaultStatus: okStatus("/x/App.app"), SetDefault: func() ([]SchemeResult, error) { return nil, nil }})
	for _, path := range []string{"/api/settings/default", "/api/settings/sync"} {
		tests := []struct {
			name, cookie, origin, ctype string
			want                        int
		}{
			{"no cookie", "", "http://" + testAddr, "application/json", 403},
			{"foreign origin", testSession, "http://evil.example.com", "application/json", 403},
			{"no origin", testSession, "", "application/json", 403},
			{"form content type", testSession, "http://" + testAddr, "text/plain", 415},
		}
		for _, tc := range tests {
			req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{}`))
			if tc.cookie != "" {
				req.AddCookie(&http.Cookie{Name: cookieName, Value: tc.cookie})
			}
			req.Host = testAddr
			if tc.origin != "" {
				req.Header.Set("Origin", tc.origin)
			}
			req.Header.Set("Content-Type", tc.ctype)
			w := httptest.NewRecorder()
			r.srv.Handler().ServeHTTP(w, req)
			if w.Code != tc.want {
				t.Errorf("%s %s: %d, want %d", path, tc.name, w.Code, tc.want)
			}
		}
	}
	req := httptest.NewRequest(http.MethodGet, "/api/settings", nil)
	req.Host = testAddr
	w := httptest.NewRecorder()
	r.srv.Handler().ServeHTTP(w, req)
	if w.Code != 403 {
		t.Errorf("GET /api/settings without a cookie: %d", w.Code)
	}
}

func TestSettingsGet(t *testing.T) {
	r := settingsRig(t, Options{
		ConfigFile:    "/cfg/config.yaml",
		DefaultStatus: okStatus("/x/App.app"),
		SetDefault:    func() ([]SchemeResult, error) { return nil, nil },
		Sync: &SyncFuncs{ICloudDir: func() string { return "/icloud/Portero" }, ICloudAvailable: func() bool { return true },
			Pinned: func() bool { return false }},
	})
	code, m := r.get("/api/settings")
	if code != 200 {
		t.Fatalf("%d %v", code, m)
	}
	def := m["default"].(map[string]any)
	if def["supported"] != true || def["app"] != "/x/App.app" || def["bundle_id"] != "dev.example.app" || def["pending"] != false {
		t.Errorf("default = %v", def)
	}
	schemes := def["schemes"].([]any)
	if len(schemes) != 2 || schemes[1].(map[string]any)["portero"] != true || schemes[0].(map[string]any)["portero"] != false {
		t.Errorf("schemes = %v", schemes)
	}
	sy := m["sync"].(map[string]any)
	if sy["supported"] != true || sy["mode"] != "local" || sy["rules_path"] != r.rules || sy["icloud_dir"] != "/icloud/Portero" ||
		sy["icloud_available"] != true || sy["pinned"] != false {
		t.Errorf("sync = %v", sy)
	}
	if m["log_file"] != r.log || m["config_file"] != "/cfg/config.yaml" {
		t.Errorf("files = %v %v", m["log_file"], m["config_file"])
	}
}

func TestSettingsGetUnsupported(t *testing.T) {
	r := settingsRig(t, Options{})
	_, m := r.get("/api/settings")
	if m["default"].(map[string]any)["supported"] != false || m["sync"].(map[string]any)["supported"] != false {
		t.Errorf("%v", m)
	}
}

func TestSetDefault(t *testing.T) {
	tests := []struct {
		name    string
		set     func() ([]SchemeResult, error)
		wantErr string
		note    string
	}{
		{"already set", func() ([]SchemeResult, error) {
			return []SchemeResult{{"http", "dev.example.app", "already set"}}, nil
		}, "", "already set"},
		{"set now", func() ([]SchemeResult, error) {
			return []SchemeResult{{"http", "dev.example.app", "set now"}}, nil
		}, "", "set now"},
		{"still handled", func() ([]SchemeResult, error) {
			return nil, errors.New("http: still handled by \"com.example.other\"")
		}, "still handled by", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := settingsRig(t, Options{DefaultStatus: okStatus("/x/App.app"), SetDefault: tc.set})
			code, m := r.post("/api/settings/default", map[string]any{})
			if code != 200 {
				t.Fatalf("%d %v", code, m)
			}
			if tc.wantErr != "" {
				msg, _ := m["error"].(map[string]any)["message"].(string)
				if !strings.Contains(msg, tc.wantErr) {
					t.Errorf("error = %v", m["error"])
				}
				return
			}
			if got := m["schemes"].([]any)[0].(map[string]any)["note"]; got != tc.note {
				t.Errorf("note = %v", got)
			}
		})
	}
}

func TestSetDefaultRefusals(t *testing.T) {
	called := false
	set := func() ([]SchemeResult, error) { called = true; return nil, nil }
	tests := []struct {
		name string
		o    Options
		want int
	}{
		{"unsupported", Options{}, 501},
		{"no status", Options{SetDefault: set}, 501},
		{"not from the app", Options{DefaultStatus: okStatus(""), SetDefault: set}, 412},
	}
	for _, tc := range tests {
		r := settingsRig(t, tc.o)
		code, m := r.post("/api/settings/default", map[string]any{})
		if code != tc.want {
			t.Errorf("%s: %d %v", tc.name, code, m)
		}
		if tc.want == 412 && !strings.Contains(m["error"].(map[string]any)["message"].(string), "run portero serve from Portero.app") {
			t.Errorf("412 text: %v", m)
		}
	}
	if called {
		t.Error("SetDefault ran although a precondition failed")
	}
	r := settingsRig(t, Options{DefaultStatus: okStatus("/x/App.app"), SetDefault: set})
	if code, _ := r.post("/api/settings/default", map[string]any{"app": "/evil"}); code != 400 {
		t.Errorf("unknown field: %d", code)
	}
}

func TestSetDefaultPending(t *testing.T) {
	release := make(chan struct{})
	var calls atomic.Int32
	r := settingsRig(t, Options{
		DefaultStatus: okStatus("/x/App.app"), SetDefaultWait: 20 * time.Millisecond,
		SetDefault: func() ([]SchemeResult, error) {
			calls.Add(1)
			<-release
			return []SchemeResult{{"http", "dev.example.app", "set now"}}, nil
		},
	})
	code, m := r.post("/api/settings/default", map[string]any{})
	if code != 202 || m["pending"] != true {
		t.Fatalf("first: %d %v", code, m)
	}
	_, g := r.get("/api/settings")
	if def := g["default"].(map[string]any); def["pending"] != true || def["last"] != nil {
		t.Errorf("while pending: %v", def)
	}
	if code, _ := r.post("/api/settings/default", map[string]any{}); code != 409 {
		t.Errorf("second POST while pending: %d", code)
	}
	close(release)
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, g = r.get("/api/settings")
		def := g["default"].(map[string]any)
		if def["pending"] == false && def["last"] != nil {
			if got := def["last"].(map[string]any)["schemes"].([]any)[0].(map[string]any)["note"]; got != "set now" {
				t.Errorf("last = %v", def["last"])
			}
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("result never landed: %v", def)
		}
		time.Sleep(5 * time.Millisecond)
	}
	if calls.Load() != 1 {
		t.Errorf("SetDefault ran %d times", calls.Load())
	}
}

type syncFake struct {
	enable, disable int
	err             error
	pinned, avail   bool
	hook            func()
}

func (f *syncFake) funcs(path string) *SyncFuncs {
	do := func(n *int) func() (string, error) {
		return func() (string, error) {
			*n++
			if f.hook != nil {
				f.hook()
			}
			return path, f.err
		}
	}
	return &SyncFuncs{Enable: do(&f.enable), Disable: do(&f.disable), ICloudDir: func() string { return "/icloud/Portero" },
		ICloudAvailable: func() bool { return f.avail }, Pinned: func() bool { return f.pinned }}
}

func TestSettingsSync(t *testing.T) {
	t.Run("bad requests", func(t *testing.T) {
		f := &syncFake{avail: true}
		r := settingsRig(t, Options{Sync: f.funcs("/x")})
		for _, body := range []map[string]any{{"mode": "dropbox"}, {"mode": ""}, {}, {"mode": "icloud", "path": "/etc"}} {
			if code, _ := r.post("/api/settings/sync", body); code != 400 {
				t.Errorf("%v: %d", body, code)
			}
		}
		if f.enable+f.disable != 0 {
			t.Error("a bad request reached the sync functions")
		}
	})
	t.Run("unsupported", func(t *testing.T) {
		r := settingsRig(t, Options{})
		if code, _ := r.post("/api/settings/sync", map[string]any{"mode": "icloud"}); code != 501 {
			t.Errorf("%d", code)
		}
	})
	t.Run("pinned", func(t *testing.T) {
		f := &syncFake{avail: true, pinned: true}
		r := settingsRig(t, Options{Sync: f.funcs("/x")})
		if code, _ := r.post("/api/settings/sync", map[string]any{"mode": "icloud"}); code != 409 {
			t.Errorf("%d", code)
		}
		if f.enable != 0 {
			t.Error("Enable ran while pinned")
		}
	})
	t.Run("same mode", func(t *testing.T) {
		f := &syncFake{avail: true}
		r := settingsRig(t, Options{Sync: f.funcs("/x")})
		code, m := r.post("/api/settings/sync", map[string]any{"mode": "off"})
		if code != 200 || m["changed"] != false || m["mode"] != "local" || m["rules_path"] != r.rules {
			t.Errorf("%d %v", code, m)
		}
		r = settingsRig(t, Options{RulesPlace: "icloud", Sync: f.funcs("/x")})
		if code, m = r.post("/api/settings/sync", map[string]any{"mode": "icloud"}); code != 200 || m["changed"] != false {
			t.Errorf("%d %v", code, m)
		}
		if f.enable+f.disable != 0 {
			t.Error("a no-op called the sync functions")
		}
	})
	t.Run("icloud missing", func(t *testing.T) {
		f := &syncFake{avail: false}
		r := settingsRig(t, Options{Sync: f.funcs("/x")})
		code, m := r.post("/api/settings/sync", map[string]any{"mode": "icloud"})
		if code != 422 || !strings.Contains(m["error"].(map[string]any)["message"].(string), "iCloud Drive is not available on this Mac") {
			t.Errorf("%d %v", code, m)
		}
		if f.enable != 0 {
			t.Error("Enable ran without iCloud Drive")
		}
	})
	t.Run("enable and disable", func(t *testing.T) {
		f := &syncFake{avail: true}
		r := settingsRig(t, Options{Sync: f.funcs("/icloud/Portero/rules.yaml")})
		code, m := r.post("/api/settings/sync", map[string]any{"mode": "icloud"})
		if code != 200 || m["changed"] != true || m["mode"] != "icloud" || m["rules_path"] != "/icloud/Portero/rules.yaml" || f.enable != 1 {
			t.Errorf("%d %v", code, m)
		}
		r = settingsRig(t, Options{RulesPlace: "icloud", Sync: f.funcs("/local/rules.yaml")})
		code, m = r.post("/api/settings/sync", map[string]any{"mode": "off"})
		if code != 200 || m["changed"] != true || m["mode"] != "local" || f.disable != 1 {
			t.Errorf("%d %v", code, m)
		}
	})
	t.Run("error", func(t *testing.T) {
		f := &syncFake{avail: true, err: errors.New("no rules file to sync")}
		r := settingsRig(t, Options{Sync: f.funcs("")})
		code, m := r.post("/api/settings/sync", map[string]any{"mode": "icloud"})
		if code != 422 || m["error"].(map[string]any)["message"] != "no rules file to sync" {
			t.Errorf("%d %v", code, m)
		}
	})
}

// A sync request waits for an Apply in flight, because both take the same mutex.
func TestSettingsSyncWaitsForApply(t *testing.T) {
	f := &syncFake{avail: true}
	r := settingsRig(t, Options{Sync: f.funcs("/icloud/Portero/rules.yaml")})
	r.srv.mu.Lock() // an Apply is writing
	done := make(chan int, 1)
	go func() {
		code, _ := r.post("/api/settings/sync", map[string]any{"mode": "icloud"})
		done <- code
	}()
	select {
	case <-done:
		t.Fatal("sync did not wait for the Apply mutex")
	case <-time.After(50 * time.Millisecond):
	}
	if f.enable != 0 {
		t.Error("Enable ran while Apply held the mutex")
	}
	r.srv.mu.Unlock()
	select {
	case code := <-done:
		if code != 200 {
			t.Errorf("%d", code)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("sync never finished")
	}
}

// No field of a request is a path: the file moved is whatever the wired functions say.
func TestSettingsSyncIgnoresRequestPaths(t *testing.T) {
	f := &syncFake{avail: true}
	r := settingsRig(t, Options{Sync: f.funcs("/icloud/Portero/rules.yaml")})
	if code, _ := r.post("/api/settings/sync", map[string]any{"mode": "icloud", "rules_path": "/etc/passwd"}); code != 400 {
		t.Errorf("path field accepted: %d", code)
	}
	_, m := r.post("/api/settings/sync", map[string]any{"mode": "icloud"})
	if m["rules_path"] != "/icloud/Portero/rules.yaml" {
		t.Errorf("%v", m)
	}
}

func TestSettingsGetSyncExtras(t *testing.T) {
	f := &syncFake{avail: true}
	sf := f.funcs("/x")
	sf.ICloudExists = func() bool { return true }
	sf.LocalPath = func() string { return "/cfg/rules.yaml" }
	r := settingsRig(t, Options{Sync: sf})
	_, m := r.get("/api/settings")
	sy := m["sync"].(map[string]any)
	if sy["icloud_exists"] != true || sy["local_path"] != "/cfg/rules.yaml" {
		t.Errorf("sync = %v", sy)
	}
}

func TestSettingsBodyTooLargeIs413(t *testing.T) {
	f := &syncFake{avail: true}
	r := settingsRig(t, Options{Sync: f.funcs("/x")})
	body := `{"mode":"` + strings.Repeat("a", maxBody+10) + `"}`
	if w := r.do(http.MethodPost, "/api/settings/sync", body); w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("%d", w.Code)
	}
}

func TestSetDefaultStalePending(t *testing.T) {
	release := make(chan struct{})
	defer close(release)
	var calls atomic.Int32
	r := settingsRig(t, Options{
		DefaultStatus: okStatus("/x/App.app"), SetDefaultWait: 10 * time.Millisecond, DefaultStale: 40 * time.Millisecond,
		SetDefault: func() ([]SchemeResult, error) {
			if calls.Add(1) == 1 {
				<-release // the first attempt never gets an answer
			}
			return []SchemeResult{{"http", "dev.example.app", "set now"}}, nil
		},
	})
	if code, _ := r.post("/api/settings/default", map[string]any{}); code != 202 {
		t.Fatalf("first: %d", code)
	}
	if code, _ := r.post("/api/settings/default", map[string]any{}); code != 409 {
		t.Errorf("fresh pending: %d", code)
	}
	_, g := r.get("/api/settings")
	if def := g["default"].(map[string]any); def["pending"] != true || def["stale"] != false {
		t.Errorf("fresh: %v", def)
	}
	time.Sleep(60 * time.Millisecond)
	_, g = r.get("/api/settings")
	if def := g["default"].(map[string]any); def["pending"] != true || def["stale"] != true {
		t.Errorf("stale: %v", def)
	}
	code, m := r.post("/api/settings/default", map[string]any{})
	if code != 200 || m["schemes"].([]any)[0].(map[string]any)["note"] != "set now" {
		t.Fatalf("retry: %d %v", code, m)
	}
	_, g = r.get("/api/settings")
	if def := g["default"].(map[string]any); def["pending"] != false || def["last"] == nil {
		t.Errorf("after retry: %v", def)
	}
}

func TestSyncRefusesSymlinkedLocalRules(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "elsewhere.yaml")
	if err := os.WriteFile(target, []byte("keep me\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	local := filepath.Join(dir, "rules.yaml")
	if err := os.Symlink(target, local); err != nil {
		t.Fatal(err)
	}
	called := false
	r := newRig(t, goodRules)
	r.srv.opts.Rules = func() (string, string) { return filepath.Join(dir, "icloud", "rules.yaml"), "icloud" }
	r.srv.opts.Sync = &SyncFuncs{
		Enable:    func() (string, error) { called = true; return "", nil },
		Disable:   func() (string, error) { called = true; return local, nil },
		LocalPath: func() string { return local },
	}
	w := r.do("POST", "/api/settings/sync", `{"mode":"off"}`)
	if w.Code != 422 || called {
		t.Fatalf("got %d, called=%v: %s", w.Code, called, w.Body.String())
	}
	if b, _ := os.ReadFile(target); string(b) != "keep me\n" {
		t.Error("the symlink target was changed")
	}
}

func TestSyncRefusesSymlinkedICloudRules(t *testing.T) {
	dir := t.TempDir()
	icloud := filepath.Join(dir, "icloud")
	if err := os.MkdirAll(icloud, 0o700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(dir, "elsewhere.yaml")
	if err := os.WriteFile(target, []byte("other\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(icloud, "rules.yaml")); err != nil {
		t.Fatal(err)
	}
	called := false
	r := newRig(t, goodRules)
	r.srv.opts.Sync = &SyncFuncs{
		Enable:          func() (string, error) { called = true; return "", nil },
		Disable:         func() (string, error) { called = true; return "", nil },
		ICloudDir:       func() string { return icloud },
		ICloudAvailable: func() bool { return true },
	}
	w := r.do("POST", "/api/settings/sync", `{"mode":"icloud"}`)
	if w.Code != 422 || called {
		t.Fatalf("got %d, called=%v: %s", w.Code, called, w.Body.String())
	}
}
