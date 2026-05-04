package command

import (
	"errors"
	"fmt"
	"testing"
)

func TestSilentError_Error(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  *SilentError
		want string
	}{
		{"with underlying error", &SilentError{Err: fmt.Errorf("oops")}, "oops"},
		{"nil underlying error", &SilentError{}, "silent error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("SilentError.Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSilentError_Unwrap(t *testing.T) {
	t.Parallel()
	inner := fmt.Errorf("inner")
	se := &SilentError{Err: inner}
	if got := se.Unwrap(); got != inner {
		t.Errorf("Unwrap() returned wrong error")
	}
}

func TestSilentError_Unwrap_Nil(t *testing.T) {
	t.Parallel()
	se := &SilentError{}
	if got := se.Unwrap(); got != nil {
		t.Errorf("Unwrap() = %v, want nil", got)
	}
}

func TestIsSilent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"plain error", fmt.Errorf("oops"), false},
		{"direct SilentError", &SilentError{Err: fmt.Errorf("oops")}, true},
		{"wrapped SilentError", fmt.Errorf("wrap: %w", &SilentError{Err: fmt.Errorf("oops")}), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			if got := IsSilent(tt.err); got != tt.want {
				t.Errorf("IsSilent() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestSilentError_ErrorsAs(t *testing.T) {
	t.Parallel()
	inner := fmt.Errorf("the real error")
	se := &SilentError{Err: inner}
	wrapped := fmt.Errorf("context: %w", se)

	var target *SilentError
	if !errors.As(wrapped, &target) {
		t.Fatal("errors.As should find SilentError in chain")
	}
	if target.Err != inner {
		t.Errorf("target.Err = %v, want %v", target.Err, inner)
	}
}
