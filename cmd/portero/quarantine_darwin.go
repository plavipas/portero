//go:build darwin

package main

import "os/exec"

// quarantined reports whether macOS marked the app as downloaded (com.apple.quarantine): the first link would then
// be blocked by Gatekeeper. xattr -p exits non-zero when the attribute is absent; any failure counts as "not
// quarantined" so the check never gets in the way.
func quarantined(app string) bool {
	if app == "" {
		return false
	}
	return exec.Command("/usr/bin/xattr", "-p", "com.apple.quarantine", app).Run() == nil
}
