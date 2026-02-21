package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RegisterBuiltinTools registers a standard set of filesystem and git tools
// into the given registry. The workDir is used as the base directory for all
// filesystem operations (and as the working directory for commands).
//
// Registered tools:
//   - read_file: Read the contents of a file
//   - write_file: Write content to a file
//   - list_dir: List files and directories
//   - exec: Execute a shell command
//   - grep: Search for a pattern in files
//   - git_diff: Show git diff output
//   - git_log: Show git log output
func RegisterBuiltinTools(r *ToolRegistry, workDir string) error {
	tools := []ToolDef{
		readFileTool(workDir),
		writeFileTool(workDir),
		listDirTool(workDir),
		execTool(workDir),
		grepTool(workDir),
		gitDiffTool(workDir),
		gitLogTool(workDir),
	}

	for _, t := range tools {
		if err := r.Register(t); err != nil {
			return err
		}
	}
	return nil
}

// resolvePath safely resolves a relative path against the work directory.
// It prevents path traversal outside the work directory.
func resolvePath(workDir, path string) (string, error) {
	if filepath.IsAbs(path) {
		// Allow absolute paths only if they're within workDir.
		cleaned := filepath.Clean(path)
		if !strings.HasPrefix(cleaned, filepath.Clean(workDir)+string(filepath.Separator)) &&
			cleaned != filepath.Clean(workDir) {
			return "", fmt.Errorf("absolute path %q is outside work directory %q", path, workDir)
		}
		return cleaned, nil
	}

	resolved := filepath.Join(workDir, path)
	cleaned := filepath.Clean(resolved)

	// Verify path doesn't escape workDir via ".." traversal.
	if !strings.HasPrefix(cleaned, filepath.Clean(workDir)+string(filepath.Separator)) &&
		cleaned != filepath.Clean(workDir) {
		return "", fmt.Errorf("path %q escapes work directory", path)
	}

	return cleaned, nil
}

func readFileTool(workDir string) ToolDef {
	return ToolDef{
		Name:        "read_file",
		Description: "Read the contents of a file. Returns the file content as text.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {
					"type": "string",
					"description": "Path to the file to read (relative to work directory or absolute)"
				}
			},
			"required": ["path"]
		}`),
		Handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
			path, _ := args["path"].(string)
			if path == "" {
				return "", fmt.Errorf("read_file: path is required")
			}

			resolved, err := resolvePath(workDir, path)
			if err != nil {
				return "", fmt.Errorf("read_file: %w", err)
			}

			data, err := os.ReadFile(resolved)
			if err != nil {
				return "", fmt.Errorf("read_file: %w", err)
			}

			return string(data), nil
		},
	}
}

func writeFileTool(workDir string) ToolDef {
	return ToolDef{
		Name:        "write_file",
		Description: "Write content to a file. Creates the file if it doesn't exist, or overwrites it if it does. Creates parent directories as needed.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {
					"type": "string",
					"description": "Path to the file to write (relative to work directory or absolute)"
				},
				"content": {
					"type": "string",
					"description": "Content to write to the file"
				}
			},
			"required": ["path", "content"]
		}`),
		Handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
			path, _ := args["path"].(string)
			if path == "" {
				return "", fmt.Errorf("write_file: path is required")
			}
			content, _ := args["content"].(string)

			resolved, err := resolvePath(workDir, path)
			if err != nil {
				return "", fmt.Errorf("write_file: %w", err)
			}

			dir := filepath.Dir(resolved)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				return "", fmt.Errorf("write_file: create dir: %w", err)
			}

			if err := os.WriteFile(resolved, []byte(content), 0o644); err != nil {
				return "", fmt.Errorf("write_file: %w", err)
			}

			return fmt.Sprintf("Wrote %d bytes to %s", len(content), path), nil
		},
	}
}

func listDirTool(workDir string) ToolDef {
	return ToolDef{
		Name:        "list_dir",
		Description: "List the contents of a directory. Returns filenames with '/' suffix for directories.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"path": {
					"type": "string",
					"description": "Path to the directory to list (relative to work directory or absolute). Defaults to work directory root."
				}
			}
		}`),
		Handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
			path, _ := args["path"].(string)
			if path == "" {
				path = "."
			}

			resolved, err := resolvePath(workDir, path)
			if err != nil {
				return "", fmt.Errorf("list_dir: %w", err)
			}

			entries, err := os.ReadDir(resolved)
			if err != nil {
				return "", fmt.Errorf("list_dir: %w", err)
			}

			var b strings.Builder
			for _, entry := range entries {
				name := entry.Name()
				if entry.IsDir() {
					name += "/"
				}
				b.WriteString(name)
				b.WriteByte('\n')
			}

			return b.String(), nil
		},
	}
}

func execTool(workDir string) ToolDef {
	return ToolDef{
		Name:        "exec",
		Description: "Execute a shell command and return its combined stdout and stderr output. Use for running build commands, tests, or other shell operations.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"command": {
					"type": "string",
					"description": "The shell command to execute"
				}
			},
			"required": ["command"]
		}`),
		Handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
			command, _ := args["command"].(string)
			if command == "" {
				return "", fmt.Errorf("exec: command is required")
			}

			cmd := exec.CommandContext(ctx, "sh", "-c", command)
			cmd.Dir = workDir

			var output bytes.Buffer
			cmd.Stdout = &output
			cmd.Stderr = &output

			err := cmd.Run()
			result := output.String()

			// Truncate very large output.
			const maxOutput = 100_000
			if len(result) > maxOutput {
				result = result[:maxOutput] + "\n... (output truncated)"
			}

			if err != nil {
				return fmt.Sprintf("Error: %v\n\n%s", err, result), nil
			}

			return result, nil
		},
	}
}

