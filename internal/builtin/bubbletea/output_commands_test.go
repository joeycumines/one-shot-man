package bubbletea

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"github.com/joeycumines/goja"
)

func TestRunProgram_TerminalControlCommands(t *testing.T) {
	if testing.Short() {
		t.Skip("runs BubbleTea terminal commands")
	}
	for _, command := range []struct {
		name     string
		cmd      tea.Cmd
		sequence string
	}{
		{"foreground_color", tea.RequestForegroundColor, ansi.RequestForegroundColor},
		{"background_color", tea.RequestBackgroundColor, ansi.RequestBackgroundColor},
		{"cursor_color", tea.RequestCursorColor, ansi.RequestCursorColor},
		{"cursor_position", tea.RequestCursorPosition, ansi.RequestCursorPositionReport},
		{"terminal_version", tea.RequestTerminalVersion, ansi.RequestNameVersion},
		{"program_status", tea.RequestProgramStatusSupport, ansi.RequestProgramStatusSupport},
		{"capability", tea.RequestCapability("RGB"), ansi.RequestTermcap("RGB")},
		{"read_clipboard", tea.ReadClipboard, ansi.RequestSystemClipboard},
		{"read_primary_clipboard", tea.ReadPrimaryClipboard, ansi.RequestPrimaryClipboard},
		{"set_clipboard", tea.SetClipboard("clipboard payload"), ansi.SetSystemClipboard("clipboard payload")},
		{"set_primary_clipboard", tea.SetPrimaryClipboard("primary payload"), ansi.SetPrimaryClipboard("primary payload")},
	} {
		t.Run(command.name, func(t *testing.T) {
			for _, outputTTY := range []bool{false, true} {
				name := "redirected"
				if outputTTY {
					name = "terminal"
				}
				t.Run(name, func(t *testing.T) {
					r, w, err := os.Pipe()
					if err != nil {
						t.Fatal(err)
					}
					defer r.Close()
					defer w.Close()
					// The capability is injected, but the descriptor is a pipe:
					// this never changes the host's terminal or clipboard.
					output := &policyOutput{fd: w.Fd(), terminal: outputTTY}
					ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
					defer cancel()
					manager := NewManagerWithStderr(ctx, strings.NewReader(""), output, io.Discard, &SyncJSRunner{Runtime: goja.New()}, nil, nil)
					const directOutput = "ordinary output\n"
					model := outputPolicyModel{
						ready: make(chan struct{}),
						// Sequence sends the query before the marker and Quit,
						// without input timing or renderer ticks as a barrier.
						initialCmd: tea.Sequence(command.cmd, tea.Raw(directOutput), tea.Quit),
					}
					if err := manager.runProgram(model); err != nil {
						t.Fatal(err)
					}
					if !outputTTY {
						if got := output.String(); got != directOutput {
							t.Fatalf("redirected output = %q, want only %q", got, directOutput)
						}
						return
					}
					for _, want := range []string{command.sequence, directOutput} {
						if !strings.Contains(output.String(), want) {
							t.Errorf("terminal output %q is missing %q", output.String(), want)
						}
					}
				})
			}
		})
	}
}

func TestRunProgram_HeadlessJSColorQueries(t *testing.T) {
	if testing.Short() {
		t.Skip("runs BubbleTea with a JavaScript model")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	// Like scripting.TUIReader with piped stdin, this explicitly provided
	// wrapper is nonterminal and unwraps to a real file. Input remains enabled
	// even at EOF, so BubbleTea's input-disabled query guard cannot mask a leak.
	input := &stdinBackedReaderStub{file: r}
	var output bytes.Buffer
	vm := goja.New()
	manager := NewManagerWithStderr(ctx, input, &output, io.Discard, &SyncJSRunner{Runtime: vm}, nil, nil)
	module := vm.NewObject()
	Require(ctx, manager)(vm, module)
	if err := vm.Set("tea", module.Get("exports")); err != nil {
		t.Fatal(err)
	}
	wrapper, err := vm.RunString(`tea.newModel({
		init: function() { return [{}, tea.sequence(tea.requestForegroundColor(), tea.requestBackgroundColor(), tea.quit())]; },
		update: function(msg, model) { return [model, null]; },
		view: function() { return ""; }
	})`)
	if err != nil {
		t.Fatal(err)
	}
	getModel, ok := goja.AssertFunction(wrapper.ToObject(vm).Get("_getModel"))
	if !ok {
		t.Fatal("newModel returned no model")
	}
	modelValue, err := getModel(goja.Undefined())
	if err != nil {
		t.Fatal(err)
	}
	model, ok := modelValue.Export().(*jsModel)
	if !ok {
		t.Fatal("newModel did not wrap a JavaScript model")
	}
	// The VM is now used only by the program's serialized model callbacks.
	if err := manager.runProgram(model); err != nil {
		t.Fatal(err)
	}
	if output.Len() != 0 {
		t.Fatalf("redirected output received JavaScript-requested terminal controls: %q", output.String())
	}
}

const outputExecHelperArg = "osm-bubbletea-output-helper"

func TestRunProgram_OutputExecHelper(t *testing.T) {
	if len(os.Args) == 0 || os.Args[len(os.Args)-1] != outputExecHelperArg {
		return
	}
	if _, err := io.WriteString(os.Stdout, "external output\n"); err != nil {
		os.Exit(2)
	}
	os.Exit(0)
}

type outputExecResult struct{ err error }

type outputExecModel struct {
	initialCmd tea.Cmd
	completed  bool
	err        error
}

func (m *outputExecModel) Init() tea.Cmd { return m.initialCmd }
func (m *outputExecModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if result, ok := msg.(outputExecResult); ok {
		m.completed = true
		m.err = result.err
		return m, tea.Sequence(tea.Raw("direct after\n"), tea.Quit)
	}
	return m, nil
}
func (*outputExecModel) View() tea.View { return tea.NewView("") }

func TestRunProgram_HeadlessPreservesDirectAndExecOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("runs BubbleTea and a subprocess")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	var output bytes.Buffer
	manager := NewManagerWithStderr(ctx, strings.NewReader(""), &output, io.Discard, &SyncJSRunner{Runtime: goja.New()}, nil, nil)
	command := exec.CommandContext(ctx, os.Args[0], "-test.run=^TestRunProgram_OutputExecHelper$", "--", outputExecHelperArg)
	model := &outputExecModel{
		initialCmd: tea.Sequence(
			tea.RequestForegroundColor,
			tea.RequestBackgroundColor,
			tea.Raw("direct before\n"),
			tea.ExecProcess(command, func(err error) tea.Msg { return outputExecResult{err: err} }),
		),
	}
	if err := manager.runProgram(model); err != nil {
		t.Fatal(err)
	}
	if !model.completed || model.err != nil {
		t.Fatalf("external command did not complete successfully: completed=%v, error=%v", model.completed, model.err)
	}
	const want = "direct before\nexternal output\ndirect after\n"
	if got := output.String(); got != want {
		t.Fatalf("redirected output = %q, want %q", got, want)
	}
}
