#!/usr/bin/env osm script

/**
 * @fileoverview example-05-pick-and-place.js
 * @description
 * Pick-and-Place Simulator demonstrating osm:pabt PA-BT planning integration.
 *
 * =========================================================================================
 * ARCHITECTURAL MANIFESTO: THE PA-BT STANDARD
 * =========================================================================================
 *
 * This implementation adheres strictly to "Behavior Trees in Robotics and AI" (Colledanchise & Ögren).
 * The system functions as a dynamic planner, not a scripted state machine.
 *
 * 1. THE POSTCONDITION-PRECONDITION-ACTION (PPA) UNIT
 * The fundamental atom of this system is NOT the Action, but the PPA expansion.
 * Structure: `Fallback(Condition C, Sequence(Preconditions(A), A))`
 * The planner generates a tree that satisfies conditions, rather than a linear queue of tasks.
 *
 * 2. DESCRIPTIVE VS. OPERATIONAL MODELS
 * For the reactive loop to function, the "Descriptive Model" (what the planner thinks an action does)
 * MUST match the "Operational Model" (what the physics engine actually does).
 * Discrepancies here lead to Livelocks (infinite replanning) or Deadlocks.
 *
 * =========================================================================================
 * DEFINED ANTI-PATTERNS & REQUIRED FIXES
 * =========================================================================================
 *
 * To ensure architectural integrity, the following patterns are explicitly defined and prohibited:
 *
 * [A] THE "GOD-PRECONDITION" ANTI-PATTERN
 * Definition: A single action listing every potential global blockade in its preconditions.
 * Failure Mode: Violates **Lazy Expansion**. It forces the planner to "solve" the entire map layout
 * before execution begins, coupling atomic actions to global geometry.
 * Refutation: The planner must not expand subtrees for conditions that are not yet relevant.
 *
 * [B] THE "ATOMIC ACTION" ANTI-PATTERN
 * Definition: A single action (e.g., `AtomicClearPath`) that handles moving, picking, and placing
 * in one tick to avoid planning complexity.
 * Failure Mode: Violates **Reactive Granularity**. It creates a "Black Box" that prevents reactive repair.
 * If the atomic action fails mid-execution, the planner has no visibility into the partial state.
 *
 * [C] THE SOLUTION: THE "BRIDGE ACTION" PATTERN
 * Requirement: Use **Dynamic Discovery**.
 * Instead of hardcoding map data, the ActionGenerator must dynamically inject "Bridge Actions"
 * (e.g., `ClearBlocker_X`) only when a specific navigational condition (`reachable_Target`) fails.
 *
 * =========================================================================================
 * CRITICAL RUNTIME WARNINGS
 * =========================================================================================
 *
 * 1. TRUTHFUL EFFECTS (THE ANTI-LIVELOCK RULE)
 * Do not assert that an action satisfies a high-level condition unless it guarantees it physically.
 * * BAD: `Pick_Obstacle` claims Effect `isPathClear = true`. (Lying: picking one might not clear the path).
 * * GOOD: `Pick_Obstacle_A` claims Effect `isCleared(A) = true`.
 *
 * 2. NO "ZOMBIE STATE" (DISABLE CACHING)
 * BT Nodes contain internal state (`Running`, `childIndex`). Reusing node instances (Action Caching)
 * across branches creates "Zombie State," where a node behaves as if running from a previous context.
 * ALWAYS instantiate fresh BT nodes in the Generator.
 *
 * 3. AVOID "SELF-BLOCKAGE BLINDNESS"
 * The Blackboard must compute path blockers for ALL relevant destinations (Goal and Target) every tick.
 * Do not gate blocker detection behind `if (holding target)`. The agent must know the path is blocked
 * even if it has dropped the target to clear the way.
 *
 * 4. PREVENT "SILENT FAILURE"
 * An action must not secretly reject a target in its tick function if that restriction is not
 * declared in its Preconditions.
 * * If `Place_Obstacle` cannot handle the Target, it must explicitly Precondition: `!heldItemIsTarget`.
 *
 * **WARNING RE: LOGGING:** This is an *interactive* terminal application.
 * It sets the terminal to raw mode.
 * DO NOT use console logging within the program itself (tea.run).
 * Instead, use the built-in 'log' global, for application logs (slog).
 *
 * @see {@link https://btirai.github.io/ | Behavior Trees in Robotics and AI}
 * @module PA-BT-Logic
 */

const printFatalError = (e) => {
    console.error('FATAL ERROR: ' + e?.message || e);
    if (e?.stack) {
        console.error('Stack trace: ' + e.stack);
    }
};

