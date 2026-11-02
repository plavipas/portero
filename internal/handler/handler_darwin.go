//go:build darwin

// Package handler is the macOS side: the agent app that receives links from other apps
// (Apple event kAEGetURL, with the sender's bundle id) and the default-browser setting.
package handler

/*
#cgo CFLAGS: -x objective-c -fobjc-arc -fmodules
#cgo LDFLAGS: -framework Cocoa
#include <stdlib.h>
#include "handler.h"
*/
import "C"

import (
	"errors"
	"strings"
	"unsafe"
)

// Handler is called for every link the app receives; it runs on its own goroutine. The app quits
// two seconds after the last link has been handled, never while one is still being routed.
type Handler func(url, opener string)

var handle Handler

//export porteroHandleURL
func porteroHandleURL(url, opener *C.char) {
	u, o := C.GoString(url), C.GoString(opener)
	go func() {
		defer C.porteroDone() // always: the app must not stay alive because routing failed
		if handle != nil {
			handle(u, o)
		}
	}()
}

// Run starts the app loop and never returns; macOS delivers links to h.
func Run(h Handler) {
	handle = h
	C.porteroRunApp()
}

// Schemes Portero handles.
var Schemes = []string{"http", "https"}

// DefaultHandler returns the bundle id of the app that opens scheme links now.
func DefaultHandler(scheme string) string {
	cs := C.CString(scheme)
	defer C.free(unsafe.Pointer(cs))
	out := C.porteroDefaultHandler(cs)
	defer C.free(unsafe.Pointer(out))
	return C.GoString(out)
}

// DefaultApp returns the path of the app that opens scheme links now ("" when none).
func DefaultApp(scheme string) string {
	cs := C.CString(scheme)
	defer C.free(unsafe.Pointer(cs))
	out := C.porteroDefaultAppPath(cs)
	defer C.free(unsafe.Pointer(out))
	return C.GoString(out)
}

// BundleVersion returns CFBundleShortVersionString and CFBundleVersion of the app bundle at appPath.
func BundleVersion(appPath string) (short, build string) {
	cp := C.CString(appPath)
	defer C.free(unsafe.Pointer(cp))
	out := C.porteroBundleVersion(cp)
	defer C.free(unsafe.Pointer(out))
	short, build, _ = strings.Cut(C.GoString(out), "\n")
	return short, build
}

// SetDefaultHandler makes the app bundle at appPath the handler for scheme (macOS asks the user).
func SetDefaultHandler(appPath, scheme string) error {
	cp, cs := C.CString(appPath), C.CString(scheme)
	defer C.free(unsafe.Pointer(cp))
	defer C.free(unsafe.Pointer(cs))
	out := C.porteroSetDefaultHandler(cp, cs)
	defer C.free(unsafe.Pointer(out))
	if msg := C.GoString(out); msg != "" {
		return errors.New(msg)
	}
	return nil
}

// StartDownload asks iCloud to download an offloaded file. It returns at once; the data arrives later.
func StartDownload(path string) error {
	cp := C.CString(path)
	defer C.free(unsafe.Pointer(cp))
	out := C.porteroStartDownload(cp)
	defer C.free(unsafe.Pointer(out))
	if msg := C.GoString(out); msg != "" {
		return errors.New(msg)
	}
	return nil
}
