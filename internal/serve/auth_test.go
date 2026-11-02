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
		req.AddCookie(&http.Cookie{Name: r.srv.cookieName(), Value: cookie})
	}
	w := httptest.NewRecorder()
	r.srv.Handler().ServeHTTP(w, req)
	return w
}

func TestAuthRedirectSetsSessionCookie(t *testing.T) {
	r := newRig(t, goodRules)
	w := rawReq(r, "GET", "/auth/"+testNonce, "", "")
	loc := w.Header().Get("Location")
	cs := w.Result().Cookies()
	if w.Code != http.StatusSeeOther || loc != "/" || len(cs) != 1 {
		t.Fatalf("%d %q %v", w.Code, loc, cs)
	}
	c := cs[0]
	if c.Name != "portero_ui_7777" || c.Value != testSession || c.Value == testNonce || !c.HttpOnly ||
		c.SameSite != http.SameSiteStrictMode || c.Path != "/" || c.Secure {
		t.Errorf("cookie %+v", c)
	}
	if w := rawReq(r, "GET", "/", c.Value, ""); w.Code != 200 {
		t.Errorf("/ with the session cookie: %d", w.Code)
	}
	if r.notice.Len() != 0 {
		t.Errorf("notice on first use: %q", r.notice.String())
	}
}

func TestAuthLinkWorksOnce(t *testing.T) {
	r := newRig(t, goodRules)
	rawReq(r, "GET", "/auth/"+testNonce, "", "")
	for i := 0; i < 2; i++ {
		w := rawReq(r, "GET", "/auth/"+testNonce, "", "")
		if w.Code != http.StatusForbidden || len(w.Result().Cookies()) != 0 {
			t.Fatalf("reuse %d: %d %v", i, w.Code, w.Result().Cookies())
		}
	}
	want := "portero serve: the UI link was used again; if that was not you, stop the server\n"
	if r.notice.String() != want { // printed once, however often the link is replayed
		t.Errorf("notice %q", r.notice.String())
	}
	// a wrong nonce after use is just 403, no notice
	r.notice.Reset()
	if w := rawReq(r, "GET", "/auth/wrong", "", ""); w.Code != 403 || r.notice.Len() != 0 {
		t.Errorf("wrong nonce: %d %q", w.Code, r.notice.String())
	}
}

func TestNonceIsNotTheCookie(t *testing.T) {
	r := newRig(t, goodRules)
	if w := rawReq(r, "GET", "/api/state", testNonce, ""); w.Code != 403 {
		t.Errorf("cookie = nonce: %d", w.Code)
	}
	rawReq(r, "GET", "/auth/"+testNonce, "", "")
	if w := rawReq(r, "GET", "/api/state", testNonce, ""); w.Code != 403 {
		t.Errorf("cookie = nonce after use: %d", w.Code)
	}
}

func TestAuthWrongNonce(t *testing.T) {
	r := newRig(t, goodRules)
	for _, tok := range []string{"wrong", "", testNonce + "x", strings.ToUpper(testNonce), testSession} {
		w := rawReq(r, "GET", "/auth/"+tok, "", "")
		if w.Code != http.StatusForbidden || len(w.Result().Cookies()) != 0 {
			t.Errorf("nonce %q: %d %v", tok, w.Code, w.Result().Cookies())
		}
	}
	if w := rawReq(r, "GET", "/auth/"+testNonce, "", ""); w.Code != http.StatusSeeOther {
		t.Errorf("wrong tries must not burn the nonce: %d", w.Code)
	}
}

func TestCookieRequired(t *testing.T) {
	r := newRig(t, goodRules)
	os.WriteFile(r.log, []byte(okLine(1)), 0o600)
	paths := []string{"/api/state", "/api/graph", "/api/decisions", "/", "/app.js", "/app.css", "/logo.svg", "/nope"}
	for _, p := range paths {
		for name, cookie := range map[string]string{"none": "", "wrong": "wrong", "prefix": testSession[:3]} {
			w := rawReq(r, "GET", p, cookie, "")
			if w.Code != 403 || !strings.Contains(w.Body.String(), "open the URL printed by portero serve") {
				t.Errorf("GET %s (%s cookie): %d", p, name, w.Code)
			}
			if strings.Contains(w.Body.String(), "example.com") {
				t.Errorf("GET %s leaked data", p)
			}
		}
		if w := rawReq(r, "GET", p, testSession, ""); p != "/nope" && w.Code != 200 {
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

func TestRandomSecretsPerServer(t *testing.T) {
	hex64 := regexp.MustCompile(`^[0-9a-f]{64}$`)
	a, b := New(Options{}), New(Options{})
	if !hex64.MatchString(a.Nonce()) || a.Nonce() == b.Nonce() {
		t.Errorf("nonces %q %q", a.Nonce(), b.Nonce())
	}
	if !hex64.MatchString(a.opts.Session) || a.opts.Session == a.Nonce() || a.opts.Session == b.opts.Session {
		t.Errorf("sessions %q %q", a.opts.Session, b.opts.Session)
	}
}

func TestTokenCompareIsConstantTime(t *testing.T) {
	src, err := os.ReadFile("guard.go")
	if err != nil || !strings.Contains(string(src), "subtle.ConstantTimeCompare") || strings.Contains(string(src), "== s.opts.Nonce") {
		t.Error("guard.go must compare the token with crypto/subtle")
	}
}
