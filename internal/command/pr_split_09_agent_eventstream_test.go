package command

// Tests for the optional TUIStateMachine / EventStream integration in
// AgentCodeExecutor (pr_split_09_agent.js). These verify that:
//   - initEventTracking creates a stateMachine when aimux provides
//     newTUIStateMachine
//   - startEventLoop feeds output lines to the state machine, causing
//     state transitions
//   - close() stops the event loop and nulls out all enhancement fields

import (
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// TestAgentEventStream_StateMachineCreation verifies that initEventTracking
// creates a non-null stateMachine with the expected methods when the aimux
// module provides newTUIStateMachine.
func TestAgentEventStream_StateMachineCreation(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	evalJS := prsplittest.NewFullEngine(t, nil)

	_, err := evalJS(`
		var exec = new prSplit.AgentCodeExecutor({
			agentCommand: 'echo',
			agentArgs: [],
		});
		exec.handle = {
			isAlive: function() { return true; },
			receiveEventAsync: function() { return Promise.resolve(null); },
			close: function() {},
		};
		exec.initEventTracking();
		globalThis.__testExec = exec;
	`)
	if err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`__testExec.stateMachine !== null`)
	if err != nil {
		t.Fatal(err)
	}
	if val != true {
		t.Fatal("expected stateMachine to be non-null after initEventTracking")
	}

	for _, method := range []string{"processOutput", "stateName", "state", "reset", "checkTimeout"} {
		val, err := evalJS(`typeof __testExec.stateMachine.` + method)
		if err != nil {
			t.Fatal(err)
		}
		if val != "function" {
			t.Errorf("stateMachine.%s = %v, want 'function'", method, val)
		}
	}

	val, err = evalJS(`__testExec.stateMachine.stateName()`)
	if err != nil {
		t.Fatal(err)
	}
	stateName, _ := val.(string)
	if stateName != "Initializing" {
		t.Errorf("initial stateName = %q, want 'Initializing'", stateName)
	}
}

// TestAgentEventStream_StateMachineCreation_NoHandle verifies that
// initEventTracking does not crash when handle is null. The stateMachine
// is a standalone state tracker and can be created without a handle,
// but eventStream and healthMonitor require a handle and should be null.
// The event loop should not start without a handle.
func TestAgentEventStream_StateMachineCreation_NoHandle(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	evalJS := prsplittest.NewFullEngine(t, nil)

	_, err := evalJS(`
		var exec = new prSplit.AgentCodeExecutor({
			agentCommand: 'echo',
			agentArgs: [],
		});
		exec.handle = null;
		exec.initEventTracking();
		globalThis.__testExec = exec;
	`)
	if err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`__testExec.eventStream`)
	if err != nil {
		t.Fatal(err)
	}
	if val != nil {
		t.Errorf("expected eventStream to be null when handle is null, got %v", val)
	}

	val, err = evalJS(`__testExec.healthMonitor`)
	if err != nil {
		t.Fatal(err)
	}
	if val != nil {
		t.Errorf("expected healthMonitor to be null when handle is null, got %v", val)
	}

	val, err = evalJS(`__testExec._eventLoopRunning`)
	if err != nil {
		t.Fatal(err)
	}
	if val == true {
		t.Error("expected event loop to not be running without a handle")
	}
}

// TestAgentEventStream_EventLoopFeedsEvents verifies that startEventLoop
// reads lines from the handle via receiveEventAsync and feeds them to
// the state machine's processOutput, causing a state transition.
func TestAgentEventStream_EventLoopFeedsEvents(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	evalJS := prsplittest.NewFullEngine(t, nil)

	// Mock handle: returns ">" (matches the default ReadyPattern ^>\s*$)
	// on the first call, then null (EOF) to stop the loop.
	_, err := evalJS(`
		var __callCount = 0;
		var exec = new prSplit.AgentCodeExecutor({
			agentCommand: 'echo',
			agentArgs: [],
		});
		exec.handle = {
			isAlive: function() { return true; },
			receiveEventAsync: function() {
				__callCount++;
				if (__callCount === 1) {
					return Promise.resolve('>');
				}
				return Promise.resolve(null);
			},
			close: function() {},
		};
		exec.initEventTracking();
		globalThis.__testExec = exec;
		globalThis.__callCount = __callCount;
	`)
	if err != nil {
		t.Fatal(err)
	}

	// Allow the JS event loop to process the Promise chain.
	_, err = evalJS(`await new Promise(function(resolve) { setTimeout(resolve, 200); })`)
	if err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`__testExec.stateMachine.stateName()`)
	if err != nil {
		t.Fatal(err)
	}
	stateName, _ := val.(string)
	if stateName != "Ready" {
		t.Errorf("stateName = %q, want 'Ready' (event loop should have fed '>' to processOutput)", stateName)
	}

	val, err = evalJS(`__callCount`)
	if err != nil {
		t.Fatal(err)
	}
	callCount := toInt64(val)
	if callCount < 1 {
		t.Errorf("receiveEventAsync call count = %d, want >= 1", callCount)
	}
}

