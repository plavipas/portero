// Package launch opens a link in a browser. Per-browser profile selection comes in the next step;
// for now every target gets the platform's plain "open in this app".
package launch

import (
	"fmt"
	"net/url"
	"os/exec"
	"runtime"

	"github.com/plavipas/portero/internal/rules"
)

// Open opens u in the target's browser.
func Open(u *url.URL, t rules.Target) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("/usr/bin/open", "-a", t.Browser, u.String())
	default:
		return fmt.Errorf("launch: %s is not supported yet", runtime.GOOS)
	}
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("launch: %v: %s", err, out)
	}
	return nil
}
