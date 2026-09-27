package scripting

import (
	"errors"

	"github.com/joeycumines/goja"
	gojaEventloop "github.com/joeycumines/goja-eventloop"
)

// ExitCode reports the script's settled Node exit status: the code published
// by process.exit, by an assignment to process.exitCode, or by a fatal path.
// The ok result is false when the script never set an exit code. It is safe to
// call after script execution or Wait.
func (e *Engine) ExitCode() (code int, ok bool) {
	adapter := e.Adapter()
	if adapter == nil {
		return 0, false
	}
	return adapter.ExitCode()
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
			if IsProcessExitSignal(err) {
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

// IsProcessExitSignal reports whether err represents a controlled process.exit request.
func IsProcessExitSignal(err error) bool {
	var exitSignal gojaEventloop.ProcessExitSignal
	return errors.As(err, &exitSignal)
}
