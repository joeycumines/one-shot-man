package eventlooputil

import (
	"context"
	"time"

	goeventloop "github.com/joeycumines/go-eventloop"
	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	"github.com/joeycumines/goroutineid"
)

func IsLoopThread(storedID int64) bool {
	return storedID != 0 && goroutineid.Get() == storedID
}

func RunSync(ctx context.Context, loop *goeventloop.Loop, adapter *gojaeventloop.Adapter, vm *goja.Runtime, storedID int64, timeout time.Duration, fn func(*goja.Runtime) error) error {
	if IsLoopThread(storedID) {
		return fn(vm)
	}
	if loop == nil {
		return context.Canceled
	}
	done := make(chan error, 1)
	err := loop.Submit(func() {
		done <- fn(vm)
	})
	if err != nil {
		return err
	}
	if timeout > 0 {
		timer := time.NewTimer(timeout)
		defer timer.Stop()
		select {
		case err := <-done:
			return err
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return context.DeadlineExceeded
		}
	}
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
