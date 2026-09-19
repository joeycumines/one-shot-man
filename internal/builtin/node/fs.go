// Package node registers Node-standard modules under their bare Node names
// ("fs", "net", "crypto"). The surface is async-only — every operation
// settles a promise — and implements the necessary subset with precise
// Node-26 behavior; the osm: prefix stays reserved for domain modules.
package node

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/joeycumines/goja"
	gojaeventloop "github.com/joeycumines/goja-eventloop"
)

// fsRequire returns the loader for require("fs"). Only fs.promises exists:
// Node's callback/sync surface is deliberately absent, matching the
// async-only mandate.
func FsRequire(ctx context.Context, adapter *gojaeventloop.Adapter) func(*goja.Runtime, *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)
		promises := runtime.NewObject()

		// readFile(path[, options]) -> Promise<Buffer|string>
		// With no options (Node's default encoding is null) the promise
		// resolves to a Uint8Array of the file's bytes. With an
		// options.encoding string, UTF-8 is the supported named encoding and
		// the promise resolves to a string; everything else is rejected.
		_ = promises.Set("readFile", func(call goja.FunctionCall) goja.Value {
			path, ok := stringArg(call, 0)
			if !ok {
				return rejectTypeError(adapter, "readFile", "The \"path\" argument must be of type string")
			}
			encoding := ""
			if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
				if opts, isObj := call.Argument(1).(*goja.Object); isObj {
					if enc := opts.Get("encoding"); enc != nil && !goja.IsUndefined(enc) && !goja.IsNull(enc) {
						encoding = strings.ToLower(enc.String())
					}
				} else {
					encoding = strings.ToLower(call.Argument(1).String())
				}
				if encoding != "" && encoding != "utf8" && encoding != "utf-8" {
					return rejectCode(adapter, "readFile", "ERR_INVALID_ARG_VALUE", "readFile: unsupported encoding "+encoding)
				}
			}
			return adapter.TrackPromise(ctx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
				data, err := os.ReadFile(path)
				if err != nil {
					_ = settle.Settle(true, func(rt *goja.Runtime) any { return nodeFSError(rt, "open", path, err) })
					return
				}
				_ = settle.Settle(false, func(rt *goja.Runtime) any {
					if encoding == "" {
						return bytesToUint8Array(rt, data)
					}
					return rt.ToValue(string(data))
				})
			})
		})

		// writeFile(path, data[, options]) -> Promise<void>
		// options: {encoding?, mode?, flag?} — Node's default flag is "w";
		// "wx" rejects an existing file with EEXIST, "a"/"ax" append. The
		// mode (default 0o666, masked by the process umask) applies only
		// when the file is created.
		_ = promises.Set("writeFile", func(call goja.FunctionCall) goja.Value {
			path, ok := stringArg(call, 0)
			if !ok {
				return rejectTypeError(adapter, "writeFile", "The \"path\" argument must be of type string")
			}
			data := call.Argument(1).String()
			mode := fs.FileMode(0o666)
			flag := "w"
			if len(call.Arguments) > 2 && !goja.IsUndefined(call.Argument(2)) && !goja.IsNull(call.Argument(2)) {
				if opts, isObj := call.Argument(2).(*goja.Object); isObj {
					if v := opts.Get("mode"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
						mode = fs.FileMode(v.ToInteger())
					}
					// Node documents options.flag; accept options.flags
					// as the common misspelling, matching Node's own
					// leniency in fs Promises API option parsing.
					for _, key := range []string{"flag", "flags"} {
						if v := opts.Get(key); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
							flag = v.String()
							break
						}
					}
				}
			}
			fsFlag, err := parseWriteFlag(flag)
			if err != nil {
				return rejectCode(adapter, "writeFile", "ERR_INVALID_ARG_VALUE", "writeFile: "+err.Error())
			}
			return adapter.TrackPromise(ctx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
				f, err := os.OpenFile(path, fsFlag, mode)
				if err != nil {
					_ = settle.Settle(true, func(rt *goja.Runtime) any { return nodeFSError(rt, "open", path, err) })
					return
				}
				defer f.Close()
				if _, err := f.WriteString(data); err != nil {
					_ = settle.Settle(true, func(rt *goja.Runtime) any { return nodeFSError(rt, "write", path, err) })
					return
				}
				if err := f.Close(); err != nil {
					_ = settle.Settle(true, func(rt *goja.Runtime) any { return nodeFSError(rt, "close", path, err) })
					return
				}
				_ = settle.Settle(false, func(*goja.Runtime) any { return goja.Undefined() })
			})
		})

		// unlink(path) -> Promise<void>
		_ = promises.Set("unlink", func(call goja.FunctionCall) goja.Value {
			path, ok := stringArg(call, 0)
			if !ok {
				return rejectTypeError(adapter, "unlink", "The \"path\" argument must be of type string")
			}
			return adapter.TrackPromise(ctx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
				if err := os.Remove(path); err != nil {
					_ = settle.Settle(true, func(rt *goja.Runtime) any { return nodeFSError(rt, "unlink", path, err) })
					return
				}
				_ = settle.Settle(false, func(*goja.Runtime) any { return goja.Undefined() })
			})
		})

		// lstat(path) -> Promise<Stats>
		// The stats object carries the Node fields scripts read: isSymbolicLink,
		// isFile, isDirectory, size, mode, mtimeMs.
		_ = promises.Set("lstat", func(call goja.FunctionCall) goja.Value {
			path, ok := stringArg(call, 0)
			if !ok {
				return rejectTypeError(adapter, "lstat", "The \"path\" argument must be of type string")
			}
			return adapter.TrackPromise(ctx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
				info, err := os.Lstat(path)
				if err != nil {
					_ = settle.Settle(true, func(rt *goja.Runtime) any { return nodeFSError(rt, "lstat", path, err) })
					return
				}
				_ = settle.Settle(false, func(rt *goja.Runtime) any { return statsObject(rt, info) })
			})
		})

		// mkdir(path[, options]) -> Promise<string|undefined>
		// With {recursive: true} Node resolves the first created directory
		// (or undefined when nothing was created); without it the leaf only.
		_ = promises.Set("mkdir", func(call goja.FunctionCall) goja.Value {
			path, ok := stringArg(call, 0)
			if !ok {
				return rejectTypeError(adapter, "mkdir", "The \"path\" argument must be of type string")
			}
			recursive := false
			if len(call.Arguments) > 1 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
				if opts, isObj := call.Argument(1).(*goja.Object); isObj {
					if v := opts.Get("recursive"); v != nil && !goja.IsUndefined(v) && !goja.IsNull(v) {
						recursive = v.ToBoolean()
					}
				}
			}
			return adapter.TrackPromise(ctx, func(ctx context.Context, settle gojaeventloop.TrackedSettlement) {
				if recursive {
					created, err := mkdirAll(path)
					if err != nil {
						_ = settle.Settle(true, func(rt *goja.Runtime) any { return nodeFSError(rt, "mkdir", path, err) })
						return
					}
					_ = settle.Settle(false, func(rt *goja.Runtime) any {
						if created == "" {
							return goja.Undefined()
						}
						return rt.ToValue(created)
					})
					return
				}
				if err := os.Mkdir(path, 0o777); err != nil {
					_ = settle.Settle(true, func(rt *goja.Runtime) any { return nodeFSError(rt, "mkdir", path, err) })
					return
				}
				_ = settle.Settle(false, func(*goja.Runtime) any { return goja.Undefined() })
			})
		})

		_ = exports.Set("promises", promises)
	}
}

