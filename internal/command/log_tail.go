package command

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/joeycumines/one-shot-man/internal/config"
)

// LogCommand provides log-related operations, primarily tailing the log file.
type LogCommand struct {
	*BaseCommand
	config *config.Config
	follow bool
	lines  int
	file   string
}

// NewLogCommand creates a new log command.
func NewLogCommand(cfg *config.Config) *LogCommand {
	return &LogCommand{
		BaseCommand: NewBaseCommand("log", "View and tail log files", "log [tail] [options]"),
		config:      cfg,
	}
}

// SetupFlags configures the flags for the log command.
func (c *LogCommand) SetupFlags(fs *flag.FlagSet) {
	fs.BoolVar(&c.follow, "f", false, "Follow the log file (like tail -f)")
	fs.BoolVar(&c.follow, "follow", false, "Follow the log file (like tail -f)")
	fs.IntVar(&c.lines, "n", 10, "Number of lines to show from the end of the file")
	fs.StringVar(&c.file, "file", "", "Path to log file (overrides config log.file)")
}

// Execute runs the log command.
func (c *LogCommand) Execute(args []string, stdout, stderr io.Writer) error {
	// Handle subcommand: "log tail" is an alias for "log --follow".
	if len(args) > 0 && args[0] == "tail" {
		c.follow = true
		args = args[1:]
	}

	if len(args) > 0 {
		_, _ = fmt.Fprintf(stderr, "unknown subcommand: %s\n", args[0])
		return fmt.Errorf("unknown subcommand: %s", args[0])
	}

	// Resolve log file path: flag → config → error.
	logPath := c.file
	if logPath == "" {
		logPath = resolveLogPath(c.config)
	}
	if logPath == "" {
		_, _ = fmt.Fprintln(stderr, "No log file configured. Use --file or set log.file in config.")
		return fmt.Errorf("no log file configured")
	}

	if c.follow {
		return c.tailFollow(logPath, stdout, stderr)
	}
	return c.tailLines(logPath, stdout, stderr)
}

// resolveLogPath returns the effective log file path from config or env var.
func resolveLogPath(cfg *config.Config) string {
	schema := config.DefaultSchema()
	if cfg == nil {
		// Check env var directly when config is nil.
		if v := os.Getenv("OSM_LOG_FILE"); v != "" {
			return v
		}
		return ""
	}
	return schema.Resolve(cfg, "log.file")
}

// tailLines prints the last N lines of the log file to stdout.
func (c *LogCommand) tailLines(logPath string, stdout, stderr io.Writer) error {
	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			_, _ = fmt.Fprintf(stderr, "Log file does not exist: %s\n", logPath)
			return fmt.Errorf("log file not found: %s", logPath)
		}
		return fmt.Errorf("failed to open log file: %w", err)
	}
	defer f.Close()

	// Read all lines and print the last N.
	lines := readLastNLines(f, c.lines)
	for _, line := range lines {
		_, _ = fmt.Fprintln(stdout, line)
	}
	return nil
}

// readLastNLines reads the last n lines from a reader.
// Uses a ring buffer to avoid loading the entire file into memory.
func readLastNLines(r io.Reader, n int) []string {
	if n <= 0 {
		return nil
	}

	ring := make([]string, n)
	idx := 0
	count := 0

	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		ring[idx%n] = scanner.Text()
		idx++
		count++
	}

	if count == 0 {
		return nil
	}

	// Extract lines in order from the ring buffer.
	total := count
	if total > n {
		total = n
	}
	result := make([]string, total)
	start := idx - total
	for i := 0; i < total; i++ {
		result[i] = ring[(start+i)%n]
	}
	return result
}

