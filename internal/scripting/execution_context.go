package scripting

import (
	"fmt"
	"strings"

	"github.com/dop251/goja"
)

// ExecutionContext provides the execution environment for scripts, similar to testing.T.
type ExecutionContext struct {
	engine   *Engine
	script   *Script
	name     string
	parent   *ExecutionContext
	failed   bool
	output   strings.Builder
	deferred []func()
}

// Run executes a sub-test, similar to testing.T.Run() (Go-style method for internal use).
func (ctx *ExecutionContext) Run(name string, fn goja.Callable) bool {
	subCtx := &ExecutionContext{
		engine: ctx.engine,
		script: ctx.script,
		name:   fmt.Sprintf("%s/%s", ctx.name, name),
		parent: ctx,
	}

	// Set up the sub-context in JavaScript with both Go-style and JS-style methods
	contextObj := map[string]interface{}{
		// JavaScript-style methods (camelCase)
		"run":    subCtx.Run,
		"defer":  subCtx.Defer,
		"log":    subCtx.Log,
		"logf":   subCtx.Logf,
		"error":  subCtx.Error,
		"errorf": subCtx.Errorf,
		"fatal":  subCtx.Fatal,
		"fatalf": subCtx.Fatalf,
		"failed": subCtx.Failed,
		"name":   subCtx.Name,
	}
	ctx.engine.vm.Set("ctx", contextObj)

	// Execute the test function
	_, err := fn(goja.Undefined())
	if err != nil {
		subCtx.failed = true
		subCtx.Errorf("Test failed: %v", err)
	}

	// Run deferred functions for sub-context
	if err := subCtx.runDeferred(); err != nil {
		subCtx.failed = true
		subCtx.Errorf("Deferred function failed: %v", err)
	}

	// Restore parent context
	if ctx.parent != nil {
		parentObj := map[string]interface{}{
			// JavaScript-style methods (camelCase)
			"run":    ctx.parent.Run,
			"defer":  ctx.parent.Defer,
			"log":    ctx.parent.Log,
			"logf":   ctx.parent.Logf,
			"error":  ctx.parent.Error,
			"errorf": ctx.parent.Errorf,
			"fatal":  ctx.parent.Fatal,
			"fatalf": ctx.parent.Fatalf,
			"failed": ctx.parent.Failed,
			"name":   ctx.parent.Name,
		}
		ctx.engine.vm.Set("ctx", parentObj)
	} else {
		currentObj := map[string]interface{}{
			// JavaScript-style methods
			"run":    ctx.Run,
			"defer":  ctx.Defer,
			"log":    ctx.Log,
			"logf":   ctx.Logf,
			"error":  ctx.Error,
			"errorf": ctx.Errorf,
			"fatal":  ctx.Fatal,
			"fatalf": ctx.Fatalf,
			"failed": ctx.Failed,
			"name":   ctx.Name,
		}
		ctx.engine.vm.Set("ctx", currentObj)
	}

	// Report result
	if subCtx.failed {
		ctx.Errorf("Sub-test %s failed", name)
		return false
	}

	ctx.Logf("Sub-test %s passed", name)
	return true
}

// Defer schedules a function to be executed when the current context completes.
func (ctx *ExecutionContext) Defer(fn goja.Callable) {
	ctx.deferred = append(ctx.deferred, func() {
		_, err := fn(goja.Undefined())
		if err != nil {
			ctx.Errorf("Deferred function failed: %v", err)
		}
	})
}

// Log logs a message to the test output (Go-style method for internal use).
func (ctx *ExecutionContext) Log(args ...interface{}) {
	fmt.Fprintf(&ctx.output, "[%s] %s\n", ctx.name, fmt.Sprint(args...))
	if ctx.engine.testMode {
		fmt.Fprintf(ctx.engine.stdout, "[%s] %s\n", ctx.name, fmt.Sprint(args...))
	}
}

// Logf logs a formatted message to the test output (Go-style method for internal use).
func (ctx *ExecutionContext) Logf(format string, args ...interface{}) {
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(&ctx.output, "[%s] %s\n", ctx.name, msg)
	if ctx.engine.testMode {
		fmt.Fprintf(ctx.engine.stdout, "[%s] %s\n", ctx.name, msg)
	}
}

// Error marks the current test as failed and logs an error message.
func (ctx *ExecutionContext) Error(args ...interface{}) {
	ctx.failed = true
	msg := fmt.Sprint(args...)
	fmt.Fprintf(&ctx.output, "[%s] ERROR: %s\n", ctx.name, msg)
	fmt.Fprintf(ctx.engine.stderr, "[%s] ERROR: %s\n", ctx.name, msg)
}

// Errorf marks the current test as failed and logs a formatted error message.
func (ctx *ExecutionContext) Errorf(format string, args ...interface{}) {
	ctx.failed = true
	msg := fmt.Sprintf(format, args...)
	fmt.Fprintf(&ctx.output, "[%s] ERROR: %s\n", ctx.name, msg)
	fmt.Fprintf(ctx.engine.stderr, "[%s] ERROR: %s\n", ctx.name, msg)
}

// Fatal marks the current test as failed and stops execution.
func (ctx *ExecutionContext) Fatal(args ...interface{}) {
	ctx.Error(args...)
	panic("test failed")
}

// Fatalf marks the current test as failed and stops execution with a formatted message.
func (ctx *ExecutionContext) Fatalf(format string, args ...interface{}) {
	ctx.Errorf(format, args...)
	panic("test failed")
}

// Failed reports whether the current test has failed.
func (ctx *ExecutionContext) Failed() bool {
	return ctx.failed
}

// Name returns the name of the current test.
func (ctx *ExecutionContext) Name() string {
	return ctx.name
}

// runDeferred executes all deferred functions for this context.
func (ctx *ExecutionContext) runDeferred() error {
	// Execute deferred functions in reverse order (LIFO)
	for i := len(ctx.deferred) - 1; i >= 0; i-- {
		func() {
			defer func() {
				if r := recover(); r != nil {
					ctx.Errorf("Deferred function panicked: %v", r)
				}
			}()
			ctx.deferred[i]()
		}()
	}

	if ctx.failed {
		return fmt.Errorf("test context failed")
	}
	return nil
}
