package userk8s

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/joeycumines/one-shot-man/internal/userk8s/api/v1alpha1"
	"k8s.io/apimachinery/pkg/labels"
)

// defaultCommandTimeout bounds one resolver command when the step omits a
// timeout. It must outlast a human Touch ID / desktop approval on `op item
// get` (Hana: the old 5s default killed approvable prompts as "failed").
const defaultCommandTimeout = 60 * time.Second

// maxResolverBytes bounds every byte a resolver may hand the engine: command
// stdout and stderr are captured up to this many bytes, and a credential file
// contributes at most its first line up to this size. A resolver that emits
// more cannot make the engine allocate without bound.
const maxResolverBytes = 64 * 1024

// CommandRunner executes one resolver command without a shell.
type CommandRunner interface {
	Run(ctx context.Context, argv []string, timeout time.Duration) (string, error)
}

// failure is a resolver failure described without error text or credential
// material, so it stays safe to report to callers and logs.
type failure string

const (
	failureEmptyArgv      failure = "empty argv"
	failureEnvUnset       failure = "unset"
	failureFileUnreadable failure = "unreadable"
	failureFileEmpty      failure = "empty"
	failureCommandFailed  failure = "failed"
	failureCommandTimeout failure = "timed out"
	failureCommandCancel  failure = "canceled"
)

// ExecRunner is the production runner. It never uses a shell and never places
// command output in an error message.
type ExecRunner struct{}

// Run executes argv and returns the first line of stdout, trimmed.
// Stdin is left unset (/dev/null): resolvers are non-interactive secret
// printers — interactive approval is not delivered on this process's stdin.
func (ExecRunner) Run(ctx context.Context, argv []string, timeout time.Duration) (string, error) {
	if len(argv) == 0 {
		return "", errors.New("resolver command has an empty argv")
	}
	if timeout <= 0 {
		timeout = defaultCommandTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	stdout := &limitedBuffer{limit: maxResolverBytes}
	stderr := &countingWriter{limit: maxResolverBytes}
	cmd.Stdout = stdout
	cmd.Stderr = stderr

	err := cmd.Run()
	if err != nil {
		switch {
		case errors.Is(ctx.Err(), context.DeadlineExceeded):
			return "", fmt.Errorf("resolver command %q timed out after %s", argv[0], timeout)
		case errors.Is(ctx.Err(), context.Canceled):
			return "", fmt.Errorf("resolver command %q canceled", argv[0])
		}
		return "", fmt.Errorf("resolver command %q failed: %w (%d bytes of output suppressed)", argv[0], err, stderr.Count())
	}
	return firstLine(stdout.String()), nil
}

// limitedBuffer keeps at most limit bytes and reports truncation, so a
// chatty helper cannot drive the engine's allocation.
type limitedBuffer struct {
	buffer    strings.Builder
	limit     int
	truncated bool
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if remaining := b.limit - b.buffer.Len(); remaining > 0 {
		if len(p) > remaining {
			b.buffer.Write(p[:remaining])
			b.truncated = true
		} else {
			b.buffer.Write(p)
		}
	} else if len(p) > 0 {
		b.truncated = true
	}
	return len(p), nil
}

func (b *limitedBuffer) String() string { return b.buffer.String() }

// countingWriter counts bytes without retaining them; resolver stderr is
// never stored, quoted, or logged.
type countingWriter struct {
	limit int
	count int
}

func (w *countingWriter) Write(p []byte) (int, error) {
	if remaining := w.limit - w.count; remaining > 0 {
		if len(p) > remaining {
			w.count += remaining
		} else {
			w.count += len(p)
		}
	}
	return len(p), nil
}

func (w *countingWriter) Count() int { return w.count }

// firstLine returns the first line of s with surrounding space removed.
func firstLine(s string) string {
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

// expandHome resolves a path that begins with exactly ~, ~/, $HOME, or
// $HOME/. Any other spelling is left to os.ExpandEnv, so "~other/file" and
// "$HOMEfoo" are not silently rewritten into the current user's home.
func expandHome(path string) string {
	if path == "~" || strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "~"), "/"))
		}
		return path
	}
	if path == "$HOME" || strings.HasPrefix(path, "$HOME/") {
		if home, err := os.UserHomeDir(); err == nil {
			return filepath.Join(home, strings.TrimPrefix(strings.TrimPrefix(path, "$HOME"), "/"))
		}
		return path
	}
	return os.ExpandEnv(path)
}

