//go:build darwin

package main

import (
	"os/exec"
	"strconv"
	"strings"
)

// quarantined reports whether the app still carries an unapproved com.apple.quarantine attribute: Gatekeeper
// would block the first link. xattr -p exits non-zero when the attribute is absent, which counts as "not
// quarantined" (so does a failure to run xattr, so the check never gets in the way).
func quarantined(app string) bool {
	if app == "" {
		return false
	}
	out, err := exec.Command("/usr/bin/xattr", "-p", "com.apple.quarantine", app).Output()
	if err != nil {
		return false
	}
	return quarantineBlocks(string(out))
}

// quarantineBlocks reads the flags of a quarantine attribute value ("0081;5f0c...;Safari;UUID"). Once the user
// approved the app macOS sets flag 0x40 and keeps the attribute; only an attribute without it is a block.
// Unparseable flags count as quarantined.
func quarantineBlocks(value string) bool {
	flags, _, _ := strings.Cut(strings.TrimSpace(value), ";")
	n, err := strconv.ParseUint(flags, 16, 32)
	if err != nil {
		return true
	}
	return n&0x40 == 0
}
