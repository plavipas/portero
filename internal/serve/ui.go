package serve

import (
	"embed"
	"net/http"
)

//go:embed ui
var uiFS embed.FS

const csp = "default-src 'none'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self' data:; " +
	"frame-ancestors 'none'; base-uri 'none'; form-action 'none'"

var uiFiles = map[string]struct{ file, ctype string }{
	"/":         {"ui/index.html", "text/html; charset=utf-8"},
	"/app.js":   {"ui/app.js", "text/javascript; charset=utf-8"},
	"/app.css":  {"ui/app.css", "text/css; charset=utf-8"},
	"/logo.svg": {"ui/logo.svg", "image/svg+xml"},
}

func (s *Server) routesUI(mux *http.ServeMux) {
	mux.HandleFunc("GET /{$}", serveUI)
	for p := range uiFiles {
		if p != "/" {
			mux.HandleFunc("GET "+p, serveUI)
		}
	}
}

func serveUI(w http.ResponseWriter, r *http.Request) {
	f, ok := uiFiles[r.URL.Path]
	if !ok {
		http.NotFound(w, r)
		return
	}
	data, err := uiFS.ReadFile(f.file)
	if err != nil {
		http.Error(w, "missing", http.StatusInternalServerError)
		return
	}
	h := w.Header()
	h.Set("Content-Type", f.ctype)
	h.Set("Content-Security-Policy", csp)
	h.Set("Referrer-Policy", "no-referrer")
	_, _ = w.Write(data)
}
