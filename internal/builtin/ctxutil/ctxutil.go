package ctxutil

import (
	"bytes"
	"context"
	_ "embed"
	"fmt"
	"os/exec"
	"reflect"
	"strings"

	"github.com/dop251/goja"
	gosmargv "github.com/joeycumines/one-shot-man/internal/argv"
)

//go:embed contextManager.js
var contextManagerScript string

var (
	runGitDiffFn            = runGitDiff
	getDefaultGitDiffArgsFn = getDefaultGitDiffArgs
)

// contentBlock holds the processed data for each distinct section before rendering.
type contentBlock struct {
	Title   string // The full title, e.g., "Note: Important", "Diff: git diff HEAD~1"
	Content string // The raw payload for the section
	Lang    string // The code block language (e.g., "diff"). Empty for non-code blocks.
	IsError bool   // Flag to distinguish successful diffs from errors.
}

// SetRunGitDiffFn sets the git diff function for testing.
func SetRunGitDiffFn(fn func(context.Context, []string) (string, string, bool)) func() {
	old := runGitDiffFn
	runGitDiffFn = fn
	return func() { runGitDiffFn = old }
}

// SetGetDefaultGitDiffArgsFn sets the default git diff args function for testing.
func SetGetDefaultGitDiffArgsFn(fn func(context.Context) []string) func() {
	old := getDefaultGitDiffArgsFn
	getDefaultGitDiffArgsFn = fn
	return func() { getDefaultGitDiffArgsFn = old }
}

// calculateBacktickFence determines the required fence length based on content.
// It scans all provided content strings for the longest consecutive run of backticks,
// then returns a fence that is one character longer, with a minimum of 5 backticks.
func calculateBacktickFence(contents []string) string {
	maxLength := 0
	for _, content := range contents {
		currentRun := 0
		for _, ch := range content {
			if ch == '`' {
				currentRun++
				if currentRun > maxLength {
					maxLength = currentRun
				}
			} else {
				currentRun = 0
			}
		}
	}

	fenceLen := maxLength + 1
	if fenceLen < 5 {
		fenceLen = 5
	}
	return strings.Repeat("`", fenceLen)
}

