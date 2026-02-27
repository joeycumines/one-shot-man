package termmux

// DefaultToggleKey is Ctrl+] (ASCII GS, 0x1D).
const DefaultToggleKey byte = 0x1D

// DefaultToggleKeyName is the human-readable name for DefaultToggleKey.
const DefaultToggleKeyName = "Ctrl+]"

// Config holds the configuration for a Mux instance.
type Config struct {
	// ToggleKey is the byte that triggers passthrough exit.
	ToggleKey byte

	// StatusEnabled controls whether the status bar is rendered.
	StatusEnabled bool

	// InitialStatus is the initial status string shown in the bar.
	InitialStatus string

	// ResizeFn is called to propagate terminal resize to the child PTY.
	ResizeFn func(rows, cols uint16) error
}

// Option configures a Mux.
type Option func(*Config)

// defaultConfig returns a Config with sensible defaults.
func defaultConfig() Config {
	return Config{
		ToggleKey:     DefaultToggleKey,
		StatusEnabled: true,
		InitialStatus: "idle",
		ResizeFn:      nil,
	}
}

// applyOptions applies functional options to a config.
func applyOptions(cfg *Config, opts []Option) {
	for _, opt := range opts {
		opt(cfg)
	}
}
