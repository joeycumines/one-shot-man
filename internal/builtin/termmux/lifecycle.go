package termmux

import (
	"context"

	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

type managerLifecycle interface {
	Started() <-chan struct{}
	Done() <-chan struct{}
}

func managerStarted(mgr managerLifecycle) bool {
	select {
	case <-mgr.Started():
		return true
	default:
		return false
	}
}

// managerRunOnce starts a manager only when its worker has not already been
// started by the caller. Run and close admission share managerRunMu so close
// cannot resolve before a concurrently admitted run is handled.
func (s *muxState) managerRunOnce() {
	s.runOnce.Do(func() {
		s.managerRunMu.Lock()
		if s.managerCloseRequested {
			s.managerRunMu.Unlock()
			s.finishManagerRun(nil)
			return
		}
		if managerStarted(s.mgr) {
			s.managerRunStarted = true
			s.managerRunMu.Unlock()
			go s.initializeManagerCache()
			go s.observeManagerRun()
			return
		}
		s.managerRunStarted = true
		s.managerRunMu.Unlock()
		go func() {
			s.managerRunMu.Lock()
			cancelledBeforeStart := s.managerCloseRequested
			s.managerRunMu.Unlock()
			if cancelledBeforeStart {
				s.finishManagerRun(nil)
				return
			}
			go func() {
				select {
				case <-s.mgr.Started():
					s.initializeManagerCache()
				case <-s.mgr.Done():
				case <-s.lifecycleCtx.Done():
				}
			}()
			err := s.mgr.Run(s.lifecycleCtx)
			s.finishManagerRun(err)
		}()
	})
}

func (s *muxState) observeManagerRun() {
	go func() {
		<-s.mgr.Done()
		s.finishManagerRun(nil)
	}()
}

func (s *muxState) finishManagerRun(err error) {
	s.managerRunMu.Lock()
	s.managerRunErr = err
	s.managerRunMu.Unlock()
	select {
	case <-s.managerRunDone:
	default:
		close(s.managerRunDone)
	}
	s.lifecycleOnce.Do(func() {
		close(s.lifecycleDone)
		s.lifecycleCancel()
	})
}

func (s *muxState) closeManagerPromise() goja.Value {
	return s.adapter.TrackPromise(s.ctx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
		select {
		case <-ctx.Done():
			_ = settle.Settle(true, func(owner *goja.Runtime) any { return owner.NewGoError(ctx.Err()) })
			return
		default:
		}

		s.managerRunMu.Lock()
		started := managerStarted(s.mgr)
		finished := false
		select {
		case <-s.managerRunDone:
			finished = true
		default:
		}
		admitted := s.managerRunStarted
		if !started && !finished && !admitted {
			s.managerCloseRequested = true
			s.managerRunMu.Unlock()
			_ = settle.Settle(false, func(*goja.Runtime) any { return goja.Undefined() })
			return
		}
		s.managerCloseRequested = true
		s.managerRunMu.Unlock()

		if !started && !finished {
			select {
			case <-s.mgr.Started():
				started = true
			case <-s.managerRunDone:
				finished = true
			case <-ctx.Done():
				_ = settle.Settle(true, func(owner *goja.Runtime) any { return owner.NewGoError(ctx.Err()) })
				return
			}
		}
		if started {
			s.mgr.Close()
		}
		_ = settle.Settle(false, func(*goja.Runtime) any { return goja.Undefined() })
	})
}
