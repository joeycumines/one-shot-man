package command

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
	"github.com/joeycumines/one-shot-man/internal/scripting"
)

// The native leaf adapter completes on the event loop and returns that result
// on a later Tick. A click can therefore arrive after an empty-path move has
// completed, letting the interaction run before its new path has been visited.
// Retiring an interaction must also retire that path: otherwise the actor walks
// away after pickup and the next real key starts from the wrong position.
func TestPickAndPlace_CompletedInteractionRetiresNativePath(t *testing.T) {
	skipSlow(t)
	t.Setenv("OSM_TEST_MODE", "0")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	engine, err := scripting.NewEngine(ctx, io.Discard, io.Discard, t.Name(), "memory", nil, 0, slog.LevelInfo)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	engine.SetTestMode(true)
	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "example-05-pick-and-place.js"))
	if err != nil {
		t.Fatal(err)
	}
	engine.SetGlobal("__manualPathScript", scriptPath)
	evalJS := prsplittest.MakeEvalJS(t, engine, 5*time.Second)
	evaluate := func(source string) any {
		t.Helper()
		value, err := evalJS(source)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}

	// Import the actual example without starting its renderer or automatic
	// ticker. The production movement function establishes the same scene as
	// the PTY test; only the scheduling of manual Tick messages is controlled.
	evaluate(`
		var pickExample = require(__manualPathScript);
		var pickState = pickExample.initializeSimulation();
		var pickActor = pickState.actors.get(pickState.activeActorId);
		for (var step = 0; step < 4; step++) {
			if (!pickExample.manualKeysMovement(pickState, pickActor, 0, 1)) throw new Error('blocked fixture approach');
		}
		if (pickActor.x !== 5 || pickActor.y !== 15 || pickActor.heldItem !== null || pickState.spatialGrid.get(6, 16) !== 100) {
			throw new Error('unexpected pickup fixture');
		}
		function pickSnapshot() {
			return {x: pickActor.x, y: pickActor.y, held: pickActor.heldItem ? pickActor.heldItem.id : -1,
				target: pickState.manualMoveTarget, path: pickState.manualPath.length, stuck: pickState.pathStuckTicks};
		}
		pickExample.update(pickState, {type: 'Key', key: 'm'});
		pickExample.update(pickState, {type: 'Tick', id: 'tick'});
	`)

	// Each evaluation is a distinct FIFO event-loop submission. A native
	// leaf submitted by the previous evaluation (and its finalizing promise
	// microtasks) completes before this one starts; no sleeps or polling.
	evaluate(`
		pickState.pathStuckTicks = 3;
		pickExample.update(pickState, {type: 'MouseClick', button: 'left',
			x: 6 + Math.floor((pickState.width - pickState.spaceWidth) / 2) + 1, y: 16});
		pickExample.update(pickState, {type: 'Tick', id: 'tick'});
		if (pickActor.heldItem !== null || pickState.manualPath.length === 0) {
			throw new Error('test must admit interaction asynchronously with a pending click path');
		}
	`)
	evaluate(`
		var pickCompleted = pickSnapshot();
		if (pickCompleted.held !== 100) throw new Error('native interaction did not pick cube100');
		pickExample.update(pickState, {type: 'Tick', id: 'tick'});
	`)
	evaluate(`
		var pickAfterMove = pickSnapshot();
		pickExample.update(pickState, {type: 'Key', key: 'w'});
		pickExample.update(pickState, {type: 'Tick', id: 'tick'});
	`)
	value := evaluate(`JSON.stringify({completed: pickCompleted, afterMove: pickAfterMove, afterKey: pickSnapshot()})`)
	encoded, ok := value.(string)
	if !ok {
		t.Fatalf("unexpected observation type %T", value)
	}
	type snapshot struct {
		X, Y   float64
		Held   int
		Target any
		Path   int
		Stuck  int
	}
	var observed struct {
		Completed snapshot
		AfterMove snapshot
		AfterKey  snapshot
	}
	if err := json.Unmarshal([]byte(encoded), &observed); err != nil {
		t.Fatal(err)
	}
	if got := observed.Completed; got.Held != 100 || got.Target != nil || got.Path != 0 || got.Stuck != 0 {
		t.Errorf("completed pickup left movement queued: %+v", got)
	}
	if got := observed.AfterMove; got.X != 5 || got.Y != 15 || got.Held != 100 || got.Path != 0 {
		t.Errorf("post-pick move callback displaced the actor: %+v", got)
	}
	if got := observed.AfterKey; got.X != 5 || got.Y != 14 || got.Held != 100 || got.Target != nil || got.Path != 0 {
		t.Errorf("single up key did not preserve the completed pickup position: %+v", got)
	}
}

