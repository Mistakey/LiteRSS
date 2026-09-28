//go:build windows

package legacyprobe

import (
	"fmt"
	"os"
	"testing"

	"golang.org/x/sys/windows"
)

func TestMutexExistsSeesAMutexOnlyWhileItIsHeld(t *testing.T) {
	name := fmt.Sprintf("literss-legacyprobe-test-%d", os.Getpid())
	if mutexExists(name) {
		t.Fatal("found a mutex nobody created")
	}
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		t.Fatal(err)
	}
	h, err := windows.CreateMutex(nil, false, namePtr)
	if err != nil {
		t.Fatal(err)
	}
	if !mutexExists(name) {
		windows.CloseHandle(h)
		t.Fatal("did not find the mutex while it is held")
	}
	if err := windows.CloseHandle(h); err != nil {
		t.Fatal(err)
	}
	if mutexExists(name) {
		t.Fatal("still found the mutex after its last handle closed")
	}
}
