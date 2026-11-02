package serve

import (
	"crypto/subtle"
	"errors"
	"mime"
	"net"
	"net/http"
	"strings"
)

var errUnknown = errors.New("unknown")

const maxBody = 1 << 20

// SetAddr tells the guard which Host values belong to this server: the bound address, localhost and 127.0.0.1
// on the same port. Anything else (DNS rebinding) gets 403.
func (s *Server) SetAddr(hostport string) {
	_, port, err := net.SplitHostPort(hostport)
	s.gmu.Lock()
	defer s.gmu.Unlock()
	s.hosts = map[string]bool{}
	if err != nil {
		return
	}
	s.hosts[strings.ToLower(hostport)] = true
	s.hosts["localhost:"+port] = true
	s.hosts["127.0.0.1:"+port] = true
}

func (s *Server) hostOK(h string) bool {
	s.gmu.RLock()
	defer s.gmu.RUnlock()
	return s.hosts[strings.ToLower(h)]
}

// guard checks Host on every request and Origin/Content-Type on every POST, caps bodies and sets the response
// headers. There are no CORS headers: other origins can neither read nor (with the Origin check) write.
func (s *Server) guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("Cache-Control", "no-store")
		h.Set("X-Content-Type-Options", "nosniff")
		if !s.hostOK(r.Host) {
			http.Error(w, "forbidden host", http.StatusForbidden)
			return
		}
		if t, ok := strings.CutPrefix(r.URL.Path, "/auth/"); ok && (r.Method == http.MethodGet || r.Method == http.MethodHead) {
			s.auth(w, r, t)
			return
		}
		if !s.cookieOK(r) {
			http.Error(w, "open the URL printed by portero serve", http.StatusForbidden)
			return
		}
		switch r.Method {
		case http.MethodGet, http.MethodHead:
		case http.MethodPost:
			origin := r.Header.Get("Origin")
			if !strings.EqualFold(origin, "http://"+r.Host) { // exactly the page's own origin; also rejects "" and "null"
				http.Error(w, "forbidden origin", http.StatusForbidden)
				return
			}
			if !isJSON(r.Header.Get("Content-Type")) {
				http.Error(w, "content type must be application/json", http.StatusUnsupportedMediaType)
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, maxBody)
		default:
			w.Header().Set("Allow", "GET, HEAD, POST")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// isJSON accepts "application/json" with optional parameters, not "application/jsonx".
func isJSON(ct string) bool {
	mt, _, err := mime.ParseMediaType(ct)
	return err == nil && mt == "application/json"
}

const cookieName = "portero_ui"

// equal compares secrets in constant time.
func equal(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

// auth trades the per-run token for the session cookie. Loopback is reachable by every local process, other
// macOS users and containers, so Host and Origin alone are not enough: only whoever read the printed URL gets in.
func (s *Server) auth(w http.ResponseWriter, _ *http.Request, token string) {
	if !equal(token, s.opts.Token) {
		http.Error(w, "open the URL printed by portero serve", http.StatusForbidden)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: s.opts.Token, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	w.Header().Set("Location", "/")
	w.WriteHeader(http.StatusSeeOther)
}

func (s *Server) cookieOK(r *http.Request) bool {
	c, err := r.Cookie(cookieName)
	return err == nil && equal(c.Value, s.opts.Token)
}
