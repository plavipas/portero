package serve

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// waitLast polls GET /api/settings until the pending SetDefault has finished and returns default.last.
func waitLast(t *testing.T, r *rig) map[string]any {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, g := r.get("/api/settings")
		def := g["default"].(map[string]any)
		if def["pending"] == false && def["last"] != nil {
			return def["last"].(map[string]any)
		}
		if time.Now().After(deadline) {
			t.Fatalf("the result never landed: %v", def)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestSettingsPostWrongHostIsRefused(t *testing.T) {
	var calls atomic.Int32
	f := &syncFake{avail: true}
	r := settingsRig(t, Options{DefaultStatus: okStatus("/x/App.app"), Sync: f.funcs("/x"),
		SetDefault: func() ([]SchemeResult, error) { calls.Add(1); return nil, nil }})
	for _, path := range []string{"/api/settings/default", "/api/settings/sync"} {
		req := httptest.NewRequest(http.MethodPost, path, strings.NewReader(`{"mode":"icloud"}`))
		req.AddCookie(&http.Cookie{Name: cookieName, Value: testSession})
		req.Host = "evil.example.com:7777"
		req.Header.Set("Origin", "http://evil.example.com:7777")
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.srv.Handler().ServeHTTP(w, req)
		if w.Code != http.StatusForbidden {
			t.Errorf("%s with a rebinding host: %d", path, w.Code)
		}
	}
	if calls.Load() != 0 || f.enable+f.disable != 0 {
		t.Error("a refused request reached a settings function")
	}
}

func TestSettingsPostBodyIsCapped(t *testing.T) {
	var calls atomic.Int32
	f := &syncFake{avail: true}
	r := settingsRig(t, Options{DefaultStatus: okStatus("/x/App.app"), Sync: f.funcs("/x"),
		SetDefault: func() ([]SchemeResult, error) { calls.Add(1); return nil, nil }})
	big := `{"mode":"` + strings.Repeat("a", 2<<20) + `"}`
	for _, path := range []string{"/api/settings/default", "/api/settings/sync"} {
		w := r.do(http.MethodPost, path, big)
		if w.Code < 400 || w.Code >= 500 {
			t.Errorf("%s with a 2 MiB body: %d", path, w.Code)
		}
	}
	if calls.Load() != 0 || f.enable+f.disable != 0 {
		t.Error("an oversized request reached a settings function")
	}
}

// A page reload while macOS waits gets a fresh GET; the pending flag and later the result must still be there.
func TestSetDefaultResultLandsAfterTheClientLeft(t *testing.T) {
	release := make(chan struct{})
	r := settingsRig(t, Options{DefaultStatus: okStatus("/x/App.app"), SetDefaultWait: time.Minute,
		SetDefault: func() ([]SchemeResult, error) {
			<-release
			return []SchemeResult{{"http", "dev.example.app", "set now"}}, nil
		}})
	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequest(http.MethodPost, "/api/settings/default", strings.NewReader(`{}`)).WithContext(ctx)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: testSession})
	req.Host = testAddr
	req.Header.Set("Origin", "http://"+testAddr)
	req.Header.Set("Content-Type", "application/json")
	served := make(chan struct{})
	go func() { r.srv.Handler().ServeHTTP(httptest.NewRecorder(), req); close(served) }()
	deadline := time.Now().Add(2 * time.Second)
	for {
		_, g := r.get("/api/settings")
		if g["default"].(map[string]any)["pending"] == true {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("never pending")
		}
		time.Sleep(time.Millisecond)
	}
	cancel() // the tab is reloaded: the first request goes away
	select {
	case <-served:
	case <-time.After(2 * time.Second):
		t.Fatal("the handler did not return when the client left")
	}
	_, g := r.get("/api/settings")
	if g["default"].(map[string]any)["pending"] != true {
		t.Errorf("pending was lost with the request: %v", g["default"])
	}
	close(release)
	last := waitLast(t, r)
	if got := last["schemes"].([]any)[0].(map[string]any)["note"]; got != "set now" {
		t.Errorf("last = %v", last)
	}
}

