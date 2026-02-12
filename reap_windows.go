//go:build windows

package main

// reapChildren is a no-op on Windows.
func reapChildren() {}
