package scripting

import (
	"errors"
	"fmt"

	"github.com/joeycumines/goja"
)

func (rt *Runtime) bindProcessLifecycle() error {
	processValue := rt.vm.Get("process")
	process, ok := processValue.(*goja.Object)
	if !ok {
		return errors.New("process object is unavailable")
	}

	on, ok := goja.AssertFunction(process.Get("on"))
	if !ok {
		return errors.New("process.on is unavailable")
	}
	exitListener := rt.vm.ToValue(func(call goja.FunctionCall) goja.Value {
		exitCode := process.Get("exitCode")
		if exitCode != nil && !goja.IsUndefined(exitCode) && !goja.IsNull(exitCode) {
			rt.processExitCode.Store(exitCode.ToInteger())
			rt.processExitCodeSet.Store(true)
		}
		return goja.Undefined()
	})
	if _, err := on(process, rt.vm.ToValue("exit"), exitListener); err != nil {
		return fmt.Errorf("register process exit listener: %w", err)
	}

	exit, ok := goja.AssertFunction(process.Get("exit"))
	if !ok {
		return errors.New("process.exit is unavailable")
	}
	if err := process.Set("exit", func(call goja.FunctionCall) goja.Value {
		rt.processExitRequested.Store(true)
		result, err := exit(process, call.Arguments...)
		if err != nil {
			var interrupted *goja.InterruptedError
			if !errors.As(err, &interrupted) {
				rt.processExitRequested.Store(false)
				if exception, ok := err.(*goja.Exception); ok {
					panic(exception.Value())
				}
				panic(rt.vm.NewGoError(err))
			}
		}
		return result
	}); err != nil {
		return fmt.Errorf("wrap process.exit: %w", err)
	}
	return nil
}

// ExitCode reports the script's settled Node exit status: the code published
// by process.exit, by an assignment to process.exitCode, or by a fatal path.
// The ok result is false when the script never set an exit code. It is safe to
// call after script execution or Wait.
func (e *Engine) ExitCode() (code int, ok bool) {
	if e.runtime == nil {
		return 0, false
	}
	return e.runtime.ExitCode()
}

// IsProcessExitSignal reports whether err is the interruption requested by
// the wrapped process.exit function.
func (e *Engine) IsProcessExitSignal(err error) bool {
	if e.runtime == nil || err == nil || !e.runtime.processExitRequested.Load() {
		return false
	}
	var interrupted *goja.InterruptedError
	return errors.As(err, &interrupted)
}

// DeliverSignal sends a POSIX-style signal name to process.emit and reports
// whether a script listener handled it.
func (e *Engine) DeliverSignal(name string) bool {
	delivered := false
	err := e.executeOnLoop(func(rt *goja.Runtime) error {
		procVal := rt.Get("process")
		if procVal == nil || goja.IsUndefined(procVal) || goja.IsNull(procVal) {
			return nil
		}
		procObj, ok := procVal.(*goja.Object)
		if !ok {
			return nil
		}
		emit, ok := goja.AssertFunction(procObj.Get("emit"))
		if !ok {
			return nil
		}
		result, err := emit(procObj, rt.ToValue(name))
		if err != nil {
			if e.IsProcessExitSignal(err) {
				delivered = true
				return nil
			}
			return err
		}
		delivered = result != nil && result.ToBoolean()
		return nil
	})
	if err != nil {
		e.Logger().Error("signal delivery failed", "signal", name, "error", err)
		return false
	}
	return delivered
}
