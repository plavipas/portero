//go:build !unix

package rulesfile

// Lock is a no-op where flock is unavailable.
func Lock(dir string) (unlock func(), err error) { return func() {}, nil }
