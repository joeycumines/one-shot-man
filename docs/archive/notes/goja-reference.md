# "goja" Module Reference

> NOTICE: These are reference notes.

## Introduction

This document surfaces notable features, usage patterns, and implementation notes regarding the [github.com/dop251/goja](https://pkg.go.dev/github.com/dop251/goja) module, which implements a JavaScript virtual machine in Go. It is intended as a high-level reference to complement the full API documentation.

-----

## The `Runtime`

The `goja.Runtime` is the core of the library, representing an isolated JavaScript VM instance. Each `Runtime` is a self-contained environment with its own global object and state. Multiple `Runtime` instances can be created and used concurrently, but `goja.Value` objects cannot be shared or passed between them.

A new runtime is instantiated via `goja.New()`. Interaction with the runtime's global scope is primarily handled through two methods:

  * `Runtime.Set(name string, value interface{})`: Binds a Go value to a global JavaScript variable. The value is first converted to a `goja.Value` using the rules of `ToValue`.
  * `Runtime.Get(name string) Value`: Retrieves a global JavaScript variable as a `goja.Value`.

Direct access to the global object (`globalThis` in JavaScript) is available via `Runtime.GlobalObject()`.

-----

## Executing Scripts

While `goja` provides methods like `RunString()` and `RunScript()` for direct execution of source code, a more performant and reusable pattern involves pre-compilation.

1.  **Compilation**: The `goja.Compile(name, src, strict)` function parses and compiles JavaScript source code into a `*goja.Program` object. A `Program` is a thread-safe, runtime-independent representation of the script. It can be cached and reused across multiple `Runtime` instances, even concurrently.
2.  **Execution**: The `Runtime.RunProgram(p *Program)` method executes a compiled `Program` within the runtime's context.

This two-step process avoids the overhead of parsing and compiling the same script repeatedly, making it ideal for applications that execute a fixed set of scripts multiple times.

```go
// Compile the script once and reuse it.
program, err := goja.Compile("main.js", `function add(a, b) { return a + b; }`, false)
if err != nil {
    // Handle compilation error
}

vm1 := goja.New()
_, err = vm1.RunProgram(program)
// ...

vm2 := goja.New()
_, err = vm2.RunProgram(program)
// ...
```

-----

## Type Mapping and Conversion

Interoperability between Go and JavaScript requires a robust system for mapping types and converting values between the two environments. `goja` provides several mechanisms to manage this data exchange.

### Go to JavaScript (`Runtime.ToValue`)

The `Runtime.ToValue(i interface{})` method is the primary mechanism for converting a Go value into a `goja.Value`.

  * **Primitives**: Go's primitive types (`string`, `int`, `float64`, `bool`) are converted to their direct JavaScript equivalents. `nil` is converted to `null`.
  * **Structs, Slices, Maps**: These Go types are not copied but are **wrapped** in a `goja.Object`. This means modifications made in JavaScript to the object's properties or elements are reflected in the original Go value. This behavior has important implications:
      * Assigning a standard JavaScript object as an element of a wrapped Go slice or map will cause the JavaScript object to be exported (copied) into a corresponding Go type.
      * Due to Go's value semantics, accessing nested non-pointer structs or slices can lead to copy-on-change behavior that may be unintuitive from a JavaScript perspective where all objects are references. Passing pointers to composite types (`*MyStruct`, `*[]MyType`) ensures modifications are consistently applied.
  * **Functions**: Go functions are wrapped into callable JavaScript functions. `goja` supports two modes:
    1.  `func(goja.FunctionCall) goja.Value`: A low-level, high-performance signature that gives direct access to the call arguments and requires manual handling of the return value.
    2.  Other function signatures: `goja` uses reflection to automatically convert JavaScript arguments to the required Go types and convert the Go return value back to a `goja.Value`. If the last return type is an `error`, it will be converted into a JavaScript exception if non-nil.

### JavaScript to Go (`Value.Export` and `Runtime.ExportTo`)

Converting a `goja.Value` back into a Go type can be done in two ways.

  * **`Value.Export() interface{}`**: This method performs a default conversion of a `goja.Value` into a suitable plain Go type.

      * `null` and `undefined` become `nil`.
      * Numbers become `int64` if they are whole, otherwise `float64`.
      * Strings become `string`.
      * Booleans become `bool`.
      * A standard `Object` becomes a `map[string]interface{}`.
      * An `Array` becomes an `[]interface{}`.
      * If the `Value` wraps a Go type, the original Go value is returned.

  * **`Runtime.ExportTo(v Value, target interface{}) error`**: This method provides more control by converting a `goja.Value` into a specific Go type pointed to by the `target` argument (which must be a non-nil pointer). This is particularly useful for converting a JavaScript function into a statically typed Go function variable, or for populating slices and maps of specific types.

### JavaScript Type Coercion (`Value.To...` methods)

The `Value` interface provides methods that mimic JavaScript's internal type coercion rules, such as `ToInteger()`, `ToString()`, and `ToBoolean()`. These methods are generally safe and follow predictable ECMAScript semantics (e.g., `ToInteger()` on `undefined` yields `0`).

However, `Value.ToObject()` will panic if called on `null` or `undefined` values, consistent with JavaScript's `Object()` constructor behavior. This requires careful handling when converting arbitrary values.

-----

## Advanced Interoperability

For more complex integration patterns, `goja` offers powerful constructs that provide fine-grained control over how Go types are exposed in the JavaScript runtime.

### Dynamic Objects and Arrays

The `DynamicObject` and `DynamicArray` interfaces allow a Go type to implement custom behavior for property access (`get`, `set`, `has`, `delete`, `keys`).

By implementing these interfaces and creating an object with `Runtime.NewDynamicObject()` or `Runtime.NewDynamicArray()`, you can expose Go data structures that do not map cleanly to standard maps or slices. This can be used to create virtual objects backed by a database, a configuration store, or any other Go service. This pattern is often more efficient than using a `Proxy` because it avoids certain invariant checks.

### Promises

`goja` supports asynchronous operations through the `goja.Promise` type. A new promise and its corresponding resolving functions can be created in Go using `Runtime.NewPromise()`.

```go
p, resolve, reject := vm.NewPromise()

// Set the promise in the JS environment
vm.Set("myPromise", p)

// In a separate goroutine, perform a task and resolve the promise
go func() {
    result, err := someLongRunningTask()
    if err != nil {
        // It is crucial to schedule the rejection on the runtime's thread/event loop
        // to avoid race conditions.
        reject(err)
        return
    }
    // Schedule the resolution on the runtime's thread/event loop.
    resolve(result)
}()
```

**Warning**: The `resolve` and `reject` functions are **not goroutine-safe** and must not be called concurrently while the VM is running. They should be scheduled for execution on the same goroutine that owns the `Runtime`, typically via an event loop mechanism (as demonstrated in the `goja_nodejs` project).

-----

## Error Handling

JavaScript exceptions are propagated in `goja` as Go **panics**. The value of the panic is a `*goja.Exception`, which wraps the original JavaScript error `Value` and provides access to the stack trace.

To safely interact with the JavaScript runtime from Go, it is essential to catch these panics. The idiomatic way to do this is with the `Runtime.Try()` function. It accepts a function literal, executes it, and catches any `*goja.Exception` that may be thrown, returning it as a standard Go `error`.

```go
var result goja.Value
err := vm.Try(func() {
    // Any goja call that can throw a JS exception should be wrapped.
    // This includes property access, function calls, etc.
    result = obj.Get("potentiallyMissingProperty")
    if goja.IsUndefined(result) {
        panic(vm.NewTypeError("Property is missing"))
    }
})

if err != nil {
    // Handle the JavaScript exception as a Go error.
    if exception, ok := err.(*goja.Exception); ok {
        fmt.Println("JavaScript exception:", exception.Value())
    } else {
        fmt.Println("Go error:", err)
    }
}
```

Additionally, a running script can be forcefully stopped using `Runtime.Interrupt()`, which causes the runtime to return a `*goja.InterruptedError`.
