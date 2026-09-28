package shell

import (
	"errors"

	"golang.org/x/sys/windows/registry"
)

// RunKey is the per-user key whose values Windows starts at sign-in.
const RunKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// launchCommand quotes the path: a Run value is a command line.
func launchCommand(exe string) string {
	return `"` + exe + `"`
}

type runKey struct{}

func platformLoginItems() loginItems { return runKey{} }

func (runKey) set(name, command string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, RunKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	return k.SetStringValue(name, command)
}

func (runKey) remove(name string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, RunKey, registry.SET_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(name); err != nil && !errors.Is(err, registry.ErrNotExist) {
		return err
	}
	return nil
}
