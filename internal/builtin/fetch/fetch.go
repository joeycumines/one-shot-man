// Package fetch provides a Goja module wrapping Go's net/http client for JS scripts.
// It is registered as "osm:fetch" and provides a synchronous, non-streaming
// fetch() function modeled after the browser Fetch API.
package fetch

import (
	"bufio"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/dop251/goja"
)

// Require is the Goja module loader for osm:fetch.
func Require(runtime *goja.Runtime, module *goja.Object) {
	exports := module.Get("exports").(*goja.Object)

	// fetch(url: string, options?: object): Response
	//
	// Options:
	//   method  - HTTP method (default: "GET")
	//   headers - object of header key/value pairs
	//   body    - request body string
	//   timeout - request timeout in seconds (default: 30)
	//
	// Response:
	//   status     - HTTP status code (number)
	//   ok         - true if status is 200-299 (boolean)
	//   statusText - HTTP status line, e.g. "200 OK" (string)
	//   url        - final URL after redirects (string)
	//   headers    - response headers object (lowercase keys)
	//   text()     - response body as string
	//   json()     - response body parsed as JSON (native JS object)
	_ = exports.Set("fetch", func(call goja.FunctionCall) goja.Value {
		url := call.Argument(0).String()

		method := "GET"
		timeout := 30 * time.Second
		var bodyReader io.Reader
		var reqHeaders map[string]interface{}

		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Arguments[1]) && !goja.IsNull(call.Arguments[1]) {
			if opts, ok := call.Arguments[1].Export().(map[string]interface{}); ok {
				if m, ok := opts["method"]; ok {
					if s, ok := m.(string); ok {
						method = strings.ToUpper(s)
					}
				}
				if t, ok := opts["timeout"]; ok {
					switch v := t.(type) {
					case int64:
						timeout = time.Duration(v) * time.Second
					case float64:
						timeout = time.Duration(v * float64(time.Second))
					}
				}
				if b, ok := opts["body"]; ok {
					if s, ok := b.(string); ok {
						bodyReader = strings.NewReader(s)
					}
				}
				if h, ok := opts["headers"]; ok {
					if m, ok := h.(map[string]interface{}); ok {
						reqHeaders = m
					}
				}
			}
		}

		req, err := http.NewRequest(method, url, bodyReader)
		if err != nil {
			panic(runtime.NewGoError(err))
		}

		for k, v := range reqHeaders {
			if s, ok := v.(string); ok {
				req.Header.Set(k, s)
			}
		}

		client := &http.Client{Timeout: timeout}
		resp, err := client.Do(req)
		if err != nil {
			panic(runtime.NewGoError(err))
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			panic(runtime.NewGoError(err))
		}

		result := runtime.NewObject()
		_ = result.Set("status", resp.StatusCode)
		_ = result.Set("ok", resp.StatusCode >= 200 && resp.StatusCode < 300)
		_ = result.Set("statusText", resp.Status)
		_ = result.Set("url", resp.Request.URL.String())

		headersObj := runtime.NewObject()
		for k, v := range resp.Header {
			if len(v) == 1 {
				_ = headersObj.Set(strings.ToLower(k), v[0])
			} else {
				_ = headersObj.Set(strings.ToLower(k), strings.Join(v, ", "))
			}
		}
		_ = result.Set("headers", headersObj)

		bodyStr := string(body)
		_ = result.Set("text", func() string { return bodyStr })

		_ = result.Set("json", func() goja.Value {
			var parsed interface{}
			if err := json.Unmarshal(body, &parsed); err != nil {
				panic(runtime.NewGoError(err))
			}
			return runtime.ToValue(parsed)
		})

		return result
	})

	// fetchStream(url: string, options?: object): StreamResponse
	//
	// Same options as fetch(). Returns a StreamResponse that allows
	// incremental reading without buffering the entire body.
	//
	// StreamResponse:
	//   status     - HTTP status code (number)
	//   ok         - true if status is 200-299 (boolean)
	//   statusText - HTTP status line (string)
	//   url        - final URL after redirects (string)
	//   headers    - response headers object (lowercase keys)
	//   readLine() - read next line (string), returns null at EOF
	//   readAll()  - read remaining body as string
	//   close()    - release HTTP connection resources
	_ = exports.Set("fetchStream", func(call goja.FunctionCall) goja.Value {
		url := call.Argument(0).String()

		method := "GET"
		timeout := 30 * time.Second
		var bodyReader io.Reader
		var reqHeaders map[string]interface{}

		if len(call.Arguments) > 1 && !goja.IsUndefined(call.Arguments[1]) && !goja.IsNull(call.Arguments[1]) {
			if opts, ok := call.Arguments[1].Export().(map[string]interface{}); ok {
				if m, ok := opts["method"]; ok {
					if s, ok := m.(string); ok {
						method = strings.ToUpper(s)
					}
				}
				if t, ok := opts["timeout"]; ok {
					switch v := t.(type) {
					case int64:
						timeout = time.Duration(v) * time.Second
					case float64:
						timeout = time.Duration(v * float64(time.Second))
					}
				}
				if b, ok := opts["body"]; ok {
					if s, ok := b.(string); ok {
						bodyReader = strings.NewReader(s)
					}
				}
				if h, ok := opts["headers"]; ok {
					if m, ok := h.(map[string]interface{}); ok {
						reqHeaders = m
					}
				}
			}
		}

		req, err := http.NewRequest(method, url, bodyReader)
		if err != nil {
			panic(runtime.NewGoError(err))
		}

		for k, v := range reqHeaders {
			if s, ok := v.(string); ok {
				req.Header.Set(k, s)
			}
		}

		client := &http.Client{Timeout: timeout}
		resp, err := client.Do(req)
		if err != nil {
			panic(runtime.NewGoError(err))
		}

		reader := bufio.NewReader(resp.Body)

		result := runtime.NewObject()
		_ = result.Set("status", resp.StatusCode)
		_ = result.Set("ok", resp.StatusCode >= 200 && resp.StatusCode < 300)
		_ = result.Set("statusText", resp.Status)
		_ = result.Set("url", resp.Request.URL.String())

		headersObj := runtime.NewObject()
		for k, v := range resp.Header {
			if len(v) == 1 {
				_ = headersObj.Set(strings.ToLower(k), v[0])
			} else {
				_ = headersObj.Set(strings.ToLower(k), strings.Join(v, ", "))
			}
		}
		_ = result.Set("headers", headersObj)

		_ = result.Set("readLine", func() goja.Value {
			line, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					if line != "" {
						return runtime.ToValue(strings.TrimRight(line, "\n\r"))
					}
					return goja.Null()
				}
				panic(runtime.NewGoError(err))
			}
			return runtime.ToValue(strings.TrimRight(line, "\n\r"))
		})

		_ = result.Set("readAll", func() string {
			data, err := io.ReadAll(reader)
			if err != nil {
				panic(runtime.NewGoError(err))
			}
			return string(data)
		})

		_ = result.Set("close", func() {
			resp.Body.Close()
		})

		return result
	})
}
