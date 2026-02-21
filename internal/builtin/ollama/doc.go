package ollama
// Package ollama provides a Go client and Goja JS module for the Ollama REST API.
// It is registered as "osm:ollama" and provides tool-calling support, an agentic
// execution loop, and a tool definition registry.
//
// The HTTP client communicates with a local Ollama instance (default: http://localhost:11434)
// and supports both streaming and non-streaming chat completions, model management,
// and tool calling (function calling) following the Ollama API specification.
//
// # Tool Calling












package ollama// goja-eventloop adapter.// for registering built-in tools. All async operations return Promises via the// The module exposes: createClient, createToolRegistry, createAgent, and helpers//// # JS Module (osm:ollama)//// respond → repeat — until the model produces a final text response.// AgenticRunner orchestrates a multi-turn loop: chat → tool_calls → execute →// defined with JSON Schema parameters and registered in a ToolRegistry. The// Ollama models with the "tools" capability support function calling. Tools are//