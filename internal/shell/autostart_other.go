//go:build !windows && !darwin

package shell

import "errors"

func launchCommand(exe string) string { return exe }

// Only Windows and macOS are supported (spec D3).
type unsupported struct{}

func platformLoginItems() loginItems { return unsupported{} }

var errUnsupported = errors.New("launch at login is only supported on Windows and macOS")

func (unsupported) set(string, string) error { return errUnsupported }
func (unsupported) remove(string) error      { return nil }
