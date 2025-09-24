package ctxutil

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"reflect"
	"strings"

	"github.com/dop251/goja"
	gosmargv "github.com/joeycumines/one-shot-man/internal/argv"
)

// ModuleLoader returns a CommonJS native module under "osm:ctxutil".
// It exposes helpers to build context strings from a list of items while
// resolving lazy diffs at call-time to ensure always-fresh content.
//
// API (JS):
//
//	const { buildContext } = require('osm:ctxutil');
//	const text = buildContext(itemsArray, { toTxtar: () => context.toTxtar() });
//
// itemsArray: Array<{ id?: number, type: 'note'|'diff'|'diff-error'|'lazy-diff', label?: string, payload?: any }>
// options.toTxtar: optional function returning string to append as fenced txtar block.
//
// Behavior:
// - note: emits a markdown Note section using payload string.
// - diff: emits a markdown Diff section using payload string.
// - diff-error: emits a markdown Diff Error section using payload string.
// - lazy-diff: payload can be string (shell-like) or string[] (argv). Runs `git diff ...` and emits Diff or Diff Error.
func ModuleLoader(baseCtx context.Context) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)

		// buildContext(items, options?) -> string
		_ = exports.Set("buildContext", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return runtime.ToValue("")
			}

			// Extract items as []any to iterate with minimal assumptions
			var items []goja.Value
			if call.Argument(0).ToObject(runtime).ClassName() == "Array" {
				o := call.Argument(0).ToObject(runtime)
				l := int(o.Get("length").ToInteger())
				items = make([]goja.Value, 0, l)
				for i := 0; i < l; i++ {
					items = append(items, o.Get(fmt.Sprintf("%d", i)))
				}
			} else {
				// Fall back to Export
				if err := runtime.ExportTo(call.Argument(0), &items); err != nil {
					return runtime.ToValue("")
				}
			}

			var buf strings.Builder

			for _, v := range items {
				obj := v.ToObject(runtime)

				// type is required; skip if missing
				typeVal := obj.Get("type")
				if goja.IsUndefined(typeVal) || goja.IsNull(typeVal) {
					continue
				}
				t := typeVal.String()

				// optional label
				var label string
				labelVal := obj.Get("label")
				if !goja.IsUndefined(labelVal) && !goja.IsNull(labelVal) {
					label = labelVal.String()
				}

				switch t {
				case "note":
					payload := safeGetString(obj, "payload")
					buf.WriteString("### Note: ")
					if label != "" {
						buf.WriteString(label)
					} else {
						buf.WriteString("note")
					}
					buf.WriteString("\n\n")
					buf.WriteString(payload)
					buf.WriteString("\n\n---\n")
				case "diff":
					payload := safeGetString(obj, "payload")
					buf.WriteString("### Diff: ")
					if label != "" {
						buf.WriteString(label)
					} else {
						buf.WriteString("git diff")
					}
					buf.WriteString("\n\n```diff\n")
					buf.WriteString(payload)
					buf.WriteString("\n```\n\n---\n")
				case "diff-error":
					payload := safeGetString(obj, "payload")
					buf.WriteString("### Diff Error: ")
					if label != "" {
						buf.WriteString(label)
					} else {
						buf.WriteString("git diff")
					}
					buf.WriteString("\n\n")
					buf.WriteString(payload)
					buf.WriteString("\n\n---\n")
				case "lazy-diff":
					// Determine argv for `git diff ...`
					payloadVal := obj.Get("payload")
					var args []string
					var hadErr bool
					var errMsg string

					if goja.IsUndefined(payloadVal) || goja.IsNull(payloadVal) {
						// Unspecified -> choose robust default
						args = getDefaultGitDiffArgs(baseCtx)
					} else {
						// Try array first
						if payloadObj := payloadVal.ToObject(runtime); payloadObj != nil && payloadObj.ClassName() == "Array" {
							// Strictly require an array of strings
							l := int(payloadObj.Get("length").ToInteger())
							tmp := make([]string, 0, l)
							for i := 0; i < l; i++ {
								el := payloadObj.Get(fmt.Sprintf("%d", i))
								if goja.IsUndefined(el) || goja.IsNull(el) {
									hadErr = true
									errMsg = fmt.Sprintf("Invalid payload: expected a string array, but found null/undefined at index %d", i)
									break
								}
								et := el.ExportType()
								if et == nil || et.Kind() != reflect.String {
									typeName := ""
									if et != nil {
										typeName = et.Name()
									}
									hadErr = true
									errMsg = fmt.Sprintf("Invalid payload: expected a string array, but found non-string element at index %d (type '%s')", i, typeName)
									break
								}
								tmp = append(tmp, el.String())
							}
							if !hadErr {
								args = tmp
							}
						} else if et := payloadVal.ExportType(); et != nil && et.Kind() == reflect.String {
							// String -> parse into argv
							args = gosmargv.ParseSlice(payloadVal.String())
						} else {
							// Unsupported type
							typeName := ""
							if et := payloadVal.ExportType(); et != nil {
								typeName = et.Name()
							}
							hadErr = true
							errMsg = fmt.Sprintf("Invalid payload: expected a string or string array, but got type '%s'", typeName)
						}
					}

					// If args were provided as the common default HEAD~1, but it's invalid in this repo
					// (e.g. only a single commit exists), upgrade to an empty-tree vs HEAD diff.
					if !hadErr && len(args) == 1 && strings.TrimSpace(args[0]) == "HEAD~1" {
						if def := getDefaultGitDiffArgs(baseCtx); len(def) == 2 && def[0] != "HEAD~1" {
							args = def
						}
					}

					// If still no args (e.g. empty string payload), choose robust default
					if !hadErr && len(args) == 0 {
						args = getDefaultGitDiffArgs(baseCtx)
					}

					// Execute git diff with args
					var out string
					if !hadErr {
						var gitErr bool
						out, errMsg, gitErr = runGitDiff(baseCtx, args)
						hadErr = gitErr
					}

					finalLabel := label
					if finalLabel == "" {
						finalLabel = "git diff " + strings.TrimSpace(strings.Join(args, " "))
					}

					if hadErr {
						buf.WriteString("### Diff Error: ")
						buf.WriteString(finalLabel)
						buf.WriteString("\n\n")
						buf.WriteString("Error executing git diff: ")
						buf.WriteString(errMsg)
						buf.WriteString("\n\n---\n")
					} else {
						buf.WriteString("### Diff: ")
						buf.WriteString(finalLabel)
						buf.WriteString("\n\n```diff\n")
						buf.WriteString(out)
						buf.WriteString("\n```\n\n---\n")
					}
				}
			}

			// Append txtar using options.toTxtar() if provided
			if len(call.Arguments) >= 2 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
				// options is an object; look for toTxtar
				optObj := call.Argument(1).ToObject(runtime)
				if v := optObj.Get("toTxtar"); !goja.IsUndefined(v) && !goja.IsNull(v) {
					if callable, ok := goja.AssertFunction(v); ok {
						if res, err := callable(goja.Undefined(), nil...); err == nil {
							if !goja.IsUndefined(res) && !goja.IsNull(res) && res.String() != "" {
								buf.WriteString("```\n")
								buf.WriteString(res.String())
								buf.WriteString("\n```")
							}
						}
					}
				}
			}

			return runtime.ToValue(buf.String())
		})
	}
}

