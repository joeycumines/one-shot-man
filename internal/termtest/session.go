package termtest

import (
	"context"
	"fmt"
	"time"
)

// TestSession represents a complete test scenario with multiple interactions.
type TestSession struct {
	pty     *PTYTest
	steps   []TestStep
	timeout time.Duration
}

// TestStep represents a single step in a test scenario.
type TestStep struct {
	Name     string
	Action   StepAction
	Input    string
	Expected string
	Timeout  time.Duration
}

// StepAction defines the type of action for a test step.
type StepAction int

const (
	ActionSendInput StepAction = iota
	ActionSendLine
	ActionSendKeys
	ActionWaitForOutput
	ActionWaitForPrompt
	ActionAssertOutput
	ActionAssertNotOutput
	ActionClearOutput
)

// NewTestSession creates a new test session.
func NewTestSession(ctx context.Context, timeout time.Duration) (*TestSession, error) {
	pty, err := NewForProgram(ctx)
	if err != nil {
		return nil, err
	}

	return &TestSession{
		pty:     pty,
		steps:   make([]TestStep, 0),
		timeout: timeout,
	}, nil
}

// GetPTY returns the underlying PTY for direct access.
func (s *TestSession) GetPTY() *PTYTest {
	return s.pty
}

// SendInput adds a step to send input.
func (s *TestSession) SendInput(name, input string) *TestSession {
	s.steps = append(s.steps, TestStep{
		Name:   name,
		Action: ActionSendInput,
		Input:  input,
	})
	return s
}

// SendLine adds a step to send a line (with Enter).
func (s *TestSession) SendLine(name, input string) *TestSession {
	s.steps = append(s.steps, TestStep{
		Name:   name,
		Action: ActionSendLine,
		Input:  input,
	})
	return s
}

// SendKeys adds a step to send special keys.
func (s *TestSession) SendKeys(name, keys string) *TestSession {
	s.steps = append(s.steps, TestStep{
		Name:   name,
		Action: ActionSendKeys,
		Input:  keys,
	})
	return s
}

// WaitForOutput adds a step to wait for specific output.
func (s *TestSession) WaitForOutput(name, expected string, timeout time.Duration) *TestSession {
	s.steps = append(s.steps, TestStep{
		Name:     name,
		Action:   ActionWaitForOutput,
		Expected: expected,
		Timeout:  timeout,
	})
	return s
}

// WaitForPrompt adds a step to wait for a prompt.
func (s *TestSession) WaitForPrompt(name, prompt string, timeout time.Duration) *TestSession {
	s.steps = append(s.steps, TestStep{
		Name:     name,
		Action:   ActionWaitForPrompt,
		Expected: prompt,
		Timeout:  timeout,
	})
	return s
}

// AssertOutput adds a step to assert output contains text.
func (s *TestSession) AssertOutput(name, expected string) *TestSession {
	s.steps = append(s.steps, TestStep{
		Name:     name,
		Action:   ActionAssertOutput,
		Expected: expected,
	})
	return s
}

// AssertNotOutput adds a step to assert output does NOT contain text.
func (s *TestSession) AssertNotOutput(name, unexpected string) *TestSession {
	s.steps = append(s.steps, TestStep{
		Name:     name,
		Action:   ActionAssertNotOutput,
		Expected: unexpected,
	})
	return s
}

// ClearOutput adds a step to clear the output buffer.
func (s *TestSession) ClearOutput(name string) *TestSession {
	s.steps = append(s.steps, TestStep{
		Name:   name,
		Action: ActionClearOutput,
	})
	return s
}

// Execute runs all the test steps in sequence.
func (s *TestSession) Execute() error {
	for i, step := range s.steps {
		if err := s.executeStep(step); err != nil {
			return fmt.Errorf("step %d (%s) failed: %w", i+1, step.Name, err)
		}
	}
	return nil
}

// executeStep executes a single test step.
func (s *TestSession) executeStep(step TestStep) error {
	timeout := step.Timeout
	if timeout == 0 {
		timeout = s.timeout
	}

	switch step.Action {
	case ActionSendInput:
		return s.pty.SendInput(step.Input)

	case ActionSendLine:
		return s.pty.SendLine(step.Input)

	case ActionSendKeys:
		return s.pty.SendKeys(step.Input)

	case ActionWaitForOutput:
		return s.pty.WaitForOutput(step.Expected, timeout)

	case ActionWaitForPrompt:
		return s.pty.WaitForPrompt(step.Expected, timeout)

	case ActionAssertOutput:
		return s.pty.AssertOutput(step.Expected)

	case ActionAssertNotOutput:
		return s.pty.AssertNotOutput(step.Expected)

	case ActionClearOutput:
		s.pty.ClearOutput()
		return nil

	default:
		return fmt.Errorf("unknown action: %v", step.Action)
	}
}

// Close closes the test session.
func (s *TestSession) Close() error {
	return s.pty.Close()
}

// GetOutput returns the current output.
func (s *TestSession) GetOutput() string {
	return s.pty.GetOutput()
}
