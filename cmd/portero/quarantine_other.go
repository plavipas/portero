//go:build !darwin

package main

func quarantined(app string) bool { return false }
