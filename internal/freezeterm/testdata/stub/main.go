// Command stub emulates the subset of the freeze CLI that internal/freezeterm
// exercises. Tests build it with `go build` and point Options.Executable at
// the binary; it records each invocation to the file named by
// FREEZETERM_STUB_RECORD and writes an artifact matching the requested output
// extension.
package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"
)

type invocation struct {
	Argv             []string `json:"argv"`
	Stdin            string   `json:"stdin"`
	StdinCharDevice  bool     `json:"stdinCharDevice"`
	Output           string   `json:"output"`
	Execute          string   `json:"execute"`
	InputPath        string   `json:"inputPath"`
	WorkingDirectory string   `json:"workingDirectory"`
}

func main() {
	args := os.Args[1:]

	if hasArg(args, "--version") {
		fmt.Println("freeze version v0.0.0 (stub0000)")
		return
	}

	if sleepMs, err := strconv.Atoi(os.Getenv("FREEZETERM_STUB_SLEEP_MS")); err == nil && sleepMs > 0 {
		time.Sleep(time.Duration(sleepMs) * time.Millisecond)
	}

	stdin, _ := io.ReadAll(os.Stdin)
	stdinCharDevice := false
	if info, err := os.Stdin.Stat(); err == nil {
		stdinCharDevice = info.Mode()&os.ModeCharDevice != 0
	}

	output := ""
	execute := ""
	inputPath := ""
	for i := 0; i < len(args); i++ {
		arg := args[i]
		name, inline, hasInline := strings.Cut(arg, "=")
		if valueFlags[name] {
			if !hasInline && i+1 < len(args) {
				inline = args[i+1]
				i++
			}
			switch name {
			case "-o", "--output":
				output = inline
			case "-x", "--execute":
				execute = inline
			}
			continue
		}
		if name == "-o" || name == "--output" {
			output = inline
			continue
		}
		if name == "-x" || name == "--execute" {
			execute = inline
			continue
		}
		if !strings.HasPrefix(arg, "-") {
			inputPath = arg
		}
	}

	wd, _ := os.Getwd()
	record := invocation{
		Argv:             args,
		Stdin:            base64.StdEncoding.EncodeToString(stdin),
		StdinCharDevice:  stdinCharDevice,
		Output:           output,
		Execute:          execute,
		InputPath:        inputPath,
		WorkingDirectory: wd,
	}
	appendRecord(record)

	if diagnostic := os.Getenv("FREEZETERM_STUB_STDERR"); diagnostic != "" {
		fmt.Fprintln(os.Stderr, diagnostic)
	}
	if code, err := strconv.Atoi(os.Getenv("FREEZETERM_STUB_EXIT")); err == nil && code != 0 {
		os.Exit(code)
	}

	if output == "" {
		fmt.Println("stub: missing output")
		os.Exit(2)
	}
	var err error
	switch {
	case strings.HasSuffix(output, ".png"):
		err = os.WriteFile(output, append([]byte("\x89PNG\r\n\x1a\n"), []byte("stub-png-bytes")...), 0o600)
	default:
		svg := fmt.Sprintf("<svg xmlns=\"http://www.w3.org/2000/svg\"><text>stub %s</text></svg>", base64.StdEncoding.EncodeToString(stdin))
		err = os.WriteFile(output, []byte(svg), 0o600)
	}
	if err != nil {
		fmt.Fprintf(os.Stderr, "stub: write artifact: %v\n", err)
		os.Exit(2)
	}
	fmt.Println("stub wrote", output)
}

func hasArg(args []string, name string) bool {
	for _, arg := range args {
		if arg == name || strings.HasPrefix(arg, name+"=") {
			return true
		}
	}
	return false
}

// valueFlags consume the following argv element when no inline value is
// present.
var valueFlags = map[string]bool{
	"-o": true, "--output": true,
	"-x": true, "--execute": true,
	"-t": true, "--theme": true,
	"-b": true, "--background": true,
	"-l": true, "--language": true,
	"-W": true, "--width": true,
	"-H": true, "--height": true,
	"-w": true, "--wrap": true,
	"-c": true, "--config": true,
	"--margin": true, "--padding": true,
	"--line-height": true, "--lines": true,
	"--border.radius": true, "--border.width": true, "--border.color": true,
	"--shadow.blur": true, "--shadow.x": true, "--shadow.y": true,
	"--font.family": true, "--font.file": true, "--font.size": true,
	"--execute.timeout": true,
}

func appendRecord(record invocation) {
	path := os.Getenv("FREEZETERM_STUB_RECORD")
	if path == "" {
		return
	}
	line, err := json.Marshal(record)
	if err != nil {
		return
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = f.Close() }()
	_, _ = f.Write(append(line, '\n'))
}
