package serve

import (
	"net/http"
	"net/http/httptest"
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
	w := httptest.NewRecorder()
	s.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("no addr set: %d", w.Code)
	}
}
