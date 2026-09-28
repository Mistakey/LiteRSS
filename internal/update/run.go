package update

import (
	"fmt"
	"log"
	"os/exec"
)

// RunPlan runs a plan's steps in order: a Detach step is started and left
// running, the others must exit successfully. When a step fails, Cleanup runs
// and the error is returned.
func RunPlan(p Plan) error {
	for _, c := range p.Steps {
		if err := runCommand(c); err != nil {
			for _, undo := range p.Cleanup {
				if uerr := runCommand(undo); uerr != nil {
					log.Printf("Update cleanup %s: %v", undo, uerr)
				}
			}
			return err
		}
	}
	return nil
}

func runCommand(c Command) error {
	if c.Detach || c.Elevate {
		return start(c)
	}
	if out, err := exec.Command(c.Path, c.Args...).CombinedOutput(); err != nil {
		return fmt.Errorf("%s: %w: %s", c, err, out)
	}
	return nil
}
