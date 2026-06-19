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
	gojaeventloop "github.com/joeycumines/goja-eventloop"
	gosmargv "github.com/joeycumines/one-shot-man/internal/argv"
)

//go:embed contextManager.js
var contextManagerScript string

var (
	runGitDiffFn            = runGitDiff
	getDefaultGitDiffArgsFn = getDefaultGitDiffArgs
	runExecFn               = runExec
)

// contentBlock holds the processed data for each distinct section before rendering.
type contentBlock struct {
	Title   string
	Content string
	Lang    string
	IsError bool
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

// SetRunExecFn sets the exec function for testing.
func SetRunExecFn(fn func(context.Context, []string) (string, string, bool)) func() {
	old := runExecFn
	runExecFn = fn
	return func() { runExecFn = old }
}

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
	fenceLen := max(maxLength+1, 5)
	return strings.Repeat("`", fenceLen)
}

// extractedItem is the Go representation of a context item, extracted from goja
// on the event loop before processing on a goroutine.
type extractedItem struct {
	Type    string
	Label   string
	Payload any
}

// Require returns a CommonJS native module under "osm:ctxutil".
// buildContext returns a Promise<string> because lazy-diff and lazy-exec
// perform blocking subprocess I/O that must not stall the event loop.
func Require(baseCtx context.Context, adapter *gojaeventloop.Adapter) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exportsVal := module.Get("exports")
		var exports *goja.Object
		if exportsVal == nil || goja.IsUndefined(exportsVal) || goja.IsNull(exportsVal) {
			exports = runtime.NewObject()
			_ = module.Set("exports", exports)
		} else {
			exports = exportsVal.ToObject(runtime)
		}

		// buildContext(items, options?) -> Promise<string>
		_ = exports.Set("buildContext", func(call goja.FunctionCall) goja.Value {
			items := extractItems(runtime, call.Argument(0))
			txtarContent := extractTxtar(runtime, call)

			promise, resolve, _ := adapter.JS().NewChainedPromise()
			adapter.Loop().Promisify(baseCtx, func(ctx context.Context) (any, error) {
				result := renderContext(baseCtx, items, txtarContent)
				_ = adapter.Loop().Submit(func() {
					resolve(runtime.ToValue(result))
				})
				return nil, nil
			})
			return adapter.GojaWrapPromise(promise)
		})

		// Load contextManager
		tempModule := runtime.NewObject()
		tempExports := runtime.NewObject()
		_ = tempModule.Set("exports", tempExports)

		wrappedScript := "(function(module) { " + contextManagerScript + "\nreturn module.exports; })"

		compiledScript, err := runtime.RunString(wrappedScript)
		if err != nil {
			panic(fmt.Errorf("failed to compile contextManager script: %w", err))
		}

		fn, ok := goja.AssertFunction(compiledScript)
		if !ok {
			panic(fmt.Errorf("contextManager script wrapper did not return a function"))
		}

		result, err := fn(goja.Undefined(), runtime.ToValue(tempModule))
		if err != nil {
			panic(fmt.Errorf("failed to execute contextManager script: %w", err))
		}

		tempExports = result.ToObject(runtime)
		contextManagerFn := tempExports.Get("contextManager")
		if goja.IsUndefined(contextManagerFn) || goja.IsNull(contextManagerFn) {
			panic(fmt.Errorf("contextManager function not found in module exports"))
		}

		_ = exports.Set("contextManager", contextManagerFn)
	}
}

