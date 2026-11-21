package bubbletea

import (
	"bytes"
	"context"
	"io"
	"os"
	"reflect"
	"runtime"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/creack/pty"
	"github.com/joeycumines/goja"
	"golang.org/x/term"
)

// These wrappers inject the TerminalChecker contract independently for each
// stream. Their descriptors are real pipes, so BubbleTea itself never changes
// the test process's terminal state. The PTY test below checks actual detection.
type policyInput struct {
	io.Reader
	fd       uintptr
	terminal bool
}

func (p *policyInput) Fd() uintptr      { return p.fd }
func (p *policyInput) IsTerminal() bool { return p.terminal }

type policyOutput struct {
	bytes.Buffer
	fd       uintptr
	terminal bool
}

func (p *policyOutput) Fd() uintptr      { return p.fd }
func (p *policyOutput) IsTerminal() bool { return p.terminal }

type outputPolicyModel struct {
	ready           chan struct{}
	keys            chan string
	initialCmd      tea.Cmd
	toggleComplete  chan struct{}
	quitAfterToggle bool
}

func (m outputPolicyModel) Init() tea.Cmd {
	close(m.ready)
	return m.initialCmd
}

func (m outputPolicyModel) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if _, ok := message.(toggleReturnMsg); ok {
		if m.toggleComplete != nil {
			close(m.toggleComplete)
		}
		if m.quitAfterToggle {
			return m, tea.Quit
		}
	}
	if key, ok := message.(tea.KeyPressMsg); ok && key.String() == "q" {
		m.keys <- key.String()
		return m, tea.Quit
	}
	return m, nil
}

func TestRunProgram_HeadlessToggleDoesNotWriteEscapes(t *testing.T) {
	if testing.Short() {
		t.Skip("runs a BubbleTea toggle callback")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	vm := goja.New()
	runner := &SyncJSRunner{Runtime: vm}
	var output bytes.Buffer
	manager := NewManagerWithStderr(ctx, strings.NewReader(""), &output, io.Discard, runner, nil, nil)
	called := make(chan struct{}, 1)
	var wrapper *toggleModel
	wrapper = &toggleModel{
		inner: outputPolicyModel{
			ready: make(chan struct{}), keys: make(chan string, 1), quitAfterToggle: true,
			initialCmd: func() tea.Msg { return wrapper.toggleCmd()() },
		},
		toggleKey: 0x1d,
		jsRunner:  runner,
		ctx:       ctx,
		output:    &output,
		onToggle: func(goja.Value, ...goja.Value) (goja.Value, error) {
			called <- struct{}{}
			return goja.Undefined(), nil
		},
	}
	if err := manager.runProgram(wrapper); err != nil {
		t.Fatal(err)
	}
	if len(called) != 1 {
		t.Fatal("toggle callback was not executed")
	}
	if output.Len() != 0 {
		t.Fatalf("headless toggle polluted output: %q", output.String())
	}
}

func TestRunProgram_RedirectedOutputToggleRestoresTerminal(t *testing.T) {
	if testing.Short() {
		t.Skip("runs BubbleTea terminal handoff with a native PTY")
	}
	if runtime.GOOS == "windows" {
		t.Skip("Unix PTY handoff; the portable headless toggle test also runs on Windows")
	}
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer master.Close()
	defer slave.Close()
	original, err := term.GetState(int(slave.Fd()))
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	vm := goja.New()
	runner := &SyncJSRunner{Runtime: vm}
	var output bytes.Buffer
	manager := NewManagerWithStderr(ctx, slave, &output, io.Discard, runner, nil, nil)
	model := outputPolicyModel{ready: make(chan struct{}), keys: make(chan string, 1), toggleComplete: make(chan struct{})}
	released := make(chan bool, 1)
	wrapper := &toggleModel{
		inner: model, toggleKey: 0x1d, jsRunner: runner, ctx: ctx, output: &output,
		onToggle: func(goja.Value, ...goja.Value) (goja.Value, error) {
			state, err := term.GetState(int(slave.Fd()))
			released <- err == nil && reflect.DeepEqual(state, original)
			return goja.Undefined(), err
		},
	}
	finished := make(chan error, 1)
	go func() { finished <- manager.runProgram(wrapper) }()
	select {
	case <-model.ready:
	case err := <-finished:
		t.Fatalf("program exited before input: %v", err)
	case <-ctx.Done():
		t.Fatal("program did not initialize")
	}
	if state, err := term.GetState(int(slave.Fd())); err != nil || reflect.DeepEqual(state, original) {
		t.Fatalf("terminal input was not made raw: %v", err)
	}
	if _, err := master.Write([]byte{0x1d}); err != nil {
		t.Fatal(err)
	}
	select {
	case <-model.toggleComplete:
	case <-ctx.Done():
		t.Fatal("terminal handoff did not complete")
	}
	if !<-released {
		t.Fatal("toggle callback did not receive the restored input terminal")
	}
	if state, err := term.GetState(int(slave.Fd())); err != nil || reflect.DeepEqual(state, original) {
		t.Fatalf("terminal input was not made raw again after handoff: %v", err)
	}
	if _, err := master.WriteString("q"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("program did not consume the key after handoff")
	}
	if state, err := term.GetState(int(slave.Fd())); err != nil || !reflect.DeepEqual(state, original) {
		t.Fatalf("terminal input was not restored after program exit: %v", err)
	}
	if output.Len() != 0 {
		t.Fatalf("redirected toggle polluted output: %q", output.String())
	}
}

func (m outputPolicyModel) View() tea.View { return tea.NewView("terminal view") }

func TestRunProgram_OutputTerminalPolicy(t *testing.T) {
	if testing.Short() {
		t.Skip("runs BubbleTea programs")
	}
	for _, tc := range []struct {
		name      string
		inputTTY  bool
		outputTTY bool
	}{
		{"neither_terminal", false, false},
		{"terminal_input_redirected_output", true, false},
		{"piped_input_terminal_output", false, true},
		{"both_terminals", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r, w, err := os.Pipe()
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			defer w.Close()
			input := &policyInput{Reader: strings.NewReader("q"), fd: r.Fd(), terminal: tc.inputTTY}
			output := &policyOutput{fd: w.Fd(), terminal: tc.outputTTY}
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			manager := NewManagerWithStderr(ctx, input, output, io.Discard, &SyncJSRunner{Runtime: goja.New()}, nil, nil)
			if manager.IsTTY() != (tc.inputTTY || tc.outputTTY) {
				t.Fatal("combined terminal capability changed")
			}
			model := outputPolicyModel{ready: make(chan struct{}), keys: make(chan string, 1)}
			if err := manager.runProgram(model); err != nil {
				t.Fatal(err)
			}
			select {
			case key := <-model.keys:
				if key != "q" {
					t.Fatalf("key = %q", key)
				}
			default:
				t.Fatal("explicit input was not consumed")
			}
			if !tc.outputTTY {
				if output.Len() != 0 {
					t.Fatalf("redirected output contains terminal rendering: %q", output.String())
				}
				return
			}
			for _, sequence := range []string{"\x1b[?2004h", "\x1b[?2004l"} {
				if !strings.Contains(output.String(), sequence) {
					t.Fatalf("terminal output is missing %q: %q", sequence, output.String())
				}
			}
		})
	}
}

