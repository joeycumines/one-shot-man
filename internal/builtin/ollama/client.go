package ollama

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client is an HTTP client for the Ollama REST API.
type Client struct {
	baseURL    string
	httpClient *http.Client
}

// ClientOption configures a Client.
type ClientOption func(*Client)

// WithHTTPClient sets a custom http.Client for the Ollama client.
func WithHTTPClient(c *http.Client) ClientOption {
	return func(cl *Client) {
		cl.httpClient = c
	}
}

// WithTimeout sets the HTTP client timeout.
func WithTimeout(d time.Duration) ClientOption {
	return func(cl *Client) {
		cl.httpClient.Timeout = d
	}
}

// NewClient creates a new Ollama API client.
// The baseURL should be the root URL of the Ollama server (e.g., "http://localhost:11434").
// If baseURL is empty, DefaultEndpoint is used.
func NewClient(baseURL string, opts ...ClientOption) (*Client, error) {
	if baseURL == "" {
		baseURL = DefaultEndpoint
	}

	// Validate URL.
	u, err := url.Parse(baseURL)
	if err != nil {
		return nil, fmt.Errorf("ollama: invalid base URL %q: %w", baseURL, err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("ollama: base URL must use http or https scheme, got %q", u.Scheme)
	}

	// Strip trailing slash for consistent path joining.
	baseURL = strings.TrimRight(baseURL, "/")

	c := &Client{
		baseURL: baseURL,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

// DefaultClient returns a client configured for the default endpoint (localhost:11434).
func DefaultClient() *Client {
	c, _ := NewClient(DefaultEndpoint)
	return c
}

// BaseURL returns the base URL of the Ollama server.
func (c *Client) BaseURL() string {
	return c.baseURL
}

// Health checks if the Ollama server is running.
// GET / returns "Ollama is running" when healthy.
func (c *Client) Health(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL, nil)
	if err != nil {
		return fmt.Errorf("ollama: health check: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ollama: health check: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama: health check: unexpected status %d", resp.StatusCode)
	}

	return nil
}

// IsHealthy returns true if the Ollama server is reachable and healthy.
func (c *Client) IsHealthy(ctx context.Context) bool {
	return c.Health(ctx) == nil
}

// Version returns the Ollama server version.
func (c *Client) Version(ctx context.Context) (VersionResponse, error) {
	var result VersionResponse
	if err := c.doJSON(ctx, http.MethodGet, "/api/version", nil, &result); err != nil {
		return result, err
	}
	return result, nil
}

// ListModels returns all locally available models.
func (c *Client) ListModels(ctx context.Context) ([]Model, error) {
	var result ModelListResponse
	if err := c.doJSON(ctx, http.MethodGet, "/api/tags", nil, &result); err != nil {
		return nil, err
	}
	return result.Models, nil
}

// ShowModel returns detailed information about a specific model.
func (c *Client) ShowModel(ctx context.Context, name string) (ModelInfo, error) {
	body := struct {
		Name string `json:"name"`
	}{Name: name}

	var result ModelInfo
	if err := c.doJSON(ctx, http.MethodPost, "/api/show", body, &result); err != nil {
		return result, err
	}
	return result, nil
}

// ListRunning returns currently loaded/running models.
func (c *Client) ListRunning(ctx context.Context) ([]RunningModel, error) {
	var result RunningModelResponse
	if err := c.doJSON(ctx, http.MethodGet, "/api/ps", nil, &result); err != nil {
		return nil, err
	}
	return result.Models, nil
}

// Chat performs a non-streaming chat completion.
// The request's Stream field is forced to false.
func (c *Client) Chat(ctx context.Context, req ChatRequest) (ChatResponse, error) {
	req.Stream = BoolPtr(false)

	var result ChatResponse
	if err := c.doJSON(ctx, http.MethodPost, "/api/chat", req, &result); err != nil {
		return result, err
	}
	return result, nil
}

// ChatStream starts a streaming chat completion.
// Returns a ChatStreamReader that yields incremental ChatResponse objects.
// The caller MUST call Close() on the reader when done.
func (c *Client) ChatStream(ctx context.Context, req ChatRequest) (*ChatStreamReader, error) {
	req.Stream = BoolPtr(true)

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("ollama: chat stream: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("ollama: chat stream: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
		return nil, parseHTTPError(resp.StatusCode, resp.Status, body)
	}

	return &ChatStreamReader{
		body:    resp.Body,
		scanner: bufio.NewScanner(resp.Body),
	}, nil
}

// ChatStreamReader reads streaming chat responses (NDJSON).
type ChatStreamReader struct {
	body    io.ReadCloser
	scanner *bufio.Scanner
	done    bool
	closed  bool
}

// Next reads the next ChatResponse from the stream.
// Returns io.EOF when the stream is complete (the final done:true response
// has already been returned by the previous call).
func (r *ChatStreamReader) Next() (ChatResponse, error) {
	var resp ChatResponse

	if r.done {
		return resp, io.EOF
	}
	if r.closed {
		return resp, fmt.Errorf("ollama: stream reader is closed")
	}

	for r.scanner.Scan() {
		line := r.scanner.Text()
		if line == "" {
			continue
		}

		if err := json.Unmarshal([]byte(line), &resp); err != nil {
			return resp, fmt.Errorf("ollama: stream decode: %w", err)
		}

		if resp.Done {
			r.done = true
		}

		return resp, nil
	}

	if err := r.scanner.Err(); err != nil {
		return resp, fmt.Errorf("ollama: stream read: %w", err)
	}

	// Scanner finished without done:true — unexpected EOF.
	return resp, io.EOF
}

// Close releases the stream resources. Safe to call multiple times.
func (r *ChatStreamReader) Close() error {
	if r.closed {
		return nil
	}
	r.closed = true
	r.done = true
	return r.body.Close()
}

// doJSON performs a JSON request/response cycle.
func (c *Client) doJSON(ctx context.Context, method, path string, reqBody interface{}, result interface{}) error {
	var bodyReader io.Reader
	if reqBody != nil {
		bodyBytes, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("ollama: marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(bodyBytes)
	}

	httpReq, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, bodyReader)
	if err != nil {
		return fmt.Errorf("ollama: %s %s: %w", method, path, err)
	}
	if bodyReader != nil {
		httpReq.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("ollama: %s %s: %w", method, path, err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20)) // 10 MiB limit
	if err != nil {
		return fmt.Errorf("ollama: %s %s: read body: %w", method, path, err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseHTTPError(resp.StatusCode, resp.Status, body)
	}

	if result != nil && len(body) > 0 {
		if err := json.Unmarshal(body, result); err != nil {
			return fmt.Errorf("ollama: %s %s: decode response: %w", method, path, err)
		}
	}

	return nil
}

// parseHTTPError parses an error response from the Ollama API.
func parseHTTPError(statusCode int, status string, body []byte) *OllamaError {
	e := &OllamaError{
		StatusCode: statusCode,
		Status:     status,
		Body:       string(body),
	}

	// Try to parse JSON error message.
	var jsonErr struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(body, &jsonErr); err == nil && jsonErr.Error != "" {
		e.ErrorMessage = jsonErr.Error
	}

	return e
}
