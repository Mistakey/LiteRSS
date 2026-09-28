package shell

import (
	"fmt"
	"os"
	"path/filepath"

	"LiteRSS/internal/identity"
)

// Autostart registers this executable to start at login under the build
// identity's AutostartValueName, so an installed build, a development build
// and the legacy MrRSS ("MrRSS") never share an entry (spec D2).
type Autostart struct {
	name    string
	command string
	entries loginItems
}

// loginItems is the platform's list of programs started at login.
type loginItems interface {
	set(name, command string) error
	// remove deletes name; a missing entry is not an error.
	remove(name string) error
}

// NewAutostart returns the autostart entry of the running executable.
func NewAutostart() (*Autostart, error) {
	exe, err := os.Executable()
	if err != nil {
		return nil, fmt.Errorf("autostart: locate the executable: %w", err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return newAutostart(identity.Current().AutostartValueName, exe, platformLoginItems()), nil
}

func newAutostart(name, exe string, entries loginItems) *Autostart {
	return &Autostart{name: name, command: launchCommand(exe), entries: entries}
}

// Apply adds or removes the entry. Adding again rewrites the command, so a
// build that moved since it was registered starts from its new place.
func (a *Autostart) Apply(enabled bool) error {
	var err error
	if enabled {
		err = a.entries.set(a.name, a.command)
	} else {
		err = a.entries.remove(a.name)
	}
	if err != nil {
		return fmt.Errorf("autostart %s: %w", a.name, err)
	}
	return nil
}
