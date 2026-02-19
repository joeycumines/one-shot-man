package claudemux

import (
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

// --- State name helpers ---

func TestPanelStateName(t *testing.T) {
	t.Parallel()
	tests := []struct {
		state PanelState
		want  string
	}{
		{PanelIdle, "Idle"},
		{PanelActive, "Active"},
		{PanelClosed, "Closed"},
		{PanelState(99), "Unknown(99)"},
	}
	for _, tt := range tests {
		if got := PanelStateName(tt.state); got != tt.want {
			t.Errorf("PanelStateName(%d) = %q, want %q", int(tt.state), got, tt.want)
		}
	}
}

// --- DefaultPanelConfig ---

func TestDefaultPanelConfig(t *testing.T) {
	t.Parallel()
	cfg := DefaultPanelConfig()
	if cfg.MaxPanes != 9 {
		t.Errorf("MaxPanes = %d, want 9", cfg.MaxPanes)
	}
	if cfg.ScrollbackSize != 10000 {
		t.Errorf("ScrollbackSize = %d, want 10000", cfg.ScrollbackSize)
	}
}

// --- NewPanel ---

func TestNewPanel(t *testing.T) {
	t.Parallel()
	p := NewPanel(DefaultPanelConfig())
	snap := p.Snapshot()
	if snap.State != PanelIdle {
		t.Errorf("state = %s, want Idle", snap.StateName)
	}
	if len(snap.Panes) != 0 {
		t.Errorf("panes = %d, want 0", len(snap.Panes))
	}
}

func TestNewPanel_ClampConfig(t *testing.T) {
	t.Parallel()
	// Test min clamp.
	p := NewPanel(PanelConfig{MaxPanes: 0, ScrollbackSize: 0})
	if p.config.MaxPanes != 1 {
		t.Errorf("MaxPanes = %d, want 1 (clamped)", p.config.MaxPanes)
	}
	if p.config.ScrollbackSize != 100 {
		t.Errorf("ScrollbackSize = %d, want 100 (clamped)", p.config.ScrollbackSize)
	}

	// Test max clamp.
	p = NewPanel(PanelConfig{MaxPanes: 20})
	if p.config.MaxPanes != 9 {
		t.Errorf("MaxPanes = %d, want 9 (clamped)", p.config.MaxPanes)
	}
}

// --- Start ---

func TestPanel_Start(t *testing.T) {
	t.Parallel()
	p := NewPanel(DefaultPanelConfig())
	if err := p.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}
	snap := p.Snapshot()
	if snap.State != PanelActive {
		t.Errorf("state = %s, want Active", snap.StateName)
	}
}

func TestPanel_StartFromNonIdle(t *testing.T) {
	t.Parallel()
	p := NewPanel(DefaultPanelConfig())
	_ = p.Start()
	if err := p.Start(); err == nil {
		t.Error("expected error starting from Active")
	}
}

// --- AddPane ---

func TestPanel_AddPane(t *testing.T) {
	t.Parallel()
	p := NewPanel(DefaultPanelConfig())
	_ = p.Start()

	idx, err := p.AddPane("inst-1", "Agent 1")
	if err != nil {
		t.Fatalf("AddPane: %v", err)
	}
	if idx != 0 {
		t.Errorf("idx = %d, want 0", idx)
	}

	// First pane should become active.
	active := p.ActivePane()
	if active == nil {
		t.Fatal("ActivePane is nil")
	}
	if active.ID != "inst-1" {
		t.Errorf("active.ID = %q, want inst-1", active.ID)
	}
}

func TestPanel_AddPane_Full(t *testing.T) {
	t.Parallel()
	p := NewPanel(PanelConfig{MaxPanes: 2, ScrollbackSize: 100})
	_ = p.Start()

	_, _ = p.AddPane("a", "A")
	_, _ = p.AddPane("b", "B")
	_, err := p.AddPane("c", "C")
	if err == nil {
		t.Error("expected panel full error")
	}
}

func TestPanel_AddPane_Duplicate(t *testing.T) {
	t.Parallel()
	p := NewPanel(DefaultPanelConfig())
	_ = p.Start()

	_, _ = p.AddPane("x", "X")
	_, err := p.AddPane("x", "X2")
	if err == nil {
		t.Error("expected duplicate pane error")
	}
}