// safeGetString reads a property and returns "" if undefined or null.
func safeGetString(obj *goja.Object, propName string) string {
	val := obj.Get(propName)
	if goja.IsUndefined(val) || goja.IsNull(val) {
		return ""
	}
	return val.String()
}

func runGitDiff(ctx context.Context, args []string) (stdout string, message string, hadErr bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	argv := append([]string{"diff"}, args...)
	cmd := exec.CommandContext(ctx, "git", argv...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		return "", strings.TrimSpace(errBuf.String() + " " + err.Error()), true
	}
	return outBuf.String(), "", false
}

// getDefaultGitDiffArgs returns a robust default for `git diff`.
// Prefer HEAD~1 when available; otherwise, use the empty-tree hash vs HEAD
// which produces the initial commit contents as a diff.
func getDefaultGitDiffArgs(ctx context.Context) []string {
	if ctx == nil {
		ctx = context.Background()
	}
	// Check if HEAD~1 exists
	if err := exec.CommandContext(ctx, "git", "rev-parse", "-q", "--verify", "HEAD~1").Run(); err == nil {
		return []string{"HEAD~1"}
	}
	// Fallback: empty tree vs HEAD
	return []string{"4b825dc642cb6eb9a060e54bf8d69288fbee4904", "HEAD"}
}
