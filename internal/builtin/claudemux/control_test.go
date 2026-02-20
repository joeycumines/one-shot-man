package claudemux

import (
	"encoding/json"
	"errors"
	"os"
	"sync"
	"testing"
)

// --- Mock handler ---

type mockHandler struct {
	mu         sync.Mutex
	queue      []string
	activeTask string
	interruptN int
	enqueueErr error
	interrErr  error
}

func (h *mockHandler) EnqueueTask(task string) (int, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.enqueueErr != nil {
		return 0, h.enqueueErr
	}
	h.queue = append(h.queue, task)
	return len(h.queue) - 1, nil
}

func (h *mockHandler) InterruptCurrent() error {
	h.mu.Lock()
	defer h.mu.Unlock()
	if h.interrErr != nil {
		return h.interrErr
	}
	h.interruptN++
	return nil
}

func (h *mockHandler) GetStatus() GetStatusResult {
	h.mu.Lock()
	defer h.mu.Unlock()
	q := make([]string, len(h.queue))
	copy(q, h.queue)
	return GetStatusResult{
		ActiveTask: h.activeTask,
		QueueDepth: len(q),
		Queue:      q,
	}
}

// --- Helpers ---

// tempSockPath returns a short Unix socket path suitable for macOS's 104-char
// limit. Uses /tmp directly with a unique suffix.
func tempSockPath(t *testing.T) string {
	t.Helper()
	f, err := os.CreateTemp("", "ctrl*.sock")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	path := f.Name()
	f.Close()
	os.Remove(path)
	t.Cleanup(func() { os.Remove(path) })
	return path
}

func startTestServer(t *testing.T, handler ControlHandler) (*ControlServer, string) {
	t.Helper()
	sockPath := tempSockPath(t)
	srv := NewControlServer(sockPath, handler)
	if err := srv.Start(); err != nil {
		t.Fatalf("start server: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	return srv, sockPath
}

// --- Unit tests: protocol types ---

func TestControlRequest_JSONRoundTrip(t *testing.T) {
	params, _ := json.Marshal(EnqueueTaskParams{Task: "hello"})
	req := ControlRequest{Method: "EnqueueTask", Params: params}
	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ControlRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.Method != "EnqueueTask" {
		t.Fatalf("method = %q, want EnqueueTask", decoded.Method)
	}
	var p EnqueueTaskParams
	if err := json.Unmarshal(decoded.Params, &p); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if p.Task != "hello" {
		t.Fatalf("task = %q, want hello", p.Task)
	}
}

func TestControlResponse_JSONRoundTrip(t *testing.T) {
	result, _ := json.Marshal(EnqueueTaskResult{Position: 3})
	resp := ControlResponse{OK: true, Result: result}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ControlResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !decoded.OK {
		t.Fatal("ok = false, want true")
	}
	var r EnqueueTaskResult
	if err := json.Unmarshal(decoded.Result, &r); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if r.Position != 3 {
		t.Fatalf("position = %d, want 3", r.Position)
	}
}

func TestControlResponse_Error(t *testing.T) {
	resp := ControlResponse{OK: false, Error: "something broke"}
	data, err := json.Marshal(resp)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}

	var decoded ControlResponse
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if decoded.OK {
		t.Fatal("ok = true, want false")
	}
	if decoded.Error != "something broke" {
		t.Fatalf("error = %q, want 'something broke'", decoded.Error)
	}
}

// --- Integration tests: server + client loopback ---

func TestControl_EnqueueTask(t *testing.T) {
	handler := &mockHandler{}
	_, sockPath := startTestServer(t, handler)
	client := NewControlClient(sockPath)

	pos, err := client.EnqueueTask("fix bug #42")
	if err != nil {
		t.Fatalf("EnqueueTask: %v", err)
	}
	if pos != 0 {
		t.Fatalf("pos = %d, want 0", pos)
	}

	pos, err = client.EnqueueTask("add feature X")
	if err != nil {
		t.Fatalf("EnqueueTask 2: %v", err)
	}
	if pos != 1 {
		t.Fatalf("pos = %d, want 1", pos)
	}

	handler.mu.Lock()
	if len(handler.queue) != 2 {
		t.Fatalf("queue len = %d, want 2", len(handler.queue))
	}
	if handler.queue[0] != "fix bug #42" {
		t.Fatalf("queue[0] = %q", handler.queue[0])
	}
	handler.mu.Unlock()
}

func TestControl_EnqueueTask_EmptyTask(t *testing.T) {
	handler := &mockHandler{}
	_, sockPath := startTestServer(t, handler)
	client := NewControlClient(sockPath)

	_, err := client.EnqueueTask("")
	if err == nil {
		t.Fatal("expected error for empty task")
	}
	if err.Error() != "task is required" {
		t.Fatalf("error = %q, want 'task is required'", err)
	}
}

func TestControl_EnqueueTask_HandlerError(t *testing.T) {
	handler := &mockHandler{enqueueErr: errors.New("queue full")}
	_, sockPath := startTestServer(t, handler)
	client := NewControlClient(sockPath)

	_, err := client.EnqueueTask("overflow")
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "queue full" {
		t.Fatalf("error = %q, want 'queue full'", err)
	}
}

func TestControl_InterruptCurrent(t *testing.T) {
	handler := &mockHandler{}
	_, sockPath := startTestServer(t, handler)
	client := NewControlClient(sockPath)

	if err := client.InterruptCurrent(); err != nil {
		t.Fatalf("InterruptCurrent: %v", err)
	}

	handler.mu.Lock()
	if handler.interruptN != 1 {
		t.Fatalf("interruptN = %d, want 1", handler.interruptN)
	}
	handler.mu.Unlock()
}

func TestControl_InterruptCurrent_Error(t *testing.T) {
	handler := &mockHandler{interrErr: errors.New("no active task")}
	_, sockPath := startTestServer(t, handler)
	client := NewControlClient(sockPath)

	err := client.InterruptCurrent()
	if err == nil {
		t.Fatal("expected error")
	}
	if err.Error() != "no active task" {
		t.Fatalf("error = %q, want 'no active task'", err)
	}
}

func TestControl_GetStatus(t *testing.T) {
	handler := &mockHandler{
		activeTask: "building stuff",
		queue:      []string{"task A", "task B"},
	}
	_, sockPath := startTestServer(t, handler)
	client := NewControlClient(sockPath)

	status, err := client.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if status.ActiveTask != "building stuff" {
		t.Fatalf("activeTask = %q", status.ActiveTask)
	}
	if status.QueueDepth != 2 {
		t.Fatalf("queueDepth = %d, want 2", status.QueueDepth)
	}
	if len(status.Queue) != 2 || status.Queue[0] != "task A" {
		t.Fatalf("queue = %v", status.Queue)
	}
}

func TestControl_GetStatus_Empty(t *testing.T) {
	handler := &mockHandler{}
	_, sockPath := startTestServer(t, handler)
	client := NewControlClient(sockPath)

	status, err := client.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if status.ActiveTask != "" {
		t.Fatalf("activeTask = %q, want empty", status.ActiveTask)
	}
	if status.QueueDepth != 0 {
		t.Fatalf("queueDepth = %d, want 0", status.QueueDepth)
	}
}

func TestControl_UnknownMethod(t *testing.T) {
	handler := &mockHandler{}
	_, sockPath := startTestServer(t, handler)
	client := NewControlClient(sockPath)

	// Use the raw send method by constructing a custom request.
	resp, err := client.send(ControlRequest{Method: "DoSomethingWeird"})
	if err != nil {
		t.Fatalf("send: %v", err)
	}
	if resp.OK {
		t.Fatal("ok = true, want false")
	}
	if resp.Error == "" {
		t.Fatal("expected non-empty error")
	}
}

func TestControl_ConcurrentClients(t *testing.T) {
	handler := &mockHandler{}
	_, sockPath := startTestServer(t, handler)

	const numClients = 10
	var wg sync.WaitGroup
	errs := make([]error, numClients)

	for i := range numClients {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			c := NewControlClient(sockPath)
			_, errs[idx] = c.EnqueueTask("task from goroutine")
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("client %d: %v", i, err)
		}
	}

	handler.mu.Lock()
	if len(handler.queue) != numClients {
		t.Fatalf("queue len = %d, want %d", len(handler.queue), numClients)
	}
	handler.mu.Unlock()
}