// Require returns a CommonJS native module under "osm:ctxutil".
// It exposes helpers to build context strings from a list of items while
// resolving lazy diffs at call-time to ensure always-fresh content.
//
// API (JS):
//
//	const { buildContext, contextManager } = require('osm:ctxutil');
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
//
// contextManager: Factory function for creating reusable context management patterns.
// See contextManager.js for detailed documentation.
func Require(baseCtx context.Context) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		// Get or create exports object
		exportsVal := module.Get("exports")
		var exports *goja.Object
		if goja.IsUndefined(exportsVal) || goja.IsNull(exportsVal) {
			exports = runtime.NewObject()
			_ = module.Set("exports", exports)
		} else {
			exports = exportsVal.ToObject(runtime)
		}

		// buildContext(items, options?) -> string
		_ = exports.Set("buildContext", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) < 1 {
				return runtime.ToValue("")
			}

			itemsArg := call.Argument(0)
			if goja.IsUndefined(itemsArg) || goja.IsNull(itemsArg) {
				return runtime.ToValue("")
			}

			obj, objErr := toObject(runtime, itemsArg)
			if objErr != nil {
				return runtime.ToValue("")
			}

			// Extract items as []goja.Value to iterate with minimal assumptions
			var items []goja.Value
			if obj != nil && obj.ClassName() == "Array" {
				l := int(obj.Get("length").ToInteger())
				items = make([]goja.Value, 0, l)
				for i := 0; i < l; i++ {
					items = append(items, obj.Get(fmt.Sprintf("%d", i)))
				}
			} else {
				// Fall back to exporting into generic slice (e.g., Go slices exposed to JS)
				var itemsGo []interface{}
				if err := runtime.ExportTo(itemsArg, &itemsGo); err != nil {
					return runtime.ToValue("")
				}
				items = make([]goja.Value, 0, len(itemsGo))
				for _, item := range itemsGo {
					items = append(items, runtime.ToValue(item))
				}
			}

			// Stage 1: Collection
			var blocks []*contentBlock
			var codeContents []string

			for _, v := range items {
				if goja.IsUndefined(v) || goja.IsNull(v) {
					continue
				}
				obj, objErr := toObject(runtime, v)
				if objErr != nil {
					continue
				}

				// type is required; skip if missing
				typeVal := valueOrUndefined(obj.Get("type"))
				if goja.IsUndefined(typeVal) || goja.IsNull(typeVal) {
					continue
				}
				t := typeVal.String()

				// optional label
				var label string
				labelVal := valueOrUndefined(obj.Get("label"))
				if !goja.IsUndefined(labelVal) && !goja.IsNull(labelVal) {
					label = labelVal.String()
				}

				switch t {
				case "note":
					payload := safeGetString(obj, "payload")
					title := "Note: "
					if label != "" {
						title += label
					} else {
						title += "note"
					}
					blocks = append(blocks, &contentBlock{
						Title:   title,
						Content: payload,
						Lang:    "",
						IsError: false,
					})

				case "diff":
					payload := safeGetString(obj, "payload")
					title := "Diff: "
					if label != "" {
						title += label
					} else {
						title += "git diff"
					}
					blocks = append(blocks, &contentBlock{
						Title:   title,
						Content: payload,
						Lang:    "diff",
						IsError: false,
					})
					codeContents = append(codeContents, payload)

				case "diff-error":
					payload := safeGetString(obj, "payload")
					title := "Diff Error: "
					if label != "" {
						title += label
					} else {
						title += "git diff"
					}
					blocks = append(blocks, &contentBlock{
						Title:   title,
						Content: payload,
						Lang:    "",
						IsError: true,
					})

				case "lazy-diff":
					// Determine argv for `git diff ...`
					payloadVal := valueOrUndefined(obj.Get("payload"))
					var args []string
					var hadErr bool
					var errMsg string

					if goja.IsUndefined(payloadVal) || goja.IsNull(payloadVal) {
						// Unspecified -> choose robust default
						args = getDefaultGitDiffArgsFn(baseCtx)
					} else {
						if arr, arrErr := toArrayObject(runtime, payloadVal); arrErr != nil {
							hadErr = true
							errMsg = fmt.Sprintf("Invalid payload: %v", arrErr)
						} else if arr != nil {
							length := int(arr.Get("length").ToInteger())
							tmp := make([]string, 0, length)
							for i := 0; i < length; i++ {
								itemVal := valueOrUndefined(arr.Get(fmt.Sprintf("%d", i)))
								if goja.IsUndefined(itemVal) || goja.IsNull(itemVal) {
									hadErr = true
									errMsg = fmt.Sprintf("Invalid payload: expected a string array, but found non-string element at index %d (type '%v')", i, itemVal)
									break
								}
								exported, err := exportGojaValue(runtime, itemVal)
								if err != nil {
									hadErr = true
									errMsg = fmt.Sprintf("Invalid payload: expected a string array, but found non-string element at index %d (type '%s')", i, err)
									break
								}
								str, ok := exported.(string)
								if !ok {
									typeName := ""
									if exported != nil {
										typeName = reflect.TypeOf(exported).String()
									} else {
										typeName = "undefined"
									}
									hadErr = true
									errMsg = fmt.Sprintf("Invalid payload: expected a string array, but found non-string element at index %d (type '%s')", i, typeName)
									break
								}
								tmp = append(tmp, str)
							}
							if !hadErr {
								args = tmp
							}
						} else {
							exported, err := exportGojaValue(runtime, payloadVal)
							if err != nil {
								hadErr = true
								errMsg = fmt.Sprintf("Invalid payload: %v", err)
							} else {
								switch exported := exported.(type) {
								case []interface{}:
									tmp := make([]string, 0, len(exported))
									for i, item := range exported {
										str, ok := item.(string)
										if !ok {
											typeName := "undefined"
											if item != nil {
												typeName = reflect.TypeOf(item).String()
											}
											hadErr = true
											errMsg = fmt.Sprintf("Invalid payload: expected a string array, but found non-string element at index %d (type '%s')", i, typeName)
											break
										}
										tmp = append(tmp, str)
									}
									if !hadErr {
										args = tmp
									}
								case []string:
									args = append(args, exported...)
								case string:
									args = gosmargv.ParseSlice(exported)
								default:
									typeName := ""
									if exported != nil {
										typeName = reflect.TypeOf(exported).String()
									} else {
										typeName = "undefined"
									}
									hadErr = true
									errMsg = fmt.Sprintf("Invalid payload: expected a string or string array, but got type '%s'", typeName)
								}
							}
						}
					}

					// If args were provided as the common default HEAD~1, but it's invalid in this repo
					// (e.g. only a single commit exists), upgrade to an empty-tree vs HEAD diff.
					if !hadErr && len(args) == 1 && strings.TrimSpace(args[0]) == "HEAD~1" {
						if def := getDefaultGitDiffArgsFn(baseCtx); len(def) == 2 && def[0] != "HEAD~1" {
							args = def
						}
					}

					// If still no args (e.g. empty string payload), choose robust default
					if !hadErr && len(args) == 0 {
						args = getDefaultGitDiffArgsFn(baseCtx)
					}

					// Execute git diff with args
					var out string
					if !hadErr {
						var gitErr bool
						out, errMsg, gitErr = runGitDiffFn(baseCtx, args)
						hadErr = gitErr
					}

					finalLabel := label
					if finalLabel == "" {
						finalLabel = "git diff " + strings.TrimSpace(strings.Join(args, " "))
					}

					if hadErr {
						title := "Diff Error: " + finalLabel
						content := "Error executing git diff: " + errMsg
						blocks = append(blocks, &contentBlock{
							Title:   title,
							Content: content,
							Lang:    "",
							IsError: true,
						})
					} else {
						title := "Diff: " + finalLabel
						blocks = append(blocks, &contentBlock{
							Title:   title,
							Content: out,
							Lang:    "diff",
							IsError: false,
						})
						codeContents = append(codeContents, out)
					}
				}
			}

			// Process txtar content if provided
			var txtarContent string
			if len(call.Arguments) >= 2 && !goja.IsUndefined(call.Argument(1)) && !goja.IsNull(call.Argument(1)) {
				// options is an object; look for toTxtar
				optObj := call.Argument(1).ToObject(runtime)
				if v := valueOrUndefined(optObj.Get("toTxtar")); !goja.IsUndefined(v) && !goja.IsNull(v) {
					if callable, ok := goja.AssertFunction(v); ok {
						if res, err := callable(goja.Undefined(), nil...); err == nil {
							if !goja.IsUndefined(res) && !goja.IsNull(res) && res.String() != "" {
								txtarContent = res.String()
								codeContents = append(codeContents, txtarContent)
							}
						}
					}
				}
			}

			// Stage 2: Rendering
			fence := calculateBacktickFence(codeContents)
			var buf strings.Builder

			for _, block := range blocks {
				buf.WriteString("### ")
				buf.WriteString(block.Title)
				buf.WriteString("\n\n")

				if block.Lang == "diff" {
					buf.WriteString(fence)
					buf.WriteString("diff\n")
					buf.WriteString(block.Content)
					buf.WriteString("\n")
					buf.WriteString(fence)
					buf.WriteString("\n\n---\n")
				} else {
					buf.WriteString(block.Content)
					buf.WriteString("\n\n---\n")
				}
			}

			// Append txtar block if present
			if txtarContent != "" {
				buf.WriteString(fence)
				buf.WriteString("txtar\n")
				buf.WriteString(txtarContent)
				buf.WriteString("\n")
				buf.WriteString(fence)
			}

			return runtime.ToValue(buf.String())
		})

		// Load contextManager by setting up a temporary module context
		// We need to execute contextManager.js which uses module.exports pattern
		tempModule := runtime.NewObject()
		tempExports := runtime.NewObject()
		_ = tempModule.Set("exports", tempExports)

		// Wrap contextManager script in a function to avoid polluting global scope
		wrappedScript := "(function(module) { " + contextManagerScript + "\nreturn module.exports; })"

		// Compile and call the wrapped function
		compiledScript, err := runtime.RunString(wrappedScript)
		if err != nil {
			panic(fmt.Errorf("failed to compile contextManager script: %w", err))
		}

		// Call the function with tempModule
		fn, ok := goja.AssertFunction(compiledScript)
		if !ok {
			panic(fmt.Errorf("contextManager script wrapper did not return a function"))
		}

		result, err := fn(goja.Undefined(), runtime.ToValue(tempModule))
		if err != nil {
			panic(fmt.Errorf("failed to execute contextManager script: %w", err))
		}

		// Extract the contextManager function from the result
		tempExports = result.ToObject(runtime)
		contextManagerFn := tempExports.Get("contextManager")
		if goja.IsUndefined(contextManagerFn) || goja.IsNull(contextManagerFn) {
			panic(fmt.Errorf("contextManager function not found in module exports"))
		}

		// Set it on our exports object
		_ = exports.Set("contextManager", contextManagerFn)
	}
}

