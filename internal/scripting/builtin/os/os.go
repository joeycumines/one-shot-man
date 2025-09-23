package osmod

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/dop251/goja"
)

// ModuleLoader returns a module loader for `osm:os` that uses the provided base context
// and a TUI sink for fallback messaging (may be nil).
func ModuleLoader(ctx context.Context, tuiSink func(string)) func(runtime *goja.Runtime, module *goja.Object) {
	return func(runtime *goja.Runtime, module *goja.Object) {
		exports := module.Get("exports").(*goja.Object)

		// readFile(path: string): { content: string, error: bool, message: string }
		_ = exports.Set("readFile", func(call goja.FunctionCall) goja.Value {
			var path string
			if len(call.Arguments) > 0 {
				path = call.Argument(0).String()
			}
			if path == "" {
				return runtime.ToValue(map[string]interface{}{"error": true, "message": "empty path", "content": ""})
			}
			data, err := os.ReadFile(path)
			if err != nil {
				return runtime.ToValue(map[string]interface{}{"error": true, "message": err.Error(), "content": ""})
			}
			return runtime.ToValue(map[string]interface{}{"error": false, "message": "", "content": string(data)})
		})

		// fileExists(path: string): boolean
		_ = exports.Set("fileExists", func(call goja.FunctionCall) goja.Value {
			var path string
			if len(call.Arguments) > 0 {
				path = call.Argument(0).String()
			}
			if path == "" {
				return runtime.ToValue(false)
			}
			_, err := os.Stat(path)
			return runtime.ToValue(err == nil)
		})

		// openEditor(title, initialContent)
		_ = exports.Set("openEditor", func(call goja.FunctionCall) goja.Value {
			var nameHint, initialContent string
			if len(call.Arguments) > 0 {
				nameHint = call.Argument(0).String()
			}
			if len(call.Arguments) > 1 {
				initialContent = call.Argument(1).String()
			}
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			return runtime.ToValue(openEditor(ctx, nameHint, initialContent))
		})

		// clipboardCopy(text)
		_ = exports.Set("clipboardCopy", func(call goja.FunctionCall) goja.Value {
			var text string
			if len(call.Arguments) > 0 {
				text = call.Argument(0).String()
			}
			ctx, cancel := context.WithCancel(ctx)
			defer cancel()
			if err := clipboardCopy(ctx, tuiSink, text); err != nil {
				panic(runtime.NewGoError(err))
			}
			return goja.Undefined()
		})

		// getenv(key: string): string
		_ = exports.Set("getenv", func(call goja.FunctionCall) goja.Value {
			if len(call.Arguments) == 0 || goja.IsUndefined(call.Argument(0)) || goja.IsNull(call.Argument(0)) {
				return runtime.ToValue("")
			}
			return runtime.ToValue(os.Getenv(call.Argument(0).String()))
		})
	}
}

func openEditor(ctx context.Context, nameHint string, initialContent string) string {
	if nameHint == "" {
		nameHint = "oneshot"
	}
	dir, err := os.MkdirTemp("", "one-shot-man-editor-*")
	if err != nil {
		return initialContent
	}
	base := sanitizeFilename(nameHint)
	if base == "" {
		base = "oneshot"
	}
	path := filepath.Join(dir, base)
	if err := os.WriteFile(path, []byte(initialContent), 0600); err != nil {
		_ = os.RemoveAll(dir)
		return initialContent
	}

	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		if editor != "" {
			cmd = exec.CommandContext(ctx, editor, path)
		} else {
			cmd = exec.CommandContext(ctx, "notepad", path)
		}
	default:
		if editor == "" {
			if _, err := exec.LookPath("nano"); err == nil {
				editor = "nano"
			} else if _, err := exec.LookPath("vi"); err == nil {
				editor = "vi"
			} else {
				editor = "ed"
			}
		}
		cmd = exec.CommandContext(ctx, editor, path)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		data, _ := os.ReadFile(path)
		_ = os.RemoveAll(dir)
		if len(data) > 0 {
			return string(data)
		}
		return initialContent
	}

	data, readErr := os.ReadFile(path)
	_ = os.RemoveAll(dir)
	if readErr != nil {
		return initialContent
	}
	return string(data)
}

func clipboardCopy(ctx context.Context, tuiSink func(string), text string) error {
	if cmdStr := os.Getenv("ONESHOT_CLIPBOARD_CMD"); cmdStr != "" {
		var c *exec.Cmd
		if runtime.GOOS == "windows" {
			c = exec.CommandContext(ctx, "cmd", "/c", cmdStr)
		} else {
			c = exec.CommandContext(ctx, "/bin/sh", "-c", cmdStr)
		}
		c.Stdin = strings.NewReader(text)
		if err := c.Run(); err == nil {
			return nil
		}
	}

	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("pbcopy"); err == nil {
			c := exec.CommandContext(ctx, "pbcopy")
			c.Stdin = strings.NewReader(text)
			return c.Run()
		}
	case "windows":
		if _, err := exec.LookPath("clip"); err == nil {
			c := exec.CommandContext(ctx, "clip")
			c.Stdin = strings.NewReader(text)
			return c.Run()
		}
	default:
		if _, err := exec.LookPath("wl-copy"); err == nil {
			c := exec.CommandContext(ctx, "wl-copy")
			c.Stdin = strings.NewReader(text)
			return c.Run()
		}
		if _, err := exec.LookPath("xclip"); err == nil {
			c := exec.CommandContext(ctx, "xclip", "-selection", "clipboard")
			c.Stdin = strings.NewReader(text)
			return c.Run()
		}
		if _, err := exec.LookPath("xsel"); err == nil {
			c := exec.CommandContext(ctx, "xsel", "--clipboard", "--input")
			c.Stdin = strings.NewReader(text)
			return c.Run()
		}
	}

	// best-effort fallback: route via provided TUI sink if available
	if tuiSink != nil {
		tuiSink("[clipboard] No system clipboard utility available; printing content below\n" + text + "\n")
		return nil
	}
	return fmt.Errorf("no system clipboard available")
}

// sanitizeFilename produces a filesystem-safe portion for temp filenames
func sanitizeFilename(s string) string {
	if s == "" {
		return "untitled"
	}
	var b strings.Builder
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '-' || r == '_' || r == '.' {
			b.WriteRune(r)
		} else {
			b.WriteByte('-')
		}
	}
	out := b.String()
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	out = strings.Trim(out, "-")
	if out == "" {
		out = "untitled"
	}
	return out
}

// (no JS ctx extraction needed; module uses base context)
