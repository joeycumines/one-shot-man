//go:build unix

package mouseharness

import (
	"context"
	"testing"
	"time"

	"github.com/joeycumines/go-prompt/termtest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConsole_Integration_New(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	binaryPath := getDummyBinaryPath(t)

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath),
		termtest.WithDefaultTimeout(10*time.Second),
	)
	require.NoError(t, err)
	defer cp.Close()

	// Create Console with the termtest.Console
	console, err := New(
		WithTermtestConsole(cp),
		WithTestingTB(t),
		WithHeight(24),
		WithWidth(80),
	)
	require.NoError(t, err)

	assert.Equal(t, 24, console.Height())
	assert.Equal(t, 80, console.Width())
	assert.NotNil(t, console.TermtestConsole())
}

func TestConsole_Integration_FindElement(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	binaryPath := getDummyBinaryPath(t)

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath),
		termtest.WithDefaultTimeout(10*time.Second),
	)
	require.NoError(t, err)
	defer cp.Close()

	console, err := New(
		WithTermtestConsole(cp),
		WithTestingTB(t),
	)
	require.NoError(t, err)

	// Wait for the dummy program to render
	snap := cp.Snapshot()
	err = cp.Expect(ctx, snap, termtest.Contains("Click Me"), "wait for button")
	require.NoError(t, err)

	// Find the button
	loc := console.FindElement("[Click Me]")
	require.NotNil(t, loc, "button not found")
	assert.Equal(t, "[Click Me]", loc.Text)
	assert.Greater(t, loc.Row, 0)
	assert.Greater(t, loc.Col, 0)

	// Send quit
	_, err = cp.WriteString("q")
	require.NoError(t, err)
}

func TestConsole_Integration_ClickElement(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	binaryPath := getDummyBinaryPath(t)

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath),
		termtest.WithDefaultTimeout(10*time.Second),
	)
	require.NoError(t, err)
	defer cp.Close()

	console, err := New(
		WithTermtestConsole(cp),
		WithTestingTB(t),
	)
	require.NoError(t, err)

	// Wait for the dummy program to render
	snap := cp.Snapshot()
	err = cp.Expect(ctx, snap, termtest.Contains("Click Me"), "wait for button")
	require.NoError(t, err)

	// Verify the button changed to "Clicked!"
	snap = cp.Snapshot()
	err = console.ClickElement(ctx, "[Click Me]", 5*time.Second)
	require.NoError(t, err)

	err = cp.Expect(ctx, snap, termtest.Contains("Clicked!"), "wait for clicked state")
	require.NoError(t, err)

	// Send quit
	_, err = cp.WriteString("q")
	require.NoError(t, err)
}

func TestConsole_Integration_ScrollWheel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	binaryPath := getDummyBinaryPath(t)

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath),
		termtest.WithDefaultTimeout(10*time.Second),
	)
	require.NoError(t, err)
	defer cp.Close()

	console, err := New(
		WithTermtestConsole(cp),
		WithTestingTB(t),
	)
	require.NoError(t, err)

	// Wait for render
	snap := cp.Snapshot()
	err = cp.Expect(ctx, snap, termtest.Contains("Scroll: 0"), "wait for scroll display")
	require.NoError(t, err)

	// Send scroll up events
	snap = cp.Snapshot()
	err = console.ScrollWheel(10, 5, "up")
	require.NoError(t, err)
	err = console.ScrollWheel(10, 5, "up")
	require.NoError(t, err)

	// Verify scroll counter increased
	err = cp.Expect(ctx, snap, termtest.Contains("Scroll: 2"), "wait for scroll count")
	require.NoError(t, err)

	// Send scroll down
	snap = cp.Snapshot()
	err = console.ScrollWheel(10, 5, "down")
	require.NoError(t, err)

	err = cp.Expect(ctx, snap, termtest.Contains("Scroll: 1"), "wait for scroll count decrease")
	require.NoError(t, err)

	// Send quit
	_, err = cp.WriteString("q")
	require.NoError(t, err)
}

