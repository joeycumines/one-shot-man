package termmux

import (
	"fmt"
	"math/rand"
	"strings"
	"testing"

	"github.com/dop251/goja"
)

func TestHandlePrefixKey_NewWindow(t *testing.T) {
	if testing.Short() {
		t.Skip("JS integration test skipped in short mode")
	}
	ctx := t.Context()
	runtime, _, env := testRequireCtx(t, ctx)
	defer env.stop()
	mgrName := newTestManager(t, runtime)

	res, err := runtime.RunString(fmt.Sprintf(`require('osm:termmux').handlePrefixKey({ manager: %s, key: "c" })`, mgrName))
	if err != nil {
		t.Fatalf("handlePrefixKey(c): %v", err)
	}
	obj := res.ToObject(runtime)
	if !obj.Get("consumed").ToBoolean() {
		t.Fatalf("expected consumed")
	}
	if obj.Get("action").String() != "NewWindow" {
		t.Fatalf("expected action NewWindow, got %s", obj.Get("action").String())
	}
	if obj.Get("result").ToInteger() == 0 {
		t.Fatalf("expected non-zero window ID")
	}
}

func TestHandlePrefixKey_ClosePane(t *testing.T) {
	if testing.Short() {
		t.Skip("JS integration test skipped in short mode")
	}
	ctx := t.Context()
	runtime, _, env := testRequireCtx(t, ctx)
	defer env.stop()
	mgrName := newTestManager(t, runtime)

	if _, err := runtime.RunString(fmt.Sprintf(`require('osm:termmux').handlePrefixKey({ manager: %s, key: "c" })`, mgrName)); err != nil {
		t.Fatalf("new window: %v", err)
	}

	res, err := runtime.RunString(fmt.Sprintf(`require('osm:termmux').handlePrefixKey({ manager: %s, key: "x" })`, mgrName))
	if err != nil {
		t.Fatalf("handlePrefixKey(x): %v", err)
	}
	obj := res.ToObject(runtime)
	if !obj.Get("consumed").ToBoolean() {
		t.Fatalf("expected consumed")
	}
	if obj.Get("action").String() != "ClosePane" {
		t.Fatalf("expected action ClosePane, got %s", obj.Get("action").String())
	}
}

func TestHandlePrefixKey_ListKeys(t *testing.T) {
	if testing.Short() {
		t.Skip("JS integration test skipped in short mode")
	}
	ctx := t.Context()
	runtime, _, env := testRequireCtx(t, ctx)
	defer env.stop()
	mgrName := newTestManager(t, runtime)

	res, err := runtime.RunString(fmt.Sprintf(`require('osm:termmux').handlePrefixKey({ manager: %s, key: "?" })`, mgrName))
	if err != nil {
		t.Fatalf("handlePrefixKey(?): %v", err)
	}
	obj := res.ToObject(runtime)
	if !obj.Get("consumed").ToBoolean() {
		t.Fatalf("expected consumed")
	}
	if obj.Get("action").String() != "ListKeys" {
		t.Fatalf("expected action ListKeys, got %s", obj.Get("action").String())
	}
	listKeys := obj.Get("listKeys").String()
	if listKeys == "" {
		t.Fatalf("expected non-empty listKeys")
	}
	for _, want := range []string{"NewWindow", "NextWindow", "ClosePane"} {
		if !strings.Contains(listKeys, want) {
			t.Fatalf("listKeys missing %q: %s", want, listKeys)
		}
	}
}

func newTestManager(t *testing.T, runtime *goja.Runtime) string {
	t.Helper()
	name := fmt.Sprintf("__mgr_%d__", rand.Int())
	script := fmt.Sprintf(`
		var %s = require('osm:termmux').newSessionManager({ rows: 24, cols: 80 });
		%s.run();
		var deadline = Date.now() + 2000;
		while (!%s.started() && Date.now() < deadline) {}
		%s`, name, name, name, name)
	if _, err := runtime.RunString(script); err != nil {
		t.Fatalf("create manager: %v", err)
	}
	return name
}
