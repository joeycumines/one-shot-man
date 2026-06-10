package claudemux

import (
	"context"
	"fmt"
	"io"
	"time"
)

// PTYReader abstracts reading from a PTY process for settle detection.
type PTYReader interface {
	// Read returns the next chunk of output from the PTY. Returns (nil, io.EOF)
	// when the process has exited. May block until data is available.
	Read() ([]byte, error)
}

// SettleConfig configures settle detection behavior.
type SettleConfig struct {
	StableDuration time.Duration // Unchanged duration to consider settled (default: 500ms)
	PollInterval   time.Duration // How often to check state (default: 50ms)
	TargetState    TUIState      // Required state before settling (0 = any)
}

func DefaultSettleConfig() SettleConfig {
	return SettleConfig{
		StableDuration: 500 * time.Millisecond,
		PollInterval:   50 * time.Millisecond,
		TargetState:    0,
	}
}

type readResult struct {
	chunk []byte
	err   error
}

// WaitSettle reads PTY output and feeds it through a VTStateDetector until
// the detector's state has remained unchanged for StableDuration. Returns the
// settled state and the duration waited.
//
// Reads from the PTY in a separate goroutine to avoid blocking when the
// process is idle. The stability check runs on every loop iteration
// regardless of whether new data arrived, so WaitSettle completes even
// when the PTY reader blocks on an idle process.
func WaitSettle(ctx context.Context, proc PTYReader, det *VTStateDetector, config SettleConfig) (TUIState, time.Duration, error) {
	if config.StableDuration <= 0 {
		config.StableDuration = 500 * time.Millisecond
	}
	if config.PollInterval <= 0 {
		config.PollInterval = 50 * time.Millisecond
	}

	start := time.Now()
	lastChange := start
	prevState := det.State()

	// Pending read from a previous iteration. Non-nil means a goroutine
	// is still reading and we should select on readCh instead of starting
	// a new read.
	var readCh chan readResult

	for {
		if err := ctx.Err(); err != nil {
			return det.State(), time.Since(start), fmt.Errorf("claudemux: settle cancelled: %w", err)
		}

		// Check stability before reading — allows settle detection even
		// when the PTY reader blocks on an idle process.
		currentState := det.State()
		if currentState != prevState {
			lastChange = time.Now()
			prevState = currentState
		}
		if time.Since(lastChange) >= config.StableDuration {
			if config.TargetState == 0 || currentState == config.TargetState {
				return currentState, time.Since(start), nil
			}
		}

		if readCh == nil {
			readCh = make(chan readResult, 1)
			go func() {
				ch, err := proc.Read()
				readCh <- readResult{ch, err}
			}()
		}

		select {
		case <-ctx.Done():
			return det.State(), time.Since(start), fmt.Errorf("claudemux: settle cancelled: %w", ctx.Err())
		case <-time.After(config.PollInterval):
		case res := <-readCh:
			readCh = nil
			if res.err != nil {
				if res.err == io.EOF {
					return det.State(), time.Since(start), nil
				}
				return det.State(), time.Since(start), fmt.Errorf("claudemux: settle read error: %w", res.err)
			}
			if len(res.chunk) > 0 {
				det.ProcessRaw(res.chunk, time.Now())
			}
		}
	}
}
