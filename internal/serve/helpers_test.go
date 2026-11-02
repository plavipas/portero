package serve

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testAddr = "127.0.0.1:7777"

const goodRules = `# my rules
version: 1
default: {browser: Safari}
rules:
  - name: work
    match: {host: example.com}
    open: {browser: Safari, profile: Work}
`

type rig struct {
	t     *testing.T
	dir   string
	rules string
	log   string
	srv   *Server
}

func newRig(t *testing.T, rulesText string) *rig {
	t.Helper()
	dir := t.TempDir()
	r := &rig{t: t, dir: dir, rules: filepath.Join(dir, "rules.yaml"), log: filepath.Join(dir, "decisions.log")}
	if rulesText != "" {
		if err := os.WriteFile(r.rules, []byte(rulesText), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	r.srv = New(Options{RulesFile: r.rules, LogFile: r.log})
	r.srv.SetAddr(testAddr)
	return r
}

func (r *rig) do(method, path, body string) *httptest.ResponseRecorder {
	r.t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Host = testAddr
	if method == http.MethodPost {
		req.Header.Set("Origin", "http://"+testAddr)
		req.Header.Set("Content-Type", "application/json")
	}
	w := httptest.NewRecorder()
	r.srv.Handler().ServeHTTP(w, req)
	return w
}

func (r *rig) post(path string, v any) (int, map[string]any) {
	r.t.Helper()
	b, _ := json.Marshal(v)
	return decode(r.t, r.do(http.MethodPost, path, string(b)))
}

func (r *rig) get(path string) (int, map[string]any) {
	r.t.Helper()
	return decode(r.t, r.do(http.MethodGet, path, ""))
}

func decode(t *testing.T, w *httptest.ResponseRecorder) (int, map[string]any) {
	t.Helper()
	data, _ := io.ReadAll(w.Body)
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatalf("not JSON (%d): %q", w.Code, data)
	}
	return w.Code, m
}
