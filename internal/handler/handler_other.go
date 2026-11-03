//go:build !darwin

package handler

import "errors"

type Handler func(url, opener string)

var Schemes = []string{"http", "https"}

var errUnsupported = errors.New("the link handler app exists only on macOS")

func Run(h Handler)                                  { panic(errUnsupported) }
func DefaultHandler(scheme string) string            { return "" }
func SetDefaultHandler(appPath, scheme string) error { return errUnsupported }

// StartDownload is not supported here.
func StartDownload(path string) error {
	return errors.New("iCloud download is not supported on this system")
}

// DefaultApp is not supported here.
func DefaultApp(scheme string) string { return "" }

// BundleVersion is not supported here.
func BundleVersion(appPath string) (short, build string) { return "", "" }

// Ask shows nothing here and cancels at once, so the default target is used.
func Ask(req AskRequest) AskResult { return Cancelled }

// Notify shows nothing here.
func Notify(title, body string) string { return "notification failed: unsupported" }