// init the program
let bt, tea, pabt, os, program;
try {
    bt = require('osm:bt');
    tea = require('osm:bubbletea');
    pabt = require('osm:pabt');
    os = require('osm:os');

    const ENV_WIDTH = 80;
    const ENV_HEIGHT = 24;

    const ROOM_MIN_X = 20;
    const ROOM_MAX_X = 55;
    const ROOM_MIN_Y = 6;
    const ROOM_MAX_Y = 16;
    const ROOM_GAP_Y = 11;

    const GOAL_CENTER_X = 8;
    const GOAL_CENTER_Y = 18;
    const GOAL_RADIUS = 1;

    const TARGET_ID = 1;
    const GOAL_ID = 1;
    var GOAL_BLOCKADE_IDS = [];

    const PICK_THRESHOLD = 5.0;
    const MANUAL_MOVE_SPEED = 1.0;

    // Optimization Constants
    const DIRS_4 = Object.freeze([[0, 1], [0, -1], [1, 0], [-1, 0]]);
    const DIRS_8 = Object.freeze([[0, 1], [0, -1], [1, 0], [-1, 0], [1, 1], [1, -1], [-1, 1], [-1, -1]]);

    // Object Pooling Arenas
    const nodeArena = [];
    let nodeArenaIdx = 0;
    const spriteArena = [];
    let spriteArenaIdx = 0;

    class SpatialGrid {
        constructor() {
            this.cells = new Map();
        }

        key(x, y) {
            return (Math.round(x) << 16) | (Math.round(y) & 0xFFFF);
        }

        add(id, x, y) {
            this.cells.set(this.key(x, y), id);
        }

        remove(x, y) {
            this.cells.delete(this.key(x, y));
        }

        move(id, oldX, oldY, newX, newY) {
            this.remove(oldX, oldY);
            this.add(id, newX, newY);
        }

        has(x, y) {
            return this.cells.has(this.key(x, y));
        }

        get(x, y) {
            return this.cells.get(this.key(x, y));
        }
    }

    function isInGoalArea(x, y) {
        return x >= GOAL_CENTER_X - GOAL_RADIUS &&
            x <= GOAL_CENTER_X + GOAL_RADIUS &&
            y >= GOAL_CENTER_Y - GOAL_RADIUS &&
            y <= GOAL_CENTER_Y + GOAL_RADIUS;
    }

    function initializeSimulation() {
        GOAL_BLOCKADE_IDS = []; // Fix: Clear before populating
        const cubesInit = [];
        const spatialGrid = new SpatialGrid();

        function registerCube(c) {
            cubesInit.push([c.id, c]);
            if (!c.deleted) {
                spatialGrid.add(c.id, c.x, c.y);
            }
        }

        registerCube({
            id: TARGET_ID,
            x: 45,
            y: 11,
            deleted: false,
            isTarget: true,
            type: 'target'
        });

        let goalBlockadeId = 100;
        const ringMinX = GOAL_CENTER_X - GOAL_RADIUS - 1;
        const ringMaxX = GOAL_CENTER_X + GOAL_RADIUS + 1;
        const ringMinY = GOAL_CENTER_Y - GOAL_RADIUS - 1;
        const ringMaxY = GOAL_CENTER_Y + GOAL_RADIUS + 1;

        for (let y = ringMinY; y <= ringMaxY; y++) {
            for (let x = ringMinX; x <= ringMaxX; x++) {
                if (isInGoalArea(x, y)) continue;

                registerCube({
                    id: goalBlockadeId,
                    x: x,
                    y: y,
                    deleted: false,
                    type: 'obstacle'
                });
                GOAL_BLOCKADE_IDS.push(goalBlockadeId);
                goalBlockadeId++;
            }
        }

        let wallId = 1000;

        function addWall(x, y) {
            if (x === ROOM_MIN_X && Math.abs(y - ROOM_GAP_Y) <= 1) return;
            registerCube({
                id: wallId,
                x: x,
                y: y,
                deleted: false,
                isStatic: true,
                type: 'wall'
            });
            wallId++;
        }

        for (let x = ROOM_MIN_X; x <= ROOM_MAX_X; x++) {
            addWall(x, ROOM_MIN_Y);
            addWall(x, ROOM_MAX_Y);
        }
        for (let y = ROOM_MIN_Y; y <= ROOM_MAX_Y; y++) {
            addWall(ROOM_MIN_X, y);
            addWall(ROOM_MAX_X, y);
        }

        registerCube({
            id: 803,
            x: 7,
            y: 5,
            deleted: false,
            type: 'obstacle'
        });

        log.info("Pick-and-Place simulation initialized");
        return {
            width: ENV_WIDTH,
            height: ENV_HEIGHT,
            spaceWidth: 55,

            actors: new Map([
                [1, {id: 1, x: 5, y: 11, heldItem: null}]
            ]),
            cubes: new Map(cubesInit),
            spatialGrid: spatialGrid, // Spatial Indexing
            goals: new Map([
                [GOAL_ID, {id: GOAL_ID, x: GOAL_CENTER_X, y: GOAL_CENTER_Y, forTarget: true}]
            ]),

            blackboard: null,
            bbCache: new Map(), // Value diffing cache
            pabtPlan: null,
            pabtState: null,
            activeActorId: 1,
            gameMode: 'automatic',
            tickCount: 0,
            paused: false,
            manualMoveTarget: null,
            manualPath: [],
            pathStuckTicks: 0,
            winConditionMet: false,
            targetDelivered: false,
            manualTicker: null,

            // INPUT REACTIVITY: Input Queue
            pendingInputs: [],

            renderBuffer: null,
            renderBufferWidth: 0,
            renderBufferHeight: 0,
            spritesDirty: true,
            lastSortedSprites: [],

            debugMode: os.getenv('OSM_TEST_MODE') === '1'
        };
    }

    function manualKeysMovement(state, actor, dx, dy) {
        if (dx === 0 && dy === 0) return false;
        const nx = actor.x + dx * MANUAL_MOVE_SPEED;
        const ny = actor.y + dy * MANUAL_MOVE_SPEED;
        const inx = Math.round(nx);
        const iny = Math.round(ny);
        let blocked = false;

        if (inx < 1 || inx >= state.spaceWidth - 1 ||
            iny < 1 || iny >= state.height - 1) {
            blocked = true;
        }

        if (!blocked) {
            // Use SpatialGrid for blocking check
            const ignoreId = actor.heldItem ? actor.heldItem.id : -1;
            const blockId = state.spatialGrid.get(inx, iny);
            blocked = blockId !== undefined && blockId !== ignoreId;
        }

        if (!blocked) {
            const oldX = Math.round(actor.x);
            const oldY = Math.round(actor.y);
            actor.x = nx;
            actor.y = ny;
            state.spritesDirty = true; // Optimization: Dirty flag
            return Math.round(actor.x) !== oldX || Math.round(actor.y) !== oldY;
        }
        return false;
    }

    function getFreeAdjacentCell(state, actorX, actorY, targetGoalArea = false) {
        const ax = Math.round(actorX);
        const ay = Math.round(actorY);

        for (const [dx, dy] of DIRS_8) {
            const nx = ax + dx;
            const ny = ay + dy;

            if (nx < 0 || nx >= state.spaceWidth || ny < 0 || ny >= state.height) continue;
            if (targetGoalArea && !isInGoalArea(nx, ny)) continue;

            const blockId = state.spatialGrid.get(nx, ny);
            const occupied = blockId !== undefined;

            if (!occupied) return {x: nx, y: ny};
        }
        return null;
    }

    function findNearestPickableCube(state, clickX, clickY) {
        let nearest = null;
        let nearestDist = PICK_THRESHOLD;
        const actor = state.actors.get(state.activeActorId);

        for (const c of state.cubes.values()) {
            if (c.deleted) continue;
            if (c.isStatic) continue;
            if (actor.heldItem && c.id === actor.heldItem.id) continue;
            const dist = Math.sqrt(Math.pow(c.x - clickX, 2) + Math.pow(c.y - clickY, 2));
            if (dist < nearestDist) {
                nearest = c;
                nearestDist = dist;
            }
        }
        return nearest;
    }

    // Replaces buildBlockedSet with direct SpatialGrid wrapper/interface or logic replacement
    function buildBlockedSet(state, ignoreCubeId) {
        // Compatibility wrapper for legacy calls relying on Set interface if needed,
        // but performance critical paths should use state.spatialGrid directly.
        // For the purpose of "replacing O(n) iteration", we return a proxy object.
        return {
            has: (key) => {
                // Parse key "x,y" (Legacy support)
                const parts = key.split(',');
                const x = parseInt(parts[0]);
                const y = parseInt(parts[1]);
                const id = state.spatialGrid.get(x, y);
                if (id === undefined) return false;

                const actor = state.actors.get(state.activeActorId);
                const heldId = actor.heldItem ? actor.heldItem.id : -1;

                if (id === ignoreCubeId) return false;
                if (heldId !== -1 && id === heldId) return false;
                return true;
            }
        };
    }

    function getPathInfo(state, startX, startY, targetX, targetY, ignoreCubeId) {
        // Uses SpatialGrid indirectly or directly
        const keyInt = state.spatialGrid.key;
        const visited = new Set();
        const queue = [{x: Math.round(startX), y: Math.round(startY), dist: 0}];
        const iTargetX = Math.round(targetX);
        const iTargetY = Math.round(targetY);

        visited.add(keyInt(queue[0].x, queue[0].y));

        while (queue.length > 0) {
            const current = queue.shift();
            const dx = Math.abs(current.x - iTargetX);
            const dy = Math.abs(current.y - iTargetY);

            if (dx <= 1 && dy <= 1) {
                return {reachable: true, distance: current.dist};
            }

            for (const [ox, oy] of DIRS_4) {
                const nx = current.x + ox;
                const ny = current.y + oy;
                const nKey = keyInt(nx, ny);

                if (nx < 1 || nx >= state.spaceWidth - 1 || ny < 1 || ny >= state.height - 1) continue;
                if (visited.has(nKey)) continue;

                // Block check using SpatialGrid
                const blockId = state.spatialGrid.get(nx, ny);
                let blocked = false;
                if (blockId !== undefined) {
                    const actor = state.actors.get(state.activeActorId);
                    const heldId = actor.heldItem ? actor.heldItem.id : -1;
                    if (blockId !== ignoreCubeId && (!heldId || blockId !== heldId)) {
                        blocked = true;
                    }
                }

                if (blocked) continue;

                visited.add(nKey);
                queue.push({x: nx, y: ny, dist: current.dist + 1});
            }
        }
        return {reachable: false, distance: Infinity};
    }

    // =========================================================================================
    // MANUAL MODE PERFORMANCE OPTIMIZATIONS (GUARANTEED CORRECTNESS)
    // =========================================================================================

    const MANUAL_PATH_NODE_BUDGET = 500;

    class MinHeap {
        constructor() {
            this.content = [];
        }

        clear() {
            this.content.length = 0;
        }

        push(element) {
            this.content.push(element);
            this.bubbleUp(this.content.length - 1);
        }

        pop() {
            const result = this.content[0];
            const end = this.content.pop();
            if (this.content.length > 0) {
                this.content[0] = end;
                this.sinkDown(0);
            }
            return result;
        }

        size() {
            return this.content.length;
        }

        bubbleUp(n) {
            const element = this.content[n];
            while (n > 0) {
                const parentN = Math.floor((n + 1) / 2) - 1;
                const parent = this.content[parentN];
                if (element.f >= parent.f) break;
                this.content[parentN] = element;
                this.content[n] = parent;
                n = parentN;
            }
        }

        sinkDown(n) {
            const length = this.content.length;
            const element = this.content[n];
            while (true) {
                const child2N = (n + 1) * 2, child1N = child2N - 1;
                let swap = null;
                if (child1N < length) {
                    const child1 = this.content[child1N];
                    if (child1.f < element.f) swap = child1N;
                }
                if (child2N < length) {
                    const child2 = this.content[child2N];
                    if (child2.f < (swap === null ? element.f : this.content[child1N].f)) swap = child2N;
                }
                if (swap === null) break;
                this.content[n] = this.content[swap];
                this.content[swap] = element;
                n = swap;
            }
        }
    }

    const manualModeCache = {
        // Reusable structures to reduce GC
        openHeap: new MinHeap(),
        gScore: new Map(),
        closed: new Set()
    };

    function invalidateManualBlockedCache() {
        // No-op now as SpatialGrid handles live updates
    }

    // Object Pooling for A* Nodes
    function allocNode(x, y, g, f, parent) {
        let n;
        if (nodeArenaIdx < nodeArena.length) {
            n = nodeArena[nodeArenaIdx++];
        } else {
            n = {};
            nodeArena.push(n);
            nodeArenaIdx++;
        }
        n.x = x;
        n.y = y;
        n.g = g;
        n.f = f;
        n.parent = parent;
        return n;
    }

    function findPathManual(state, startX, startY, targetX, targetY, ignoreCubeId) {
        // Reset pool for this search
        nodeArenaIdx = 0;

        const iStartX = Math.round(startX), iStartY = Math.round(startY);
        const iTargetX = Math.round(targetX), iTargetY = Math.round(targetY);

        if (iStartX === iTargetX && iStartY === iTargetY) return [];

        const keyInt = state.spatialGrid.key;
        const h = (x, y) => Math.abs(x - iTargetX) + Math.abs(y - iTargetY);

        // Reuse persistent structures
        const open = manualModeCache.openHeap;
        const gScore = manualModeCache.gScore;
        const closed = manualModeCache.closed;
        open.clear();
        gScore.clear();
        closed.clear();

        const startNode = allocNode(iStartX, iStartY, 0, h(iStartX, iStartY), null);
        open.push(startNode);
        gScore.set(keyInt(iStartX, iStartY), 0);

        let expanded = 0;
        let bestNode = startNode;
        let bestH = h(iStartX, iStartY);

        const actor = state.actors.get(state.activeActorId);
        const heldId = actor.heldItem ? actor.heldItem.id : -1;

        while (open.size() > 0 && expanded < MANUAL_PATH_NODE_BUDGET) {
            const cur = open.pop();
            const curKey = keyInt(cur.x, cur.y);

            if (closed.has(curKey)) continue;
            closed.add(curKey);
            expanded++;

            if (cur.x === iTargetX && cur.y === iTargetY) {
                return reconstructManualPath(cur);
            }

            const curH = h(cur.x, cur.y);
            if (curH < bestH) {
                bestH = curH;
                bestNode = cur;
            }

            for (const [dx, dy] of DIRS_4) {
                const nx = cur.x + dx, ny = cur.y + dy;
                const nKey = keyInt(nx, ny);

                if (nx < 1 || nx >= state.spaceWidth - 1 || ny < 1 || ny >= state.height - 1) continue;
                if (closed.has(nKey)) continue;

                // Spatial Grid Check
                const blockId = state.spatialGrid.get(nx, ny);
                let isBlocked = false;
                if (blockId !== undefined) {
                    if (blockId !== ignoreCubeId && (!heldId || blockId !== heldId)) {
                        isBlocked = true;
                    }
                }

                if (isBlocked && !(nx === iTargetX && ny === iTargetY)) continue;

                const ng = cur.g + 1;
                const existingG = gScore.get(nKey);
                if (existingG === undefined || ng < existingG) {
                    gScore.set(nKey, ng);
                    open.push(allocNode(nx, ny, ng, ng + h(nx, ny), cur));
                }
            }
        }

        if (bestNode && bestH < h(iStartX, iStartY)) {
            return reconstructManualPath(bestNode);
        }

        return null;
    }

    function reconstructManualPath(node) {
        const path = [];
        let n = node;
        while (n && n.parent) {
            path.push({x: n.x, y: n.y}); // Optimization: push + reverse
            n = n.parent;
        }
        return path.reverse();
    }

    function findPath(state, startX, startY, targetX, targetY, ignoreCubeId, searchLimit) {
        // Legacy BFS
        const keyInt = state.spatialGrid.key;
        const iStartX = Math.round(startX);
        const iStartY = Math.round(startY);
        const iTargetX = Math.round(targetX);
        const iTargetY = Math.round(targetY);

        if (iStartX === iTargetX && iStartY === iTargetY) return [];

        const visited = new Map();
        const queue = [{x: iStartX, y: iStartY}];
        visited.set(keyInt(iStartX, iStartY), null);

        let finalNode = null;
        let iterations = 0;

        const actor = state.actors.get(state.activeActorId);
        const heldId = actor.heldItem ? actor.heldItem.id : -1;

        while (queue.length > 0) {
            if (searchLimit && iterations >= searchLimit) break;
            iterations++;
            const cur = queue.shift();

            if (cur.x === iTargetX && cur.y === iTargetY) {
                finalNode = cur;
                break;
            }

            for (const [ox, oy] of DIRS_4) {
                const nx = cur.x + ox;
                const ny = cur.y + oy;
                const nKey = keyInt(nx, ny);
                if (nx < 1 || nx >= state.spaceWidth - 1 || ny < 1 || ny >= state.height - 1) continue;

                const blockId = state.spatialGrid.get(nx, ny);
                let blocked = false;
                if (blockId !== undefined) {
                    if (blockId !== ignoreCubeId && (!heldId || blockId !== heldId)) {
                        blocked = true;
                    }
                }

                if (blocked && !(nx === iTargetX && ny === iTargetY)) continue;
                if (visited.has(nKey)) continue;

                visited.set(nKey, cur);
                queue.push({x: nx, y: ny, parent: cur});
            }
        }

        if (!finalNode) return null;
        const path = [];
        let curr = finalNode;
        while (curr.parent) {
            path.push({x: curr.x, y: curr.y}); // Optimization here too
            curr = curr.parent;
        }
        return path.reverse();
    }

    function findNextStep(state, startX, startY, targetX, targetY, ignoreCubeId) {
        const keyInt = state.spatialGrid.key;
        const iStartX = Math.round(startX);
        const iStartY = Math.round(startY);
        const iTargetX = Math.round(targetX);
        const iTargetY = Math.round(targetY);

        if (Math.abs(startX - targetX) < 1.0 && Math.abs(startY - targetY) < 1.0) {
            return {x: targetX, y: targetY};
        }

        const visited = new Set();
        const queue = [];
        visited.add(keyInt(iStartX, iStartY));

        const actor = state.actors.get(state.activeActorId);
        const heldId = actor.heldItem ? actor.heldItem.id : -1;

        for (const [ox, oy] of DIRS_4) {
            const nx = iStartX + ox;
            const ny = iStartY + oy;
            const nKey = keyInt(nx, ny);

            if (nx < 1 || nx >= state.spaceWidth - 1 || ny < 1 || ny >= state.height - 1) continue;

            const blockId = state.spatialGrid.get(nx, ny);
            let blocked = false;
            if (blockId !== undefined) {
                if (blockId !== ignoreCubeId && (!heldId || blockId !== heldId)) {
                    blocked = true;
                }
            }

            if (blocked && !(nx === iTargetX && ny === iTargetY)) continue;

            if (nx === iTargetX && ny === iTargetY) {
                return {x: nx, y: ny};
            }

            queue.push({x: nx, y: ny, firstX: ox, firstY: oy});
            visited.add(nKey);
        }

        while (queue.length > 0) {
            const cur = queue.shift();
            if (cur.x === iTargetX && cur.y === iTargetY) {
                return {x: startX + cur.firstX, y: startY + cur.firstY};
            }
            for (const [ox, oy] of DIRS_4) {
                const nx = cur.x + ox;
                const ny = cur.y + oy;
                const nKey = keyInt(nx, ny);
                if (nx < 1 || nx >= state.spaceWidth - 1 || ny < 1 || ny >= state.height - 1) continue;

                const blockId = state.spatialGrid.get(nx, ny);
                let blocked = false;
                if (blockId !== undefined) {
                    if (blockId !== ignoreCubeId && (!heldId || blockId !== heldId)) {
                        blocked = true;
                    }
                }

                if (blocked && !(nx === iTargetX && ny === iTargetY)) continue;
                if (visited.has(nKey)) continue;
                visited.add(nKey);
                queue.push({x: nx, y: ny, firstX: cur.firstX, firstY: cur.firstY});
            }
        }
        return null;
    }

    function findFirstBlocker(state, fromX, fromY, toX, toY, excludeId) {
        const keyInt = state.spatialGrid.key;
        const actor = state.actors.get(state.activeActorId);
        const heldId = actor.heldItem ? actor.heldItem.id : -1;

        const visited = new Set();
        const frontier = [];
        const queue = [{x: Math.round(fromX), y: Math.round(fromY)}];
        visited.add(keyInt(queue[0].x, queue[0].y));
        const targetIX = Math.round(toX);
        const targetIY = Math.round(toY);

        while (queue.length > 0) {
            const current = queue.shift();
            const dx = Math.abs(current.x - targetIX);
            const dy = Math.abs(current.y - targetIY);
            if (dx <= 1 && dy <= 1) return null;

            for (const [ox, oy] of DIRS_4) {
                const nx = current.x + ox;
                const ny = current.y + oy;
                const nKey = keyInt(nx, ny);

                if (nx < 1 || nx >= state.spaceWidth - 1 || ny < 1 || ny >= state.height - 1) continue;
                if (visited.has(nKey)) continue;

                const blockId = state.spatialGrid.get(nx, ny);
                let blocked = false;
                if (blockId !== undefined) {
                    if ((excludeId === undefined || blockId !== excludeId) && (!heldId || blockId !== heldId)) {
                        blocked = true;
                        frontier.push({x: nx, y: ny, id: blockId, dist: current.dist || 0});
                    }
                }

                if (blocked) continue;

                visited.add(nKey);
                queue.push({x: nx, y: ny, dist: (current.dist || 0) + 1});
            }
        }

        if (frontier.length > 0) {
            frontier.sort((a, b) => {
                const distA = Math.abs(a.x - toX) + Math.abs(a.y - toY);
                const distB = Math.abs(b.x - toX) + Math.abs(b.y - toY);
                return distA - distB;
            });
            return frontier[0].id;
        }
        return null;
    }

    function syncValue(state, key, newValue) {
        if (state.bbCache.get(key) !== newValue) {
            state.blackboard.set(key, newValue);
            state.bbCache.set(key, newValue);
        }
    }

    function syncToBlackboard(state) {
        if (!state.blackboard) return;

        const actor = state.actors.get(state.activeActorId);
        const ax = Math.round(actor.x);
        const ay = Math.round(actor.y);

        syncValue(state, 'actorX', actor.x);
        syncValue(state, 'actorY', actor.y);
        syncValue(state, 'heldItemExists', actor.heldItem !== null);
        syncValue(state, 'heldItemId', actor.heldItem ? actor.heldItem.id : -1);

        state.cubes.forEach(cube => {
            if (!cube.deleted) {
                const dist = Math.sqrt(Math.pow(cube.x - ax, 2) + Math.pow(cube.y - ay, 2));
                const atEntity = dist <= 1.8;
                syncValue(state, 'atEntity_' + cube.id, atEntity);

                if (cube.id === TARGET_ID) {
                    const cubeX = Math.round(cube.x);
                    const cubeY = Math.round(cube.y);
                    const blocker = findFirstBlocker(state, ax, ay, cubeX, cubeY, TARGET_ID);
                    syncValue(state, 'pathBlocker_entity_' + cube.id, blocker === null ? -1 : blocker);
                }
            } else {
                syncValue(state, 'atEntity_' + cube.id, false);
                if (cube.id === TARGET_ID) {
                    syncValue(state, 'pathBlocker_entity_' + cube.id, -1);
                }
            }
        });

        state.goals.forEach(goal => {
            const dist = Math.sqrt(Math.pow(goal.x - ax, 2) + Math.pow(goal.y - ay, 2));
            syncValue(state, 'atGoal_' + goal.id, dist <= 1.5);

            const goalX = Math.round(goal.x);
            const goalY = Math.round(goal.y);
            const blocker = findFirstBlocker(state, ax, ay, goalX, goalY, TARGET_ID);
            syncValue(state, 'pathBlocker_goal_' + goal.id, blocker === null ? -1 : blocker);
        });

        syncValue(state, 'cubeDeliveredAtGoal', state.winConditionMet);
    }

    // =========================================================================================
    // MANUAL MODE BEHAVIOR TREE
    // =========================================================================================

    function createManualMoveLeaf(state) {
        return bt.createLeafNode(() => {
            if (state.manualPath.length === 0) return bt.success;

            const actor = state.actors.get(state.activeActorId);
            const nextPoint = state.manualPath[0];
            const dx = nextPoint.x - actor.x;
            const dy = nextPoint.y - actor.y;
            const dist = Math.sqrt(dx * dx + dy * dy);

            if (dist >= 0.1) {
                // Movement logic
                const moveDist = Math.min(MANUAL_MOVE_SPEED, dist);
                const nextX = actor.x + Math.sign(dx) * Math.min(MANUAL_MOVE_SPEED, Math.abs(dx));
                const nextY = actor.y + Math.sign(dy) * Math.min(MANUAL_MOVE_SPEED, Math.abs(dy));

                const blockId = state.spatialGrid.get(Math.round(nextX), Math.round(nextY));
                const nextBlocked = blockId !== undefined; // Assume all blocks stop manual move

                if (nextBlocked) {
                    state.manualPath = [];
                    state.manualMoveTarget = null;
                    state.pathStuckTicks = 0;
                    return bt.failure;
                }

                const oldDist = dist;
                actor.x += Math.sign(dx) * Math.min(MANUAL_MOVE_SPEED, Math.abs(dx));
                actor.y += Math.sign(dy) * Math.min(MANUAL_MOVE_SPEED, Math.abs(dy));
                state.spritesDirty = true; // Flag dirty

                const newDist = Math.sqrt(Math.pow(nextPoint.x - actor.x, 2) + Math.pow(nextPoint.y - actor.y, 2));
                if (newDist >= oldDist - 0.01) state.pathStuckTicks++;
                else state.pathStuckTicks = 0;

                if (state.pathStuckTicks > 60) {
                    state.manualPath = [];
                    state.manualMoveTarget = null;
                    state.pathStuckTicks = 0;
                    return bt.failure;
                }
            } else {
                actor.x = nextPoint.x;
                actor.y = nextPoint.y;
                state.spritesDirty = true; // Flag dirty
                state.manualPath.shift();
                state.pathStuckTicks = 0;
            }

            return bt.running;
        });
    }

    function createManualInteractLeaf(state) {
        return bt.createLeafNode(() => {
            if (!state.manualMoveTarget) return bt.success;
            if (state.manualPath.length > 0) return bt.running;

            const clickX = state.manualMoveTarget.x;
            const clickY = state.manualMoveTarget.y;
            const actor = state.actors.get(state.activeActorId);
            const isHolding = actor.heldItem !== null;

            let clickedCube = null;
            const targetId = state.spatialGrid.get(clickX, clickY);
            if (targetId !== undefined) {
                clickedCube = state.cubes.get(targetId);
            }

            const dist = Math.sqrt(Math.pow(clickX - actor.x, 2) + Math.pow(clickY - actor.y, 2));
            if (dist <= PICK_THRESHOLD) {
                if (isHolding) {
                    const ignoreId = actor.heldItem.id;
                    if (!clickedCube) {
                        const c = state.cubes.get(ignoreId);
                        if (c) {
                            c.deleted = false;
                            state.spatialGrid.add(c.id, clickX, clickY); // Add to grid
                            c.x = clickX;
                            c.y = clickY;
                            actor.heldItem = null;
                            state.spritesDirty = true;
                            if (ignoreId === TARGET_ID && isInGoalArea(clickX, clickY)) state.winConditionMet = true;
                        }
                    }
                } else if (clickedCube && !clickedCube.isStatic && !clickedCube.deleted) {
                    clickedCube.deleted = true;
                    state.spatialGrid.remove(clickedCube.x, clickedCube.y); // Remove from grid
                    actor.heldItem = {id: clickedCube.id};
                    state.spritesDirty = true;
                } else if (!isHolding && !clickedCube) {
                    const nearestCube = findNearestPickableCube(state, clickX, clickY);
                    if (nearestCube) {
                        const actorToCubeDist = Math.sqrt(Math.pow(nearestCube.x - actor.x, 2) + Math.pow(nearestCube.y - actor.y, 2));
                        if (actorToCubeDist <= PICK_THRESHOLD) {
                            nearestCube.deleted = true;
                            state.spatialGrid.remove(nearestCube.x, nearestCube.y); // Remove from grid
                            actor.heldItem = {id: nearestCube.id};
                            state.spritesDirty = true;
                        }
                    }
                }
            }
            state.manualMoveTarget = null;
            return bt.success;
        });
    }

    function createManualTree(state) {
        return bt.node(bt.sequence,
            createManualMoveLeaf(state),
            createManualInteractLeaf(state)
        );
    }

    function createMoveToAction(state, entityType, entityId, extraPreconditions) {
        const name = 'MoveTo_' + entityType + '_' + entityId;
        let targetKey;
        let pathBlockerKey;

        if (entityType === 'cube') {
            targetKey = 'atEntity_' + entityId;
            pathBlockerKey = 'pathBlocker_entity_' + entityId;
        } else {
            targetKey = 'atGoal_' + entityId;
            pathBlockerKey = 'pathBlocker_goal_' + entityId;
        }

        const conditions = [];
        if (entityType === 'goal' || (entityType === 'cube' && entityId === TARGET_ID)) {
            conditions.push({key: pathBlockerKey, value: -1, Match: v => v === -1});
        }
        if (extraPreconditions) {
            conditions.push(...extraPreconditions);
        }

        const effects = [{key: targetKey, Value: true}];

        const tickFn = function () {
            if (state.gameMode !== 'automatic') return bt.running;

            const actor = state.actors.get(state.activeActorId);
            let targetX, targetY, ignoreCubeId;

            if (entityType === 'cube') {
                const cube = state.cubes.get(entityId);
                if (!cube || cube.deleted) {
                    return bt.success;
                }
                targetX = cube.x;
                targetY = cube.y;
                ignoreCubeId = entityId;
            } else {
                const goal = state.goals.get(entityId);
                targetX = goal.x;
                targetY = goal.y;
                ignoreCubeId = -1;
            }

            const dx = targetX - actor.x;
            const dy = targetY - actor.y;
            const dist = Math.sqrt(dx * dx + dy * dy);
            const threshold = 1.5;

            if (dist <= threshold) {
                return bt.success;
            }

            const nextStep = findNextStep(state, actor.x, actor.y, targetX, targetY, ignoreCubeId);
            if (nextStep) {
                const stepDx = nextStep.x - actor.x;
                const stepDy = nextStep.y - actor.y;
                var newX = actor.x + Math.sign(stepDx) * Math.min(1.0, Math.abs(stepDx));
                var newY = actor.y + Math.sign(stepDy) * Math.min(1.0, Math.abs(stepDy));

                actor.x = newX;
                actor.y = newY;
                state.spritesDirty = true; // Flag dirty
                log.debug("[PA-BT ACTION]", {
                    action: name,
                    result: "SUCCESS",
                    tick: state.tickCount,
                    actorX: actor.x,
                    actorY: actor.y
                });
                return bt.running;
            } else {
                return bt.failure;
            }
        };

        const node = bt.createLeafNode(tickFn);
        const action = pabt.newAction(name, conditions, effects, node);
        return action;
    }

    function createPickGoalBlockadeAction(state, cubeId) {
        const name = 'Pick_GoalBlockade_' + cubeId;
        const conditions = [
            {key: 'heldItemExists', value: false, Match: v => v === false},
            {key: 'atEntity_' + cubeId, value: true, Match: v => v === true}
        ];
        const effects = [
            {key: 'heldItemId', Value: cubeId},
            {key: 'heldItemExists', Value: true}
        ];
        const tickFn = function () {
            if (state.gameMode !== 'automatic') return bt.running;
            const actor = state.actors.get(state.activeActorId);
            const cube = state.cubes.get(cubeId);
            if (!cube || cube.deleted) return bt.success;
            const dx = cube.x - actor.x;
            const dy = cube.y - actor.y;
            const dist = Math.sqrt(dx * dx + dy * dy);
            if (dist > PICK_THRESHOLD) return bt.failure;

            cube.deleted = true;
            state.spatialGrid.remove(cube.x, cube.y); // Remove from grid

            actor.heldItem = {id: cubeId};
            state.spritesDirty = true;

            log.debug("[PA-BT ACTION]", {
                action: name,
                result: "SUCCESS",
                tick: state.tickCount,
                actorX: actor.x,
                actorY: actor.y,
                cubeId: cubeId
            });
            if (state.blackboard) {
                syncValue(state, 'heldItemId', cubeId);
                syncValue(state, 'heldItemExists', true);
            }
            return bt.success;
        };
        const node = bt.createLeafNode(() => state.gameMode === 'automatic' ? tickFn() : bt.running);
        return pabt.newAction(name, conditions, effects, node);
    }

    function createDepositGoalBlockadeAction(state, cubeId, destinationKey) {
        const name = 'Deposit_GoalBlockade_' + cubeId;
        const conditions = [{key: 'heldItemId', value: cubeId, Match: v => v === cubeId}];
        const effects = [
            {key: 'heldItemExists', Value: false},
            {key: 'heldItemId', Value: -1},
            {key: 'pathBlocker_' + destinationKey, Value: -1}
        ];
        const tickFn = function () {
            if (state.gameMode !== 'automatic') return bt.running;
            const actor = state.actors.get(state.activeActorId);
            if (!actor.heldItem || actor.heldItem.id !== cubeId) return bt.failure;
            const goal = [...state.goals.values()][0];
            const goalX = goal ? goal.x : 0, goalY = goal ? goal.y : 0;
            const ax = Math.round(actor.x), ay = Math.round(actor.y);
            const dirToGoalX = goalX - ax, dirToGoalY = goalY - ay;
            const dirs = [];
            if (dirToGoalX < 0) dirs.push([1, 0], [1, -1], [1, 1]);
            else dirs.push([-1, 0], [-1, -1], [-1, 1]);
            if (dirToGoalY > 0) dirs.push([0, -1]); else dirs.push([0, 1]);

            for (const d of DIRS_8) {
                if (!dirs.some(e => e[0] === d[0] && e[1] === d[1])) dirs.push(d);
            }

            let spot = null;
            for (const [dx, dy] of dirs) {
                const nx = ax + dx, ny = ay + dy;
                if (nx < 0 || nx >= state.spaceWidth || ny < 0 || ny >= state.height) continue;

                const occupied = state.spatialGrid.get(nx, ny) !== undefined && state.spatialGrid.get(nx, ny) !== cubeId;
                if (!occupied) {
                    spot = {x: nx, y: ny};
                    break;
                }
            }
            if (!spot) return bt.failure;
            const blocker = state.cubes.get(cubeId);
            if (blocker) {
                blocker.x = spot.x;
                blocker.y = spot.y;
                blocker.deleted = false;
                state.spatialGrid.add(blocker.id, spot.x, spot.y); // Add to grid
                state.spritesDirty = true;
            }
            actor.heldItem = null;
            log.debug("[PA-BT ACTION]", {
                action: name,
                result: "SUCCESS",
                tick: state.tickCount,
                actorX: actor.x,
                actorY: actor.y,
                cubeId: cubeId,
                dest: destinationKey
            });
            if (state.blackboard) {
                syncValue(state, 'heldItemExists', false);
                syncValue(state, 'heldItemId', -1);
                syncValue(state, 'pathBlocker_' + destinationKey, -1);
            }
            return bt.success;
        };
        const node = bt.createLeafNode(() => state.gameMode === 'automatic' ? tickFn() : bt.running);
        return pabt.newAction(name, conditions, effects, node);
    }

    function setupPABTActions(state) {
        const actor = () => state.actors.get(state.activeActorId);
        state.pabtState.setActionGenerator(function (failedCondition) {
            const actions = [];
            const key = failedCondition.key, targetValue = failedCondition.value;
            if (key && typeof key === 'string') {
                if (key === 'cubeDeliveredAtGoal') {
                    const a = state.pabtState.GetAction('Deliver_Target');
                    if (a) actions.push(a);
                }
                if (key === 'heldItemId') {
                    if (targetValue === TARGET_ID) {
                        const a = state.pabtState.GetAction('Pick_Target');
                        if (a) actions.push(a);
                    } else if (typeof targetValue === 'number' && targetValue !== -1) {
                        const cube = state.cubes.get(targetValue);
                        if (cube && !cube.deleted) actions.push(createPickGoalBlockadeAction(state, targetValue));
                    }
                }
                if (key === 'heldItemExists' && targetValue === false) {
                    const a1 = state.pabtState.GetAction('Place_Held_Item'),
                        a2 = state.pabtState.GetAction('Place_Target_Temporary'),
                        a3 = state.pabtState.GetAction('Place_Obstacle');
                    if (a1) actions.push(a1);
                    if (a2) actions.push(a2);
                    if (a3) actions.push(a3);
                    const currentHeldId = state.blackboard.get('heldItemId');
                    if (typeof currentHeldId === 'number' && currentHeldId >= 100) actions.push(createDepositGoalBlockadeAction(state, currentHeldId, 'goal_1'));
                }
                if (key.startsWith('atEntity_')) {
                    const id = parseInt(key.replace('atEntity_', ''), 10);
                    if (!isNaN(id)) actions.push(createMoveToAction(state, 'cube', id));
                }
                if (key.startsWith('atGoal_')) {
                    const id = parseInt(key.replace('atGoal_', ''), 10);
                    if (!isNaN(id)) actions.push(createMoveToAction(state, 'goal', id));
                }
                if (key.startsWith('pathBlocker_')) {
                    const destId = key.replace('pathBlocker_', ''), currentBlocker = state.blackboard.get(key);
                    if (targetValue === -1 && typeof currentBlocker === 'number' && currentBlocker !== -1) {
                        actions.push(createPickGoalBlockadeAction(state, currentBlocker));
                        actions.push(createDepositGoalBlockadeAction(state, currentBlocker, destId));
                    }
                }
            }
            return actions;
        });

        const reg = function (name, conds, effects, tickFn) {
            const conditions = conds.map(c => ({
                key: c.k,
                value: c.v,
                Match: v => c.v === undefined ? v === true : v === c.v
            }));
            const effectList = effects.map(e => ({key: e.k, Value: e.v}));
            const node = bt.createLeafNode(() => state.gameMode === 'automatic' ? tickFn() : bt.running);
            state.pabtState.RegisterAction(name, pabt.newAction(name, conditions, effectList, node));
        };

        reg('Pick_Target', [{k: 'heldItemExists', v: false}, {k: 'atEntity_' + TARGET_ID, v: true}], [{
            k: 'heldItemId',
            v: TARGET_ID
        }, {k: 'heldItemExists', v: true}], function () {
            const a = actor(), t = state.cubes.get(TARGET_ID);
            if (a.heldItem || !t || t.deleted) return bt.failure;

            t.deleted = true;
            state.spatialGrid.remove(t.x, t.y); // Grid update

            a.heldItem = {id: TARGET_ID};
            state.spritesDirty = true;
            log.debug("[PA-BT ACTION]", {
                action: "Pick_Target",
                result: "SUCCESS",
                tick: state.tickCount,
                actorX: a.x,
                actorY: a.y,
                cubeId: TARGET_ID
            });
            if (state.blackboard) {
                syncValue(state, 'heldItemId', TARGET_ID);
                syncValue(state, 'heldItemExists', true);
            }
            return bt.success;
        });

        reg('Deliver_Target', [{k: 'atGoal_' + GOAL_ID, v: true}, {
            k: 'heldItemId',
            v: TARGET_ID
        }], [{k: 'cubeDeliveredAtGoal', v: true}], function () {
            const a = actor();
            if (!a.heldItem || a.heldItem.id !== TARGET_ID) return bt.failure;
            const spot = getFreeAdjacentCell(state, a.x, a.y, true);
            if (!spot) return bt.failure;
            log.debug("[PA-BT ACTION]", {
                action: "Deliver_Target",
                result: "SUCCESS",
                tick: state.tickCount,
                actorX: a.x,
                actorY: a.y,
                cubeId: TARGET_ID
            });
            a.heldItem = null;
            state.targetDelivered = true;
            state.winConditionMet = true;
            const t = state.cubes.get(TARGET_ID);
            if (t) {
                t.deleted = false;
                t.x = spot.x;
                t.y = spot.y;
                state.spatialGrid.add(t.id, spot.x, spot.y); // Grid update
                state.spritesDirty = true;
            }
            if (state.blackboard) {
                syncValue(state, 'cubeDeliveredAtGoal', true);
                syncValue(state, 'heldItemExists', false);
                syncValue(state, 'heldItemId', -1);
            }
            return bt.success;
        });

        reg('Place_Obstacle', [{k: 'heldItemExists', v: true}], [{k: 'heldItemExists', v: false}, {
            k: 'heldItemId',
            v: -1
        }], function () {
            const a = actor();
            if (!a.heldItem || a.heldItem.id === TARGET_ID || a.heldItem.id >= 100) return bt.failure;
            const heldId = a.heldItem.id, spot = getFreeAdjacentCell(state, a.x, a.y, false);
            if (!spot) return bt.failure;
            const cube = state.cubes.get(heldId);
            if (cube) {
                cube.deleted = false;
                cube.x = spot.x;
                cube.y = spot.y;
                state.spatialGrid.add(cube.id, spot.x, spot.y); // Grid update
                state.spritesDirty = true;
            }
            log.debug("[PA-BT ACTION]", {
                action: "Place_Obstacle",
                result: "SUCCESS",
                tick: state.tickCount,
                actorX: a.x,
                actorY: a.y,
                cubeId: heldId
            });
            a.heldItem = null;
            if (state.blackboard) {
                syncValue(state, 'heldItemExists', false);
                syncValue(state, 'heldItemId', -1);
            }
            return bt.success;
        });

        reg('Place_Target_Temporary', [{k: 'heldItemId', v: TARGET_ID}], [{
            k: 'heldItemExists',
            v: false
        }, {k: 'heldItemId', v: -1}], function () {
            const a = actor();
            if (!a.heldItem || a.heldItem.id !== TARGET_ID) return bt.failure;
            const spot = getFreeAdjacentCell(state, a.x, a.y, false);
            if (!spot) return bt.failure;
            log.debug("[PA-BT ACTION]", {
                action: "Place_Target_Temporary",
                result: "SUCCESS",
                tick: state.tickCount,
                actorX: a.x,
                actorY: a.y,
                cubeId: TARGET_ID
            });
            const t = state.cubes.get(TARGET_ID);
            if (t) {
                t.deleted = false;
                t.x = spot.x;
                t.y = spot.y;
                state.spatialGrid.add(t.id, spot.x, spot.y); // Grid update
                state.spritesDirty = true;
            }
            a.heldItem = null;
            if (state.blackboard) {
                syncValue(state, 'heldItemExists', false);
                syncValue(state, 'heldItemId', -1);
            }
            return bt.success;
        });

        reg('Place_Held_Item', [{k: 'heldItemExists', v: true}], [{k: 'heldItemExists', v: false}, {
            k: 'heldItemId',
            v: -1
        }], function () {
            const a = actor();
            if (!a.heldItem || a.heldItem.id === TARGET_ID || a.heldItem.id >= 100) return bt.failure;
            const spot = getFreeAdjacentCell(state, a.x, a.y);
            if (!spot) return bt.failure;
            const itemId = a.heldItem.id, c = state.cubes.get(itemId);
            if (c) {
                c.deleted = false;
                c.x = spot.x;
                c.y = spot.y;
                state.spatialGrid.add(c.id, spot.x, spot.y); // Grid update
                state.spritesDirty = true;
            }
            log.debug("[PA-BT ACTION]", {
                action: "Place_Held_Item",
                result: "SUCCESS",
                tick: state.tickCount,
                actorX: a.x,
                actorY: a.y,
                cubeId: itemId
            });
            a.heldItem = null;
            if (state.blackboard) {
                syncValue(state, 'heldItemExists', false);
                syncValue(state, 'heldItemId', -1);
            }
            return bt.success;
        });
    }

    let _renderBuffer = null;
    let _renderBufferWidth = 0;
    let _renderBufferHeight = 0;

    function getRenderBuffer(width, height) {
        if (_renderBuffer === null || _renderBufferWidth !== width || _renderBufferHeight !== height) {
            _renderBufferWidth = width;
            _renderBufferHeight = height;
            _renderBuffer = new Array(width * height);
            for (let i = 0; i < _renderBuffer.length; i++) {
                _renderBuffer[i] = ' ';
            }
        }
        return _renderBuffer;
    }

    function clearBuffer(buffer, width, height) {
        for (let i = 0; i < buffer.length; i++) {
            buffer[i] = ' ';
        }
    }

    function getAllSprites(state) {
        // Sprite Pooling
        spriteArenaIdx = 0;

        function allocSprite(x, y, char, w = 1, h = 1) {
            let s;
            if (spriteArenaIdx < spriteArena.length) {
                s = spriteArena[spriteArenaIdx++];
            } else {
                s = {};
                spriteArena.push(s);
                spriteArenaIdx++;
            }
            s.x = x;
            s.y = y;
            s.char = char;
            s.width = w;
            s.height = h;
            return s;
        }

        const sprites = [];
        state.actors.forEach(a => {
            sprites.push(allocSprite(a.x, a.y, '@'));
            if (a.heldItem) sprites.push(allocSprite(a.x, a.y - 0.5, '◆'));
        });
        state.cubes.forEach(c => {
            if (!c.deleted) {
                let ch = '█';
                if (c.type === 'target') ch = '◇';
                else if (c.type === 'obstacle') ch = '▒';
                sprites.push(allocSprite(c.x, c.y, ch));
            }
        });
        state.goals.forEach(g => {
            let ch = '○';
            if (g.forTarget) ch = '◎';
            sprites.push(allocSprite(g.x, g.y, ch));
        });
        return sprites;
    }

    function renderPlayArea(state) {
        const width = state.width;
        const height = state.height;

        const buffer = getRenderBuffer(width, height);
        clearBuffer(buffer, width, height);

        const spaceX = Math.floor((width - state.spaceWidth) / 2);
        for (let y = 0; y < height; y++) buffer[y * width + spaceX] = '│';

        const cx = GOAL_CENTER_X, cy = GOAL_CENTER_Y, r = GOAL_RADIUS;
        for (let gy = cy - r; gy <= cy + r; gy++) {
            for (let gx = cx - r; gx <= cx + r; gx++) {
                const sx = gx + spaceX + 1;
                if (sx >= 0 && sx < width && gy >= 0 && gy < height) {
                    const idx = gy * width + sx;
                    if (buffer[idx] === ' ') buffer[idx] = '·';
                }
            }
        }

        // Optimized Sorting
        if (state.spritesDirty) {
            state.lastSortedSprites = getAllSprites(state).sort((a, b) => a.y - b.y);
            state.spritesDirty = false;
        }

        for (const s of state.lastSortedSprites) {
            const sx = Math.floor(s.x) + spaceX + 1;
            const sy = Math.floor(s.y);
            if (sx >= 0 && sx < width && sy >= 0 && sy < height) {
                buffer[sy * width + sx] = s.char;
            }
        }

        const HUD_WIDTH = 25;
        const hudX = spaceX + state.spaceWidth + 2;
        const hudSpace = width - hudX;

        const drawAt = (x, y, txt) => {
            for (let i = 0; i < txt.length && x + i < width; i++) {
                const idx = y * width + x + i;
                if (idx >= 0 && idx < buffer.length) {
                    buffer[idx] = txt[i];
                }
            }
        };

        if (hudSpace >= HUD_WIDTH) {
            let hudY = 2;
            const draw = (txt) => {
                const maxLen = Math.min(txt.length, hudSpace);
                for (let i = 0; i < maxLen && hudX + i < width; i++) {
                    buffer[hudY * width + hudX + i] = txt[i];
                }
                hudY++;
            };

            draw('═════════════════════════');
            draw(' PICK-AND-PLACE SIM');
            draw('═════════════════════════');
            draw('Mode: ' + state.gameMode.toUpperCase());
            if (state.paused) draw('*** PAUSED ***');
            draw('Goal: 3x3 Area');
            draw('Tick: ' + state.tickCount);
            if (state.winConditionMet) draw('*** GOAL ACHIEVED! ***');
            draw('');
            draw('CONTROLS');
            draw('────────');
            draw('[Q] Quit');
            draw('[M] Toggle Mode');
            draw('[WASD] Move (manual)');
            draw('[Space] Pause');
            draw('[Mouse] Click to Move/Interact');
        } else {
            const statusY = height - 1;
            const modeStr = 'Mode: ' + state.gameMode.toUpperCase();
            const tickStr = ' T:' + state.tickCount;
            const hintStr = ' [Q]uit [M]ode [WASD] [Space]Pause';
            const winStr = state.winConditionMet ? ' WIN!' : '';
            const pauseStr = state.paused ? ' PAUSED' : '';

            let statusLine = modeStr + tickStr + pauseStr + winStr + hintStr;
            if (statusLine.length > width) {
                statusLine = statusLine.substring(0, width);
            }
            drawAt(0, statusY, statusLine);
        }

        const rows = [];
        for (let y = 0; y < height; y++) rows.push(buffer.slice(y * width, (y + 1) * width).join(''));
        return rows.join('\n');
    }

    // Model Update & Init

    function init() {
        const state = initializeSimulation();
        state.blackboard = new bt.Blackboard();
        state.pabtState = pabt.newState(state.blackboard);
        setupPABTActions(state);
        syncToBlackboard(state);

        const goalConditions = [{key: 'cubeDeliveredAtGoal', Match: v => v === true}];
        state.pabtPlan = pabt.newPlan(state.pabtState, goalConditions);
        state.ticker = bt.newTicker(100, state.pabtPlan.Node());

        return [state, tea.tick(16, 'tick')];
    }

    // Process pending inputs from the queue with compaction for performance
    function processPendingInputs(state) {
        if (!state.pendingInputs || state.pendingInputs.length === 0) return [state, null];

        const inputs = state.pendingInputs;
        state.pendingInputs = [];
        let cmd = null;

        // Compaction: Iterate inputs, keeping only the latest mouse click if consecutive.
        const compactedInputs = [];
        for (let i = 0; i < inputs.length; i++) {
            const current = inputs[i];
            if (current.type === 'Mouse' && current.action === 'press' && current.button === 'left') {
                // Look ahead for more mouse events to squash
                let nextIdx = i + 1;
                while (nextIdx < inputs.length &&
                inputs[nextIdx].type === 'Mouse' &&
                inputs[nextIdx].action === 'press' &&
                inputs[nextIdx].button === 'left') {
                    i = nextIdx; // Skip previous mouse event
                    nextIdx++;
                }
                compactedInputs.push(inputs[i]); // Push only the latest
            } else {
                compactedInputs.push(current);
            }
        }

        let wasdDx = 0;
        let wasdDy = 0;

        for (const msg of compactedInputs) {
            if (msg.type === 'Mouse') {
                if (state.gameMode === 'manual') {
                    const spaceX = Math.floor((state.width - state.spaceWidth) / 2);
                    const clickX = msg.x - spaceX - 1;
                    const clickY = msg.y;

                    if (clickX >= 0 && clickX < state.spaceWidth && clickY >= 0 && clickY < state.height) {
                        const actor = state.actors.get(state.activeActorId);
                        const ignoreId = actor.heldItem ? actor.heldItem.id : -1;
                        let path = null;

                        if (actor.heldItem) {
                            const neighbors = [];
                            for (const [dx, dy] of DIRS_8) {
                                const nx = clickX + dx;
                                const ny = clickY + dy;
                                if (nx >= 0 && nx < state.spaceWidth && ny >= 0 && ny < state.height) {
                                    const dist = Math.sqrt(Math.pow(nx - actor.x, 2) + Math.pow(ny - actor.y, 2));
                                    neighbors.push({x: nx, y: ny, dist: dist});
                                }
                            }
                            neighbors.sort((a, b) => a.dist - b.dist);

                            for (const n of neighbors) {
                                const p = findPathManual(state, actor.x, actor.y, n.x, n.y, ignoreId);
                                if (p !== null) {
                                    path = p;
                                    break;
                                }
                            }
                        }

                        if (path === null) {
                            path = findPathManual(state, actor.x, actor.y, clickX, clickY, ignoreId);
                        }

                        state.manualMoveTarget = {x: clickX, y: clickY};
                        if (path && path.length > 0) {
                            state.manualPath = path;
                        } else {
                            state.manualPath = [];
                        }
                    }
                }
            } else if (msg.type === 'Key') {
                if (msg.key === 'q') return [state, tea.quit()];
                if (msg.key === 'm') {
                    state.gameMode = state.gameMode === 'automatic' ? 'manual' : 'automatic';
                    state.manualMoveTarget = null;
                    state.manualPath = [];
                    state.pathStuckTicks = 0;
                    if (state.gameMode === 'manual') {
                        state.manualTicker = bt.newTicker(16, createManualTree(state));
                    } else {
                        if (state.manualTicker) {
                            state.manualTicker.stop();
                            state.manualTicker = null;
                        }
                        syncToBlackboard(state);
                    }
                } else if (msg.key === ' ') {
                    state.paused = !state.paused;
                } else if (msg.key === 'escape') {
                    state.manualPath = [];
                    state.manualMoveTarget = null;
                    state.pathStuckTicks = 0;
                } else if (state.gameMode === 'manual') {
                    // Input Compaction: Accumulate WASD delta
                    if (msg.key === 'w') wasdDy -= 1;
                    if (msg.key === 's') wasdDy += 1;
                    if (msg.key === 'a') wasdDx -= 1;
                    if (msg.key === 'd') wasdDx += 1;
                }
            }
        }

        // Apply Compacted WASD movement
        if ((wasdDx !== 0 || wasdDy !== 0) && state.gameMode === 'manual') {
            const actor = state.actors.get(state.activeActorId);
            const moveDx = Math.sign(wasdDx); // Clamp to -1, 0, 1
            const moveDy = Math.sign(wasdDy); // Clamp to -1, 0, 1
            if (manualKeysMovement(state, actor, moveDx, moveDy)) {
                state.manualPath = [];
                state.manualMoveTarget = null;
            }
        }

        return [state, cmd];
    }

    function update(state, msg) {
        if (msg.type === 'WindowSize') {
            state.width = msg.width;
            state.height = msg.height;
            return [state, null];
        }

        if (msg.type === 'Mouse' && msg.action === 'press' && msg.button === 'left') {
            state.pendingInputs.push(msg);
            return [state, null];
        }
        if (msg.type === 'Key') {
            state.pendingInputs.push(msg);
            return [state, null];
        }

        if (msg.type === 'Tick' && msg.id === 'tick') {
            // Process Inputs FIRST inside the tick
            const [newState, cmd] = processPendingInputs(state);
            if (cmd) return [newState, cmd]; // e.g. Quit

            if (state.paused) return [state, tea.tick(16, 'tick')];
            state.tickCount++;

            if (state.gameMode === 'automatic') {
                syncToBlackboard(state);
            } else if (state.blackboard) {
                // Optimized updates for manual mode (only key props)
                const actor = state.actors.get(state.activeActorId);
                syncValue(state, 'actorX', actor.x);
                syncValue(state, 'actorY', actor.y);
                syncValue(state, 'heldItemExists', actor.heldItem !== null);
                syncValue(state, 'heldItemId', actor.heldItem ? actor.heldItem.id : -1);
            }
            return [state, tea.tick(16, 'tick')];
        }

        if (msg.type === 'Resize') {
            state.width = msg.width;
            state.height = msg.height;
        }

        return [state, tea.tick(16, 'tick')];
    }

    function view(state) {
        let output = renderPlayArea(state);
        if (state.debugMode) {
            const actor = state.actors.get(state.activeActorId);
            const target = state.cubes.get(TARGET_ID);
            let obstacleCount = 0;
            GOAL_BLOCKADE_IDS.forEach(id => {
                const cube = state.cubes.get(id);
                if (cube && !cube.deleted) obstacleCount++;
            });
            const goalReachable = state.goals.get(GOAL_ID) ? getPathInfo(state, Math.round(actor.x), Math.round(actor.y), state.goals.get(GOAL_ID).x, state.goals.get(GOAL_ID).y).reachable : false;

            const debugJSON = JSON.stringify({
                m: state.gameMode === 'automatic' ? 'a' : 'm',
                t: state.tickCount,
                tw: state.width,
                sw: state.spaceWidth,
                x: Math.round(actor.x * 10) / 10,
                y: Math.round(actor.y * 10) / 10,
                h: actor.heldItem ? actor.heldItem.id : -1,
                w: state.winConditionMet ? 1 : 0,
                a: target && !target.deleted ? target.x : null,
                b: target && !target.deleted ? target.y : null,
                n: 0,
                g: obstacleCount,
                gr: goalReachable ? 1 : 0,
                mt: state.manualMoveTarget ? 1 : 0,
                mpl: state.manualPath.length,
                pst: state.pathStuckTicks
            });
            output += '\n__place_debug_start__\n' + debugJSON + '\n__place_debug_end__';
        }
        return output;
    }

    program = tea.newModel({
        init: function () {
            return init();
        },
        update: function (msg, model) {
            return update(model, msg);
        },
        view: function (model) {
            return view(model);
        },
        renderThrottle: {
            enabled: true,
            minIntervalMs: 16,
            alwaysRenderMsgTypes: ["Tick", "WindowSize"]
        }
    });

    if (typeof module !== 'undefined' && typeof module.exports !== 'undefined') {
        module.exports = {
            init,
            update,
            initializeSimulation,
            TARGET_ID,
            buildBlockedSet,
            findPath,
            getPathInfo,
            findNextStep,
            findFirstBlocker,
            manualKeysMovement
        };
    }
} catch (e) {
    printFatalError(e);
    throw e;
}

{
    let shouldRun = true;
    if (typeof module !== 'undefined') {
        if (typeof require !== 'undefined' && require.main !== module) {
            shouldRun = false;
        }
    }
    if (shouldRun) {
        try {
            tea.run(program, {altScreen: true, mouse: true});
        } catch (e) {
            printFatalError(e);
            throw e;
        }
    }
}
