package serve

import (
	"errors"
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
		switch r.Method {
		case http.MethodGet, http.MethodHead:
		case http.MethodPost:
			origin := r.Header.Get("Origin")
			if origin == "" || origin == "null" || !s.hostOK(strings.TrimPrefix(origin, "http://")) || !strings.HasPrefix(origin, "http://") {
				http.Error(w, "forbidden origin", http.StatusForbidden)
				return
			}
			if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
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