func TestPanel_AddPane_Closed(t *testing.T) {
	t.Parallel()
	p := NewPanel(DefaultPanelConfig())
	p.Close()

	_, err := p.AddPane("a", "A")
	if err == nil {
		t.Error("expected panel closed error")
	}
}

// --- RemovePane ---

func TestPanel_RemovePane(t *testing.T) {
	t.Parallel()
	p := NewPanel(DefaultPanelConfig())
	_ = p.Start()

	_, _ = p.AddPane("a", "A")
	_, _ = p.AddPane("b", "B")
	_, _ = p.AddPane("c", "C")

	if err := p.RemovePane("b"); err != nil {
		t.Fatalf("RemovePane: %v", err)
	}
	if p.PaneCount() != 2 {
		t.Errorf("PaneCount = %d, want 2", p.PaneCount())
	}
}

func TestPanel_RemovePane_AdjustsActive(t *testing.T) {
	t.Parallel()
	p := NewPanel(DefaultPanelConfig())
	_ = p.Start()

	_, _ = p.AddPane("a", "A")
	_, _ = p.AddPane("b", "B")
	_ = p.SetActive(1) // Active is B (index 1).

	// Remove B — active should move to A (index 0).
	_ = p.RemovePane("b")
	if p.ActiveIndex() != 0 {
		t.Errorf("activeIdx = %d, want 0", p.ActiveIndex())
	}
}

func TestPanel_RemovePane_NotFound(t *testing.T) {
	t.Parallel()
	p := NewPanel(DefaultPanelConfig())
	_ = p.Start()

	err := p.RemovePane("nonexistent")
	if err == nil {
		t.Error("expected not found error")
	}
}

// --- Input Routing ---

func TestPanel_RouteInput_AltSwitch(t *testing.T) {
	t.Parallel()
	p := NewPanel(DefaultPanelConfig())
	_ = p.Start()
	_, _ = p.AddPane("a", "A")
	_, _ = p.AddPane("b", "B")
	_, _ = p.AddPane("c", "C")

	// Switch to pane 2 (index 1).
	result := p.RouteInput("alt+2")
	if !result.Consumed {
		t.Error("expected consumed")
	}
	if result.Action != "switch" {
		t.Errorf("Action = %q, want switch", result.Action)
	}
	if p.ActiveIndex() != 1 {
		t.Errorf("activeIdx = %d, want 1", p.ActiveIndex())
	}
}

func TestPanel_RouteInput_AltOutOfRange(t *testing.T) {
	t.Parallel()
	p := NewPanel(DefaultPanelConfig())
	_ = p.Start()
	_, _ = p.AddPane("a", "A")

	// Alt+9 but only 1 pane — should be "none".
	result := p.RouteInput("alt+9")
	if result.Action != "none" {
		t.Errorf("Action = %q, want none", result.Action)
	}
}

func TestPanel_RouteInput_Forward(t *testing.T) {
	t.Parallel()
	p := NewPanel(DefaultPanelConfig())
	_ = p.Start()
	_, _ = p.AddPane("inst-1", "Agent 1")

	result := p.RouteInput("enter")
	if result.TargetPaneID != "inst-1" {
		t.Errorf("TargetPaneID = %q, want inst-1", result.TargetPaneID)
	}
	if result.Action != "forward" {
		t.Errorf("Action = %q, want forward", result.Action)
	}
}

func TestPanel_RouteInput_PgUp(t *testing.T) {
	t.Parallel()
	p := NewPanel(PanelConfig{MaxPanes: 2, ScrollbackSize: 1000})
	_ = p.Start()
	_, _ = p.AddPane("a", "A")

	// Fill some scrollback.
	for i := 0; i < 50; i++ {
		_ = p.AppendOutput("a", fmt.Sprintf("line %d", i))
	}

	result := p.RouteInput("pgup")
	if result.Action != "scroll-up" {
		t.Errorf("Action = %q, want scroll-up", result.Action)
	}
	if !result.Consumed {
		t.Error("expected consumed")
	}

	pane := p.ActivePane()
	if pane.ScrollPos != 20 {
		t.Errorf("ScrollPos = %d, want 20", pane.ScrollPos)
	}
}