// safeGetString reads a property and returns "" if undefined or null.
func safeGetString(obj *goja.Object, propName string) string {
	if obj == nil {
		return ""
	}
	val := valueOrUndefined(obj.Get(propName))
	if goja.IsUndefined(val) || goja.IsNull(val) {
		return ""
	}
	return val.String()
}

func valueOrUndefined(val goja.Value) goja.Value {
	if val == nil {
		return goja.Undefined()
	}
	return val
}

func exportGojaValue(runtime *goja.Runtime, value goja.Value) (interface{}, error) {
	var (
		result    interface{}
		exportErr error
	)

	func() {
		defer func() {
			if r := recover(); r != nil {
				exportErr = fmt.Errorf("%v", r)
			}
		}()
		exportErr = runtime.ExportTo(value, &result)
	}()

	if exportErr != nil {
		return nil, exportErr
	}

	return result, nil
}

func toArrayObject(runtime *goja.Runtime, value goja.Value) (*goja.Object, error) {
	obj, err := toObject(runtime, value)
	if err != nil {
		return nil, err
	}

	if obj == nil || obj.ClassName() != "Array" {
		return nil, nil
	}

	return obj, nil
}

func toObject(runtime *goja.Runtime, value goja.Value) (*goja.Object, error) {
	var (
		obj     *goja.Object
		convErr error
	)

	func() {
		defer func() {
			if r := recover(); r != nil {
				convErr = fmt.Errorf("%v", r)
			}
		}()
		obj = value.ToObject(runtime)
	}()

	if convErr != nil {
		return nil, convErr
	}

	if obj == nil {
		return nil, fmt.Errorf("goja.ToObject returned nil")
	}

	return obj, nil
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