func TestControl_ServerClose_CleansSocket(t *testing.T) {
	sockPath := tempSockPath(t)
	handler := &mockHandler{}
	srv := NewControlServer(sockPath, handler)

	if err := srv.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Socket file should exist.
	if _, err := os.Stat(sockPath); os.IsNotExist(err) {
		t.Fatal("socket file does not exist after start")
	}

	if err := srv.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}

	// Socket file should be removed.
	if _, err := os.Stat(sockPath); !os.IsNotExist(err) {
		t.Fatalf("socket file still exists after close (err: %v)", err)
	}
}

func TestControl_ServerClose_Idempotent(t *testing.T) {
	handler := &mockHandler{}
	sockPath := tempSockPath(t)
	srv := NewControlServer(sockPath, handler)
	if err := srv.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}

	// Double close must not panic or error.
	if err := srv.Close(); err != nil {
		t.Fatalf("close 1: %v", err)
	}
	if err := srv.Close(); err != nil {
		t.Fatalf("close 2: %v", err)
	}
}

func TestControl_ClientConnect_NoServer(t *testing.T) {
	client := NewControlClient("/tmp/nonexistent-socket-path-12345.sock")
	_, err := client.EnqueueTask("hello")
	if err == nil {
		t.Fatal("expected error when no server")
	}
}

func TestControl_StaleSocketRemoved(t *testing.T) {
	sockPath := tempSockPath(t)
	// Create a stale socket file.
	f, err := os.Create(sockPath)
	if err != nil {
		t.Fatalf("create stale: %v", err)
	}
	f.Close()

	handler := &mockHandler{}
	srv := NewControlServer(sockPath, handler)
	if err := srv.Start(); err != nil {
		t.Fatalf("start over stale socket: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })

	// Verify the server is functional even though a stale file existed.
	client := NewControlClient(sockPath)
	status, err := client.GetStatus()
	if err != nil {
		t.Fatalf("GetStatus: %v", err)
	}
	if status.QueueDepth != 0 {
		t.Fatalf("queueDepth = %d, want 0", status.QueueDepth)
	}
}

func TestControl_SocketPath(t *testing.T) {
	sockPath := tempSockPath(t)
	srv := NewControlServer(sockPath, &mockHandler{})
	if got := srv.SocketPath(); got != sockPath {
		t.Fatalf("SocketPath() = %q, want %q", got, sockPath)
	}
}
