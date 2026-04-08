package bubbletea

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
)

// parseMouseEvent parses a string representation of a mouse event back into a tea.Mouse.
// This is used only in tests. The string format matches tea.Mouse.String().
func parseMouseEvent(s string) (tea.Mouse, bool) {
	var m tea.Mouse

	if s == "" {
		return m, false
	}

	// Parse modifiers
	remaining := s
	var mod tea.KeyMod
	for {
		if strings.HasPrefix(remaining, "ctrl+") {
			mod |= tea.ModCtrl
			remaining = remaining[5:]
		} else if strings.HasPrefix(remaining, "alt+") {
			mod |= tea.ModAlt
			remaining = remaining[4:]
		} else if strings.HasPrefix(remaining, "shift+") {
			mod |= tea.ModShift
			remaining = remaining[6:]
		} else {
			break
		}
	}
	m.Mod = mod

	if remaining == "motion" {
		m.Button = tea.MouseNone
		return m, true
	}
	if remaining == "release" {
		m.Button = tea.MouseNone
		return m, true
	}

	var bestButtonDef *MouseButtonDef
	var bestButtonLen int
	for buttonStr, def := range MouseButtonDefs {
		if strings.HasPrefix(remaining, buttonStr) && len(buttonStr) > bestButtonLen {
			defCopy := def
			bestButtonDef = &defCopy
			bestButtonLen = len(buttonStr)
		}
	}

	if bestButtonDef == nil {
		return m, false
	}

	m.Button = bestButtonDef.Button
	return m, true
}

func TestParseMouseEvent(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   tea.Mouse
		wantOk bool
	}{
		{
			name:  "left press",
			input: "left",
			want: tea.Mouse{
				Button: tea.MouseLeft,
			},
			wantOk: true,
		},
		{
			name:  "right press",
			input: "right",
			want: tea.Mouse{
				Button: tea.MouseRight,
			},
			wantOk: true,
		},
		{
			name:  "middle press",
			input: "middle",
			want: tea.Mouse{
				Button: tea.MouseMiddle,
			},
			wantOk: true,
		},
		{
			name:  "wheel up",
			input: "wheel up",
			want: tea.Mouse{
				Button: tea.MouseWheelUp,
			},
			wantOk: true,
		},
		{
			name:  "wheel down",
			input: "wheel down",
			want: tea.Mouse{
				Button: tea.MouseWheelDown,
			},
			wantOk: true,
		},
		{
			name:  "wheel left",
			input: "wheel left",
			want: tea.Mouse{
				Button: tea.MouseWheelLeft,
			},
			wantOk: true,
		},
		{
			name:  "wheel right",
			input: "wheel right",
			want: tea.Mouse{
				Button: tea.MouseWheelRight,
			},
			wantOk: true,
		},
		{
			name:  "motion",
			input: "motion",
			want: tea.Mouse{
				Button: tea.MouseNone,
			},
			wantOk: true,
		},
		{
			name:  "release alone",
			input: "release",
			want: tea.Mouse{
				Button: tea.MouseNone,
			},
			wantOk: true,
		},
		{
			name:  "ctrl left",
			input: "ctrl+left",
			want: tea.Mouse{
				Button: tea.MouseLeft,
				Mod:    tea.ModCtrl,
			},
			wantOk: true,
		},
		{
			name:  "alt right",
			input: "alt+right",
			want: tea.Mouse{
				Button: tea.MouseRight,
				Mod:    tea.ModAlt,
			},
			wantOk: true,
		},
		{
			name:  "shift middle",
			input: "shift+middle",
			want: tea.Mouse{
				Button: tea.MouseMiddle,
				Mod:    tea.ModShift,
			},
			wantOk: true,
		},
		{
			name:  "ctrl alt left",
			input: "ctrl+alt+left",
			want: tea.Mouse{
				Button: tea.MouseLeft,
				Mod:    tea.ModCtrl | tea.ModAlt,
			},
			wantOk: true,
		},
		{
			name:  "ctrl alt shift left",
			input: "ctrl+alt+shift+left",
			want: tea.Mouse{
				Button: tea.MouseLeft,
				Mod:    tea.ModCtrl | tea.ModAlt | tea.ModShift,
			},
			wantOk: true,
		},
		{
			name:  "ctrl wheel up",
			input: "ctrl+wheel up",
			want: tea.Mouse{
				Button: tea.MouseWheelUp,
				Mod:    tea.ModCtrl,
			},
			wantOk: true,
		},
		{
			name:  "backward",
			input: "backward",
			want: tea.Mouse{
				Button: tea.MouseBackward,
			},
			wantOk: true,
		},
		{
			name:  "forward",
			input: "forward",
			want: tea.Mouse{
				Button: tea.MouseForward,
			},
			wantOk: true,
		},
		{
			name:  "button 10",
			input: "button 10",
			want: tea.Mouse{
				Button: tea.MouseButton10,
			},
			wantOk: true,
		},
		{
			name:  "button 11",
			input: "button 11",
			want: tea.Mouse{
				Button: tea.MouseButton11,
			},
			wantOk: true,
		},
		{
			name:   "empty string",
			input:  "",
			want:   tea.Mouse{},
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := parseMouseEvent(tt.input)
			if ok != tt.wantOk {
				t.Errorf("parseMouseEvent(%q) ok = %v, want %v", tt.input, ok, tt.wantOk)
				return
			}
			if !tt.wantOk {
				return
			}
			if got.Button != tt.want.Button {
				t.Errorf("parseMouseEvent(%q) button = %v, want %v", tt.input, got.Button, tt.want.Button)
			}
			if got.Mod != tt.want.Mod {
				t.Errorf("parseMouseEvent(%q) mod = %v, want %v", tt.input, got.Mod, tt.want.Mod)
			}
		})
	}
}

func TestMouseEventToJS(t *testing.T) {
	tests := []struct {
		name       string
		event      tea.MouseMsg
		wantType   string
		wantButton string
	}{
		{
			name:       "left click",
			event:      tea.MouseClickMsg{Button: tea.MouseLeft},
			wantType:   "MouseClick",
			wantButton: "left",
		},
		{
			name:       "wheel up",
			event:      tea.MouseWheelMsg{Button: tea.MouseWheelUp},
			wantType:   "MouseWheel",
			wantButton: "wheel up",
		},
		{
			name:       "motion",
			event:      tea.MouseMotionMsg{Button: tea.MouseNone},
			wantType:   "MouseMotion",
			wantButton: "none",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			js := MouseEventToJS(tt.event)
			if js["type"] != tt.wantType {
				t.Errorf("MouseEventToJS() type = %v, want %v", js["type"], tt.wantType)
			}
			if js["button"] != tt.wantButton {
				t.Errorf("MouseEventToJS() button = %v, want %v", js["button"], tt.wantButton)
			}
		})
	}
}