// readCredentialFile reads at most the first maxResolverBytes of a file and
// returns its first line. The read stops at the first newline in practice,
// because only one line can be returned.
func readCredentialFile(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()

	reader := io.LimitReader(file, maxResolverBytes)
	line, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}
	return firstLine(string(line)), nil
}

// resolveBinding runs one binding's resolver chain in order and returns the
// first non-empty value. A step that fails, or yields nothing, falls through
// to the next; the failures are reported as classes so a caller can explain a
// miss without ever receiving error text or credential material.
func resolveBinding(ctx context.Context, binding v1alpha1.LocalSecretBinding, runner CommandRunner) (Credential, bool, []string) {
	var failures []string
	record := func(step string, kind failure) {
		failures = append(failures, step+": "+string(kind))
	}

	for i := range binding.Spec.Resolvers {
		if err := ctx.Err(); err != nil {
			record("context", failureCommandCancel)
			return Credential{}, false, failures
		}
		step := binding.Spec.Resolvers[i]
		switch {
		case step.Env != nil:
			value := strings.TrimSpace(os.Getenv(step.Env.Name))
			if value == "" {
				record("env "+step.Env.Name, failureEnvUnset)
				continue
			}
			return Credential{
				EnvVar:     binding.Spec.EnvVar,
				Value:      value,
				Provenance: "env:" + step.Env.Name,
			}, true, nil

		case step.File != nil:
			value, err := readCredentialFile(expandHome(step.File.Path))
			switch {
			case err != nil:
				record("file "+step.File.Path, failureFileUnreadable)
				continue
			case value == "":
				record("file "+step.File.Path, failureFileEmpty)
				continue
			}
			return Credential{
				EnvVar:     binding.Spec.EnvVar,
				Value:      value,
				Provenance: "file:" + step.File.Path,
			}, true, nil

		case step.Command != nil:
			if len(step.Command.Argv) == 0 {
				record("command", failureEmptyArgv)
				continue
			}
			timeout := defaultCommandTimeout
			if step.Command.Timeout != nil && step.Command.Timeout.Duration > 0 {
				timeout = step.Command.Timeout.Duration
			}
			out, err := runner.Run(ctx, step.Command.Argv, timeout)
			if err != nil {
				switch {
				case errors.Is(ctx.Err(), context.DeadlineExceeded):
					record("command "+step.Command.Argv[0], failureCommandTimeout)
				case errors.Is(ctx.Err(), context.Canceled):
					record("command "+step.Command.Argv[0], failureCommandCancel)
				default:
					record("command "+step.Command.Argv[0], failureCommandFailed)
				}
				continue
			}
			if value := firstLine(out); value != "" {
				return Credential{
					EnvVar:     binding.Spec.EnvVar,
					Value:      value,
					Provenance: "command:" + step.Command.Argv[0],
				}, true, nil
			}
			record("command "+step.Command.Argv[0], failureFileEmpty)
		}
	}
	return Credential{}, false, failures
}

// bindingsForSlot returns every local binding whose selector matches the
// access labels and whose envVar equals slot, ordered by object name so the
// first-success rule does not depend on document order.
func bindingsForSlot(bindings []v1alpha1.LocalSecretBinding, set labels.Set, slot string) ([]v1alpha1.LocalSecretBinding, error) {
	var matched []v1alpha1.LocalSecretBinding
	for _, binding := range bindings {
		if binding.Spec.EnvVar != slot {
			continue
		}
		ok, err := selectorMatches(&binding.Spec.Selector, set)
		if err != nil {
			return nil, fmt.Errorf("binding %q: %w", binding.Name, err)
		}
		if ok {
			matched = append(matched, binding)
		}
	}
	sort.Slice(matched, func(i, j int) bool { return matched[i].Name < matched[j].Name })
	return matched, nil
}
