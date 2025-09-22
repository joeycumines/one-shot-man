package scripting

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/joeycumines/one-shot-man/internal/argv"
)

// ------------------- System JS API -------------------

// jsSystemExec executes a system command and returns an object with stdout, stderr, and exit code.
func (e *Engine) jsSystemExec(cmd string, args ...string) map[string]interface{} {
	c := exec.CommandContext(e.ctx, cmd, args...)
	var stdout, stderr bytes.Buffer
	c.Stdout = &stdout
	c.Stderr = &stderr
	// Use process stdio for input to allow interactive commands (e.g., git editor)
	c.Stdin = os.Stdin
	err := c.Run()
	code := 0
	errStr := ""
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else {
			code = -1
		}
		errStr = err.Error()
	}
	return map[string]interface{}{
		"stdout":  stdout.String(),
		"stderr":  stderr.String(),
		"code":    code,
		"error":   err != nil,
		"message": errStr,
	}
}

// jsSystemExecv executes a command given as argv array, e.g., ["git","diff","--staged"].
func (e *Engine) jsSystemExecv(argv interface{}) map[string]interface{} {
	if argv == nil {
		return map[string]interface{}{"error": true, "message": "no argv"}
	}
	// Strict: only accept an array of strings
	var parts []string
	if err := e.vm.ExportTo(e.vm.ToValue(argv), &parts); err != nil || len(parts) == 0 {
		return map[string]interface{}{"error": true, "message": "execv expects an array of strings"}
	}
	cmd := parts[0]
	args := []string{}
	if len(parts) > 1 {
		args = parts[1:]
	}
	return e.jsSystemExec(cmd, args...)
}

// jsSystemOpenEditor opens the user's editor ($VISUAL, $EDITOR, fallback vi) on a temp file with initial content,
// then returns the edited content.
func (e *Engine) jsSystemOpenEditor(nameHint string, initialContent string) string {
	if nameHint == "" {
		nameHint = "oneshot"
	}
	// Create a temporary directory to avoid filename collisions
	dir, err := os.MkdirTemp("", "one-shot-man-editor-*")
	if err != nil {
		return initialContent
	}
	// Use the sanitized hint as the exact basename to support tests that match on it
	base := sanitizeFilename(nameHint)
	if base == "" {
		base = "oneshot"
	}
	path := filepath.Join(dir, base)
	if err := os.WriteFile(path, []byte(initialContent), 0600); err != nil {
		_ = os.RemoveAll(dir)
		return initialContent
	}

	// Choose editor per-OS with sensible defaults
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		if editor != "" {
			cmd = exec.CommandContext(e.ctx, editor, path)
		} else {
			// notepad blocks until closed and supports a simple CLI
			cmd = exec.CommandContext(e.ctx, "notepad", path)
		}
	default:
		if editor == "" {
			// try nano, then vi, then ed
			if _, err := exec.LookPath("nano"); err == nil {
				editor = "nano"
			} else if _, err := exec.LookPath("vi"); err == nil {
				editor = "vi"
			} else {
				editor = "ed"
			}
		}
		cmd = exec.CommandContext(e.ctx, editor, path)
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = e.stdout
	cmd.Stderr = e.stderr
	if err := cmd.Run(); err != nil {
		// Return initial content if editor failed
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

// jsSystemClipboardCopy copies text to the system clipboard (supports macOS pbcopy; fallback via command)
func (e *Engine) jsSystemClipboardCopy(text string) error {
	// Allow override via env (treated as a shell-like command, basic split on spaces honoring quotes is not trivial;
	// to keep safe, require simple binary + args split by spaces with no quotes)
	if cmdStr := os.Getenv("ONESHOT_CLIPBOARD_CMD"); cmdStr != "" {
		var c *exec.Cmd
		if runtime.GOOS == "windows" {
			c = exec.CommandContext(e.ctx, "cmd", "/c", cmdStr)
		} else {
			c = exec.CommandContext(e.ctx, "/bin/sh", "-c", cmdStr)
		}
		c.Stdin = strings.NewReader(text)
		if err := c.Run(); err == nil {
			return nil
		}
	}

	switch runtime.GOOS {
	case "darwin":
		if _, err := exec.LookPath("pbcopy"); err == nil {
			c := exec.CommandContext(e.ctx, "pbcopy")
			c.Stdin = strings.NewReader(text)
			return c.Run()
		}
	case "windows":
		if _, err := exec.LookPath("clip"); err == nil {
			c := exec.CommandContext(e.ctx, "clip")
			c.Stdin = strings.NewReader(text)
			return c.Run()
		}
	default:
		// Linux / BSDs: try wl-copy, xclip, xsel in that order
		if _, err := exec.LookPath("wl-copy"); err == nil {
			c := exec.CommandContext(e.ctx, "wl-copy")
			c.Stdin = strings.NewReader(text)
			return c.Run()
		}
		if _, err := exec.LookPath("xclip"); err == nil {
			c := exec.CommandContext(e.ctx, "xclip", "-selection", "clipboard")
			c.Stdin = strings.NewReader(text)
			return c.Run()
		}
		if _, err := exec.LookPath("xsel"); err == nil {
			c := exec.CommandContext(e.ctx, "xsel", "--clipboard", "--input")
			c.Stdin = strings.NewReader(text)
			return c.Run()
		}
	}

	// Best-effort fallback: print a notice and write to stdout
	e.logger.PrintfToTUI("[clipboard] No system clipboard utility available; printing content below\n%s", text)
	return nil
}

// jsSystemReadFile reads a file from disk and returns an object with content or error info.
func (e *Engine) jsSystemReadFile(path string) map[string]interface{} {
	if path == "" {
		return map[string]interface{}{"error": true, "message": "empty path", "content": ""}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return map[string]interface{}{"error": true, "message": err.Error(), "content": ""}
	}
	return map[string]interface{}{"error": false, "content": string(data)}
}

// jsSystemParseArgv parses a shell-like command line into argv using a POSIX-compliant tokenizer.
// It returns an array of strings suitable for execv or further processing.
func (e *Engine) jsSystemParseArgv(s string) []string {
	return argv.ParseSlice(s)
}

// sanitizeFilename produces a filesystem-safe portion for temp filenames
func sanitizeFilename(s string) string {
	// Allow only alphanumeric, dash, underscore, dot; replace others with '-'
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
	// Collapse multiple dashes
	for strings.Contains(out, "--") {
		out = strings.ReplaceAll(out, "--", "-")
	}
	out = strings.Trim(out, "-")
	if out == "" {
		out = "untitled"
	}
	return out
}
