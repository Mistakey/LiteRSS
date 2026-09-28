//go:build !windows

package update

import (
	"errors"
	"fmt"
	"os/exec"
	"syscall"
)

// start runs c in a session of its own, so it outlives LiteRSS.
func start(c Command) error {
	if c.Elevate {
		return errors.New("elevation is Windows only")
	}
	cmd := exec.Command(c.Path, c.Args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("%s: %w", c, err)
	}
	return cmd.Process.Release()
}
