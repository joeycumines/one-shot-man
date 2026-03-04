//go:build !windows

package pty

import "testing"

func TestSplitCommand(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		input      string
		wantBinary string
		wantArgs   []string
		wantErr    bool
	}{
		{
			name:       "single word",
			input:      "echo",
			wantBinary: "echo",
			wantArgs:   nil,
		},
		{
			name:       "simple split",
			input:      "ollama launch claude",
			wantBinary: "ollama",
			wantArgs:   []string{"launch", "claude"},
		},
		{
			name:       "with flags",
			input:      "ollama launch claude --config /path/to/cfg",
			wantBinary: "ollama",
			wantArgs:   []string{"launch", "claude", "--config", "/path/to/cfg"},
		},
		{
			name:       "single quoted argument",
			input:      "echo 'hello world'",
			wantBinary: "echo",
			wantArgs:   []string{"hello world"},
		},
		{
			name:       "double quoted argument",
			input:      `echo "hello world"`,
			wantBinary: "echo",
			wantArgs:   []string{"hello world"},
		},
		{
			name:       "escaped space",
			input:      `echo hello\ world`,
			wantBinary: "echo",
			wantArgs:   []string{"hello world"},
		},
		{
			name:       "mixed quotes",
			input:      `cmd "arg one" 'arg two' arg\ three`,
			wantBinary: "cmd",
			wantArgs:   []string{"arg one", "arg two", "arg three"},
		},
		{
			name:       "tabs as separators",
			input:      "cmd\targ1\targ2",
			wantBinary: "cmd",
			wantArgs:   []string{"arg1", "arg2"},
		},
		{
			name:       "extra whitespace",
			input:      "  cmd   arg1   arg2  ",
			wantBinary: "cmd",
			wantArgs:   []string{"arg1", "arg2"},
		},
		{
			name:       "path with spaces in quotes",
			input:      `cmd "/path/to/my file.txt"`,
			wantBinary: "cmd",
			wantArgs:   []string{"/path/to/my file.txt"},
		},
		{
			name:       "backslash in double quotes",
			input:      `echo "hello \"world\""`,
			wantBinary: "echo",
			wantArgs:   []string{`hello "world"`},
		},
		{
			name:       "backslash non-special in double quotes preserved",
			input:      `echo "hello\nworld"`,
			wantBinary: "echo",
			wantArgs:   []string{`hello\nworld`},
		},
		{
			name:       "single quote preserves backslash",
			input:      `echo 'hello\nworld'`,
			wantBinary: "echo",
			wantArgs:   []string{`hello\nworld`},
		},
		{
			name:    "unterminated single quote",
			input:   "echo 'hello",
			wantErr: true,
		},
		{
			name:    "unterminated double quote",
			input:   `echo "hello`,
			wantErr: true,
		},
		{
			name:    "empty string",
			input:   "",
			wantErr: true,
		},
		{
			name:    "only whitespace",
			input:   "   \t  ",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			binary, args, err := splitCommand(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got binary=%q args=%v", binary, args)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if binary != tt.wantBinary {
				t.Errorf("binary: got %q, want %q", binary, tt.wantBinary)
			}
			if len(args) != len(tt.wantArgs) {
				t.Fatalf("args length: got %d (%v), want %d (%v)", len(args), args, len(tt.wantArgs), tt.wantArgs)
			}
			for i := range args {
				if args[i] != tt.wantArgs[i] {
					t.Errorf("args[%d]: got %q, want %q", i, args[i], tt.wantArgs[i])
				}
			}
		})
	}
}
