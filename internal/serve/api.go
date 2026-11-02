package serve

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/plavipas/portero/internal/check"
	"github.com/plavipas/portero/internal/rules"
)

const stateDecisions = 200

func (s *Server) routesAPI(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/state", s.handleState)
	mux.HandleFunc("GET /api/graph", s.handleGraph)
	mux.HandleFunc("GET /api/decisions", s.handleDecisions)
	mux.HandleFunc("POST /api/validate", s.handleValidate)
	mux.HandleFunc("POST /api/match", s.handleMatch)
	mux.HandleFunc("POST /api/apply", s.handleApply)
}

// Summary is the rules file as the UI lists it.
type Summary struct {
	Default string        `json:"default"`
	Rules   []RuleSummary `json:"rules"`
}

// RuleSummary is one rule.
type RuleSummary struct {
	Name  string   `json:"name"`
	Host  []string `json:"host,omitempty"`
	Path  []string `json:"path,omitempty"`
	Open  []string `json:"opener,omitempty"`
	Cwd   []string `json:"cwd,omitempty"`
	Brief string   `json:"open"`
	// Group: the element is a group; Rules are its children (match only, Brief is the group's target).
	Group bool          `json:"group,omitempty"`
	Rules []RuleSummary `json:"rules,omitempty"`
}

func summarize(r *rules.Rules) *Summary {
	if r == nil {
		return nil
	}
	sum := &Summary{Default: r.Default.String(), Rules: []RuleSummary{}}
	for i, rule := range r.Rules {
		rs := RuleSummary{Name: rules.Label(i, rule), Host: rule.Match.Host, Path: rule.Match.Path,
			Open: rule.Match.Opener, Cwd: rule.Match.Cwd, Brief: rule.Open.String()}
		if rule.IsGroup() {
			rs.Group = true
			rs.Rules = []RuleSummary{}
			for j, c := range rule.Rules {
				rs.Rules = append(rs.Rules, RuleSummary{Name: rules.Label(j, c), Host: c.Match.Host, Path: c.Match.Path,
					Open: c.Match.Opener, Cwd: c.Match.Cwd, Brief: rule.Open.String()})
			}
		}
		sum.Rules = append(sum.Rules, rs)
	}
	return sum
}

func hashOf(b []byte) string {
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

// readRules returns the bytes of the rules file; exists is false when there is none.
func (s *Server) readRules() (data []byte, exists bool, err error) {
	return readRulesAt(s.rulesPath())
}

// readRulesAt reads one resolved rules path; Apply uses it so the symlink check, the read and the write all
// refer to the same file even when `portero sync` moves the active path during the request.
func readRulesAt(path string) (data []byte, exists bool, err error) {
	data, err = os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	return data, err == nil, err
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]any{"error": map[string]any{"message": msg}})
}

func readBody(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	if err := dec.Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			fail(w, http.StatusRequestEntityTooLarge, "request body too large")
		} else {
			fail(w, http.StatusBadRequest, "bad JSON: "+err.Error())
		}
		return false
	}
	return true
}

var lineRE = regexp.MustCompile(`line (\d+)`)

// problem is a parse or check failure with the line it points at (0 when unknown).
type problem struct {
	Message string `json:"message"`
	Line    int    `json:"line"`
}

func asProblem(err error) *problem {
	p := &problem{Message: err.Error()}
	if m := lineRE.FindStringSubmatch(p.Message); m != nil {
		p.Line, _ = strconv.Atoi(m[1])
	}
	return p
}

// analyze parses text and runs the checks of `rules check`.
func (s *Server) analyze(text []byte) (r *rules.Rules, warnings []string, prob *problem) {
	r, err := rules.Parse(text)
	if err != nil {
		return nil, nil, asProblem(err)
	}
	warnings, err = check.All(r, s.opts.Env)
	if err != nil {
		return nil, nil, asProblem(err)
	}
	if warnings == nil {
		warnings = []string{}
	}
	return r, warnings, nil
}

func (s *Server) graph(r *rules.Rules, ds []Decision) Graph {
	g := BuildGraph(r, ds)
	g.markInstalled(s.opts.Browsers())
	return g
}

