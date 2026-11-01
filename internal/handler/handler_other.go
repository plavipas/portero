//go:build !darwin

package handler

import "errors"

type Handler func(url, opener string)

var Schemes = []string{"http", "https"}

var errUnsupported = errors.New("the link handler app exists only on macOS")

func Run(h Handler)                                  { panic(errUnsupported) }
func DefaultHandler(scheme string) string            { return "" }
func SetDefaultHandler(appPath, scheme string) error { return errUnsupported }
