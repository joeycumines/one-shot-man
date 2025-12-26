package bubbletea

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestParseMouseEvent(t *testing.T) {
	tests := []struct {
		name   string
		input  string
		want   tea.MouseEvent
		wantOk bool
	}{
		// Basic button presses
		{
			name:  "left press",
			input: "left press",
			want: tea.MouseEvent{
				Button: tea.MouseButtonLeft,
				Action: tea.MouseActionPress,
			},
			wantOk: true,
		},
		{
			name:  "right press",
			input: "right press",
			want: tea.MouseEvent{
				Button: tea.MouseButtonRight,
				Action: tea.MouseActionPress,
			},
			wantOk: true,
		},
		{
			name:  "middle press",
			input: "middle press",
			want: tea.MouseEvent{
				Button: tea.MouseButtonMiddle,
				Action: tea.MouseActionPress,
			},
			wantOk: true,
		},
		// Button releases
		{
			name:  "left release",
			input: "left release",
			want: tea.MouseEvent{
				Button: tea.MouseButtonLeft,
				Action: tea.MouseActionRelease,
			},
			wantOk: true,
		},
		{
			name:  "right release",
			input: "right release",
			want: tea.MouseEvent{
				Button: tea.MouseButtonRight,
				Action: tea.MouseActionRelease,
			},
			wantOk: true,
		},
		// Wheel events (no action needed - they're always press)
		{
			name:  "wheel up",
			input: "wheel up",
			want: tea.MouseEvent{
				Button: tea.MouseButtonWheelUp,
				Action: tea.MouseActionPress,
			},
			wantOk: true,
		},
		{
			name:  "wheel down",
			input: "wheel down",
			want: tea.MouseEvent{
				Button: tea.MouseButtonWheelDown,
				Action: tea.MouseActionPress,
			},
			wantOk: true,
		},
		{
			name:  "wheel left",
			input: "wheel left",
			want: tea.MouseEvent{
				Button: tea.MouseButtonWheelLeft,
				Action: tea.MouseActionPress,
			},
			wantOk: true,
		},
		{
			name:  "wheel right",
			input: "wheel right",
			want: tea.MouseEvent{
				Button: tea.MouseButtonWheelRight,
				Action: tea.MouseActionPress,
			},
			wantOk: true,
		},
		// Motion event
		{
			name:  "motion",
			input: "motion",
			want: tea.MouseEvent{
				Button: tea.MouseButtonNone,
				Action: tea.MouseActionMotion,
			},
			wantOk: true,
		},
		// Release without button
		{
			name:  "release alone",
			input: "release",
			want: tea.MouseEvent{
				Button: tea.MouseButtonNone,
				Action: tea.MouseActionRelease,
			},
			wantOk: true,
		},
		// With modifiers
		{
			name:  "ctrl left press",
			input: "ctrl+left press",
			want: tea.MouseEvent{
				Button: tea.MouseButtonLeft,
				Action: tea.MouseActionPress,
				Ctrl:   true,
			},
			wantOk: true,
		},
		{
			name:  "alt right press",
			input: "alt+right press",
			want: tea.MouseEvent{
				Button: tea.MouseButtonRight,
				Action: tea.MouseActionPress,
				Alt:    true,
			},
			wantOk: true,
		},
		{
			name:  "shift middle press",
			input: "shift+middle press",
			want: tea.MouseEvent{
				Button: tea.MouseButtonMiddle,
				Action: tea.MouseActionPress,
				Shift:  true,
			},
			wantOk: true,
		},
		{
			name:  "ctrl alt left press",
			input: "ctrl+alt+left press",
			want: tea.MouseEvent{
				Button: tea.MouseButtonLeft,
				Action: tea.MouseActionPress,
				Ctrl:   true,
				Alt:    true,
			},
			wantOk: true,
		},
		{
			name:  "ctrl alt shift left press",
			input: "ctrl+alt+shift+left press",
			want: tea.MouseEvent{
				Button: tea.MouseButtonLeft,
				Action: tea.MouseActionPress,
				Ctrl:   true,
				Alt:    true,
				Shift:  true,
			},
			wantOk: true,
		},
		{
			name:  "ctrl wheel up",
			input: "ctrl+wheel up",
			want: tea.MouseEvent{
				Button: tea.MouseButtonWheelUp,
				Action: tea.MouseActionPress,
				Ctrl:   true,
			},
			wantOk: true,
		},
		// Extended buttons
		{
			name:  "backward press",
			input: "backward press",
			want: tea.MouseEvent{
				Button: tea.MouseButtonBackward,
				Action: tea.MouseActionPress,
			},
			wantOk: true,
		},
		{
			name:  "forward press",
			input: "forward press",
			want: tea.MouseEvent{
				Button: tea.MouseButtonForward,
				Action: tea.MouseActionPress,
			},
			wantOk: true,
		},
		{
			name:  "button 10 press",
			input: "button 10 press",
			want: tea.MouseEvent{
				Button: tea.MouseButton10,
				Action: tea.MouseActionPress,
			},
			wantOk: true,
		},
		{
			name:  "button 11 press",
			input: "button 11 press",
			want: tea.MouseEvent{
				Button: tea.MouseButton11,
				Action: tea.MouseActionPress,
			},
			wantOk: true,
		},
		// Empty input
		{
			name:   "empty string",
			input:  "",
			want:   tea.MouseEvent{},
			wantOk: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := ParseMouseEvent(tt.input)
			if ok != tt.wantOk {
				t.Errorf("ParseMouseEvent(%q) ok = %v, want %v", tt.input, ok, tt.wantOk)
				return
			}
			if !tt.wantOk {
				return
			}
			if got.Button != tt.want.Button {
				t.Errorf("ParseMouseEvent(%q) button = %v, want %v", tt.input, got.Button, tt.want.Button)
			}
			if got.Action != tt.want.Action {
				t.Errorf("ParseMouseEvent(%q) action = %v, want %v", tt.input, got.Action, tt.want.Action)
			}
			if got.Ctrl != tt.want.Ctrl {
				t.Errorf("ParseMouseEvent(%q) ctrl = %v, want %v", tt.input, got.Ctrl, tt.want.Ctrl)
			}
			if got.Alt != tt.want.Alt {
				t.Errorf("ParseMouseEvent(%q) alt = %v, want %v", tt.input, got.Alt, tt.want.Alt)
			}
			if got.Shift != tt.want.Shift {
				t.Errorf("ParseMouseEvent(%q) shift = %v, want %v", tt.input, got.Shift, tt.want.Shift)
			}
		})
	}
}

