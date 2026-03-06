package command

import (
	"fmt"
	"io"
	"net"
	"os"
	"sync"
)

// McpBridgeCommand implements a stdio-to-socket bridge for MCP IPC.
// It connects to a Unix domain socket or TCP address and bidirectionally
// copies data between stdin/stdout and the socket. This replaces the
// external socat dependency for MCP callback channel setup.
type McpBridgeCommand struct {
	*BaseCommand
	// Stdin is the reader to use for input. If nil, defaults to os.Stdin.
	Stdin io.Reader
}

// NewMcpBridgeCommand creates a new McpBridgeCommand.
func NewMcpBridgeCommand() *McpBridgeCommand {
	return &McpBridgeCommand{
		BaseCommand: NewBaseCommand(
			"mcp-bridge",
			"stdio-to-socket bridge for MCP IPC (internal use)",
			"mcp-bridge <network> <address>",
		),
	}
}

// Execute runs the stdio-to-socket bridge.
// It expects exactly two arguments: network ("unix" or "tcp") and address.
func (c *McpBridgeCommand) Execute(args []string, stdout, stderr io.Writer) error {
	if len(args) != 2 {
		_, _ = fmt.Fprintf(stderr, "Usage: osm mcp-bridge <network> <address>\n")
		_, _ = fmt.Fprintf(stderr, "  network: \"unix\" or \"tcp\"\n")
		_, _ = fmt.Fprintf(stderr, "  address: socket path (unix) or host:port (tcp)\n")
		return fmt.Errorf("expected 2 arguments, got %d", len(args))
	}

	network := args[0]
	address := args[1]

	switch network {
	case "unix", "tcp":
		// valid
	default:
		return fmt.Errorf("unsupported network %q: must be \"unix\" or \"tcp\"", network)
	}

	conn, err := net.Dial(network, address)
	if err != nil {
		return fmt.Errorf("connect %s %s: %w", network, address, err)
	}
	defer conn.Close()

	// Bidirectional copy: stdin→socket, socket→stdout.
	// When either direction finishes (EOF or error), we shut down the
	// other direction to avoid hanging.
	var wg sync.WaitGroup
	wg.Add(2)

	stdin := c.Stdin
	if stdin == nil {
		stdin = os.Stdin
	}

	// stdin → socket
	go func() {
		defer wg.Done()
		_, _ = io.Copy(conn, stdin)
		// Half-close the write side so the remote end sees EOF.
		closeWrite(conn)
	}()

	// socket → stdout
	go func() {
		defer wg.Done()
		_, _ = io.Copy(stdout, conn)
	}()

	wg.Wait()
	return nil
}

// closeWrite performs a half-close (shutdown write) on the connection,
// signaling EOF to the remote while still allowing reads.
func closeWrite(conn net.Conn) {
	type writeCloser interface {
		CloseWrite() error
	}
	if wc, ok := conn.(writeCloser); ok {
		_ = wc.CloseWrite()
	}
}
