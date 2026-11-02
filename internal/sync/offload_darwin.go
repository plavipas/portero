//go:build darwin

package sync

import (
	"os"
	"syscall"
)

// sfDataless is SF_DATALESS from <sys/stat.h>: the file's data lives in the cloud only.
const sfDataless = 0x40000000

func platformDataless(fi os.FileInfo) bool {
	st, ok := fi.Sys().(*syscall.Stat_t)
	return ok && st.Flags&sfDataless != 0
}
