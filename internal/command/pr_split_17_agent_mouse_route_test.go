package command

// pr_split_17_agent_mouse_route_test.go — live agent-pane mouse routing.
//
// _routeMouseToAgentTermpane is the production seam between a mouse event
// inside the rendered agent pane and the child-terminal forwarding path.
// Its contract is deliberately mixed:
//
//   - decline (no live pane, or the pane's child session already exited)
//     returns a synchronous boolean false so the caller falls through to
//     writeMouseToPane and the input is not swallowed;
//   - accept returns a Promise (truthy) because the pane update is async,
//     and the caller short-circuits to avoid double-forwarding.
//
// A decline that leaked a Promise (or any other truthy value) silently
// disabled legacy child forwarding for the whole pane box, so both
// directions are asserted here.

import (
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

// runLiveMouseRouteProbe drives one MouseClick through update() with the live
// agent pane active and _routeMouseToAgentTermpane stubbed to return
// routeResult. It reports how often the live route was consulted and how
// many writes reached the child session.
//
// Observed counts today: a declined click consults the route twice (the 16e
// live block declines, then handleMouseClick re-routes) and still writes once;
// an accepted click consults it once and writes nothing. The assertions pin
// the user-visible half of that contract, so a regression that swallows or
// double-forwards the click fails regardless of how dispatch is layered.
const liveMouseRouteProbe = `
	function runLiveMouseRouteProbe(routeResult) {
		var writes = [];
		var routeCalls = 0;
		var s = initState('BRANCH_BUILDING');
		s.width = 80;
		s.height = 40;
		s.splitViewEnabled = true;
		s.splitViewTab = 'agent';
		s.splitViewFocus = 'agent';
		s.activeAgentSession = {
			write: function(b) { writes.push(String(b)); return true; }
		};
		var savedActive = prSplit._agentLiveActive;
		var savedInside = prSplit._isPointInAgentPane;
		var savedRoute = prSplit._routeMouseToAgentTermpane;
		var restoreZones = mockZoneHit('__no_zone__');
		prSplit._agentLiveActive = function() { return true; };
		prSplit._isPointInAgentPane = function() { return true; };
		prSplit._routeMouseToAgentTermpane = function() { routeCalls++; return routeResult; };
		try {
			update({type: 'MouseClick', button: 'left', x: 5, y: 30, mod: []}, s);
		} finally {
			restoreZones();
			prSplit._agentLiveActive = savedActive;
			prSplit._isPointInAgentPane = savedInside;
			prSplit._routeMouseToAgentTermpane = savedRoute;
		}
		return { routeCalls: routeCalls, writes: writes.length };
	}
`

// TestChunk17_LiveMouseRouteDeclineFallsThroughToChildForwarding asserts that
// a declined live route (synchronous false) still reaches the child terminal.
func TestChunk17_LiveMouseRouteDeclineFallsThroughToChildForwarding(t *testing.T) {
	t.Parallel()
	evalJS := prsplittest.NewTUIEngineWithHelpers(t)

	raw, err := evalJS(`(function() {` + liveMouseRouteProbe + `
		// Production decline value: a synchronous boolean.
		var declined = runLiveMouseRouteProbe(false);
		if (declined.routeCalls < 1) return 'FAIL: live route was never consulted';
		if (declined.writes !== 1) {
			return 'FAIL: declined route swallowed the click, child writes=' + declined.writes;
		}

		// Accept path: the async update is truthy, so the caller must
		// short-circuit instead of double-forwarding the same bytes.
		var accepted = runLiveMouseRouteProbe(Promise.resolve(true));
		if (accepted.routeCalls < 1) return 'FAIL: live route was never consulted';
		if (accepted.writes !== 0) {
			return 'FAIL: accepted route double-forwarded, child writes=' + accepted.writes;
		}
		return 'OK';
	})()`)
	if err != nil {
		t.Fatal(err)
	}
	if raw != "OK" {
		t.Errorf("live mouse route fall-through: %v", raw)
	}
}

// TestChunk17_LiveMouseRouteDeclineStaysSynchronous pins the producer half of
// the contract. The live pane state is private to the chunk closure, so the
// decline branches cannot be reached from a test engine; a source assertion
// is the only guard that keeps them synchronous. Both callers decide
// "handled?" with a plain truthiness check, so any Promise-wrapped decline
// makes every declined click look handled and silently disables legacy child
// forwarding for the whole pane box.
//
// The pins below target the two decline paths that actually gate input, and
// are written to survive reformatting: a refactor that hoists the decline into
// a helper may legitimately drop them, but a rewrap in a Promise must not.
func TestChunk17_LiveMouseRouteDeclineStaysSynchronous(t *testing.T) {
	t.Parallel()

	src, err := os.ReadFile("pr_split_17_agent_termpane.js")
	if err != nil {
		t.Fatalf("read chunk 17: %v", err)
	}
	body := string(src)
	start := strings.Index(body, "function routeMouseToAgentTermpane(msg)")
	end := strings.Index(body, "function isPointInAgentPane(")
	if start < 0 || end < 0 || end <= start {
		t.Fatal("routeMouseToAgentTermpane not found in chunk 17")
	}
	route := body[start:end]

	if strings.Contains(route, "Promise.resolve(false)") {
		t.Error("mouse route must decline with a synchronous boolean, not a resolved Promise")
	}
	inactiveDecline := regexp.MustCompile(`if \(!liveActive\(\)\)\s*return false;`)
	if !inactiveDecline.MatchString(route) {
		t.Error("inactive live pane must decline synchronously")
	}
	// The accept path stays asynchronous: the pane update is a Promise.
	if !strings.Contains(route, "live.pane.update(msg)") || !strings.Contains(route, "Promise.resolve(mouseUpdate)") {
		t.Error("accepted mouse route must forward the pane update Promise")
	}
}