func (s *Server) handleState(w http.ResponseWriter, _ *http.Request) {
	data, exists, err := s.readRules()
	if err != nil {
		fail(w, http.StatusInternalServerError, "cannot read the rules file: "+err.Error())
		return
	}
	st := map[string]any{
		"rules_path":  s.rulesPath(),
		"rules_place": s.rulesPlace(),
		"exists":      exists,
		"text":        string(data),
		"hash":        "",
	}
	var r *rules.Rules
	st["warnings"] = []string{}
	if exists {
		st["hash"] = hashOf(data)
		var warnings []string
		var prob *problem
		r, warnings, prob = s.analyze(data)
		if prob != nil {
			st["rules_error"] = prob
		} else {
			st["warnings"] = warnings
		}
	}
	st["rules"] = summarize(r)
	page, err := TailPage(s.opts.LogFile, stateDecisions)
	if err != nil {
		fail(w, http.StatusInternalServerError, "cannot read the decision log: "+err.Error())
		return
	}
	if page.Decisions == nil {
		page.Decisions = []Decision{}
	}
	st["browsers"] = nonNil(s.opts.Browsers())
	st["decisions"] = page.Decisions
	st["log_offset"] = page.Offset
	st["log_skipped"] = page.Skipped
	st["graph"] = s.graph(r, page.Decisions)
	writeJSON(w, http.StatusOK, st)
}

func nonNil(b []BrowserInfo) []BrowserInfo {
	if b == nil {
		return []BrowserInfo{}
	}
	return b
}

func (s *Server) handleGraph(w http.ResponseWriter, _ *http.Request) {
	var r *rules.Rules
	if data, exists, err := s.readRules(); err == nil && exists {
		r, _ = rules.Parse(data)
	}
	page, err := TailPage(s.opts.LogFile, stateDecisions)
	if err != nil {
		fail(w, http.StatusInternalServerError, "cannot read the decision log: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, s.graph(r, page.Decisions))
}

// handleDecisions: without an `offset` parameter it returns the tail of the log (the last decisions);
// with `offset=N` it returns what follows N, as the UI polls.
func (s *Server) handleDecisions(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()
	offset, _ := strconv.ParseInt(q.Get("offset"), 10, 64)
	limit, err := strconv.Atoi(q.Get("limit"))
	if err != nil || limit <= 0 {
		limit = stateDecisions
	}
	limit = min(limit, 1000)
	var page Page
	if q.Has("offset") {
		page, err = AfterPage(s.opts.LogFile, offset, limit)
	} else {
		page, err = TailPage(s.opts.LogFile, limit)
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, "cannot read the decision log: "+err.Error())
		return
	}
	if page.Decisions == nil {
		page.Decisions = []Decision{}
	}
	var rs *rules.Rules
	if data, exists, err := s.readRules(); err == nil && exists {
		rs, _ = rules.Parse(data)
	}
	ResolvePaths(rs, page.Decisions)
	writeJSON(w, http.StatusOK, map[string]any{"decisions": page.Decisions, "offset": page.Offset,
		"truncated": page.Truncated, "skipped": page.Skipped})
}

type validateReq struct {
	YAML     string `json:"yaml"`
	BaseHash string `json:"base_hash"`
}

func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	var req validateReq
	if !readBody(w, r, &req) {
		return
	}
	cur, exists, err := s.readRules()
	if err != nil {
		fail(w, http.StatusInternalServerError, "cannot read the rules file: "+err.Error())
		return
	}
	curHash := ""
	if exists {
		curHash = hashOf(cur)
	}
	out := map[string]any{
		"ok": false, "warnings": []string{}, "summary": nil,
		"current_hash": curHash, "stale": req.BaseHash != curHash,
	}
	parsed, warnings, prob := s.analyze([]byte(req.YAML))
	if prob != nil {
		out["error"] = prob
	} else {
		out["ok"] = true
		out["warnings"] = warnings
		out["summary"] = summarize(parsed)
	}
	lines, tooLarge := Diff(string(cur), req.YAML)
	if lines == nil {
		lines = []Line{}
	}
	out["diff"], out["diff_too_large"] = lines, tooLarge
	writeJSON(w, http.StatusOK, out)
}

type matchReq struct {
	YAML   string `json:"yaml"`
	URL    string `json:"url"`
	Opener string `json:"opener"`
	Cwd    string `json:"cwd"`
}

