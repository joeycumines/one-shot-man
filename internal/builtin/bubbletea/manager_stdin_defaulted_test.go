package bubbletea

import (
	"bytes"
	"context"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/joeycumines/goja"
	"github.com/stretchr/testify/require"
)

// stdinBackedReaderStub is a TerminalChecker + UnwrapFile wrapper that reports
// a non-terminal stdin and records whether its own Read was ever called. It
// mimics scripting.TUIReader as the engine constructs it (NewTUIReader) when
// the process runs without a TTY: runProgram must unwrap it via UnwrapFile()
// so BubbleTea reads the *os.File directly instead of invoking TUIReader.Read
// (which on Windows delegates to go-prompt/go-tty and panics on a non-TTY).
type stdinBackedReaderStub struct {
	file       *os.File
	readCalled bool
}

func (s *stdinBackedReaderStub) Read(p []byte) (int, error) {
	s.readCalled = true
	return 0, io.EOF
}
func (s *stdinBackedReaderStub) Fd() uintptr          { return s.file.Fd() }
func (s *stdinBackedReaderStub) IsTerminal() bool     { return false }
func (s *stdinBackedReaderStub) UnwrapFile() *os.File { return s.file }

// quitOnQModel quits when it receives a 'q' key message.
type quitOnQModel struct{}

func (quitOnQModel) Init() tea.Cmd { return nil }
func (quitOnQModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if km, ok := msg.(tea.KeyPressMsg); ok && km.String() == "q" {
		return quitOnQModel{}, tea.Quit
	}
	return quitOnQModel{}, nil
}
func (quitOnQModel) View() tea.View { return tea.NewView("") }

// TestRunProgram_HeadlessUnwrapsFileBackedInputAndConsumesPipedInput pins the
// headless stdin contract: an explicit file-backed wrapper (like TUIReader
// wrapping piped stdin) is NOT silently dropped (so `printf 'q' | osm` works)
// AND is unwrapped via UnwrapFile() so BubbleTea reads the *os.File directly
// without invoking the wrapper's Read() (bypassing go-prompt/go-tty on Windows).
func TestRunProgram_HeadlessUnwrapsFileBackedInputAndConsumesPipedInput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping program run in short mode")
	}
	r, w, err := os.Pipe()
	require.NoError(t, err)
	defer r.Close()
	_, err = w.WriteString("q")
	require.NoError(t, err)
	require.NoError(t, w.Close())

	vm := goja.New()
	var out bytes.Buffer
	in := &stdinBackedReaderStub{file: r}
	m := NewManagerWithStderr(context.Background(), in, &out, &out, &SyncJSRunner{Runtime: vm}, nil, nil)
	require.False(t, m.IsTTY())
	require.False(t, m.inputDefaulted, "explicit file-backed non-TTY input must not be dropped")

	errCh := make(chan error, 1)
	go func() { errCh <- m.runProgram(quitOnQModel{}) }()
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("headless program did not consume 'q' from unwrapped pipe input")
	}
	require.False(t, in.readCalled, "wrapper Read() must not be called; runProgram must unwrap *os.File directly")
}

// TestNewManager_ExplicitNonStdinInputIsNotDefaulted ensures a deliberately
// wired non-stdin reader is preserved (not defaulted).
func TestNewManager_ExplicitNonStdinInputIsNotDefaulted(t *testing.T) {
	vm := goja.New()
	var out bytes.Buffer
	m := NewManagerWithStderr(context.Background(), strings.NewReader("q"), &out, &out, &SyncJSRunner{Runtime: vm}, nil, nil)
	require.False(t, m.IsTTY())
	require.False(t, m.inputDefaulted, "an explicit non-stdin reader must stay preserved (not defaulted)")
}
