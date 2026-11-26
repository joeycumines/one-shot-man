package exec

import (
	"errors"
	"fmt"
	"os"
	osexec "os/exec"
	"reflect"
	"testing"
)

// reapOrderTree records calls through the interface, so an embedded native
// implementation cannot accidentally bypass a wrapper's close result.
type reapOrderTree struct {
	t           *testing.T
	cmd         *osexec.Cmd
	events      []string
	closeErr    error
	signalErr   error
	signalCalls int
}

func (*reapOrderTree) attach(*osexec.Cmd) error { return nil }
func (*reapOrderTree) kill(*osexec.Cmd) error   { return nil }
func (tree *reapOrderTree) signal(*osexec.Cmd, os.Signal) error {
	tree.signalCalls++
	return tree.signalErr
}
func (tree *reapOrderTree) beginReap(observed bool) {
	if tree.cmd.ProcessState != nil {
		tree.t.Error("beginReap ran after Cmd.Wait published ProcessState")
	}
	tree.events = append(tree.events, fmt.Sprintf("begin_reap_%t", observed))
}
func (tree *reapOrderTree) close(cmd *osexec.Cmd) error {
	phase := "unreaped"
	if cmd.ProcessState != nil {
		phase = "reaped"
	}
	tree.events = append(tree.events, "close_"+phase)
	return tree.closeErr
}

func TestWaitAndCloseProcessTree_SealsEveryWaitPath(t *testing.T) {
	skipSlow(t)
	for _, tc := range []struct {
		name     string
		observed bool
		probeErr error
		want     []string
	}{
		{name: "observed_exit", observed: true, want: []string{"close_unreaped", "begin_reap_true"}},
		{name: "unsupported", want: []string{"close_unreaped", "begin_reap_false"}},
		{name: "probe_error", probeErr: errors.New("observation failed"), want: []string{"close_unreaped", "begin_reap_false"}},
	} {
		for _, failClose := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/close_failure_%t", tc.name, failClose), func(t *testing.T) {
				command, args := immediateExitCommand()
				cmd := osexec.Command(command, args...)
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				tree := &reapOrderTree{t: t, cmd: cmd}
				if failClose {
					tree.closeErr = errors.New("wrapper close failed")
				}
				waitErr, closeErr := waitAndCloseProcessTreeWithProbe(cmd, tree, func(*osexec.Cmd) (bool, error) {
					return tc.observed, tc.probeErr
				})
				var exitErr *osexec.ExitError
				if waitErr != nil && (tc.observed || !errors.As(waitErr, &exitErr)) {
					t.Fatal(waitErr)
				}
				if tc.observed {
					if !errors.Is(closeErr, tree.closeErr) {
						t.Fatalf("close=%v, want wrapper result %v", closeErr, tree.closeErr)
					}
				} else {
					probeErr := tc.probeErr
					if probeErr == nil {
						probeErr = errProcessObservationUnavailable
					}
					if !errors.Is(closeErr, probeErr) || (failClose && !errors.Is(closeErr, tree.closeErr)) {
						t.Fatalf("close=%v lost probe/wrapper outcome", closeErr)
					}
				}
				if !reflect.DeepEqual(tree.events, tc.want) {
					t.Fatalf("events = %v, want %v", tree.events, tc.want)
				}
			})
		}
	}
}

func TestChildProcessSignalUsesTreeOwner(t *testing.T) {
	want := errors.New("owner declined unsafe group signal")
	tree := &reapOrderTree{signalErr: want}
	child := &ChildProcess{cmd: &osexec.Cmd{}, tree: tree, done: make(chan struct{})}
	if err := child.Signal("SIGTERM"); !errors.Is(err, want) {
		t.Fatalf("Signal = %v, want owner failure %v", err, want)
	}
	if tree.signalCalls != 1 {
		t.Fatalf("owner signal calls = %d, want 1", tree.signalCalls)
	}
}