// parseWriteFlag converts the Node write-flag strings the subset supports
// into os.OpenFile flags.
func parseWriteFlag(flag string) (int, error) {
	switch flag {
	case "w":
		return os.O_WRONLY | os.O_CREATE | os.O_TRUNC, nil
	case "wx":
		return os.O_WRONLY | os.O_CREATE | os.O_EXCL | os.O_TRUNC, nil
	case "a":
		return os.O_WRONLY | os.O_CREATE | os.O_APPEND, nil
	case "ax":
		return os.O_WRONLY | os.O_CREATE | os.O_EXCL | os.O_APPEND, nil
	case "r":
		return os.O_RDONLY, nil
	default:
		return 0, errors.New("invalid flag value: " + flag)
	}
}

// mkdirAll wraps filepath.MkdirAll to report Node's recursive-mkdir result:
// the first directory path created, or "" when the tree already existed.
func mkdirAll(path string) (string, error) {
	// Find the deepest existing ancestor to identify what we created.
	var missing []string
	current := path
	for {
		if _, err := os.Lstat(current); err == nil {
			break
		}
		missing = append(missing, current)
		parent := filepath.Dir(current)
		if parent == current {
			break
		}
		current = parent
	}
	if len(missing) == 0 {
		return "", nil
	}
	if err := os.MkdirAll(path, 0o777); err != nil {
		return "", err
	}
	// missing[len-1] is the shallowest missing ancestor: the first created.
	return missing[len(missing)-1], nil
}

