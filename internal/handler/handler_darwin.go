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
	"unsafe"
)

// Handler is called for every link the app receives; it runs on its own goroutine.
type Handler func(url, opener string)

var handle Handler

//export porteroHandleURL
func porteroHandleURL(url, opener *C.char) {
	u, o := C.GoString(url), C.GoString(opener)
	go func() {
		if handle != nil {
			handle(u, o)
		}
		C.porteroExitAfter(2000)
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
