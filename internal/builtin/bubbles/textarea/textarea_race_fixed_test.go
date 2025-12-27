//go:build race

package textarea

import (
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/dop251/goja"
)

// TestTextarea_ConcurrentSetViewportContextAndReaders_Race exercises concurrent
// execution of JS-exposed methods that touch the viewport context. It is a
// race-detection test that is only compiled/ran under `-race` builds (see
// build tag above). The test spawns multiple goroutines that repeatedly call
// `setViewportContext`, `handleClickAtScreenCoords`, and `getScrollSyncInfo`
// on the same textarea instance for a short, deterministic duration.
func TestTextarea_ConcurrentSetViewportContextAndReaders_Race(t *testing.T) {
	// Keep duration short so CI runs remain fast
	duration := 250 * time.Millisecond

	manager := NewManager()
	runtime := goja.New()
	module := runtime.NewObject()
	Require(manager)(runtime, module)
	exports := module.Get("exports").ToObject(runtime)

	newFn, _ := goja.AssertFunction(exports.Get("new"))
	res, _ := newFn(goja.Undefined())
	ta := res.ToObject(runtime)

	// Provide some sizable content so reader functions do non-trivial work
	setValueFn, _ := goja.AssertFunction(ta.Get("setValue"))
	_, _ = setValueFn(ta, runtime.ToValue(strings.Repeat("x\n", 1000)))

	setViewportContextFn, _ := goja.AssertFunction(ta.Get("setViewportContext"))
	handleClickFn, _ := goja.AssertFunction(ta.Get("handleClickAtScreenCoords"))
	getScrollSyncInfoFn, _ := goja.AssertFunction(ta.Get("getScrollSyncInfo"))

	stop := make(chan struct{})
	var wg sync.WaitGroup
	var rtMu sync.Mutex

	// Writers
	for i := 0; i < 4; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			idx := 0
			for {
				select {
				case <-stop:
					return
				default:
					rtMu.Lock()
					obj := runtime.NewObject()
					_ = obj.Set("outerYOffset", idx%100)
					_ = obj.Set("textareaContentTop", 2)
					_ = obj.Set("textareaContentLeft", 0)
					_ = obj.Set("outerViewportHeight", 50)
					_ = obj.Set("preContentHeight", 2)
					_, _ = setViewportContextFn(ta, obj)
					rtMu.Unlock()
					idx++
				}
			}
		}()
	}

	// Readers
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					rtMu.Lock()
					_, _ = handleClickFn(ta, runtime.ToValue(10), runtime.ToValue(10), runtime.ToValue(1))
					_, _ = getScrollSyncInfoFn(ta)
					rtMu.Unlock()
				}
			}
		}()
	}

	// Run and then stop
	time.Sleep(duration)
	close(stop)
	wg.Wait()
}
