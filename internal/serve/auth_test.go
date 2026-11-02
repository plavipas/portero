package serve

import (
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
)

func rawReq(r *rig, method, path, cookie, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Host = testAddr
	if method == http.MethodPost {
		req.Header.Set("Origin", "http://"+testAddr)
		req.Header.Set("Content-Type", "application/json")
	}
	if cookie != "" {
		req.AddCookie(&http.Cookie{Name: cookieName, Value: cookie})
	}
	w := httptest.NewRecorder()
	r.srv.Handler().ServeHTTP(w, req)
	return w
}

func TestAuthRedirectSetsCookie(t *testing.T) {
	r := newRig(t, goodRules)
	w := rawReq(r, "GET", "/auth/"+testToken, "", "")
	loc := w.Header().Get("Location")
	cs := w.Result().Cookies()
	if w.Code != http.StatusSeeOther || loc != "/" || len(cs) != 1 {
		t.Fatalf("%d %q %v", w.Code, loc, cs)
	}
	c := cs[0]
	if c.Name != "portero_ui" || c.Value != testToken || !c.HttpOnly || c.SameSite != http.SameSiteStrictMode || c.Path != "/" || c.Secure {
		t.Errorf("cookie %+v", c)
	}
	// the cookie it hands out opens the page and the API
	if w := rawReq(r, "GET", "/", c.Value, ""); w.Code != 200 {
		t.Errorf("/ with the cookie: %d", w.Code)
	}
}

func TestAuthWrongToken(t *testing.T) {
	r := newRig(t, goodRules)
	for _, tok := range []string{"wrong", "", testToken + "x", strings.ToUpper(testToken)} {
		w := rawReq(r, "GET", "/auth/"+tok, "", "")
		if w.Code != http.StatusForbidden || len(w.Result().Cookies()) != 0 {
			t.Errorf("token %q: %d %v", tok, w.Code, w.Result().Cookies())
		}
	}
}

func TestCookieRequired(t *testing.T) {
	r := newRig(t, goodRules)
	os.WriteFile(r.log, []byte(okLine(1)), 0o600)
	paths := []string{"/api/state", "/api/graph", "/api/decisions", "/", "/app.js", "/app.css", "/logo.svg", "/nope"}
	for _, p := range paths {
		for name, cookie := range map[string]string{"none": "", "wrong": "wrong", "prefix": testToken[:3]} {
			w := rawReq(r, "GET", p, cookie, "")
			if w.Code != 403 || !strings.Contains(w.Body.String(), "open the URL printed by portero serve") {
				t.Errorf("GET %s (%s cookie): %d", p, name, w.Code)
			}
			if strings.Contains(w.Body.String(), "example.com") {
				t.Errorf("GET %s leaked data", p)
			}
		}
		if w := rawReq(r, "GET", p, testToken, ""); p != "/nope" && w.Code != 200 {
			t.Errorf("GET %s with the cookie: %d", p, w.Code)
		}
	}
	// correct Host and Origin are not enough for a write
	body := `{"yaml":"version: 1\ndefault: {browser: /Users/Shared/x.app}\n","base_hash":"` + sum(goodRules) + `"}`
	for _, cookie := range []string{"", "wrong"} {
		if w := rawReq(r, "POST", "/api/apply", cookie, body); w.Code != 403 {
			t.Errorf("apply with cookie %q: %d", cookie, w.Code)
		}
	}
	if b, _ := os.ReadFile(r.rules); string(b) != goodRules {
		t.Error("rules written without the cookie")
	}
}

func TestRandomTokenPerServer(t *testing.T) {
	a, b := New(Options{}).Token(), New(Options{}).Token()
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(a) || a == b {
		t.Errorf("tokens %q %q", a, b)
	}
}

func TestTokenCompareIsConstantTime(t *testing.T) {
	src, err := os.ReadFile("guard.go")
	if err != nil || !strings.Contains(string(src), "subtle.ConstantTimeCompare") || strings.Contains(string(src), "== s.opts.Token") {
		t.Error("guard.go must compare the token with crypto/subtle")
	}
}