func (s *Server) handleMatch(w http.ResponseWriter, r *http.Request) {
	var req matchReq
	if !readBody(w, r, &req) {
		return
	}
	u, err := url.Parse(strings.TrimSpace(req.URL))
	if err != nil || u.Host == "" {
		fail(w, http.StatusBadRequest, "not a URL with a host: "+req.URL)
		return
	}
	text := []byte(req.YAML)
	if strings.TrimSpace(req.YAML) == "" {
		data, exists, err := s.readRules()
		if err != nil || !exists {
			fail(w, http.StatusUnprocessableEntity, "there is no rules file to match against")
			return
		}
		text = data
	}
	rs, err := rules.Parse(text)
	if err != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": asProblem(err)})
		return
	}
	cwd, note := "", "no directory given; cwd rules are skipped"
	if c := strings.TrimSpace(req.Cwd); c != "" {
		if c == "~" || strings.HasPrefix(c, "~/") {
			c = rules.ExpandCwd(c)
		}
		if dir, err := resolveWhatIf(c); err == nil {
			cwd, note = dir, ""
		} else {
			note = fmt.Sprintf("unknown (%v); cwd rules are skipped", err)
		}
	}
	opener := strings.TrimSpace(req.Opener)
	d := rs.Route(rules.Request{URL: u, Opener: opener, Cwd: cwd})
	rule := d.Rule
	if rule == "" {
		rule = "default"
	}
	oid := opener
	if oid == "" {
		oid = "-"
	}
	out := map[string]any{
		"rule": rule, "target": d.Target.String(), "browser": d.Target.Browser,
		"profile": d.Target.Profile, "container": d.Target.Container,
		"cwd_used": cwd, "cwd_note": note,
		"path": [3]string{openerID(oid), matchRuleID(d, rule), targetID(d.Target.String())},
	}
	if s.opts.Plan != nil {
		if plan, err := s.opts.Plan(u, d.Target); err != nil {
			out["plan_error"] = err.Error()
		} else {
			out["plan"] = plan
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) handleApply(w http.ResponseWriter, r *http.Request) {
	var req validateReq
	if !readBody(w, r, &req) {
		return
	}
	_, warnings, prob := s.analyze([]byte(req.YAML))
	if prob != nil {
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": prob})
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.rulesPath()
	if fi, err := os.Lstat(path); err == nil && fi.Mode()&fs.ModeSymlink != 0 {
		fail(w, http.StatusUnprocessableEntity, "rules file is a symlink; edit the target by hand")
		return
	}
	old, exists, err := readRulesAt(path) // the same path as the symlink check and the write below
	if err != nil {
		fail(w, http.StatusInternalServerError, "cannot read the rules file: "+err.Error())
		return
	}
	curHash := ""
	if exists {
		curHash = hashOf(old)
	}
	// Stale: the file changed, or it was removed/moved (e.g. `portero sync`) after the page loaded.
	if curHash != req.BaseHash {
		writeJSON(w, http.StatusConflict, map[string]any{
			"error":        map[string]any{"message": "the rules file changed on disk; reload it before applying"},
			"current_hash": curHash})
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		fail(w, http.StatusInternalServerError, "cannot create the rules directory: "+err.Error())
		return
	}
	backup := ""
	if exists {
		backup = path + ".bak"
		if err := writeAtomic(backup, old); err != nil {
			fail(w, http.StatusInternalServerError, "cannot write the backup: "+err.Error())
			return
		}
	}
	if err := writeAtomic(path, []byte(req.YAML)); err != nil {
		fail(w, http.StatusInternalServerError, "cannot write the rules file: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"hash": hashOf([]byte(req.YAML)), "warnings": warnings, "backup": backup})
}

// writeAtomic writes data with mode 0600 through a temporary file in the same directory and a rename, so a
// reader never sees half a file and a symlink at path is replaced, not followed.
func writeAtomic(path string, data []byte) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".portero-*.tmp") // 0600
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, werr := f.Write(data)
	if werr == nil {
		werr = f.Sync()
	}
	if cerr := f.Close(); werr == nil {
		werr = cerr
	}
	if werr != nil {
		os.Remove(tmp)
		return werr
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// resolveWhatIf is rules.ResolveDir for a directory that may not exist: the deepest existing ancestor is
// resolved through symlinks (so /var/x matches a rule written for /private/var/x), the rest is appended.
func resolveWhatIf(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", err
	}
	rest := ""
	for p := abs; ; p = filepath.Dir(p) {
		if real, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(real, rest), nil
		}
		if p == filepath.Dir(p) {
			return abs, nil
		}
		rest = filepath.Join(filepath.Base(p), rest)
	}
}

// matchRuleID is the node of the rule Route picked: Route returns "" when the default applied, so a user rule
// named "default" is told apart here (unlike in the log). A rule inside a group is drawn as the group.
func matchRuleID(d rules.Decision, shown string) string {
	switch {
	case d.Rule == "":
		return "s:default"
	case d.Group != "":
		return ruleID(d.Group)
	}
	return ruleID(shown)
}