// extractItems converts goja items to Go data structures on the event loop.
func extractItems(runtime *goja.Runtime, itemsArg goja.Value) []extractedItem {
	if goja.IsUndefined(itemsArg) || goja.IsNull(itemsArg) {
		return nil
	}

	obj, objErr := toObject(runtime, itemsArg)
	if objErr != nil || obj == nil {
		return nil
	}

	var gojaItems []goja.Value
	if obj.ClassName() == "Array" {
		l := int(obj.Get("length").ToInteger())
		gojaItems = make([]goja.Value, 0, l)
		for i := range l {
			gojaItems = append(gojaItems, obj.Get(fmt.Sprintf("%d", i)))
		}
	} else {
		var itemsGo []any
		if err := runtime.ExportTo(itemsArg, &itemsGo); err != nil {
			return nil
		}
		gojaItems = make([]goja.Value, 0, len(itemsGo))
		for _, item := range itemsGo {
			gojaItems = append(gojaItems, runtime.ToValue(item))
		}
	}

	var result []extractedItem
	for _, v := range gojaItems {
		if goja.IsUndefined(v) || goja.IsNull(v) {
			continue
		}
		itemObj, err := toObject(runtime, v)
		if err != nil || itemObj == nil {
			continue
		}

		typeVal := valueOrUndefined(itemObj.Get("type"))
		if goja.IsUndefined(typeVal) || goja.IsNull(typeVal) {
			continue
		}

		var label string
		labelVal := valueOrUndefined(itemObj.Get("label"))
		if !goja.IsUndefined(labelVal) && !goja.IsNull(labelVal) {
			label = labelVal.String()
		}

		var payload any
		payloadVal := valueOrUndefined(itemObj.Get("payload"))
		if !goja.IsUndefined(payloadVal) && !goja.IsNull(payloadVal) {
			payload, _ = exportGojaValue(runtime, payloadVal)
		}

		result = append(result, extractedItem{
			Type:    typeVal.String(),
			Label:   label,
			Payload: payload,
		})
	}

	return result
}

// extractTxtar calls the toTxtar JS function synchronously and returns the result.
func extractTxtar(runtime *goja.Runtime, call goja.FunctionCall) string {
	if len(call.Arguments) < 2 || goja.IsUndefined(call.Argument(1)) || goja.IsNull(call.Argument(1)) {
		return ""
	}
	optObj := call.Argument(1).ToObject(runtime)
	v := valueOrUndefined(optObj.Get("toTxtar"))
	if goja.IsUndefined(v) || goja.IsNull(v) {
		return ""
	}
	callable, ok := goja.AssertFunction(v)
	if !ok {
		return ""
	}
	res, err := callable(goja.Undefined())
	if err != nil {
		return ""
	}
	if goja.IsUndefined(res) || goja.IsNull(res) || res.String() == "" {
		return ""
	}
	return res.String()
}

// renderContext processes extracted items and renders the final context string.
// This function runs on a goroutine and performs blocking I/O (git diff, exec).
func renderContext(baseCtx context.Context, items []extractedItem, txtarContent string) string {
	var blocks []*contentBlock
	var codeContents []string

	for _, item := range items {
		t := item.Type
		label := item.Label

		switch t {
		case "note":
			payload, _ := item.Payload.(string)
			title := "Note: "
			if label != "" {
				title += label
			} else {
				title += "note"
			}
			blocks = append(blocks, &contentBlock{Title: title, Content: payload})

		case "diff":
			payload, _ := item.Payload.(string)
			title := "Diff: "
			if label != "" {
				title += label
			} else {
				title += "git diff"
			}
			blocks = append(blocks, &contentBlock{Title: title, Content: payload, Lang: "diff"})
			codeContents = append(codeContents, payload)

		case "diff-error":
			payload, _ := item.Payload.(string)
			title := "Diff Error: "
			if label != "" {
				title += label
			} else {
				title += "git diff"
			}
			blocks = append(blocks, &contentBlock{Title: title, Content: payload, IsError: true})

		case "lazy-diff":
			args, hadErr, errMsg := parsePayloadArgs(item.Payload, true)

			if !hadErr && len(args) == 0 {
				args = getDefaultGitDiffArgsFn(baseCtx)
			}

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
				blocks = append(blocks, &contentBlock{
					Title:   "Diff Error: " + finalLabel,
					Content: "Error executing git diff: " + errMsg,
					IsError: true,
				})
			} else {
				blocks = append(blocks, &contentBlock{
					Title:   "Diff: " + finalLabel,
					Content: out,
					Lang:    "diff",
				})
				codeContents = append(codeContents, out)
			}

		case "lazy-exec":
			args, hadErr, errMsg := parsePayloadArgs(item.Payload, false)

			var out string
			if !hadErr {
				var execErr bool
				out, errMsg, execErr = runExecFn(baseCtx, args)
				hadErr = execErr
			}

			finalLabel := label
			if finalLabel == "" {
				finalLabel = strings.TrimSpace(strings.Join(args, " "))
			}

			if hadErr {
				blocks = append(blocks, &contentBlock{
					Title:   "Exec Error: " + finalLabel,
					Content: "Error executing command: " + errMsg,
					IsError: true,
				})
			} else {
				blocks = append(blocks, &contentBlock{
					Title:   "Exec: " + finalLabel,
					Content: out,
				})
				codeContents = append(codeContents, out)
			}
		}
	}

	if txtarContent != "" {
		codeContents = append(codeContents, txtarContent)
	}

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

	if txtarContent != "" {
		var metaLines []string
		var bodyLines []string
		inMeta := true
		for line := range strings.SplitSeq(txtarContent, "\n") {
			if inMeta {
				trimmed := strings.TrimSpace(line)
				if strings.HasPrefix(trimmed, "context root:") ||
					strings.HasPrefix(trimmed, "common path:") ||
					strings.HasPrefix(trimmed, "tracked directories:") {
					metaLines = append(metaLines, line)
					continue
				}
				if trimmed == "" {
					continue
				}
				inMeta = false
			}
			bodyLines = append(bodyLines, line)
		}

		for _, ml := range metaLines {
			if before, after, ok := strings.Cut(ml, ": "); ok {
				buf.WriteString(strings.TrimSpace(before))
				buf.WriteString(": `")
				buf.WriteString(strings.TrimSpace(after))
				buf.WriteString("`\n")
			} else {
				buf.WriteString(ml)
				buf.WriteString("\n")
			}
		}
		if len(metaLines) > 0 {
			buf.WriteString("\n")
		}

		body := strings.Join(bodyLines, "\n")
		buf.WriteString(fence)
		buf.WriteString("txtar\n")
		buf.WriteString(body)
		buf.WriteString("\n")
		buf.WriteString(fence)
	}

	return buf.String()
}