func TestPanel_RouteInput_PgDown(t *testing.T) {
	t.Parallel()
	p := NewPanel(PanelConfig{MaxPanes: 2, ScrollbackSize: 1000})
	_ = p.Start()
	_, _ = p.AddPane("a", "A")

	for i := 0; i < 50; i++ {
		_ = p.AppendOutput("a", fmt.Sprintf("line %d", i))
	}

	// Scroll up first.
	p.RouteInput("pgup")
	p.RouteInput("pgup") // ScrollPos = 40

	result := p.RouteInput("pgdown")
	if result.Action != "scroll-down" {
		t.Errorf("Action = %q, want scroll-down", result.Action)
	}

	pane := p.ActivePane()
	if pane.ScrollPos != 20 {
		t.Errorf("ScrollPos = %d, want 20 (was 40, -20)", pane.ScrollPos)
	}
}

func TestPanel_RouteInput_NoPanes(t *testing.T) {
	t.Parallel()
	p := NewPanel(DefaultPanelConfig())
	_ = p.Start()

	result := p.RouteInput("enter")
	if result.Action != "none" {
		t.Errorf("Action = %q, want none", result.Action)
	}
}

func TestPanel_RouteInput_NotActive(t *testing.T) {
	t.Parallel()
	p := NewPanel(DefaultPanelConfig())
	// Panel is Idle, not Active.

	result := p.RouteInput("enter")
	if result.Action != "none" {
		t.Errorf("Action = %q, want none", result.Action)
	}
}

// --- SetActive ---

func TestPanel_SetActive(t *testing.T) {
	t.Parallel()
	p := NewPanel(DefaultPanelConfig())
	_ = p.Start()
	_, _ = p.AddPane("a", "A")
	_, _ = p.AddPane("b", "B")

	if err := p.SetActive(1); err != nil {
		t.Fatalf("SetActive: %v", err)
	}
	active := p.ActivePane()
	if active.ID != "b" {
		t.Errorf("active = %q, want b", active.ID)
	}
}

func TestPanel_SetActive_OutOfRange(t *testing.T) {
	t.Parallel()
	p := NewPanel(DefaultPanelConfig())
	_ = p.Start()
	_, _ = p.AddPane("a", "A")

	if err := p.SetActive(5); err == nil {
		t.Error("expected out of range error")
	}
}

// --- AppendOutput ---

func TestPanel_AppendOutput(t *testing.T) {
	t.Parallel()
	p := NewPanel(PanelConfig{MaxPanes: 2, ScrollbackSize: 100})
	_ = p.Start()
	_, _ = p.AddPane("a", "A")

	for i := 0; i < 10; i++ {
		if err := p.AppendOutput("a", fmt.Sprintf("line %d", i)); err != nil {
			t.Fatalf("AppendOutput %d: %v", i, err)
		}
	}

	pane := p.ActivePane()
	if len(pane.Scrollback) != 10 {
		t.Errorf("scrollback len = %d, want 10", len(pane.Scrollback))
	}
}

func TestPanel_AppendOutput_Trim(t *testing.T) {
	t.Parallel()
	p := NewPanel(PanelConfig{MaxPanes: 1, ScrollbackSize: 100})
	_ = p.Start()
	_, _ = p.AddPane("a", "A")

	// Fill beyond max.
	for i := 0; i < 120; i++ {
		_ = p.AppendOutput("a", fmt.Sprintf("line %d", i))
	}

	pane := p.ActivePane()
	if len(pane.Scrollback) != 100 {
		t.Errorf("scrollback len = %d, want 100 (trimmed)", len(pane.Scrollback))
	}

	// Oldest lines should be trimmed (lines 0-19 gone).
	if pane.Scrollback[0] != "line 20" {
		t.Errorf("first line = %q, want 'line 20'", pane.Scrollback[0])
	}
}

func TestPanel_AppendOutput_NotFound(t *testing.T) {
	t.Parallel()
	p := NewPanel(DefaultPanelConfig())
	_ = p.Start()

	err := p.AppendOutput("nonexistent", "hello")
	if err == nil {
		t.Error("expected not found error")
	}
}

// --- UpdateHealth ---

func TestPanel_UpdateHealth(t *testing.T) {
	t.Parallel()
	p := NewPanel(DefaultPanelConfig())
	_ = p.Start()
	_, _ = p.AddPane("a", "A")

	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	health := PaneHealth{
		State:      "running",
		TaskCount:  5,
		ErrorCount: 1,
		LastUpdate: now,
	}
	if err := p.UpdateHealth("a", health); err != nil {
		t.Fatalf("UpdateHealth: %v", err)
	}

	pane := p.ActivePane()
	if pane.Health.State != "running" {
		t.Errorf("Health.State = %q, want running", pane.Health.State)
	}
	if pane.Health.TaskCount != 5 {
		t.Errorf("TaskCount = %d, want 5", pane.Health.TaskCount)
	}
}

