package shell

import (
	"bytes"
	"encoding/xml"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

func launchCommand(exe string) string { return exe }

// launchAgents writes ~/Library/LaunchAgents/<name>.plist with <name> as the
// agent label.
type launchAgents struct{}

func platformLoginItems() loginItems { return launchAgents{} }

func (launchAgents) path(name string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, "Library", "LaunchAgents", name+".plist"), nil
}

func (a launchAgents) set(name, command string) error {
	path, err := a.path(name)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var label, program bytes.Buffer
	_ = xml.EscapeText(&label, []byte(name))
	_ = xml.EscapeText(&program, []byte(command))
	plist := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + label.String() + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + program.String() + `</string>
	</array>
	<key>RunAtLoad</key>
	<true/>
</dict>
</plist>
`
	return os.WriteFile(path, []byte(plist), 0o644)
}

func (a launchAgents) remove(name string) error {
	path, err := a.path(name)
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}
