package serve

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

func TestGuard(t *testing.T) {
	r := newRig(t, goodRules)
	tests := []struct {
		name         string
		method, path string
		host, origin string
		ctype        string
		body         string
		want         int
	}{
		{"get ok", "GET", "/api/state", testAddr, "", "", "", 200},
		{"get localhost", "GET", "/api/state", "localhost:7777", "", "", "", 200},
		{"get other port", "GET", "/api/state", "localhost:7778", "", "", "", 403},
		{"rebinding host", "GET", "/api/state", "evil.example.com:7777", "", "", "", 403},
		{"no host", "GET", "/api/state", "", "", "", "", 403},
		{"post ok", "POST", "/api/validate", testAddr, "http://" + testAddr, "application/json", `{}`, 200},
		{"post json charset", "POST", "/api/validate", testAddr, "http://" + testAddr, "application/json; charset=utf-8", `{}`, 200},
		{"post origin localhost", "POST", "/api/validate", "localhost:7777", "http://localhost:7777", "application/json", `{}`, 200},
		{"post no origin", "POST", "/api/validate", testAddr, "", "application/json", `{}`, 403},
		{"post origin null", "POST", "/api/validate", testAddr, "null", "application/json", `{}`, 403},
		{"post foreign origin", "POST", "/api/validate", testAddr, "http://evil.example.com", "application/json", `{}`, 403},
		{"post https origin", "POST", "/api/validate", testAddr, "https://" + testAddr, "application/json", `{}`, 403},
		{"post form", "POST", "/api/validate", testAddr, "http://" + testAddr, "application/x-www-form-urlencoded", `a=b`, 415},
		{"post text", "POST", "/api/validate", testAddr, "http://" + testAddr, "text/plain", `{}`, 415},
		{"post too big", "POST", "/api/validate", testAddr, "http://" + testAddr, "application/json", `{"yaml":"` + strings.Repeat("a", 2<<20) + `"}`, 413},
		{"delete", "DELETE", "/api/state", testAddr, "", "", "", 405},
		{"ui host checked", "GET", "/", "evil.example.com:7777", "", "", "", 403},
	}
	for _, tc := range tests {
		req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(tc.body))
		req.AddCookie(&http.Cookie{Name: cookieName, Value: testToken})
		req.Host = tc.host
		if tc.origin != "" {
			req.Header.Set("Origin", tc.origin)
		}
		if tc.ctype != "" {
			req.Header.Set("Content-Type", tc.ctype)
		}
		w := httptest.NewRecorder()
		r.srv.Handler().ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Errorf("%s: %d, want %d", tc.name, w.Code, tc.want)
		}
		if got := w.Header().Get("Cache-Control"); got != "no-store" {
			t.Errorf("%s: Cache-Control %q", tc.name, got)
		}
		if w.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: no nosniff", tc.name)
		}
		for k := range w.Header() {
			if strings.HasPrefix(k, "Access-Control-") {
				t.Errorf("%s: CORS header %s", tc.name, k)
			}
		}
	}
}

func TestGuardBeforeSetAddr(t *testing.T) {
	s := New(Options{})
	req := httptest.NewRequest("GET", "/api/state", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: testToken})
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("no addr set: %d", w.Code)
	}
}

func TestGuardOriginMustNameThisServer(t *testing.T) {
	r := newRig(t, goodRules)
	tests := []struct {
		name, origin string
		want         int
	}{
		{"other port", "http://127.0.0.1:7778", 403},
		{"origin with a path", "http://" + testAddr + "/x", 403},
		{"origin with user info", "http://evil@" + testAddr, 403},
		{"no scheme", testAddr, 403},
		{"lookalike host", "http://127.0.0.1:7777.evil.example", 403},
		{"alias of the Host is not the Host (D3)", "http://LOCALHOST:7777", 403},
	}
	for _, tc := range tests {
		req := httptest.NewRequest(http.MethodPost, "/api/validate", strings.NewReader(`{}`))
		req.AddCookie(&http.Cookie{Name: cookieName, Value: testToken})
		req.Host = testAddr
		req.Header.Set("Origin", tc.origin)
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.srv.Handler().ServeHTTP(w, req)
		if w.Code != tc.want {
			t.Errorf("%s: %d, want %d", tc.name, w.Code, tc.want)
		}
	}
}

func TestGuardHeadAllowedWithoutOrigin(t *testing.T) {
	r := newRig(t, goodRules)
	req := httptest.NewRequest(http.MethodHead, "/api/state", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: testToken})
	req.Host = "LOCALHOST:7777"
	w := httptest.NewRecorder()
	r.srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("HEAD with an upper-case own host: %d", w.Code)
	}
}

func TestGuardRejectedPostDoesNotWrite(t *testing.T) {
	r := newRig(t, goodRules)
	req := httptest.NewRequest(http.MethodPost, "/api/apply", strings.NewReader(`{"yaml":"version: 1\ndefault: {browser: Safari}\n","base_hash":"`+sum(goodRules)+`"}`))
	req.AddCookie(&http.Cookie{Name: cookieName, Value: testToken})
	req.Host = testAddr
	req.Header.Set("Origin", "http://evil.example.com")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("foreign origin apply: %d", w.Code)
	}
	if b, _ := os.ReadFile(r.rules); string(b) != goodRules {
		t.Error("a rejected apply changed the rules file")
	}
}
