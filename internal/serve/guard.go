package serve

import (
	"crypto/subtle"
	"errors"
	"fmt"
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
	s.port = port
}

// cookieName carries the port. Browsers store cookies per host, not per port: the name only keeps two Portero
// servers on 127.0.0.1 from overwriting each other's cookie. A browser still sends a 127.0.0.1 cookie to every
// port, so a local page the user opens on another port can receive it (low risk; a header-based design is a
// possible follow-up).
func (s *Server) cookieName() string {
	s.gmu.Lock()
	defer s.gmu.Unlock()
	if s.port == "" {
		return "portero_ui"
	}
	return "portero_ui_" + s.port
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

// equal compares secrets in constant time.
func equal(a, b string) bool { return subtle.ConstantTimeCompare([]byte(a), []byte(b)) == 1 }

// auth trades the one-time link nonce for the session cookie. Loopback is reachable by every local process,
// other macOS users and containers, so Host and Origin alone are not enough. The link is visible in the argv of
// the browser start, so it works once: later it is a dead nonce, and the cookie carries a separate secret that is
// never in a URL. A second use of the right nonce is reported on the Notice writer, so a stolen link shows.
func (s *Server) auth(w http.ResponseWriter, _ *http.Request, nonce string) {
	if !equal(nonce, s.opts.Nonce) {
		http.Error(w, "open the URL printed by portero serve", http.StatusForbidden)
		return
	}
	s.amu.Lock()
	reused := s.used
	s.used = true
	warn := reused && !s.warned
	if warn {
		s.warned = true
		fmt.Fprintln(s.opts.Notice, "portero serve: the UI link was used again; if that was not you, stop the server")
	}
	s.amu.Unlock()
	if reused {
		http.Error(w, "this link was already used; open the URL printed by portero serve", http.StatusForbidden)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: s.cookieName(), Value: s.opts.Session, Path: "/", HttpOnly: true, SameSite: http.SameSiteStrictMode})
	w.Header().Set("Location", "/")
	w.WriteHeader(http.StatusSeeOther)
}

func (s *Server) cookieOK(r *http.Request) bool {
	c, err := r.Cookie(s.cookieName())
	return err == nil && equal(c.Value, s.opts.Session)
}
