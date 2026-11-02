//go:build !darwin

package sync

import "os"

func platformDataless(os.FileInfo) bool { return false }
