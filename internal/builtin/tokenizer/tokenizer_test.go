package tokenizermod_test

import (
	"context"
	"time"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/scripting"
	"github.com/joeycumines/one-shot-man/internal/testutil"
	"bytes"
	"log/slog"
)

func newTestEngine(t *testing.T) *scripting.Engine {
	t.Helper()
	ctx := context.Background()
	var stdout, stderr bytes.Buffer
	engine, err := scripting.NewEngine(ctx, &stdout, &stderr, testutil.NewTestSessionID("tokenizer", t.Name()), "memory", nil, 0, slog.LevelInfo)
	if err != nil {
		t.Fatalf("NewEngine: %v", err)
	}
	t.Cleanup(func() { engine.Close() })
	return engine
}

func writeTempTokenizer(t *testing.T, data string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "tokenizer.json")
	if err := os.WriteFile(path, []byte(data), 0600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}

func TestLoadFile_Success(t *testing.T) {
	t.Parallel()
	jsonData := `{"type":"BPE","vocab":{"<unk>":0,"a":1,"b":2,"ab":3},"merges":["a b"],"unk_token":"<unk>","fuse_unk":true,"byte_fallback":false}`
	path := writeTempTokenizer(t, jsonData)
	engine := newTestEngine(t)
	script := engine.LoadScriptString("loadfile-success", `
		var tokMod = require('osm:tokenizer');
		var p = tokMod.loadFile("`+strings.ReplaceAll(path, `\`, `\\`)+`");
		if (typeof p.then !== 'function') throw new Error('loadFile did not return Promise');
		p.then(function(wrapper){
			if (!wrapper || typeof wrapper.encode !== 'function') throw new Error('wrapper missing encode');
			var res = wrapper.encode("ab");
			if (res.count !== 1) throw new Error('encode count want 1 got '+res.count);
			if (res.tokens[0].id !== 3) throw new Error('token id want 3 got '+res.tokens[0].id);
			globalThis.__testOK = true;
		}).catch(function(e){ globalThis.__testErr = e.message; });
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript: %v", err)
	}
	// Wait a bit for async Promise to settle via loop
	// Use ExecuteScript to check flag
	for i := 0; i < 20; i++ {
		time.Sleep(50*time.Millisecond)
		chk := engine.LoadScriptString("check-ok", `if (!globalThis.__testOK) throw new Error('not ok yet:'+(globalThis.__testErr||'pending'))`)
		if err := engine.ExecuteScript(chk); err == nil {
			return
		}
		
	}
	t.Fatalf("loadFile success did not settle")
}

func TestLoadFile_Error(t *testing.T) {
	t.Parallel()
	engine := newTestEngine(t)
	script := engine.LoadScriptString("loadfile-error", `
		var tokMod = require('osm:tokenizer');
		var p = tokMod.loadFile("/nonexistent/path407.json");
		p.then(function(){ globalThis.__testErr = 'should reject'; }).catch(function(e){
			if (e.message.indexOf('loadFile') === -1) { globalThis.__testErr = 'wrong message:'+e.message; return; }
			globalThis.__testOK = true;
		});
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript: %v", err)
	}
	for i := 0; i < 20; i++ {
		time.Sleep(50*time.Millisecond)
		chk := engine.LoadScriptString("check-err", `if (!globalThis.__testOK) throw new Error(globalThis.__testErr||'pending')`)
		if err := engine.ExecuteScript(chk); err == nil {
			return
		}
	}
	t.Fatalf("loadFile error case did not reject as expected")
}

func TestLoadFile_ConcurrentHammer(t *testing.T) {
	// No t.Parallel - uses -race hammer
	jsonData := `{"type":"BPE","vocab":{"<unk>":0,"a":1,"b":2,"ab":3},"merges":["a b"],"unk_token":"<unk>","fuse_unk":true,"byte_fallback":false}`
	path := writeTempTokenizer(t, jsonData)
	engine := newTestEngine(t)
	var wg sync.WaitGroup
	errs := make(chan error, 4)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			script := engine.LoadScriptString("hammer-"+string(rune('0'+idx)), `
				var tokMod = require('osm:tokenizer');
				var p = tokMod.loadFile("`+strings.ReplaceAll(path, `\`, `\\`)+`");
				p.then(function(w){ var r=w.encode("ab"); if(r.count!==1) throw new Error('bad'); }).catch(function(e){ throw e; });
			`)
			if err := engine.ExecuteScript(script); err != nil {
				errs <- err
			} else {
				errs <- nil
			}
		}(i)
	}
	// Concurrent other JS activity
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 50; i++ {
			script := engine.LoadScriptString("other-"+string(rune('0'+i)), `var x = 1+1;`)
			_ = engine.ExecuteScript(script)
		}
		errs <- nil
	}()
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent hammer error: %v", err)
		}
	}
	// Also hammer via direct engine loop activity for race detector
	// Run a final check that engine still responsive
	chk := engine.LoadScriptString("final-check", `if (1+1!==2) throw new Error('broken')`)
	if err := engine.ExecuteScript(chk); err != nil {
		t.Fatalf("final check failed after hammer: %v", err)
	}
}

func TestLoadFile_EmptyPath(t *testing.T) {
	t.Parallel()
	engine := newTestEngine(t)
	script := engine.LoadScriptString("empty", `
		var tokMod = require('osm:tokenizer');
		var p = tokMod.loadFile("");
		p.then(function(v){ if (v !== null) throw new Error('want null got '+v); globalThis.__testOK=true; }).catch(function(e){ globalThis.__testErr=e.message; });
	`)
	if err := engine.ExecuteScript(script); err != nil {
		t.Fatalf("ExecuteScript: %v", err)
	}
	for i := 0; i < 20; i++ {
		time.Sleep(50*time.Millisecond)
		chk := engine.LoadScriptString("check-empty", `if (!globalThis.__testOK) throw new Error(globalThis.__testErr||'pending')`)
		if err := engine.ExecuteScript(chk); err == nil {
			return
		}
	}
	t.Fatalf("empty path case pending")
}