func TestMouseEventToJS(t *testing.T) {
	tests := []struct {
		name       string
		event      tea.MouseMsg
		wantButton string
		wantAction string
		wantWheel  bool
	}{
		{
			name: "left press",
			event: tea.MouseMsg(tea.MouseEvent{
				Button: tea.MouseButtonLeft,
				Action: tea.MouseActionPress,
			}),
			wantButton: "left",
			wantAction: "press",
			wantWheel:  false,
		},
		{
			name: "right release",
			event: tea.MouseMsg(tea.MouseEvent{
				Button: tea.MouseButtonRight,
				Action: tea.MouseActionRelease,
			}),
			wantButton: "right",
			wantAction: "release",
			wantWheel:  false,
		},
		{
			name: "wheel up",
			event: tea.MouseMsg(tea.MouseEvent{
				Button: tea.MouseButtonWheelUp,
				Action: tea.MouseActionPress,
			}),
			wantButton: "wheel up",
			wantAction: "press",
			wantWheel:  true,
		},
		{
			name: "motion",
			event: tea.MouseMsg(tea.MouseEvent{
				Button: tea.MouseButtonNone,
				Action: tea.MouseActionMotion,
			}),
			wantButton: "none",
			wantAction: "motion",
			wantWheel:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			js := MouseEventToJS(tt.event)
			if js["button"] != tt.wantButton {
				t.Errorf("MouseEventToJS() button = %v, want %v", js["button"], tt.wantButton)
			}
			if js["action"] != tt.wantAction {
				t.Errorf("MouseEventToJS() action = %v, want %v", js["action"], tt.wantAction)
			}
			if js["isWheel"] != tt.wantWheel {
				t.Errorf("MouseEventToJS() isWheel = %v, want %v", js["isWheel"], tt.wantWheel)
			}
		})
	}
}

