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
// a non-terminal stdin. It mimics scripting.TUIReader as the engine constructs
// it (NewTUIReader) when the process runs without a TTY (CI, redirected stdin).
type stdinBackedReaderStub struct{ r io.Reader }

func (s *stdinBackedReaderStub) Read(p []byte) (int, error) { return s.r.Read(p) }
func (s *stdinBackedReaderStub) Fd() uintptr                 { return os.Stdin.Fd() }
func (s *stdinBackedReaderStub) IsTerminal() bool            { return false }
func (s *stdinBackedReaderStub) UnwrapFile() *os.File        { return os.Stdin }

// quitNowModel is a tea.Model that quits immediately from Init.
type quitNowModel struct{}

func (quitNowModel) Init() tea.Cmd                       { return tea.Quit }
func (quitNowModel) Update(tea.Msg) (tea.Model, tea.Cmd) { return quitNowModel{}, nil }
func (quitNowModel) View() tea.View                      { return tea.NewView("") }

// TestNewManager_StdinBackedNonTTYInputIsDefaulted pins the Windows-CI fix
// (go-tty nil-deref via TUIReader.Read during bubbletea teardown): when input
// unwraps to os.Stdin but stdin is not a terminal, the manager must mark
// inputDefaulted so runProgram's headless branch drops the reader
// (tea.WithInput(nil) => disableInput) instead of handing bubbletea a live
// stdin-backed reader whose cancelreader/go-tty path nil-derefs at teardown.
func TestNewManager_StdinBackedNonTTYInputIsDefaulted(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping program run in short mode")
	}
	vm := goja.New()
	var out bytes.Buffer
	in := &stdinBackedReaderStub{r: bytes.NewReader(nil)}
	m := NewManagerWithStderr(context.Background(), in, &out, &out, &SyncJSRunner{Runtime: vm}, nil, nil)
	require.False(t, m.IsTTY())
	require.True(t, m.inputDefaulted, "stdin-backed non-TTY input must be treated as defaulted so the headless branch drops it")

	errCh := make(chan error, 1)
	go func() { errCh <- m.runProgram(quitNowModel{}) }()
	select {
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(5 * time.Second):
		t.Fatal("headless program with stdin-backed non-TTY input did not exit")
	}
}

// TestNewManager_ExplicitNonStdinInputIsNotDefaulted ensures the broadened
// rule does not swallow a deliberately wired reader that is not stdin-backed.
func TestNewManager_ExplicitNonStdinInputIsNotDefaulted(t *testing.T) {
	vm := goja.New()
	var out bytes.Buffer
	m := NewManagerWithStderr(context.Background(), strings.NewReader("q"), &out, &out, &SyncJSRunner{Runtime: vm}, nil, nil)
	require.False(t, m.IsTTY())
	require.False(t, m.inputDefaulted, "an explicit non-stdin reader must stay preserved (not defaulted)")
}
