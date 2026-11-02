package serve

import (
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"time"
)

// SchemeHandler is the current handler of one URL scheme.
type SchemeHandler struct {
	Scheme  string `json:"scheme"`
	Handler string `json:"handler"`
	Portero bool   `json:"portero"`
}

// DefaultInfo is the default browser setting: App is the Portero.app this server runs from ("" for a bare
// binary, which cannot be registered as a handler).
type DefaultInfo struct {
	App      string          `json:"app"`
	BundleID string          `json:"bundle_id"`
	Schemes  []SchemeHandler `json:"schemes"`
}

// SchemeResult is the outcome of SetDefault for one scheme.
type SchemeResult struct {
	Scheme  string `json:"scheme"`
	Handler string `json:"handler"`
	Note    string `json:"note"`
}

// SyncFuncs moves the rules file in and out of iCloud Drive. Enable and Disable return the rules path in use.
type SyncFuncs struct {
	Enable, Disable func() (string, error)
	ICloudDir       func() string
	ICloudAvailable func() bool
	// ICloudExists reports a synced rules.yaml already in the iCloud folder (it is adopted when syncing is on).
	ICloudExists func() bool
	// LocalPath is the local rules file that switching syncing off copies back to.
	LocalPath func() string
	// Pinned reports that the rules file is fixed from outside (PORTERO_RULES), so it cannot be moved.
	Pinned func() bool
}

// defaultResult is the outcome of the last SetDefault call.
type defaultResult struct {
	Schemes []SchemeResult `json:"schemes"`
	Error   string         `json:"error,omitempty"`
}

func (s *Server) handleSettings(w http.ResponseWriter, _ *http.Request) {
	def := map[string]any{"supported": false}
	if s.opts.DefaultStatus != nil {
		info := s.opts.DefaultStatus()
		if info.Schemes == nil {
			info.Schemes = []SchemeHandler{}
		}
		s.smu.Lock()
		stale := s.pending && time.Since(s.since) > s.opts.DefaultStale
		def = map[string]any{"supported": s.opts.SetDefault != nil, "app": info.App, "bundle_id": info.BundleID,
			"schemes": info.Schemes, "pending": s.pending, "stale": stale, "last": s.last}
		s.smu.Unlock()
	}
	sy := map[string]any{"supported": false, "mode": s.rulesPlace(), "rules_path": s.rulesPath()}
	if f := s.opts.Sync; f != nil {
		sy["supported"] = true
		sy["pinned"] = f.Pinned != nil && f.Pinned()
		if f.ICloudDir != nil {
			sy["icloud_dir"] = f.ICloudDir()
		}
		sy["icloud_available"] = f.ICloudAvailable != nil && f.ICloudAvailable()
		sy["icloud_exists"] = f.ICloudExists != nil && f.ICloudExists()
		if f.LocalPath != nil {
			sy["local_path"] = f.LocalPath()
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"default": def, "sync": sy, "log_file": s.opts.LogFile, "config_file": s.opts.ConfigFile})
}

func (s *Server) handleSetDefault(w http.ResponseWriter, r *http.Request) {
	var req struct{}
	if !readStrict(w, r, &req) {
		return
	}
	if s.opts.SetDefault == nil || s.opts.DefaultStatus == nil {
		fail(w, http.StatusNotImplemented, "setting the default browser is not supported on this system")
		return
	}
	if s.opts.DefaultStatus().App == "" {
		fail(w, http.StatusPreconditionFailed, "run portero serve from Portero.app")
		return
	}
	s.smu.Lock()
	// A pending call older than DefaultStale is given up on: its goroutine may still be blocked on a macOS
	// dialog that was dismissed or never shown, and must not lock the button forever. Its late result is
	// dropped (attempt numbers), so it cannot overwrite the newer attempt.
	if s.pending && time.Since(s.since) <= s.opts.DefaultStale {
		s.smu.Unlock()
		fail(w, http.StatusConflict, "a request to macOS is already waiting for an answer")
		return
	}
	s.attempt++
	me := s.attempt
	s.pending, s.last, s.since = true, nil, time.Now()
	s.smu.Unlock()

	done := make(chan *defaultResult, 1)
	go func() {
		schemes, err := s.opts.SetDefault()
		res := &defaultResult{Schemes: schemes}
		if res.Schemes == nil {
			res.Schemes = []SchemeResult{}
		}
		if err != nil {
			res.Error = err.Error()
		}
		s.smu.Lock()
		if s.attempt == me {
			s.pending, s.last = false, res
		}
		s.smu.Unlock()
		done <- res
	}()
	timer := time.NewTimer(s.opts.SetDefaultWait)
	defer timer.Stop()
	select {
	case res := <-done:
		out := map[string]any{"schemes": res.Schemes}
		if res.Error != "" {
			out["error"] = map[string]any{"message": res.Error}
		}
		writeJSON(w, http.StatusOK, out)
	case <-timer.C:
		writeJSON(w, http.StatusAccepted, map[string]any{"pending": true})
	case <-r.Context().Done():
	}
}

func (s *Server) handleSettingsSync(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Mode string `json:"mode"`
	}
	if !readStrict(w, r, &req) {
		return
	}
	want := ""
	switch req.Mode {
	case "icloud":
		want = "icloud"
	case "off":
		want = "local"
	default:
		fail(w, http.StatusBadRequest, `mode must be "icloud" or "off"`)
		return
	}
	f := s.opts.Sync
	if f == nil || f.Enable == nil || f.Disable == nil {
		fail(w, http.StatusNotImplemented, "syncing is not supported on this system")
		return
	}
	if f.Pinned != nil && f.Pinned() {
		fail(w, http.StatusConflict, "the rules file is fixed by PORTERO_RULES; unset it to change syncing")
		return
	}
	s.mu.Lock() // the Apply mutex: a save in flight finishes before the file moves
	defer s.mu.Unlock()
	if s.rulesPlace() == want {
		writeJSON(w, http.StatusOK, map[string]any{"mode": want, "rules_path": s.rulesPath(), "changed": false})
		return
	}
	call := f.Disable
	// Neither direction may write through a symlink (the editor refuses them too): off copies the iCloud file
	// over the local path, on copies the local file into iCloud.
	for _, p := range []string{s.rulesPath(), localPathOf(f)} {
		if p == "" {
			continue
		}
		if fi, err := os.Lstat(p); err == nil && fi.Mode()&os.ModeSymlink != 0 {
			fail(w, http.StatusUnprocessableEntity, p+" is a symlink; change syncing by hand")
			return
		}
	}
	if want == "icloud" {
		if f.ICloudAvailable != nil && !f.ICloudAvailable() {
			fail(w, http.StatusUnprocessableEntity, "iCloud Drive is not available on this Mac")
			return
		}
		call = f.Enable
	}
	path, err := call()
	if err != nil {
		fail(w, http.StatusUnprocessableEntity, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"mode": want, "rules_path": path, "changed": true})
}

// readStrict decodes a JSON body and rejects unknown fields.
func readStrict(w http.ResponseWriter, r *http.Request, v any) bool {
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		var tooBig *http.MaxBytesError
		if errors.As(err, &tooBig) {
			fail(w, http.StatusRequestEntityTooLarge, "request body too large")
			return false
		}
		fail(w, http.StatusBadRequest, "bad JSON: "+err.Error())
		return false
	}
	return true
}

func localPathOf(f *SyncFuncs) string {
	if f.LocalPath == nil {
		return ""
	}
	return f.LocalPath()
}