func TestRunProgram_TerminalInputRedirectedOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("runs BubbleTea with a native PTY")
	}
	if runtime.GOOS == "windows" {
		t.Skip("Unix PTY detection; the portable policy matrix also runs on Windows")
	}
	master, slave, err := pty.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer master.Close()
	defer slave.Close()
	if !term.IsTerminal(int(slave.Fd())) {
		t.Fatal("PTY slave was not detected as a terminal")
	}
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if term.IsTerminal(int(w.Fd())) {
		t.Fatal("output pipe unexpectedly detected as a terminal")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	manager := NewManagerWithStderr(ctx, slave, w, io.Discard, &SyncJSRunner{Runtime: goja.New()}, nil, nil)
	if !manager.IsTTY() {
		t.Fatal("input terminal capability was lost")
	}
	model := outputPolicyModel{ready: make(chan struct{}), keys: make(chan string, 1)}
	finished := make(chan error, 1)
	go func() { finished <- manager.runProgram(model) }()
	select {
	case <-model.ready:
	case err := <-finished:
		t.Fatalf("program exited before input: %v", err)
	case <-ctx.Done():
		t.Fatal("program did not initialize")
	}
	if _, err := master.WriteString("q"); err != nil {
		t.Fatal(err)
	}
	select {
	case err := <-finished:
		if err != nil {
			t.Fatal(err)
		}
	case <-ctx.Done():
		t.Fatal("program did not consume the terminal key")
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	captured, err := io.ReadAll(r)
	if err != nil {
		t.Fatal(err)
	}
	if len(captured) != 0 {
		t.Fatalf("stdout pipe contains terminal rendering: %q", captured)
	}
}
