package browsers

import (
	"os"
	"path/filepath"
)

// Installed reports whether the app bundle exists in /Applications or ~/Applications.
func (b Browser) Installed() bool {
	home, _ := os.UserHomeDir()
	for _, dir := range []string{"/Applications", filepath.Join(home, "Applications")} {
		if _, err := os.Stat(filepath.Join(dir, b.Name+".app")); err == nil {
			return true
		}
	}
	return false
}