// TestAgentEventStream_EventLoopStopsOnClose verifies that close()
// stops the event loop, closes the eventStream and healthMonitor, and
// nulls out all enhancement fields.
func TestAgentEventStream_EventLoopStopsOnClose(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	evalJS := prsplittest.NewFullEngine(t, nil)

	_, err := evalJS(`
		var exec = new prSplit.AgentCodeExecutor({
			agentCommand: 'echo',
			agentArgs: [],
		});
		exec.handle = {
			isAlive: function() { return true; },
			receiveEventAsync: function() {
				return new Promise(function(resolve) {
					setTimeout(function() { resolve('test'); }, 50);
				});
			},
			close: function() {},
		};
		exec.initEventTracking();
		globalThis.__testExec = exec;
	`)
	if err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`__testExec._eventLoopRunning`)
	if err != nil {
		t.Fatal(err)
	}
	if val != true {
		t.Fatal("expected event loop to be running before close")
	}

	_, err = evalJS(`__testExec.close()`)
	if err != nil {
		t.Fatal(err)
	}

	val, err = evalJS(`__testExec._eventLoopRunning`)
	if err != nil {
		t.Fatal(err)
	}
	if val == true {
		t.Error("expected _eventLoopRunning to be false after close")
	}

	val, err = evalJS(`__testExec._eventLoopStop`)
	if err != nil {
		t.Fatal(err)
	}
	if val != true {
		t.Error("expected _eventLoopStop to be true after close")
	}

	for _, field := range []string{"stateMachine", "eventStream", "healthMonitor", "handle"} {
		val, err := evalJS(`__testExec.` + field)
		if err != nil {
			t.Fatal(err)
		}
		if val != nil {
			t.Errorf("expected %s to be null after close, got %v", field, val)
		}
	}
}

// TestAgentEventStream_WaitForPromptReady_StateMachineFastPath verifies
// that waitForPromptReady returns immediately (without screenshot polling)
// when the agentExecutor's stateMachine reports "Ready". We test this
// indirectly through sendToHandle: if the fast path works, sendToHandle
// should complete quickly without a "prompt not ready" error.
func TestAgentEventStream_WaitForPromptReady_StateMachineFastPath(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	evalJS := prsplittest.NewFullEngine(t, nil)

	_, err := evalJS(`
		var exec = new prSplit.AgentCodeExecutor({
			agentCommand: 'echo',
			agentArgs: [],
		});
		exec.handle = {
			isAlive: function() { return true; },
			receiveEventAsync: function() { return Promise.resolve(null); },
			close: function() {},
		};
		exec.initEventTracking();
		exec.stateMachine.processOutput('>');
		prSplit._state.agentExecutor = exec;

		globalThis.__mockHandle = {
			send: function(data) {},
			isAlive: function() { return true; },
			receive: function() { return ''; },
		};
		prSplit.SEND_PROMPT_READY_TIMEOUT_MS = 200;
		prSplit.SEND_PRE_SUBMIT_STABLE_TIMEOUT_MS = 200;
		prSplit.SEND_SUBMIT_ACK_TIMEOUT_MS = 200;
		prSplit.SEND_TEXT_NEWLINE_DELAY_MS = 0;
		prSplit.SEND_TEXT_CHUNK_DELAY_MS = 0;
	`)
	if err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`prSplit._state.agentExecutor.stateMachine.stateName()`)
	if err != nil {
		t.Fatal(err)
	}
	if val != "Ready" {
		t.Fatalf("precondition: stateName = %v, want 'Ready'", val)
	}

	_, err = evalJS(`globalThis.__sendResult = await prSplit.sendToHandle(__mockHandle, 'test')`)
	if err != nil {
		t.Fatal(err)
	}

	val, err = evalJS(`JSON.stringify(globalThis.__sendResult)`)
	if err != nil {
		t.Fatal(err)
	}
	result, _ := val.(string)
	if strings.Contains(result, "prompt not ready") {
		t.Errorf("sendToHandle timed out waiting for prompt ready even though stateMachine is Ready: %s", result)
	}
}

// TestAgentEventStream_WaitForPromptReady_FallbackWithoutStateMachine
// verifies that when no stateMachine is available, waitForPromptReady
// falls back to the existing screenshot-based detection (or returns
// observed=false when no screenshot transport is available).
func TestAgentEventStream_WaitForPromptReady_FallbackWithoutStateMachine(t *testing.T) {
	skipSlow(t)
	t.Parallel()

	evalJS := prsplittest.NewFullEngine(t, nil)

	_, err := evalJS(`
		prSplit._state.agentExecutor = null;
		prSplit.SEND_PROMPT_READY_TIMEOUT_MS = 100;
		prSplit.SEND_TEXT_NEWLINE_DELAY_MS = 0;
		prSplit.SEND_TEXT_CHUNK_DELAY_MS = 0;
		prSplit.SEND_PRE_SUBMIT_STABLE_TIMEOUT_MS = 100;
		prSplit.SEND_SUBMIT_ACK_TIMEOUT_MS = 100;

		globalThis.__mockHandle = {
			send: function(data) {},
			isAlive: function() { return true; },
			receive: function() { return ''; },
		};
	`)
	if err != nil {
		t.Fatal(err)
	}

	_, err = evalJS(`globalThis.__sendResult = await prSplit.sendToHandle(__mockHandle, 'test')`)
	if err != nil {
		t.Fatal(err)
	}

	val, err := evalJS(`JSON.stringify(globalThis.__sendResult)`)
	if err != nil {
		t.Fatal(err)
	}
	result, _ := val.(string)
	if strings.Contains(result, `"error":"`) {
		t.Errorf("sendToHandle failed without stateMachine: %s", result)
	}
}
