// Package decisionlog appends one line per routed link.
package decisionlog

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/plavipas/portero/internal/rules"
)

// Append writes "time opener rule target url" (tab-separated) to path, creating it with mode 0600.
func Append(path string, d rules.Decision, req rules.Request) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	opener, rule := req.Opener, d.Rule
	if opener == "" {
		opener = "-"
	}
	if rule == "" {
		rule = "default"
	}
	_, err = fmt.Fprintf(f, "%s\t%s\t%s\t%s\t%s\n",
		time.Now().UTC().Format(time.RFC3339), opener, rule, d.Target, req.URL)
	return err
}