func TestPanel_UpdateHealth_NotFound(t *testing.T) {
	t.Parallel()
	p := NewPanel(DefaultPanelConfig())
	_ = p.Start()

	err := p.UpdateHealth("nonexistent", PaneHealth{State: "error"})
	if err == nil {
		t.Error("expected not found error")
	}
}

// --- StatusBar ---

func TestPanel_StatusBar_NoPanes(t *testing.T) {
	t.Parallel()
	p := NewPanel(DefaultPanelConfig())
	_ = p.Start()

	bar := p.StatusBar()
	if bar != "[no panes]" {
		t.Errorf("StatusBar = %q, want '[no panes]'", bar)
	}
}

func TestPanel_StatusBar_SinglePane(t *testing.T) {
	t.Parallel()
	p := NewPanel(DefaultPanelConfig())
	_ = p.Start()
	_, _ = p.AddPane("a", "Agent")

	bar := p.StatusBar()
	// Active pane should be in brackets.
	if !strings.Contains(bar, "[1:") {
		t.Errorf("StatusBar = %q, missing active bracket", bar)
	}
	if !strings.Contains(bar, "Agent") {
		t.Errorf("StatusBar = %q, missing title", bar)
	}
}

func TestPanel_StatusBar_MultiplePanes(t *testing.T) {
	t.Parallel()
	p := NewPanel(DefaultPanelConfig())
	_ = p.Start()
	_, _ = p.AddPane("a", "A")
	_, _ = p.AddPane("b", "B")
	_ = p.UpdateHealth("a", PaneHealth{State: "running"})
	_ = p.UpdateHealth("b", PaneHealth{State: "error"})

	bar := p.StatusBar()
	// Pane 1 active: [1:● A]
	// Pane 2 inactive:  2:✖ B
	if !strings.Contains(bar, "[1:") {
		t.Errorf("StatusBar = %q, missing active bracket for pane 1", bar)
	}
	if !strings.Contains(bar, "●") {
		t.Errorf("StatusBar = %q, missing running indicator", bar)
	}
	if !strings.Contains(bar, "✖") {
		t.Errorf("StatusBar = %q, missing error indicator", bar)
	}
}

// --- GetVisibleLines ---

func TestPanel_GetVisibleLines(t *testing.T) {
	t.Parallel()
	p := NewPanel(PanelConfig{MaxPanes: 1, ScrollbackSize: 1000})
	_ = p.Start()
	_, _ = p.AddPane("a", "A")

	for i := 0; i < 50; i++ {
		_ = p.AppendOutput("a", fmt.Sprintf("line %d", i))
	}

	// Get last 10 lines (at bottom, ScrollPos=0).
	lines, err := p.GetVisibleLines("a", 10)
	if err != nil {
		t.Fatalf("GetVisibleLines: %v", err)
	}
	if len(lines) != 10 {
		t.Fatalf("len(lines) = %d, want 10", len(lines))
	}
	if lines[0] != "line 40" {
		t.Errorf("lines[0] = %q, want 'line 40'", lines[0])
	}
	if lines[9] != "line 49" {
		t.Errorf("lines[9] = %q, want 'line 49'", lines[9])
	}
}

func TestPanel_GetVisibleLines_Scrolled(t *testing.T) {
	t.Parallel()
	p := NewPanel(PanelConfig{MaxPanes: 1, ScrollbackSize: 1000})
	_ = p.Start()
	_, _ = p.AddPane("a", "A")

	for i := 0; i < 50; i++ {
		_ = p.AppendOutput("a", fmt.Sprintf("line %d", i))
	}

	// Scroll up 20 lines.
	p.RouteInput("pgup")

	lines, err := p.GetVisibleLines("a", 10)
	if err != nil {
		t.Fatalf("GetVisibleLines: %v", err)
	}
	if len(lines) != 10 {
		t.Fatalf("len(lines) = %d, want 10", len(lines))
	}
	// Window should be lines 20-29 (50 total - 20 scroll = end at 30, -10 height = start at 20).
	if lines[0] != "line 20" {
		t.Errorf("lines[0] = %q, want 'line 20'", lines[0])
	}
}