func TestConsole_Integration_ClickElementAndExpect(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	binaryPath := getDummyBinaryPath(t)

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath),
		termtest.WithDefaultTimeout(10*time.Second),
	)
	require.NoError(t, err)
	defer cp.Close()

	console, err := New(
		WithTermtestConsole(cp),
		WithTestingTB(t),
	)
	require.NoError(t, err)

	// Wait for render
	snap := cp.Snapshot()
	err = cp.Expect(ctx, snap, termtest.Contains("Click Me"), "wait for button")
	require.NoError(t, err)

	// Click and expect in one call
	err = console.ClickElementAndExpect(ctx, "[Click Me]", "Clicked!", 10*time.Second)
	require.NoError(t, err)

	// Send quit
	_, err = cp.WriteString("q")
	require.NoError(t, err)
}

func TestConsole_Integration_GetElementCenter(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	binaryPath := getDummyBinaryPath(t)

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath),
		termtest.WithDefaultTimeout(10*time.Second),
	)
	require.NoError(t, err)
	defer cp.Close()

	console, err := New(
		WithTermtestConsole(cp),
		WithTestingTB(t),
	)
	require.NoError(t, err)

	// Wait for render
	snap := cp.Snapshot()
	err = cp.Expect(ctx, snap, termtest.Contains("Click Me"), "wait for button")
	require.NoError(t, err)

	// Get element center
	x, y, found := console.GetElementCenter("[Click Me]")
	assert.True(t, found)
	assert.Greater(t, x, 0)
	assert.Greater(t, y, 0)

	// Non-existent element
	_, _, found = console.GetElementCenter("[NonExistent]")
	assert.False(t, found)

	// Send quit
	_, err = cp.WriteString("q")
	require.NoError(t, err)
}

func TestConsole_Integration_ClickWithButton(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	binaryPath := getDummyBinaryPath(t)

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath),
		termtest.WithDefaultTimeout(10*time.Second),
	)
	require.NoError(t, err)
	defer cp.Close()

	console, err := New(
		WithTermtestConsole(cp),
		WithTestingTB(t),
	)
	require.NoError(t, err)

	// Wait for render
	snap := cp.Snapshot()
	err = cp.Expect(ctx, snap, termtest.Contains("Click Me"), "wait for button")
	require.NoError(t, err)

	// Click with left button (0)
	snap = cp.Snapshot()
	err = console.ClickWithButton(10, 3, 0)
	require.NoError(t, err)

	// Verify the click registered
	err = cp.Expect(ctx, snap, termtest.Contains("Last: ("), "wait for position update")
	require.NoError(t, err)

	// Send quit
	_, err = cp.WriteString("q")
	require.NoError(t, err)
}

func TestConsole_Integration_DebugBuffer(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	binaryPath := getDummyBinaryPath(t)

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath),
		termtest.WithDefaultTimeout(10*time.Second),
	)
	require.NoError(t, err)
	defer cp.Close()

	console, err := New(
		WithTermtestConsole(cp),
		WithTestingTB(t),
	)
	require.NoError(t, err)

	// Wait for render
	snap := cp.Snapshot()
	err = cp.Expect(ctx, snap, termtest.Contains("Click Me"), "wait for button")
	require.NoError(t, err)

	// DebugBuffer should not panic
	console.DebugBuffer()

	// Send quit
	_, err = cp.WriteString("q")
	require.NoError(t, err)
}

func TestConsole_Integration_RequireClick(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	binaryPath := getDummyBinaryPath(t)

	cp, err := termtest.NewConsole(ctx,
		termtest.WithCommand(binaryPath),
		termtest.WithDefaultTimeout(10*time.Second),
	)
	require.NoError(t, err)
	defer cp.Close()

	console, err := New(
		WithTermtestConsole(cp),
		WithTestingTB(t),
	)
	require.NoError(t, err)

	// Wait for render
	snap := cp.Snapshot()
	err = cp.Expect(ctx, snap, termtest.Contains("Click Me"), "wait for button")
	require.NoError(t, err)

	// RequireClick should not panic for valid coordinates
	console.RequireClick(10, 5)

	// Send quit
	_, err = cp.WriteString("q")
	require.NoError(t, err)
}