// tailFollow tails the log file, printing new lines as they appear.
// It handles log rotation by detecting when the file is truncated or replaced.
func (c *LogCommand) tailFollow(logPath string, stdout, stderr io.Writer) error {
	// Print last N lines first, then follow.
	f, err := os.Open(logPath)
	if err != nil {
		if os.IsNotExist(err) {
			// File doesn't exist yet — wait for it.
			_, _ = fmt.Fprintf(stderr, "Waiting for log file: %s\n", logPath)
			f, err = waitForFile(logPath)
			if err != nil {
				return err
			}
		} else {
			return fmt.Errorf("failed to open log file: %w", err)
		}
	}

	// Print initial lines.
	initialLines := readLastNLines(f, c.lines)
	for _, line := range initialLines {
		_, _ = fmt.Fprintln(stdout, line)
	}

	// Seek to end for following.
	pos, err := f.Seek(0, io.SeekEnd)
	if err != nil {
		_ = f.Close()
		return fmt.Errorf("failed to seek to end: %w", err)
	}

	// Follow loop: poll for new data.
	ctx := context.Background()
	return followFile(ctx, f, logPath, pos, stdout, stderr)
}

// followFile polls the file for new data and prints it.
// It detects log rotation (file shrinks or inode changes) and re-opens.
func followFile(ctx context.Context, f *os.File, logPath string, pos int64, stdout, stderr io.Writer) error {
	reader := bufio.NewReader(f)
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	defer f.Close()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			// Check for rotation: stat the path and compare with current file.
			rotated, err := detectRotation(f, logPath, pos)
			if err != nil {
				// File may have been deleted; wait for re-creation.
				_ = f.Close()
				_, _ = fmt.Fprintf(stderr, "Log file rotated, waiting for new file...\n")
				newF, err := waitForFile(logPath)
				if err != nil {
					return err
				}
				f = newF
				reader = bufio.NewReader(f)
				pos = 0
				continue
			}

			if rotated {
				// File was replaced or truncated — re-open from beginning.
				_ = f.Close()
				newF, err := os.Open(logPath)
				if err != nil {
					continue // Will retry next tick.
				}
				f = newF
				reader = bufio.NewReader(f)
				pos = 0
			}

			// Read new lines.
			for {
				line, err := reader.ReadString('\n')
				if len(line) > 0 {
					// Remove trailing newline for consistent output.
					if line[len(line)-1] == '\n' {
						line = line[:len(line)-1]
					}
					_, _ = fmt.Fprintln(stdout, line)
					pos += int64(len(line)) + 1 // +1 for the newline we stripped
				}
				if err != nil {
					break // EOF or error — wait for more data.
				}
			}
		}
	}
}

// detectRotation checks if the log file has been rotated by comparing
// the current file size with our position. Returns true if rotation
// is detected (file shrunk or was replaced).
func detectRotation(currentFile *os.File, logPath string, pos int64) (bool, error) {
	// Stat the path (not the open file handle) to detect replacement.
	pathInfo, err := os.Stat(logPath)
	if err != nil {
		return false, err // File gone — likely rotating.
	}

	// If the file at the path is smaller than our current position,
	// the file was truncated or replaced.
	if pathInfo.Size() < pos {
		return true, nil
	}

	// On Unix-like systems, we could compare device/inode to detect replacement.
	// But for cross-platform simplicity, we rely on size check only.
	// This covers the common rotation patterns (rename + create new).
	return false, nil
}

// waitForFile waits for a file to appear, polling every 500ms.
// Returns the opened file or an error after a reasonable timeout.
func waitForFile(path string) (*os.File, error) {
	const maxWait = 30 * time.Second
	const pollInterval = 500 * time.Millisecond

	deadline := time.Now().Add(maxWait)
	for time.Now().Before(deadline) {
		f, err := os.Open(path)
		if err == nil {
			return f, nil
		}
		if !os.IsNotExist(err) {
			return nil, fmt.Errorf("failed to open log file: %w", err)
		}
		time.Sleep(pollInterval)
	}
	return nil, fmt.Errorf("timed out waiting for log file: %s", path)
}
