// Package sync puts the rules file into a folder another service keeps in step between Macs.
// v1 knows iCloud Drive; other backends come later.
package sync

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/plavipas/portero/internal/config"
)

// ICloudDir is the Portero folder inside iCloud Drive.
func ICloudDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, "Library", "Mobile Documents", "com~apple~CloudDocs", "Portero")
}

// EnableICloud moves the rules file to iCloud Drive (or adopts the copy already there) and points
// the config at it. Returns the path in use.
func EnableICloud() (string, error) {
	if _, err := os.Stat(filepath.Dir(ICloudDir())); err != nil {
		return "", fmt.Errorf("iCloud Drive is not available on this Mac: %w", err)
	}
	dst := filepath.Join(ICloudDir(), "rules.yaml")
	src := config.RulesFile()
	if err := os.MkdirAll(ICloudDir(), 0o755); err != nil {
		return "", err
	}
	_, dstErr := os.Stat(dst)
	_, srcErr := os.Stat(src)
	switch {
	case dstErr == nil:
		// A synced file exists already (another Mac set it up): use it, keep the local one as a backup.
		if srcErr == nil && src != dst {
			_ = os.Rename(src, src+".local-backup")
		}
	case srcErr == nil:
		if err := copyFile(src, dst); err != nil {
			return "", err
		}
		_ = os.Rename(src, src+".local-backup")
	default:
		return "", fmt.Errorf("no rules file to sync: run `portero rules init` first")
	}
	return dst, writeConfig(dst)
}

// Disable points the config back at the local rules file, copying the synced one there.
func Disable() (string, error) {
	cur := config.RulesFile()
	dst := filepath.Join(config.Dir(), "rules.yaml")
	if cur != dst {
		if err := copyFile(cur, dst); err != nil {
			return "", err
		}
	}
	return dst, os.Remove(config.File())
}

func writeConfig(rules string) error {
	if err := os.MkdirAll(config.Dir(), 0o755); err != nil {
		return err
	}
	return os.WriteFile(config.File(), []byte("# written by `portero sync`\nrules: "+rules+"\n"), 0o600)
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
