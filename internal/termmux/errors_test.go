package termmux

import (
	"errors"
	"testing"
)

func TestSentinelErrors_messages(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"ErrNoChild", ErrNoChild, "termmux: no child process attached"},
		{"ErrAlreadyAttached", ErrAlreadyAttached, "termmux: child already attached"},
		{"ErrPassthroughActive", ErrPassthroughActive, "termmux: passthrough is active"},
		{"ErrDetached", ErrDetached, "termmux: mux is detached"},
		{"ErrDetachTimeout", ErrDetachTimeout, "termmux: detach timed out acquiring lock"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.err.Error(); got != tt.want {
				t.Errorf("%s.Error() = %q, want %q", tt.name, got, tt.want)
			}
		})
	}
}

func TestSentinelErrors_distinct(t *testing.T) {
	errs := []error{ErrNoChild, ErrAlreadyAttached, ErrPassthroughActive, ErrDetached, ErrDetachTimeout}
	for i := range errs {
		for j := i + 1; j < len(errs); j++ {
			if errors.Is(errs[i], errs[j]) {
				t.Errorf("errors.Is(%v, %v) = true, want false", errs[i], errs[j])
			}
		}
	}
}

func TestSentinelErrors_Is(t *testing.T) {
	// errors.Is should match each sentinel against itself.
	errs := []error{ErrNoChild, ErrAlreadyAttached, ErrPassthroughActive, ErrDetached, ErrDetachTimeout}
	for _, err := range errs {
		if !errors.Is(err, err) {
			t.Errorf("errors.Is(%v, %v) = false, want true", err, err)
		}
	}
}
