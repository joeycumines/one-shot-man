package exec

import (
	"errors"
	osexec "os/exec"
)

// RunCommand runs cmd with the same process-tree lifetime as the exec module.
func RunCommand(cmd *osexec.Cmd) error {
	if cmd == nil {
		return errors.New("exec: nil command")
	}
	tree, err := newProcessTree()
	if err != nil {
		return err
	}
	setProcAttr(cmd)
	cmd.Cancel = func() error {
		return tree.kill(cmd)
	}
	if err := cmd.Start(); err != nil {
		return errors.Join(err, tree.close(cmd))
	}
	if err := tree.attach(cmd); err != nil {
		killErr := cmd.Process.Kill()
		waitErr := cmd.Wait()
		closeErr := tree.close(cmd)
		return errors.Join(err, killErr, waitErr, closeErr)
	}
	waitErr, closeErr := waitAndCloseProcessTree(cmd, tree)
	return errors.Join(waitErr, closeErr)
}
