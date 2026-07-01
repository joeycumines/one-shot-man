package termmux

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/joeycumines/goja"

	parent "github.com/joeycumines/one-shot-man/internal/termmux"
)

func setupStatusModule(t *testing.T) (*goja.Runtime, *goja.Object, func()) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	mgr := parent.NewSessionManager()
	var buf bytes.Buffer
	runtime := goja.New()
	obj := wrapTestSessionManager(t, ctx, runtime, mgr, nil, &buf, -1, "").(*goja.Object)
	if err := runtime.Set("mux", obj); err != nil {
		t.Fatalf("set mux: %v", err)
	}

	return runtime, obj, cancel
}

func TestStatusBindings_ChainAndDefault(t *testing.T) {
	runtime, _, cleanup := setupStatusModule(t)
	defer cleanup()

	v, err := runtime.RunString(`mux.setStatusColors({ fg: "#ffffff", bg: "#000000" }).setStatusPosition("bottom")`)
	if err != nil {
		t.Fatalf("chaining failed: %v", err)
	}
	if v == nil || goja.IsUndefined(v) {
		t.Fatal("chaining methods should return the module object")
	}

	render, err := runtime.RunString(`mux.renderStatusBar(80)`)
	if err != nil {
		t.Fatalf("renderStatusBar: %v", err)
	}
	got := render.String()
	if !strings.Contains(got, "\x1b[24;1H") {
		t.Errorf("default bottom position should target row 24; got %q", got)
	}
}

func TestStatusBindings_SetStatusPositionTop(t *testing.T) {
	runtime, _, cleanup := setupStatusModule(t)
	defer cleanup()

	_, err := runtime.RunString(`mux.setStatusPosition("top")`)
	if err != nil {
		t.Fatalf("setStatusPosition(top): %v", err)
	}

	render, err := runtime.RunString(`mux.renderStatusBar(80, "session", "window")`)
	if err != nil {
		t.Fatalf("renderStatusBar: %v", err)
	}
	got := render.String()
	if !strings.Contains(got, "\x1b[1;1H") {
		t.Errorf("top position should target row 1; got %q", got)
	}
	if strings.Contains(got, "\x1b[24;1H") {
		t.Errorf("top position should not target row 24; got %q", got)
	}
	if !strings.Contains(got, "session") {
		t.Errorf("rendered left text missing; got %q", got)
	}
	if !strings.Contains(got, "window") {
		t.Errorf("rendered right text missing; got %q", got)
	}
}

func TestStatusBindings_SetStatusColors(t *testing.T) {
	runtime, _, cleanup := setupStatusModule(t)
	defer cleanup()

	_, err := runtime.RunString(`mux.setStatusColors({ fg: "#ff5733", bg: "#1a1a2e" })`)
	if err != nil {
		t.Fatalf("setStatusColors: %v", err)
	}

	render, err := runtime.RunString(`mux.renderStatusBar(80)`)
	if err != nil {
		t.Fatalf("renderStatusBar: %v", err)
	}
	got := render.String()
	if strings.Contains(got, "\x1b[7m") {
		t.Errorf("explicit colors should replace reverse video; got %q", got)
	}
	if !strings.Contains(got, "\x1b[38;2;255;87;51m") {
		t.Errorf("foreground SGR missing; got %q", got)
	}
	if !strings.Contains(got, "\x1b[48;2;26;26;46m") {
		t.Errorf("background SGR missing; got %q", got)
	}
	if !strings.Contains(got, "\x1b[0m") {
		t.Errorf("reset SGR missing; got %q", got)
	}
}

func TestStatusBindings_InvalidColorThrows(t *testing.T) {
	runtime, _, cleanup := setupStatusModule(t)
	defer cleanup()

	_, err := runtime.RunString(`mux.setStatusColors({ fg: "not-a-color" })`)
	if err == nil {
		t.Fatal("expected setStatusColors with invalid color to throw")
	}
	if !strings.Contains(err.Error(), "setStatusColors") {
		t.Errorf("error should mention setStatusColors; got %v", err)
	}
}

func TestStatusBindings_InvalidPositionThrows(t *testing.T) {
	runtime, _, cleanup := setupStatusModule(t)
	defer cleanup()

	_, err := runtime.RunString(`mux.setStatusPosition("middle")`)
	if err == nil {
		t.Fatal("expected setStatusPosition with invalid value to throw")
	}
	if !strings.Contains(err.Error(), "setStatusPosition") {
		t.Errorf("error should mention setStatusPosition; got %v", err)
	}
}

func TestStatusBindings_RenderWidth(t *testing.T) {
	runtime, _, cleanup := setupStatusModule(t)
	defer cleanup()

	render, err := runtime.RunString(`mux.renderStatusBar(20, "left", "right")`)
	if err != nil {
		t.Fatalf("renderStatusBar: %v", err)
	}
	got := render.String()
	if strings.Contains(got, "\n") {
		t.Errorf("rendered bar should be a single line; got %q", got)
	}
	content := stripEscapeSequences(got)
	if len([]rune(content)) != 20 {
		t.Errorf("rendered visual width = %d, want 20; got %q", len([]rune(content)), content)
	}
}

func stripEscapeSequences(s string) string {
	var b strings.Builder
	inEsc := false
	for _, r := range s {
		if inEsc {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') {
				inEsc = false
			}
			continue
		}
		if r == '\x1b' {
			inEsc = true
			continue
		}
		b.WriteRune(r)
	}
	return b.String()
}