func TestPanel_GetVisibleLines_Empty(t *testing.T) {
	t.Parallel()
	p := NewPanel(DefaultPanelConfig())
	_ = p.Start()
	_, _ = p.AddPane("a", "A")

	lines, err := p.GetVisibleLines("a", 10)
	if err != nil {
		t.Fatalf("GetVisibleLines: %v", err)
	}
	if lines != nil {
		t.Errorf("lines = %v, want nil", lines)
	}
}

func TestPanel_GetVisibleLines_NotFound(t *testing.T) {
	t.Parallel()
	p := NewPanel(DefaultPanelConfig())
	_ = p.Start()

	_, err := p.GetVisibleLines("nonexistent", 10)
	if err == nil {
		t.Error("expected not found error")
	}
}

// --- Snapshot ---

func TestPanel_Snapshot(t *testing.T) {
	t.Parallel()
	p := NewPanel(DefaultPanelConfig())
	_ = p.Start()
	_, _ = p.AddPane("a", "Agent-A")
	_, _ = p.AddPane("b", "Agent-B")

	snap := p.Snapshot()
	if snap.StateName != "Active" {
		t.Errorf("StateName = %q, want Active", snap.StateName)
	}
	if len(snap.Panes) != 2 {
		t.Fatalf("len(Panes) = %d, want 2", len(snap.Panes))
	}
	if !snap.Panes[0].IsActive {
		t.Error("pane 0 should be active")
	}
	if snap.Panes[1].IsActive {
		t.Error("pane 1 should not be active")
	}
}

// --- Close ---

func TestPanel_Close(t *testing.T) {
	t.Parallel()
	p := NewPanel(DefaultPanelConfig())
	_ = p.Start()
	_, _ = p.AddPane("a", "A")

	p.Close()
	snap := p.Snapshot()
	if snap.State != PanelClosed {
		t.Errorf("state = %s, want Closed", snap.StateName)
	}
	if len(snap.Panes) != 0 {
		t.Errorf("panes = %d, want 0", len(snap.Panes))
	}
}

// --- Health indicator ---

func TestHealthIndicator(t *testing.T) {
	t.Parallel()
	tests := []struct {
		state string
		want  string
	}{
		{"running", "● "},
		{"error", "✖ "},
		{"idle", "○ "},
		{"stopped", "■ "},
		{"unknown", "? "},
	}
	for _, tt := range tests {
		if got := healthIndicator(tt.state); got != tt.want {
			t.Errorf("healthIndicator(%q) = %q, want %q", tt.state, got, tt.want)
		}
	}
}

// --- Concurrent ---

func TestPanel_ConcurrentAppendOutput(t *testing.T) {
	t.Parallel()
	p := NewPanel(PanelConfig{MaxPanes: 1, ScrollbackSize: 10000})
	_ = p.Start()
	_, _ = p.AddPane("a", "A")

	const n = 100
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			_ = p.AppendOutput("a", fmt.Sprintf("line %d", idx))
		}(i)
	}
	wg.Wait()

	pane := p.ActivePane()
	if len(pane.Scrollback) != n {
		t.Errorf("scrollback len = %d, want %d", len(pane.Scrollback), n)
	}
}

func TestPanel_ConcurrentRouteInput(t *testing.T) {
	t.Parallel()
	p := NewPanel(DefaultPanelConfig())
	_ = p.Start()
	for i := 0; i < 5; i++ {
		_, _ = p.AddPane(fmt.Sprintf("p%d", i), fmt.Sprintf("P%d", i))
	}

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			key := fmt.Sprintf("alt+%d", (idx%5)+1)
			p.RouteInput(key)
		}(i)
	}
	wg.Wait()

	// Active index should be valid.
	idx := p.ActiveIndex()
	if idx < 0 || idx >= 5 {
		t.Errorf("activeIdx = %d, out of range", idx)
	}
}

func TestPanel_ConcurrentSnapshot(t *testing.T) {
	t.Parallel()
	p := NewPanel(DefaultPanelConfig())
	_ = p.Start()
	_, _ = p.AddPane("a", "A")

	var wg sync.WaitGroup
	for i := 0; i < 30; i++ {
		wg.Add(2)
		go func(idx int) {
			defer wg.Done()
			_ = p.AppendOutput("a", fmt.Sprintf("line %d", idx))
		}(i)
		go func() {
			defer wg.Done()
			snap := p.Snapshot()
			_ = snap.StatusBar // Just ensure it doesn't panic.
		}()
	}
	wg.Wait()
}
