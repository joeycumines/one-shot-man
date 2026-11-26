//go:build unix

package exec

import (
	"errors"
	"fmt"
	osexec "os/exec"
	"sync"
	"syscall"
	"time"
)

// Darwin's killpg1 excludes zombies and returns EPERM if it finds no eligible
// group member. After reaping our leader, allow orphaned zombies up to one
// second to be reaped too. Only ESRCH proves that the group no longer exists;
// a process-table snapshot can omit a concurrently forking or exiting member.
// The same observation bounds cleanup when any Unix pre-reap probe fails.
// https://github.com/apple-oss-distributions/xnu/blob/f6217f891ac0bb64f3d375211650a4c1ff8ca1ea/bsd/kern/kern_sig.c#L1709-L1718
const processGroupExitGrace = time.Second

var errProcessGroupReaping = errors.New("cannot signal the process group after reaping has begun without an observed exit")

type unixProcessTree struct {
	mu           sync.Mutex
	reaping      bool
	exitObserved bool
	observeOnce  sync.Once
	observeErr   error
}

// beginReap waits for every admitted signal to finish while the unreaped
// leader still pins its PID, then permanently closes admission. Cmd.Wait must
// run after this method returns, without holding mu: Wait joins watchCtx,
// whose cancellation callback may itself call kill.
func (t *unixProcessTree) beginReap(observedExit bool) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.reaping {
		t.reaping = true
		t.exitObserved = observedExit
	}
}

func (t *unixProcessTree) signalGroup(cmd *osexec.Cmd, signal syscall.Signal, kill func(int, syscall.Signal) error) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !t.reaping {
		return signalProcessGroup(cmd, signal, kill)
	}
	if t.exitObserved {
		return nil
	}
	// Darwin's os.Process.Kill also uses a recyclable numeric PID. Once
	// Wait may run, neither group nor direct-process signalling is safe.
	// The fallback owner already attempted termination before sealing.
	return errProcessGroupReaping
}

func (t *unixProcessTree) closeGroup(cmd *osexec.Cmd, kill func(int, syscall.Signal) error) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return nil
	}
	if !t.reaping {
		// The gate prevents Cmd.Wait from starting until this signal returns.
		return killProcessGroup(cmd, kill)
	}
	// Never signal a former process-group ID after reap: the original group
	// may have vanished and its ID may already belong to another process.
	// Cache the bounded observation so retries cannot extend the deadline or
	// erase a failure that the owner has already observed.
	t.observeOnce.Do(func() {
		t.observeErr = observeProcessGroupExit(cmd.Process.Pid, kill)
	})
	return t.observeErr
}

func observeProcessGroupExit(pgid int, probe func(int, syscall.Signal) error) error {
	deadline := time.NewTimer(processGroupExitGrace)
	defer deadline.Stop()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	for {
		err := probe(-pgid, 0)
		if errors.Is(err, syscall.ESRCH) {
			return nil
		}
		if err != nil && !errors.Is(err, syscall.EPERM) {
			return fmt.Errorf("observe process group %d after reap: %w", pgid, err)
		}
		select {
		case <-deadline.C:
			return errors.Join(fmt.Errorf("process group %d still exists %s after child reap", pgid, processGroupExitGrace), err)
		case <-tick.C:
		}
	}
}
