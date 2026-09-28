//go:build windows

package legacyprobe

import (
	"errors"

	"golang.org/x/sys/windows"
)

// mutexExists opens the named mutex without taking it. A mutex that exists
// but refuses access still counts.
func mutexExists(name string) bool {
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return false
	}
	h, err := windows.OpenMutex(windows.SYNCHRONIZE, false, namePtr)
	if err == nil {
		_ = windows.CloseHandle(h)
		return true
	}
	return errors.Is(err, windows.ERROR_ACCESS_DENIED)
}