func TestJSToMouseEvent(t *testing.T) {
	tests := []struct {
		name      string
		button    string
		action    string
		x, y      int
		alt       bool
		ctrl      bool
		shift     bool
		wantEvent tea.MouseEvent
	}{
		{
			name:   "left press",
			button: "left",
			action: "press",
			x:      10,
			y:      20,
			wantEvent: tea.MouseEvent{
				Button: tea.MouseButtonLeft,
				Action: tea.MouseActionPress,
				X:      10,
				Y:      20,
			},
		},
		{
			name:   "wheel down with modifiers",
			button: "wheel down",
			action: "press",
			x:      5,
			y:      15,
			ctrl:   true,
			alt:    true,
			wantEvent: tea.MouseEvent{
				Button: tea.MouseButtonWheelDown,
				Action: tea.MouseActionPress,
				X:      5,
				Y:      15,
				Ctrl:   true,
				Alt:    true,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := JSToMouseEvent(tt.button, tt.action, tt.x, tt.y, tt.alt, tt.ctrl, tt.shift)
			event := tea.MouseEvent(got)
			if event.Button != tt.wantEvent.Button {
				t.Errorf("JSToMouseEvent() button = %v, want %v", event.Button, tt.wantEvent.Button)
			}
			if event.Action != tt.wantEvent.Action {
				t.Errorf("JSToMouseEvent() action = %v, want %v", event.Action, tt.wantEvent.Action)
			}
			if event.X != tt.wantEvent.X {
				t.Errorf("JSToMouseEvent() x = %v, want %v", event.X, tt.wantEvent.X)
			}
			if event.Y != tt.wantEvent.Y {
				t.Errorf("JSToMouseEvent() y = %v, want %v", event.Y, tt.wantEvent.Y)
			}
			if event.Ctrl != tt.wantEvent.Ctrl {
				t.Errorf("JSToMouseEvent() ctrl = %v, want %v", event.Ctrl, tt.wantEvent.Ctrl)
			}
			if event.Alt != tt.wantEvent.Alt {
				t.Errorf("JSToMouseEvent() alt = %v, want %v", event.Alt, tt.wantEvent.Alt)
			}
		})
	}
}

func TestMouseEventRoundTrip(t *testing.T) {
	// Test that MouseEventToJS -> JSToMouseEvent preserves the event
	events := []tea.MouseEvent{
		{Button: tea.MouseButtonLeft, Action: tea.MouseActionPress, X: 10, Y: 20},
		{Button: tea.MouseButtonRight, Action: tea.MouseActionRelease, X: 5, Y: 15, Ctrl: true},
		{Button: tea.MouseButtonWheelUp, Action: tea.MouseActionPress, X: 0, Y: 0},
		{Button: tea.MouseButtonNone, Action: tea.MouseActionMotion, X: 100, Y: 200, Alt: true, Shift: true},
	}

	for _, orig := range events {
		js := MouseEventToJS(tea.MouseMsg(orig))
		reconstructed := JSToMouseEvent(
			js["button"].(string),
			js["action"].(string),
			js["x"].(int),
			js["y"].(int),
			js["alt"].(bool),
			js["ctrl"].(bool),
			js["shift"].(bool),
		)
		got := tea.MouseEvent(reconstructed)

		if got.Button != orig.Button {
			t.Errorf("Round-trip button mismatch: got %v, want %v", got.Button, orig.Button)
		}
		if got.Action != orig.Action {
			t.Errorf("Round-trip action mismatch: got %v, want %v", got.Action, orig.Action)
		}
		if got.X != orig.X || got.Y != orig.Y {
			t.Errorf("Round-trip position mismatch: got (%d,%d), want (%d,%d)", got.X, got.Y, orig.X, orig.Y)
		}
		if got.Ctrl != orig.Ctrl || got.Alt != orig.Alt || got.Shift != orig.Shift {
			t.Errorf("Round-trip modifier mismatch: got (ctrl=%v,alt=%v,shift=%v), want (ctrl=%v,alt=%v,shift=%v)",
				got.Ctrl, got.Alt, got.Shift, orig.Ctrl, orig.Alt, orig.Shift)
		}
	}
}
