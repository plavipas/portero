package serve

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"
)

func TestUINoExternalURLs(t *testing.T) {
	ext := regexp.MustCompile(`(?i)(src|href|action)\s*=\s*["']?\s*(https?:)?//|url\(\s*["']?\s*(https?:)?//|@import\s+["']?(https?:)?//`)
	inline := regexp.MustCompile(`(?i)\s(style|on[a-z]+)\s*=`)
	n := 0
	err := fs.WalkDir(uiFS, "ui", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		n++
		b, err := uiFS.ReadFile(p)
		if err != nil {
			return err
		}
		if m := ext.Find(b); m != nil {
			t.Errorf("%s: external reference %q", p, m)
		}
		if strings.HasSuffix(p, ".html") {
			if m := inline.Find(b); m != nil {
				t.Errorf("%s: inline attribute %q", p, m)
			}
		}
		return nil
	})
	if err != nil || n != 4 {
		t.Fatalf("files %d, err %v", n, err)
	}
}

func TestUIServed(t *testing.T) {
	r := newRig(t, goodRules)
	tests := []struct{ path, ctype, contains string }{
		{"/", "text/html; charset=utf-8", `<script src="app.js">`},
		{"/app.js", "text/javascript; charset=utf-8", "api("},
		{"/app.css", "text/css; charset=utf-8", "--arrow"},
		{"/logo.svg", "image/svg+xml", "<svg"},
	}
	for _, tc := range tests {
		w := r.do(http.MethodGet, tc.path, "")
		if w.Code != 200 || w.Header().Get("Content-Type") != tc.ctype || !strings.Contains(w.Body.String(), tc.contains) {
			t.Errorf("%s: %d %q", tc.path, w.Code, w.Header().Get("Content-Type"))
		}
		if got := w.Header().Get("Content-Security-Policy"); got != csp {
			t.Errorf("%s: CSP %q", tc.path, got)
		}
	}
	if !strings.Contains(csp, "default-src 'none'") || !strings.Contains(csp, "frame-ancestors 'none'") {
		t.Error("CSP lost its core")
	}
	if w := r.do(http.MethodGet, "/nope", ""); w.Code != http.StatusNotFound {
		t.Errorf("/nope: %d", w.Code)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.AddCookie(&http.Cookie{Name: cookieName, Value: testSession})
	req.Host = "evil.example.com"
	w := httptest.NewRecorder()
	r.srv.Handler().ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("foreign host: %d", w.Code)
	}
}
