package termmux

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/joeycumines/goja"
	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

// pipeModuleHelperSource is the same stdin copier helper used by the Go
// package tests. The JS integration tests call `go run <helper> <outFile>`.
const pipeModuleHelperSource = `package main

import (
	"io"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		panic("output file required")
	}
	f, err := os.Create(os.Args[1])
	if err != nil {
		panic(err)
	}
	defer f.Close()
	if _, err := io.Copy(f, os.Stdin); err != nil {
		panic(err)
	}
}
`

func writeModulePipeHelper(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "pipehelper.go")
	if err := os.WriteFile(path, []byte(pipeModuleHelperSource), 0o644); err != nil {
		t.Fatalf("write helper: %v", err)
	}
	return path
}

func pollFileEquals(t *testing.T, path, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.After(timeout)
	for {
		data, err := os.ReadFile(path)
		if err == nil && string(data) == want {
			return
		}
		select {
		case <-deadline:
			got := ""
			if data != nil {
				got = string(data)
			}
			t.Fatalf("timed out waiting for file %q; got %q", want, got)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

// feedStringIO is a test double for StringIO whose output is fed from a
// Go channel. Closing the channel signals EOF.
type feedStringIO struct {
	feed chan string
}

func (f *feedStringIO) Send(input string) error { return nil }

func (f *feedStringIO) Receive() (string, error) {
	msg, ok := <-f.feed
	if !ok {
		return "", io.EOF
	}
	return msg, nil
}

func (f *feedStringIO) Close() error {
	select {
	case <-f.feed:
	default:
		close(f.feed)
	}
	return nil
}

// setupPipeManager creates a running SessionManager with a controllable
// session that Go tests can feed output into from the outside.
func setupPipeManager(t *testing.T) (*goja.Runtime, *parent.SessionManager, chan<- string, func()) {
	t.Helper()

	mgr := parent.NewSessionManager(parent.WithTermSize(24, 80))
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)
	go func() { errCh <- mgr.Run(ctx) }()
	<-mgr.Started()

	runtime := goja.New()
	mux := wrapTestSessionManager(t, ctx, runtime, mgr, nil, nil, -1, "")
	_ = runtime.Set("mux", mux)

	feedCh := make(chan string, 16)
	sio := &feedStringIO{feed: feedCh}
	cs := parent.NewStringIOSession(sio)
	cs.Start()
	id, err := mgr.Register(cs, parent.SessionTarget{Name: "test", Kind: "pty"})
	if err != nil {
		cancel()
		<-errCh
		t.Fatalf("Register: %v", err)
	}
	_ = runtime.Set("sid", id)

	cleanup := func() {
		cancel()
		<-errCh
	}
	return runtime, mgr, feedCh, cleanup
}

func TestModule_PipeCommand_Basic(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}
	t.Parallel()

	runtime, _, readerCh, cleanup := setupPipeManager(t)
	defer cleanup()

	helper := writeModulePipeHelper(t)
	outPath := filepath.Join(t.TempDir(), "output.log")
	_ = runtime.Set("helper", helper)
	_ = runtime.Set("outPath", outPath)

	_, err := runtime.RunString(`mux.pipeCommand(sid, 'go', ['run', helper, outPath])`)
	if err != nil {
		t.Fatalf("pipeCommand: %v", err)
	}

	readerCh <- "hello from js pipe"
	pollFileEquals(t, outPath, "hello from js pipe", 30*time.Second)
}

func TestModule_PipeCommand_Clear(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}
	t.Parallel()

	runtime, _, readerCh, cleanup := setupPipeManager(t)
	defer cleanup()

	helper := writeModulePipeHelper(t)
	outPath := filepath.Join(t.TempDir(), "output.log")
	_ = runtime.Set("helper", helper)
	_ = runtime.Set("outPath", outPath)

	_, err := runtime.RunString(`mux.pipeCommand(sid, 'go', ['run', helper, outPath])`)
	if err != nil {
		t.Fatalf("pipeCommand: %v", err)
	}

	readerCh <- "before clear"
	pollFileEquals(t, outPath, "before clear", 30*time.Second)

	_, err = runtime.RunString(`mux.clearPipe(sid)`)
	if err != nil {
		t.Fatalf("clearPipe: %v", err)
	}

	readerCh <- "after clear"
	time.Sleep(50 * time.Millisecond)
	data, _ := os.ReadFile(outPath)
	if string(data) != "before clear" {
		t.Errorf("output after clear = %q, want %q", string(data), "before clear")
	}
}

func TestModule_PipeCommand_InvalidArgs(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}
	t.Parallel()

	runtime, _, _, cleanup := setupPipeManager(t)
	defer cleanup()

	_, err := runtime.RunString(`mux.pipeCommand(sid, '', [])`)
	if err == nil {
		t.Fatal("expected error for empty command")
	}
}

func TestModule_SetPipeFile_StillWorks(t *testing.T) {
	if testing.Short() {
		t.Skip("slow: spawns SessionManager worker goroutine")
	}
	t.Parallel()

	runtime, _, readerCh, cleanup := setupPipeManager(t)
	defer cleanup()

	outPath := filepath.Join(t.TempDir(), "file.log")
	_ = runtime.Set("outPath", outPath)

	_, err := runtime.RunString(`mux.setPipeFile(sid, outPath)`)
	if err != nil {
		t.Fatalf("setPipeFile: %v", err)
	}

	readerCh <- "hello from file pipe"
	pollFileEquals(t, outPath, "hello from file pipe", 30*time.Second)
}