func TestSetDefaultErrorAfterPendingLandsInLast(t *testing.T) {
	release := make(chan struct{})
	r := settingsRig(t, Options{DefaultStatus: okStatus("/x/App.app"), SetDefaultWait: 10 * time.Millisecond,
		SetDefault: func() ([]SchemeResult, error) {
			<-release
			return nil, errors.New(`http: still handled by "com.example.other"`)
		}})
	if code, _ := r.post("/api/settings/default", map[string]any{}); code != 202 {
		t.Fatalf("first: %d", code)
	}
	close(release)
	last := waitLast(t, r)
	if !strings.Contains(last["error"].(string), "still handled by") {
		t.Errorf("last = %v", last)
	}
	if s, ok := last["schemes"].([]any); !ok || len(s) != 0 {
		t.Errorf("schemes should be an empty list: %v", last["schemes"])
	}
}

func TestSetDefaultCanRunAgainAfterAResult(t *testing.T) {
	var calls atomic.Int32
	r := settingsRig(t, Options{DefaultStatus: okStatus("/x/App.app"),
		SetDefault: func() ([]SchemeResult, error) { calls.Add(1); return nil, nil }})
	for i := 0; i < 2; i++ {
		if code, m := r.post("/api/settings/default", map[string]any{}); code != 200 {
			t.Fatalf("call %d: %d %v", i, code, m)
		}
	}
	if calls.Load() != 2 {
		t.Errorf("SetDefault ran %d times", calls.Load())
	}
}

// A new request clears the result of the previous one, so a reload never shows a stale "Done.".
func TestSetDefaultNewRequestClearsLast(t *testing.T) {
	release := make(chan struct{})
	first := true
	r := settingsRig(t, Options{DefaultStatus: okStatus("/x/App.app"), SetDefaultWait: 10 * time.Millisecond,
		SetDefault: func() ([]SchemeResult, error) {
			if first {
				first = false
				return []SchemeResult{{"http", "dev.example.app", "set now"}}, nil
			}
			<-release
			return nil, nil
		}})
	if code, _ := r.post("/api/settings/default", map[string]any{}); code != 200 {
		t.Fatal(code)
	}
	if code, _ := r.post("/api/settings/default", map[string]any{}); code != 202 {
		t.Fatal(code)
	}
	_, g := r.get("/api/settings")
	if def := g["default"].(map[string]any); def["last"] != nil || def["pending"] != true {
		t.Errorf("while the second request waits: %v", def)
	}
	close(release)
	waitLast(t, r)
}

// The late answer of an abandoned (stale) attempt must neither clear the newer attempt's pending flag nor
// overwrite its result.
func TestSetDefaultStaleAttemptLateResultIsDropped(t *testing.T) {
	oldRelease, newRelease := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	r := settingsRig(t, Options{
		DefaultStatus: okStatus("/x/App.app"), SetDefaultWait: 10 * time.Millisecond, DefaultStale: 30 * time.Millisecond,
		SetDefault: func() ([]SchemeResult, error) {
			if calls.Add(1) == 1 {
				<-oldRelease
				return nil, errors.New("old attempt: still handled by \"com.example.other\"")
			}
			<-newRelease
			return []SchemeResult{{"http", "dev.example.app", "set now"}}, nil
		},
	})
	if code, _ := r.post("/api/settings/default", map[string]any{}); code != 202 {
		t.Fatalf("first: %d", code)
	}
	time.Sleep(50 * time.Millisecond)
	if code, _ := r.post("/api/settings/default", map[string]any{}); code != 202 {
		t.Fatalf("retry after stale: %d", code)
	}
	close(oldRelease) // the abandoned attempt answers while the retry still waits
	time.Sleep(30 * time.Millisecond)
	_, g := r.get("/api/settings")
	if def := g["default"].(map[string]any); def["pending"] != true || def["last"] != nil {
		t.Errorf("the old answer leaked into the new attempt: %v", def)
	}
	close(newRelease)
	last := waitLast(t, r)
	if _, hasErr := last["error"]; hasErr {
		t.Errorf("last carries the old attempt's error: %v", last)
	}
	if got := last["schemes"].([]any)[0].(map[string]any)["note"]; got != "set now" {
		t.Errorf("last = %v", last)
	}
}