func grepTool(workDir string) ToolDef {
	return ToolDef{
		Name:        "grep",
		Description: "Search for a pattern in files using grep. Returns matching lines with file paths and line numbers.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"pattern": {
					"type": "string",
					"description": "The search pattern (regular expression)"
				},
				"path": {
					"type": "string",
					"description": "File or directory path to search (defaults to work directory). Use '.' for recursive search."
				},
				"flags": {
					"type": "string",
					"description": "Additional grep flags (e.g., '-i' for case-insensitive, '-l' for files only)"
				}
			},
			"required": ["pattern"]
		}`),
		Handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
			pattern, _ := args["pattern"].(string)
			if pattern == "" {
				return "", fmt.Errorf("grep: pattern is required")
			}

			path, _ := args["path"].(string)
			if path == "" {
				path = "."
			}

			resolved, err := resolvePath(workDir, path)
			if err != nil {
				return "", fmt.Errorf("grep: %w", err)
			}

			grepArgs := []string{"-rn", "--include=*"}
			if flags, ok := args["flags"].(string); ok && flags != "" {
				grepArgs = append(grepArgs, strings.Fields(flags)...)
			}
			grepArgs = append(grepArgs, pattern, resolved)

			cmd := exec.CommandContext(ctx, "grep", grepArgs...)
			cmd.Dir = workDir

			var output bytes.Buffer
			cmd.Stdout = &output
			cmd.Stderr = &output

			_ = cmd.Run() // grep returns exit code 1 for no matches

			result := output.String()

			const maxOutput = 50_000
			if len(result) > maxOutput {
				result = result[:maxOutput] + "\n... (output truncated)"
			}

			if result == "" {
				return "No matches found.", nil
			}

			return result, nil
		},
	}
}

func gitDiffTool(workDir string) ToolDef {
	return ToolDef{
		Name:        "git_diff",
		Description: "Show git diff output. Can show uncommitted changes, changes between refs, or changes for specific files.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"args": {
					"type": "string",
					"description": "Arguments to pass to git diff (e.g., 'HEAD~1', '--staged', 'main...HEAD -- path/to/file')"
				}
			}
		}`),
		Handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
			gitArgs := []string{"diff"}
			if extraArgs, ok := args["args"].(string); ok && extraArgs != "" {
				gitArgs = append(gitArgs, strings.Fields(extraArgs)...)
			}

			cmd := exec.CommandContext(ctx, "git", gitArgs...)
			cmd.Dir = workDir

			var output bytes.Buffer
			cmd.Stdout = &output
			cmd.Stderr = &output

			if err := cmd.Run(); err != nil {
				return fmt.Sprintf("Error: %v\n%s", err, output.String()), nil
			}

			result := output.String()
			if result == "" {
				return "No differences found.", nil
			}

			const maxOutput = 100_000
			if len(result) > maxOutput {
				result = result[:maxOutput] + "\n... (diff truncated)"
			}

			return result, nil
		},
	}
}

func gitLogTool(workDir string) ToolDef {
	return ToolDef{
		Name:        "git_log",
		Description: "Show git log output. Returns recent commit history with hashes, authors, dates, and messages.",
		Parameters: json.RawMessage(`{
			"type": "object",
			"properties": {
				"args": {
					"type": "string",
					"description": "Arguments to pass to git log (e.g., '-n 10', '--oneline', '--since=\"2 days ago\"')"
				}
			}
		}`),
		Handler: func(ctx context.Context, args map[string]interface{}) (string, error) {
			gitArgs := []string{"log"}
			if extraArgs, ok := args["args"].(string); ok && extraArgs != "" {
				gitArgs = append(gitArgs, strings.Fields(extraArgs)...)
			} else {
				// Default: last 20 commits, one line each.
				gitArgs = append(gitArgs, "--oneline", "-n", "20")
			}

			cmd := exec.CommandContext(ctx, "git", gitArgs...)
			cmd.Dir = workDir

			var output bytes.Buffer
			cmd.Stdout = &output
			cmd.Stderr = &output

			if err := cmd.Run(); err != nil {
				return fmt.Sprintf("Error: %v\n%s", err, output.String()), nil
			}

			result := output.String()
			if result == "" {
				return "No commits found.", nil
			}

			return result, nil
		},
	}
}
