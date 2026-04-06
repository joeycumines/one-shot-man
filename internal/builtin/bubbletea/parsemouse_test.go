package bubbletea

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/stretchr/testify/require"
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
		wantButton string
		wantWheel  bool
	}{
		{
			name:       "left click",
			event:      tea.MouseClickMsg{Button: tea.MouseLeft},
			wantButton: "left",
			wantWheel:  false,
		},
		{
			name:       "wheel up",
			event:      tea.MouseWheelMsg{Button: tea.MouseWheelUp},
			wantButton: "wheel up",
			wantWheel:  true,
		},
		{
			name:       "motion",
			event:      tea.MouseMotionMsg{Button: tea.MouseNone},
			wantButton: "none",
			wantWheel:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			js := MouseEventToJS(tt.event)
			if js["button"] != tt.wantButton {
				t.Errorf("MouseEventToJS() button = %v, want %v", js["button"], tt.wantButton)
			}
			if js["isWheel"] != tt.wantWheel {
				t.Errorf("MouseEventToJS() isWheel = %v, want %v", js["isWheel"], tt.wantWheel)
			}
		})
	}
}

func TestJSToMouseEvent(t *testing.T) {
	tests := []struct {
		name       string
		button     string
		action     string
		x, y       int
		alt        bool
		ctrl       bool
		shift      bool
		wantButton tea.MouseButton
		wantMod    tea.KeyMod
	}{
		{
			name:       "left press",
			button:     "left",
			action:     "press",
			x:          10,
			y:          20,
			wantButton: tea.MouseLeft,
		},
		{
			name:       "wheel down with modifiers",
			button:     "wheel down",
			action:     "press",
			x:          5,
			y:          15,
			ctrl:       true,
			alt:        true,
			wantButton: tea.MouseWheelDown,
			wantMod:    tea.ModCtrl | tea.ModAlt,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := JSToMouseEvent(tt.button, tt.action, tt.x, tt.y, tt.alt, tt.ctrl, tt.shift)
			switch m := got.(type) {
			case tea.MouseClickMsg:
				if m.Button != tt.wantButton {
					t.Errorf("JSToMouseEvent() button = %v, want %v", m.Button, tt.wantButton)
				}
				if m.X != tt.x {
					t.Errorf("JSToMouseEvent() x = %v, want %v", m.X, tt.x)
				}
				if m.Y != tt.y {
					t.Errorf("JSToMouseEvent() y = %v, want %v", m.Y, tt.y)
				}
				if m.Mod != tt.wantMod {
					t.Errorf("JSToMouseEvent() mod = %v, want %v", m.Mod, tt.wantMod)
				}
			case tea.MouseWheelMsg:
				// Wheel buttons always produce MouseWheelMsg in v2
				if m.Button != tt.wantButton {
					t.Errorf("JSToMouseEvent() button = %v, want %v", m.Button, tt.wantButton)
				}
				if m.X != tt.x {
					t.Errorf("JSToMouseEvent() x = %v, want %v", m.X, tt.x)
				}
				if m.Y != tt.y {
					t.Errorf("JSToMouseEvent() y = %v, want %v", m.Y, tt.y)
				}
				if m.Mod != tt.wantMod {
					t.Errorf("JSToMouseEvent() mod = %v, want %v", m.Mod, tt.wantMod)
				}
			default:
				t.Errorf("JSToMouseEvent() returned unexpected type %T", got)
			}
		})
	}
}

func TestMouseEventRoundTrip(t *testing.T) {
	events := []struct {
		msg  tea.MouseMsg
		want tea.Mouse
	}{
		{tea.MouseClickMsg{Button: tea.MouseLeft, X: 10, Y: 20}, tea.Mouse{Button: tea.MouseLeft, X: 10, Y: 20}},
		{tea.MouseClickMsg{Button: tea.MouseRight, X: 5, Y: 15, Mod: tea.ModCtrl}, tea.Mouse{Button: tea.MouseRight, X: 5, Y: 15, Mod: tea.ModCtrl}},
		{tea.MouseWheelMsg{Button: tea.MouseWheelUp, X: 0, Y: 0}, tea.Mouse{Button: tea.MouseWheelUp, X: 0, Y: 0}},
		{tea.MouseMotionMsg{Button: tea.MouseNone, X: 100, Y: 200, Mod: tea.ModAlt | tea.ModShift}, tea.Mouse{Button: tea.MouseNone, X: 100, Y: 200, Mod: tea.ModAlt | tea.ModShift}},
	}

	for _, tc := range events {
		js := MouseEventToJS(tc.msg)
		require.NotNil(t, js)

		button := js["button"].(string)
		x := js["x"].(int)
		y := js["y"].(int)
		mod := js["mod"].([]string)

		// Reconstruct modifiers from the mod slice
		var alt, ctrl, shift bool
		for _, m := range mod {
			switch m {
			case "alt":
				alt = true
			case "ctrl":
				ctrl = true
			case "shift":
				shift = true
			}
		}

		reconstructed := JSToMouseEvent(button, "", x, y, alt, ctrl, shift)

		// Extract the Mouse from the reconstructed message
		var gotMouse tea.Mouse
		switch m := reconstructed.(type) {
		case tea.MouseClickMsg:
			gotMouse = m.Mouse()
		case tea.MouseWheelMsg:
			gotMouse = m.Mouse()
		case tea.MouseMotionMsg:
			gotMouse = m.Mouse()
		case tea.MouseReleaseMsg:
			gotMouse = m.Mouse()
		default:
			t.Errorf("Unexpected message type: %T", reconstructed)
			continue
		}

		if gotMouse.Button != tc.want.Button {
			t.Errorf("Round-trip button mismatch: got %v, want %v", gotMouse.Button, tc.want.Button)
		}
		if gotMouse.X != tc.want.X || gotMouse.Y != tc.want.Y {
			t.Errorf("Round-trip position mismatch: got (%d,%d), want (%d,%d)", gotMouse.X, gotMouse.Y, tc.want.X, tc.want.Y)
		}
		if gotMouse.Mod != tc.want.Mod {
			t.Errorf("Round-trip modifier mismatch: got %v, want %v", gotMouse.Mod, tc.want.Mod)
		}
	}
}
