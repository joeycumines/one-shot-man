//go:build unix

package mouseharness

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOptionFunc_AppliesCorrectly(t *testing.T) {
	cfg := defaultConfig()

	opt := optionFunc(func(c *consoleConfig) error {
		c.height = 42
		return nil
	})

	err := opt.applyOption(cfg)
	require.NoError(t, err)
	assert.Equal(t, 42, cfg.height)
}

func TestWithHeight(t *testing.T) {
	tests := []struct {
		name      string
		height    int
		expectErr bool
	}{
		{"valid height", 24, false},
		{"min height", 1, false},
		{"large height", 100, false},
		{"zero height", 0, true},
		{"negative height", -5, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaultConfig()
			opt := WithHeight(tt.height)
			err := opt.applyOption(cfg)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.height, cfg.height)
			}
		})
	}
}

func TestWithWidth(t *testing.T) {
	tests := []struct {
		name      string
		width     int
		expectErr bool
	}{
		{"valid width", 80, false},
		{"min width", 1, false},
		{"large width", 200, false},
		{"zero width", 0, true},
		{"negative width", -10, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := defaultConfig()
			opt := WithWidth(tt.width)
			err := opt.applyOption(cfg)

			if tt.expectErr {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.width, cfg.width)
			}
		})
	}
}

func TestWithTestingTB_Nil(t *testing.T) {
	cfg := defaultConfig()
	opt := WithTestingTB(nil)
	err := opt.applyOption(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be nil")
}

func TestWithTestingTB_Valid(t *testing.T) {
	cfg := defaultConfig()
	opt := WithTestingTB(t)
	err := opt.applyOption(cfg)
	require.NoError(t, err)
	assert.Equal(t, t, cfg.tb)
}

func TestWithTermtestConsole_Nil(t *testing.T) {
	cfg := defaultConfig()
	opt := WithTermtestConsole(nil)
	err := opt.applyOption(cfg)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "cannot be nil")
}

func TestDefaultConfig(t *testing.T) {
	cfg := defaultConfig()

	assert.Equal(t, 24, cfg.height)
	assert.Equal(t, 80, cfg.width)
	assert.Nil(t, cfg.cp)
	assert.Nil(t, cfg.tb)
}

func TestNew_MissingTermtestConsole(t *testing.T) {
	_, err := New(WithTestingTB(t))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "WithTermtestConsole is required")
}

func TestNew_AppliesMultipleOptions(t *testing.T) {
	// Test that multiple options are applied in order
	cfg := defaultConfig()

	opts := []Option{
		WithHeight(30),
		WithWidth(120),
		WithTestingTB(t),
	}

	for _, opt := range opts {
		err := opt.applyOption(cfg)
		require.NoError(t, err)
	}

	assert.Equal(t, 30, cfg.height)
	assert.Equal(t, 120, cfg.width)
	assert.Equal(t, t, cfg.tb)
}