// stringArg extracts a non-empty string argument.
func stringArg(call goja.FunctionCall, index int) (string, bool) {
	if len(call.Arguments) <= index {
		return "", false
	}
	value := call.Argument(index)
	if goja.IsUndefined(value) || goja.IsNull(value) {
		return "", false
	}
	s, ok := value.Export().(string)
	return s, ok && s != ""
}

// nodeFSError builds a Node-shaped error: an Error subclass carrying .code
// (ENOENT, EEXIST, ...), .errno, .syscall and .path, matching what scripts
// branch on after a failed fs call.
func nodeFSError(rt *goja.Runtime, syscallName, path string, err error) *goja.Object {
	code := errnoCode(err)
	message := code + ": " + errnoMessage(err) + ", " + syscallName + " '" + path + "'"
	obj := rt.NewGoError(errors.New(message))
	_ = obj.Set("code", code)
	_ = obj.Set("errno", errnoNumber(code))
	_ = obj.Set("syscall", syscallName)
	_ = obj.Set("path", path)
	return obj
}

// errnoCode maps a Go filesystem error to Node's UV-style code string.
func errnoCode(err error) string {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		switch errno {
		case syscall.ENOENT:
			return "ENOENT"
		case syscall.EEXIST:
			return "EEXIST"
		case syscall.EACCES:
			return "EACCES"
		case syscall.EPERM:
			return "EPERM"
		case syscall.ENOTDIR:
			return "ENOTDIR"
		case syscall.EISDIR:
			return "EISDIR"
		case syscall.ENOTEMPTY:
			return "ENOTEMPTY"
		case syscall.EINVAL:
			return "EINVAL"
		}
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return errnoCode(pathErr.Err)
	}
	return "EUNKNOWN"
}

func errnoMessage(err error) string {
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno.Error()
	}
	var pathErr *os.PathError
	if errors.As(err, &pathErr) {
		return errnoMessage(pathErr.Err)
	}
	return err.Error()
}

func errnoNumber(code string) int {
	switch code {
	case "ENOENT":
		return -2
	case "EEXIST":
		return -17
	case "EACCES":
		return -13
	case "EPERM":
		return -1
	case "ENOTDIR":
		return -20
	case "EISDIR":
		return -21
	case "ENOTEMPTY":
		return -39
	case "EINVAL":
		return -22
	default:
		return 0
	}
}

// statsObject builds the lstat result with the Node-observable fields.
func statsObject(rt *goja.Runtime, info fs.FileInfo) *goja.Object {
	obj := rt.NewObject()
	_ = obj.Set("isFile", func(goja.FunctionCall) goja.Value { return rt.ToValue(info.Mode().IsRegular()) })
	_ = obj.Set("isDirectory", func(goja.FunctionCall) goja.Value { return rt.ToValue(info.Mode().IsDir()) })
	_ = obj.Set("isSymbolicLink", func(goja.FunctionCall) goja.Value { return rt.ToValue(info.Mode()&fs.ModeSymlink != 0) })
	_ = obj.Set("size", info.Size())
	_ = obj.Set("mode", uint32(info.Mode().Perm()))
	_ = obj.Set("mtimeMs", float64(info.ModTime().UnixNano())/1e6)
	return obj
}

func rejectTypeError(adapter *gojaeventloop.Adapter, _, message string) goja.Value {
	promise, settler := adapter.NewPromise()
	_ = settler.Reject(func(rt *goja.Runtime) any { return rt.NewTypeError(message) })
	return promise
}

func rejectCode(adapter *gojaeventloop.Adapter, _, code, message string) goja.Value {
	promise, settler := adapter.NewPromise()
	_ = settler.Reject(func(rt *goja.Runtime) any {
		obj := rt.NewGoError(errors.New(message))
		_ = obj.Set("code", code)
		return obj
	})
	return promise
}
