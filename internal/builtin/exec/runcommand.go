package exec

import (
	"context"
	"errors"
	osexec "os/exec"
)

// RunCommand owns cmd's process-tree lifetime and active cancellation through
// ctx. The command's configured WaitDelay still bounds output copying after
// process exit; os/exec's independent cancellation watchdog is disabled.
func RunCommand(ctx context.Context, cmd *osexec.Cmd) (resultErr error) {
	if ctx == nil {
		return errors.New("exec: nil context")
	}
	if cmd == nil {
		return errors.New("exec: nil command")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	tree, err := newProcessTree()
	if err != nil {
		return err
	}
	setProcAttr(cmd, false)
	// Start admits watchCtx when either Cancel or WaitDelay is nonzero. Its
	// delayed Process.Kill bypasses the tree gate, so disable both during
	// Start. No admitted copy goroutine reads WaitDelay. Restoring it before
	// Wait retains only awaitGoroutines' pipe-drain bound.
	waitDelay := cmd.WaitDelay
	cmd.Cancel = nil
	startErr := func() error {
		cmd.WaitDelay = 0
		defer func() { cmd.WaitDelay = waitDelay }()
		return cmd.Start()
	}()
	if startErr != nil {
		return errors.Join(startErr, tree.close(cmd))
	}

	// Install only after Start: a callback that ran while Process was nil
	// could miss a child created immediately afterwards. Stopping or joining
	// the callback before return preserves the owner's complete lifetime.
	cancelDone := make(chan struct{})
	var cancelErr error
	stopCancel := context.AfterFunc(ctx, func() {
		defer close(cancelDone)
		cancelErr = errors.Join(ctx.Err(), tree.kill(cmd))
	})
	defer func() {
		if !stopCancel() {
			<-cancelDone
			resultErr = errors.Join(resultErr, cancelErr)
		}
	}()
	if err := tree.attach(cmd); err != nil {
		killErr := cmd.Process.Kill()
		closeErr := tree.close(cmd)
		tree.beginReap(false)
		waitErr := cmd.Wait()
		closeErr = retryProcessGroupClose(func() error { return tree.close(cmd) }, closeErr)
		return errors.Join(err, killErr, waitErr, closeErr)
	}
	waitErr, closeErr := waitAndCloseProcessTree(cmd, tree)
	return errors.Join(waitErr, closeErr)
}
