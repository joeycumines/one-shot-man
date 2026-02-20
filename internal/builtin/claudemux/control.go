package claudemux

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
)

// --- Protocol types ---

// ControlRequest is a typed JSON-RPC-like request sent over the control socket.
type ControlRequest struct {
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// ControlResponse is a typed JSON-RPC-like response.
type ControlResponse struct {
	OK     bool            `json:"ok"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  string          `json:"error,omitempty"`
}

// EnqueueTaskParams are the parameters for the EnqueueTask method.
type EnqueueTaskParams struct {
	Task string `json:"task"`
}

// EnqueueTaskResult is the response for EnqueueTask.
type EnqueueTaskResult struct {
	Position int `json:"position"` // 0-based queue position
}

// GetStatusResult is the response for GetStatus.
type GetStatusResult struct {
	ActiveTask string   `json:"activeTask,omitempty"` // currently executing task, if any
	QueueDepth int      `json:"queueDepth"`           // number of queued tasks
	Queue      []string `json:"queue"`                // queued task descriptions
}

// --- Control handler interface ---

// ControlHandler processes control requests. Implementations are provided
// by the orchestrator (e.g., claudemux run command) and called by
// ControlServer for each incoming request.
type ControlHandler interface {
	// EnqueueTask adds a task to the orchestration queue.
	// Returns the 0-based position in the queue.
	EnqueueTask(task string) (int, error)

	// InterruptCurrent sends cancellation to the currently active task.
	// Returns an error if no task is active.
	InterruptCurrent() error

	// GetStatus returns the current orchestration state.
	GetStatus() GetStatusResult
}

// --- Control server ---

// ControlServer listens on a Unix domain socket and dispatches incoming
// JSON control requests to a ControlHandler. It supports concurrent
// connections, each served on a dedicated goroutine.
type ControlServer struct {
	listener net.Listener
	handler  ControlHandler
	sockPath string

	mu       sync.Mutex
	closed   bool
	connWg   sync.WaitGroup
}

// NewControlServer creates a control server bound to the given Unix socket
// path. The server does NOT start listening until Start is called.
func NewControlServer(sockPath string, handler ControlHandler) *ControlServer {
	return &ControlServer{
		sockPath: sockPath,
		handler:  handler,
	}
}

// Start begins listening on the Unix socket and accepting connections
// in a background goroutine.
func (s *ControlServer) Start() error {
	// Remove stale socket file if present (e.g., from a previous crash).
	_ = os.Remove(s.sockPath)

	ln, err := net.Listen("unix", s.sockPath)
	if err != nil {
		return fmt.Errorf("control server: listen %s: %w", s.sockPath, err)
	}
	s.listener = ln

	s.connWg.Add(1)
	go func() {
		defer s.connWg.Done()
		s.acceptLoop()
	}()

	return nil
}

// Close shuts down the server: stops accepting new connections, closes
// existing connections, and removes the socket file.
func (s *ControlServer) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	s.mu.Unlock()

	var closeErr error
	if s.listener != nil {
		closeErr = s.listener.Close()
	}

	// Wait for all connection goroutines to finish.
	s.connWg.Wait()

	// Clean up socket file.
	_ = os.Remove(s.sockPath)

	return closeErr
}

// SocketPath returns the path to the Unix socket.
func (s *ControlServer) SocketPath() string {
	return s.sockPath
}

// acceptLoop accepts incoming connections until the listener is closed.
func (s *ControlServer) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			s.mu.Lock()
			closed := s.closed
			s.mu.Unlock()
			if closed {
				return // expected: server was shut down
			}
			continue // transient accept error
		}

		s.connWg.Add(1)
		go func() {
			defer s.connWg.Done()
			defer conn.Close()
			s.handleConn(conn)
		}()
	}
}

// handleConn processes a single client connection. One request per
// connection (connect → send request → receive response → disconnect).
func (s *ControlServer) handleConn(conn net.Conn) {
	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		return // client disconnected without sending
	}
	line := scanner.Bytes()

	var req ControlRequest
	if err := json.Unmarshal(line, &req); err != nil {
		resp := ControlResponse{OK: false, Error: fmt.Sprintf("invalid request: %v", err)}
		data, _ := json.Marshal(resp)
		_, _ = conn.Write(append(data, '\n'))
		return
	}

	resp := s.dispatch(req)
	data, _ := json.Marshal(resp)
	_, _ = conn.Write(append(data, '\n'))
}

// dispatch routes a request to the appropriate handler method.
func (s *ControlServer) dispatch(req ControlRequest) ControlResponse {
	switch req.Method {
	case "EnqueueTask":
		return s.handleEnqueueTask(req.Params)
	case "InterruptCurrent":
		return s.handleInterruptCurrent()
	case "GetStatus":
		return s.handleGetStatus()
	default:
		return ControlResponse{OK: false, Error: fmt.Sprintf("unknown method: %q", req.Method)}
	}
}

func (s *ControlServer) handleEnqueueTask(params json.RawMessage) ControlResponse {
	var p EnqueueTaskParams
	if err := json.Unmarshal(params, &p); err != nil {
		return ControlResponse{OK: false, Error: fmt.Sprintf("invalid params: %v", err)}
	}
	if p.Task == "" {
		return ControlResponse{OK: false, Error: "task is required"}
	}
	pos, err := s.handler.EnqueueTask(p.Task)
	if err != nil {
		return ControlResponse{OK: false, Error: err.Error()}
	}
	result, _ := json.Marshal(EnqueueTaskResult{Position: pos})
	return ControlResponse{OK: true, Result: result}
}

func (s *ControlServer) handleInterruptCurrent() ControlResponse {
	if err := s.handler.InterruptCurrent(); err != nil {
		return ControlResponse{OK: false, Error: err.Error()}
	}
	return ControlResponse{OK: true}
}

func (s *ControlServer) handleGetStatus() ControlResponse {
	status := s.handler.GetStatus()
	result, _ := json.Marshal(status)
	return ControlResponse{OK: true, Result: result}
}

// --- Control client ---

// ControlClient connects to a ControlServer's Unix socket and sends
// typed requests.
type ControlClient struct {
	sockPath string
}

// NewControlClient creates a client that connects to the given socket path.
func NewControlClient(sockPath string) *ControlClient {
	return &ControlClient{sockPath: sockPath}
}

// EnqueueTask sends an EnqueueTask request and returns the queue position.
func (c *ControlClient) EnqueueTask(task string) (int, error) {
	params, _ := json.Marshal(EnqueueTaskParams{Task: task})
	resp, err := c.send(ControlRequest{Method: "EnqueueTask", Params: params})
	if err != nil {
		return 0, err
	}
	if !resp.OK {
		return 0, errors.New(resp.Error)
	}
	var result EnqueueTaskResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return 0, fmt.Errorf("decode result: %w", err)
	}
	return result.Position, nil
}

// InterruptCurrent sends an InterruptCurrent request.
func (c *ControlClient) InterruptCurrent() error {
	resp, err := c.send(ControlRequest{Method: "InterruptCurrent"})
	if err != nil {
		return err
	}
	if !resp.OK {
		return errors.New(resp.Error)
	}
	return nil
}

// GetStatus sends a GetStatus request and returns the orchestration state.
func (c *ControlClient) GetStatus() (*GetStatusResult, error) {
	resp, err := c.send(ControlRequest{Method: "GetStatus"})
	if err != nil {
		return nil, err
	}
	if !resp.OK {
		return nil, errors.New(resp.Error)
	}
	var result GetStatusResult
	if err := json.Unmarshal(resp.Result, &result); err != nil {
		return nil, fmt.Errorf("decode result: %w", err)
	}
	return &result, nil
}

// send connects to the socket, sends a request, reads the response,
// and disconnects.
func (c *ControlClient) send(req ControlRequest) (*ControlResponse, error) {
	conn, err := net.Dial("unix", c.sockPath)
	if err != nil {
		return nil, fmt.Errorf("connect %s: %w", c.sockPath, err)
	}
	defer conn.Close()

	data, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}
	if _, err := conn.Write(append(data, '\n')); err != nil {
		return nil, fmt.Errorf("send: %w", err)
	}

	scanner := bufio.NewScanner(conn)
	if !scanner.Scan() {
		if err := scanner.Err(); err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}
		return nil, errors.New("server closed connection without response")
	}

	var resp ControlResponse
	if err := json.Unmarshal(scanner.Bytes(), &resp); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	return &resp, nil
}