// Empty-handed empty-space clicks are movement requests even if the native
// sequence can enter its interaction leaf before the new path is traversed.
// The pickup completion fix must not retire such an unfinished walk.
func TestPickAndPlace_EmptyDestinationRetainsNativePath(t *testing.T) {
	skipSlow(t)
	t.Setenv("OSM_TEST_MODE", "0")
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	engine, err := scripting.NewEngine(ctx, io.Discard, io.Discard, t.Name(), "memory", nil, 0, slog.LevelInfo)
	if err != nil {
		t.Fatal(err)
	}
	defer engine.Close()
	engine.SetTestMode(true)
	scriptPath, err := filepath.Abs(filepath.Join("..", "..", "scripts", "example-05-pick-and-place.js"))
	if err != nil {
		t.Fatal(err)
	}
	engine.SetGlobal("__manualPathScript", scriptPath)
	evalJS := prsplittest.MakeEvalJS(t, engine, 5*time.Second)
	evaluate := func(source string) any {
		t.Helper()
		value, err := evalJS(source)
		if err != nil {
			t.Fatal(err)
		}
		return value
	}
	evaluate(`
		var walkExample = require(__manualPathScript);
		var walkState = walkExample.initializeSimulation();
		var walkActor = walkState.actors.get(walkState.activeActorId);
		function walkSnapshot() {
			return {x: walkActor.x, y: walkActor.y, held: walkActor.heldItem ? walkActor.heldItem.id : -1,
				target: walkState.manualMoveTarget, path: walkState.manualPath.length};
		}
		if (walkActor.x !== 5 || walkActor.y !== 11 || walkActor.heldItem !== null || walkState.spatialGrid.get(10, 11) !== undefined) {
			throw new Error('unexpected empty-destination fixture');
		}
		walkExample.update(walkState, {type: 'Key', key: 'm'});
		walkExample.update(walkState, {type: 'Tick', id: 'tick'});
	`)
	// The preceding FIFO submission completed an empty-path move. This click
	// must reach the actual async interaction before its new path is visited.
	evaluate(`
		walkExample.update(walkState, {type: 'MouseClick', button: 'left',
			x: 10 + Math.floor((walkState.width - walkState.spaceWidth) / 2) + 1, y: 11});
		walkExample.update(walkState, {type: 'Tick', id: 'tick'});
	`)
	evaluate(`var walkAfterInteraction = walkSnapshot();`)
	// Advance deterministic native callback turns, without a timer or a
	// condition-based retry. This covers every step and the final path pop.
	for range 24 {
		evaluate(`walkExample.update(walkState, {type: 'Tick', id: 'tick'}); void 0;`)
	}
	value := evaluate(`JSON.stringify({early: walkAfterInteraction, final: walkSnapshot()})`)
	encoded, ok := value.(string)
	if !ok {
		t.Fatalf("unexpected observation type %T", value)
	}
	type snapshot struct {
		X, Y   float64
		Held   int
		Target *struct{ X, Y int }
		Path   int
	}
	var observed struct {
		Early snapshot
		Final snapshot
	}
	if err := json.Unmarshal([]byte(encoded), &observed); err != nil {
		t.Fatal(err)
	}
	if got := observed.Early; got.X != 5 || got.Y != 11 || got.Held != -1 || got.Path == 0 || got.Target == nil || got.Target.X != 10 || got.Target.Y != 11 {
		t.Errorf("unfinished empty-space movement was retired: %+v", got)
	}
	if got := observed.Final; got.X != 10 || got.Y != 11 || got.Held != -1 || got.Path != 0 || got.Target != nil {
		t.Errorf("empty-space click did not reach its destination: %+v", got)
	}
}
