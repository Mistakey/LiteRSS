//go:build windows

package update

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"

	"golang.org/x/sys/windows"
)

// start launches c without waiting. The arguments go on the command line
// as written: NSIS reads /D= up to the end of the line and does not accept it
// quoted. An elevated command goes through ShellExecute's "runas", which
// shows UAC once.
func start(c Command) error {
	args := strings.Join(c.Args, " ")
	if c.Elevate {
		verb, _ := windows.UTF16PtrFromString("runas")
		file, err := windows.UTF16PtrFromString(c.Path)
		if err != nil {
			return err
		}
		params, err := windows.UTF16PtrFromString(args)
		if err != nil {
			return err
		}
		err = windows.ShellExecute(0, verb, file, params, nil, windows.SW_SHOWNORMAL)
		if errors.Is(err, windows.ERROR_CANCELLED) {
			return ErrCancelled
		}
		if err != nil {
			return fmt.Errorf("%s: %w", c, err)
		}
		return nil
	}
	cmd := exec.Command(c.Path)
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: syscall.EscapeArg(c.Path) + " " + args}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s: %w", c, err)
	}
	return cmd.Process.Release()
}