// parsePayloadArgs converts a payload (any) to []string args.
func parsePayloadArgs(payload any, isDiff bool) (args []string, hadErr bool, errMsg string) {
	if payload == nil {
		if isDiff {
			return nil, false, ""
		}
		return nil, true, "exec: no command specified"
	}

	switch p := payload.(type) {
	case []string:
		args = append(args, p...)
	case []any:
		tmp := make([]string, 0, len(p))
		for i, item := range p {
			str, ok := item.(string)
			if !ok {
				typeName := "undefined"
				if item != nil {
					typeName = reflect.TypeOf(item).String()
				}
				return nil, true, fmt.Sprintf("Invalid payload: expected a string array, but found non-string element at index %d (type '%s')", i, typeName)
			}
			tmp = append(tmp, str)
		}
		args = tmp
	case string:
		args = gosmargv.ParseSlice(p)
	default:
		typeName := ""
		if payload != nil {
			typeName = reflect.TypeOf(payload).String()
		} else {
			typeName = "undefined"
		}
		return nil, true, fmt.Sprintf("Invalid payload: expected a string or string array, but got type '%s'", typeName)
	}

	return args, false, ""
}

func valueOrUndefined(val goja.Value) goja.Value {
	if val == nil {
		return goja.Undefined()
	}
	return val
}

func exportGojaValue(runtime *goja.Runtime, value goja.Value) (any, error) {
	var (
		result    any
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

func getDefaultGitDiffArgs(ctx context.Context) []string {
	if ctx == nil {
		ctx = context.Background()
	}

	if out, _ := exec.CommandContext(ctx, "git", "diff", "--no-ext-diff", "--no-color", "HEAD").CombinedOutput(); len(bytes.TrimSpace(out)) > 0 {
		return []string{"HEAD"}
	}

	if err := exec.CommandContext(ctx, "git", "rev-parse", "-q", "--verify", "HEAD~1").Run(); err == nil {
		return []string{"HEAD~1"}
	}

	return []string{"4b825dc642cb6eb9a060e54bf8d69288fbee4904", "HEAD"}
}

func runExec(ctx context.Context, args []string) (stdout string, message string, hadErr bool) {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(args) == 0 {
		return "", "exec: no command specified", true
	}
	cmd := exec.CommandContext(ctx, args[0], args[1:]...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	if err != nil {
		return "", strings.TrimSpace(errBuf.String() + " " + err.Error()), true
	}
	return outBuf.String(), "", false
}
