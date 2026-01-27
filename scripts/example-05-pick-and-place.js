#!/usr/bin/env osm script

/**
 * @fileoverview example-05-pick-and-place.js
 * @description
 * Pick-and-Place Simulator demonstrating osm:pabt PA-BT planning integration.
 *
 * Implementation of Planning and Acting using Behavior Trees (PA-BT).
 * The system functions as a dynamic planner that interleaves planning and execution.
 *
 * Precondition Ordering Strategy:
 * 1. Conflict Resolution (Inter-Task): The planner dynamically handles dependencies between
 * tasks (e.g., A clobbers B) by moving conflicting subtrees to the left (higher priority).
 * 2. PPA Template (Intra-Task): Statically handles reactivity within an action unit using
 * the fallback structure: Fallback(Goal Check, Sequence(Preconditions, Action)).
 *
 * Runtime Constraints:
 * - Truthful Effects: Actions must only report success if the physical state actually changed.
 * - Fresh Nodes: BT nodes are instantiated fresh to avoid state pollution.
 * - Reactive Sensing: The Blackboard updates path blockers every tick.
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

    class FlatSpatialIndex {
        constructor(width, height) {
            this.width = width;
            this.height = height;
            // 0 = empty. IDs start at 1.
            this.buffer = new Int32Array(width * height);
            this.buffer.fill(0);
        }

        _idx(x, y) {
            // [FIX] Guarantee Bounds to prevent line-wrapping bugs
            if (x < 0 || x >= this.width || y < 0 || y >= this.height) return -1;
            return ((y | 0) * this.width + (x | 0));
        }

        add(id, x, y) {
            const idx = this._idx(x, y);
            if (idx >= 0 && idx < this.buffer.length) {
                this.buffer[idx] = id;
                // Mark state dirty for Blackboard sync (Doc 1)
                if (typeof state !== 'undefined') state.gridDirty = true;
            }
        }

        remove(x, y, id) {
            const idx = this._idx(x, y);
            if (idx >= 0 && idx < this.buffer.length) {
                // Compatibility check: only remove if it matches id (if id is provided)
                if (id === undefined || this.buffer[idx] === id) {
                    this.buffer[idx] = 0;
                    if (typeof state !== 'undefined') state.gridDirty = true;
                }
            }
        }

        move(id, oldX, oldY, newX, newY) {
            this.remove(oldX, oldY, id);
            this.add(id, newX, newY);
        }

        get(x, y) {
            const idx = this._idx(x, y);
            if (idx >= 0 && idx < this.buffer.length) {
                const val = this.buffer[idx];
                return val === 0 ? undefined : val; // Compatibility fix
            }
            return undefined;
        }
    }

    function isInGoalArea(x, y) {
        return x >= GOAL_CENTER_X - GOAL_RADIUS &&
            x <= GOAL_CENTER_X + GOAL_RADIUS &&
            y >= GOAL_CENTER_Y - GOAL_RADIUS &&
            y <= GOAL_CENTER_Y + GOAL_RADIUS;
    }

    // Capture global reference for FlatSpatialIndex dirty flagging
    var state;

    function initializeSimulation() {
        GOAL_BLOCKADE_IDS = []; // Fix: Clear before populating
        const cubesInit = [];
        const spatialGrid = new FlatSpatialIndex(ENV_WIDTH, ENV_HEIGHT);

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
        const s = {
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
            gridDirty: true,

            debugMode: os.getenv('OSM_TEST_MODE') === '1'
        };
        state = s;
        return s;
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
            state.spritesDirty = true;

            // [FIX] MOVEMENT INVALIDATES GEOMETRY CACHE
            state.gridDirty = true;

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
        const keyInt = (x, y) => (y * state.width + x); // Flat index key
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

    const MANUAL_PATH_NODE_BUDGET = 500;

    class MinHeap {
        constructor(size = 1024) {
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
        openHeap: new MinHeap(1024), // Pre-allocated Fixed Size
        // Index = y * width + x.
        gScore: new Int32Array(ENV_WIDTH * ENV_HEIGHT),
        // Stores the Generation ID of the search that visited this node
        visited: new Int32Array(ENV_WIDTH * ENV_HEIGHT),
        searchId: 1, // Increments per search

        reset: function () {
            this.searchId++; // Instant "clear"
            this.openHeap.clear();
            // No need to clear gScore/visited arrays
        }
    };

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
        manualModeCache.reset();

        const iStartX = Math.round(startX), iStartY = Math.round(startY);
        const iTargetX = Math.round(targetX), iTargetY = Math.round(targetY);

        if (iStartX === iTargetX && iStartY === iTargetY) return [];

        const keyInt = (x, y) => (y * state.width + x);
        const h = (x, y) => Math.abs(x - iTargetX) + Math.abs(y - iTargetY);

        // Reuse persistent structures
        const open = manualModeCache.openHeap;
        const gScore = manualModeCache.gScore;
        const visited = manualModeCache.visited;
        const currentSearchId = manualModeCache.searchId;

        const startNode = allocNode(iStartX, iStartY, 0, h(iStartX, iStartY), null);
        open.push(startNode);

        const startKey = keyInt(iStartX, iStartY);
        gScore[startKey] = 0;
        visited[startKey] = currentSearchId;

        let expanded = 0;
        let bestNode = startNode;
        let bestH = h(iStartX, iStartY);

        const actor = state.actors.get(state.activeActorId);
        const heldId = actor.heldItem ? actor.heldItem.id : -1;

        while (open.size() > 0 && expanded < MANUAL_PATH_NODE_BUDGET) {
            const cur = open.pop();
            const curKey = keyInt(cur.x, cur.y);

            // Lazy cleanup check if needed, but generation ID on 'visited' handles closed set logic mostly.
            // A* typically needs re-expansion if new path is better, but simple visited check is ok for this.

            if (cur.x === iTargetX && cur.y === iTargetY) {
                return reconstructManualPath(cur);
            }

            expanded++;
            const curH = h(cur.x, cur.y);
            if (curH < bestH) {
                bestH = curH;
                bestNode = cur;
            }

            for (const [dx, dy] of DIRS_4) {
                const nx = cur.x + dx, ny = cur.y + dy;
                const nKey = keyInt(nx, ny);

                if (nx < 1 || nx >= state.spaceWidth - 1 || ny < 1 || ny >= state.height - 1) continue;

                // If visited in this search and gScore is better/equal, skip
                if (visited[nKey] === currentSearchId && gScore[nKey] <= cur.g + 1) continue;

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

                // Update or First Visit
                gScore[nKey] = ng;
                visited[nKey] = currentSearchId;
                open.push(allocNode(nx, ny, ng, ng + h(nx, ny), cur));
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
            path.push({x: n.x, y: n.y});
            n = n.parent;
        }
        // Do NOT reverse.
        // Structure: [Target, ..., Step 2, Step 1]
        return path;
    }

    function findFirstBlocker(state, fromX, fromY, toX, toY, excludeId) {
        const keyInt = (x, y) => (y * state.width + x);
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
            if (dx <= 1 && dy <= 1) return null; // Path Clear

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
                        // [FIX] Removed unused 'dist' property (Dead Code)
                        frontier.push({x: nx, y: ny, id: blockId});
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
        // [FIX] Semantic Ambiguity: Return -2 for "Unreachable" (distinct from null "Clear")
        return -2;
    }

    function syncValue(state, key, newValue) {
        if (state.bbCache.get(key) !== newValue) {
            state.blackboard.set(key, newValue);
            state.bbCache.set(key, newValue);
        }
    }

    // [OPTIMIZATION] Single-Pass Reachability Map (Dijkstra Flood Fill)
    // Replaces O(N) pathfinding calls with O(1) global flood fill.
    function computeReachabilityMap(state, startX, startY) {
        const visited = new Set();
        const frontier = [];
        const queue = [{x: Math.round(startX), y: Math.round(startY)}];
        const keyInt = (x, y) => (y * state.width + x);
        const startKey = keyInt(queue[0].x, queue[0].y);

        visited.add(startKey);

        const actor = state.actors.get(state.activeActorId);
        const heldId = actor.heldItem ? actor.heldItem.id : -1;

        while (queue.length > 0) {
            const current = queue.shift();

            for (const [ox, oy] of DIRS_4) {
                const nx = current.x + ox;
                const ny = current.y + oy;
                const nKey = keyInt(nx, ny);

                if (nx < 1 || nx >= state.spaceWidth - 1 || ny < 1 || ny >= state.height - 1) continue;
                if (visited.has(nKey)) continue;

                const blockId = state.spatialGrid.get(nx, ny);
                // Treat held item as transparent
                if (blockId !== undefined && (!heldId || blockId !== heldId)) {
                    // Hit an obstacle: Add to frontier, do not traverse
                    frontier.push({x: nx, y: ny, id: blockId});
                    // We do NOT add to visited to allow re-discovery if needed, but for flood fill 'frontier' is sufficient
                } else {
                    // Open space
                    visited.add(nKey);
                    queue.push({x: nx, y: ny});
                }
            }
        }
        return {visited, frontier};
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

        // Expensive Geometry checks run ONLY when dirty
        if (state.gridDirty) {
            // [OPTIMIZATION] Compute global reachability ONCE
            const {visited, frontier} = computeReachabilityMap(state, ax, ay);
            const keyInt = (x, y) => (y * state.width + x);

            // Helper: Resolve blocker for any target using the pre-computed frontier
            const resolveBlocker = (tx, ty) => {
                const itx = Math.round(tx);
                const ity = Math.round(ty);

                // 1. Check if neighbor is reachable (Path Clear)
                for (let dx = -1; dx <= 1; dx++) {
                    for (let dy = -1; dy <= 1; dy++) {
                        if (visited.has(keyInt(itx + dx, ity + dy))) return -1; // Clear
                    }
                }

                // 2. If unreachable, find closest frontier obstacle
                if (frontier.length === 0) return -2; // Totally isolated

                let closestId = -2;
                let minDist = Infinity;
                for (const f of frontier) {
                    const d = Math.abs(f.x - itx) + Math.abs(f.y - ity);
                    if (d < minDist) {
                        minDist = d;
                        closestId = f.id;
                    }
                }
                return closestId;
            };

            state.cubes.forEach(cube => {
                if (!cube.deleted) {
                    const dist = Math.sqrt(Math.pow(cube.x - ax, 2) + Math.pow(cube.y - ay, 2));
                    const atEntity = dist <= 1.8;
                    syncValue(state, 'atEntity_' + cube.id, atEntity);

                    // [FIX] Generalized Path Blocker Logic using Optimization
                    // Only compute for Target or Obstacles (ID >= 100)
                    if (cube.id === TARGET_ID || cube.id >= 100) {
                        const blocker = resolveBlocker(cube.x, cube.y);
                        syncValue(state, 'pathBlocker_entity_' + cube.id, blocker);
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

                // [FIX] Optimized Goal Blocker Logic
                const blocker = resolveBlocker(goal.x, goal.y);
                syncValue(state, 'pathBlocker_goal_' + goal.id, blocker);
            });
            state.gridDirty = false;
        }

        syncValue(state, 'cubeDeliveredAtGoal', state.winConditionMet);
    }

    function createManualMoveLeaf(state) {
        return bt.createLeafNode(() => {
            if (state.manualPath.length === 0) return bt.success;

            const actor = state.actors.get(state.activeActorId);
            // Inverted stack: next step is at the end
            const nextPoint = state.manualPath[state.manualPath.length - 1];
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
                state.manualPath.pop(); // Remove step
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
                    // [FIX] Check for existing cube logic AND ensure spatial grid is empty at target
                    const targetCellId = state.spatialGrid.get(clickX, clickY);

                    if (!clickedCube && targetCellId === undefined) {
                        const c = state.cubes.get(ignoreId);
                        if (c) {
                            c.deleted = false;
                            c.x = clickX;
                            c.y = clickY;
                            state.spatialGrid.add(c.id, clickX, clickY);
                            actor.heldItem = null;
                            state.spritesDirty = true;
                            // [FIX] Placing an item dirties the grid cache
                            state.gridDirty = true;
                            if (ignoreId === TARGET_ID && isInGoalArea(clickX, clickY)) state.winConditionMet = true;
                        }
                    }
                } else if (clickedCube && !clickedCube.isStatic && !clickedCube.deleted) {
                    clickedCube.deleted = true;
                    state.spatialGrid.remove(clickedCube.x, clickedCube.y, clickedCube.id); // Remove from grid with check
                    actor.heldItem = {id: clickedCube.id};
                    state.spritesDirty = true;
                } else if (!isHolding && !clickedCube) {
                    const nearestCube = findNearestPickableCube(state, clickX, clickY);
                    if (nearestCube) {
                        const actorToCubeDist = Math.sqrt(Math.pow(nearestCube.x - actor.x, 2) + Math.pow(nearestCube.y - actor.y, 2));
                        if (actorToCubeDist <= PICK_THRESHOLD) {
                            nearestCube.deleted = true;
                            state.spatialGrid.remove(nearestCube.x, nearestCube.y, nearestCube.id); // Remove from grid with check
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
            conditions.push({key: pathBlockerKey, value: -1, match: v => v === -1});
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
                // Fix: Non-existent entity check
                if (!cube) return bt.failure;
                if (cube.deleted) {
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

            // REPLACEMENT: Use findPathManual (Zero-Allocation A*)
            const path = findPathManual(state, actor.x, actor.y, targetX, targetY, ignoreCubeId);

            if (path && path.length > 0) {
                // Inverted Stack: Next step is at the end (popped)
                const nextStep = path.pop();

                const stepDx = nextStep.x - actor.x;
                const stepDy = nextStep.y - actor.y;
                var newX = actor.x + Math.sign(stepDx) * Math.min(1.0, Math.abs(stepDx));
                var newY = actor.y + Math.sign(stepDy) * Math.min(1.0, Math.abs(stepDy));

                actor.x = newX;
                actor.y = newY;
                state.spritesDirty = true;

                // [FIX] MOVEMENT INVALIDATES GEOMETRY CACHE
                state.gridDirty = true;
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
            {key: 'heldItemExists', value: false, match: v => v === false},
            {key: 'atEntity_' + cubeId, value: true, match: v => v === true}
        ];
        const effects = [
            {key: 'heldItemId', Value: cubeId},
            {key: 'heldItemExists', Value: true}
        ];
        const tickFn = function () {
            if (state.gameMode !== 'automatic') return bt.running;
            const actor = state.actors.get(state.activeActorId);
            const cube = state.cubes.get(cubeId);
            // Fix: Critical correctness constraints
            if (!cube) return bt.failure;
            if (cube.deleted) return bt.success;
            if (cube.isStatic) return bt.failure;

            const dx = cube.x - actor.x;
            const dy = cube.y - actor.y;
            const dist = Math.sqrt(dx * dx + dy * dy);
            if (dist > PICK_THRESHOLD) return bt.failure;

            cube.deleted = true;
            state.spatialGrid.remove(cube.x, cube.y, cube.id); // Remove from grid

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
        const conditions = [{key: 'heldItemId', value: cubeId, match: v => v === cubeId}];
        const effects = [
            {key: 'heldItemExists', Value: false},
            {key: 'heldItemId', Value: -1},
            {key: 'pathBlocker_' + destinationKey, Value: -1}
        ];
        const tickFn = function () {
            if (state.gameMode !== 'automatic') return bt.running;
            const actor = state.actors.get(state.activeActorId);
            if (!actor.heldItem || actor.heldItem.id !== cubeId) return bt.failure;

            // [FIX] Heuristic Livelock: Parse destinationKey to avoid the specific target we are clearing path to
            // OLD: Always avoided global goal (ID 1).
            // NEW: Identifies target from pathBlocker_KEY and vectors away from that.
            let avoidX = 0, avoidY = 0;
            if (destinationKey.startsWith('goal_')) {
                const id = parseInt(destinationKey.replace('goal_', ''), 10);
                const g = state.goals.get(id);
                if (g) {
                    avoidX = g.x;
                    avoidY = g.y;
                }
            } else if (destinationKey.startsWith('entity_')) {
                const id = parseInt(destinationKey.replace('entity_', ''), 10);
                const c = state.cubes.get(id);
                if (c) {
                    avoidX = c.x;
                    avoidY = c.y;
                }
            } else {
                // Fallback to global goal if format assumes old behavior or simple goal
                const g = [...state.goals.values()][0];
                if (g) {
                    avoidX = g.x;
                    avoidY = g.y;
                }
            }

            const ax = Math.round(actor.x), ay = Math.round(actor.y);
            const dirToAvoidX = avoidX - ax, dirToAvoidY = avoidY - ay;

            const dirs = [];
            // Vector logic: Place AWAY from the blocked target.
            // If target is Left (negative), place Right (positive).
            if (dirToAvoidX < 0) dirs.push([1, 0], [1, -1], [1, 1]);
            else dirs.push([-1, 0], [-1, -1], [-1, 1]);
            if (dirToAvoidY > 0) dirs.push([0, -1]); else dirs.push([0, 1]);

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
                    // Fix: Action Generation Validation to prevent ID -2
                    if (targetValue === -1 && typeof currentBlocker === 'number' && currentBlocker > 0) {
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
                match: v => c.v === undefined ? v === true : v === c.v
            }));
            const effectList = effects.map(e => ({key: e.k, Value: e.v}));
            const node = bt.createLeafNode(() => state.gameMode === 'automatic' ? tickFn() : bt.running);
            state.pabtState.RegisterAction(name, pabt.newAction(name, conditions, effectList, node));
        };

        reg('Pick_Target', [
            {k: 'heldItemExists', v: false},
            // Correctness Constraint: Required to break planning livelock (Pick -> Path Blocked -> Place to Clear -> Pick again).
            // Ensures the path is cleared *before* the agent commits to holding the target.
            {k: 'pathBlocker_goal_' + GOAL_ID, v: -1},
            {k: 'atEntity_' + TARGET_ID, v: true}
        ], [{
            k: 'heldItemId',
            v: TARGET_ID
        }, {k: 'heldItemExists', v: true}], function () {
            const a = actor(), t = state.cubes.get(TARGET_ID);
            if (a.heldItem || !t || t.deleted) return bt.failure;

            t.deleted = true;
            state.spatialGrid.remove(t.x, t.y, t.id); // Grid update

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

        reg('Deliver_Target', [{k: 'heldItemId', v: TARGET_ID}, {
            k: 'atGoal_' + GOAL_ID,
            v: true
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
            // [FIX] Removed logic inversion: removed `|| a.heldItem.id >= 100` to allow placing obstacles
            if (!a.heldItem || a.heldItem.id === TARGET_ID) return bt.failure;
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
            // [FIX] Removed logic inversion: removed `|| a.heldItem.id >= 100` to allow placing obstacles
            if (!a.heldItem || a.heldItem.id === TARGET_ID) return bt.failure;
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

        // Painter's Algorithm: Direct Layered Rendering
        // Layer 0: Static Walls
        state.cubes.forEach(c => {
            if (c.type === 'wall' && !c.deleted) {
                const sx = Math.floor(c.x) + spaceX + 1;
                const sy = Math.floor(c.y);
                if (sx >= 0 && sx < width && sy >= 0 && sy < height) {
                    buffer[sy * width + sx] = '█';
                }
            }
        });

        // Layer 1: Goals
        state.goals.forEach(g => {
            const sx = Math.floor(g.x) + spaceX + 1;
            const sy = Math.floor(g.y);
            if (sx >= 0 && sx < width && sy >= 0 && sy < height) {
                let ch = '○';
                if (g.forTarget) ch = '◎';
                buffer[sy * width + sx] = ch;
            }
        });

        // Layer 2: Items
        state.cubes.forEach(c => {
            if (c.type !== 'wall' && !c.deleted) {
                const sx = Math.floor(c.x) + spaceX + 1;
                const sy = Math.floor(c.y);
                if (sx >= 0 && sx < width && sy >= 0 && sy < height) {
                    let ch = '█';
                    if (c.type === 'target') ch = '◇';
                    else if (c.type === 'obstacle') ch = '▒';
                    buffer[sy * width + sx] = ch;
                }
            }
        });

        // Layer 3: Actors
        state.actors.forEach(a => {
            const sx = Math.floor(a.x) + spaceX + 1;
            const sy = Math.floor(a.y);
            if (sx >= 0 && sx < width && sy >= 0 && sy < height) {
                buffer[sy * width + sx] = '@';
                if (a.heldItem) {
                    // Draw held item slightly offset if desired, or over it.
                    // Original sprite logic was a.y - 0.5. Since we are grid aligned on buffer, we can't do sub-pixel.
                    // But we can check above cell.
                    const syAbove = sy - 1;
                    if (syAbove >= 0) buffer[syAbove * width + sx] = '◆';
                }
            }
        });

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
            if (state.winConditionMet) draw('*** WIN! ***');
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

        const goalConditions = [{key: 'cubeDeliveredAtGoal', match: v => v === true}];
        state.pabtPlan = pabt.newPlan(state.pabtState, goalConditions);
        state.ticker = bt.newTicker(100, state.pabtPlan.Node());

        return [state, tea.tick(16, 'tick')];
    }

    // Process pending inputs from the queue with Stream Reduction
    function processPendingInputs(state) {
        if (!state.pendingInputs || state.pendingInputs.length === 0) return [state, null];

        // Local accumulators - Zero Allocation
        let netDx = 0;
        let netDy = 0;
        let latestMouseMsg = null;
        let quit = false;
        let toggleMode = false;
        let togglePause = false;
        let escape = false;

        // Single pass ingestion
        for (let i = 0; i < state.pendingInputs.length; i++) {
            const msg = state.pendingInputs[i];

            if (msg.type === 'Mouse' && msg.action === 'press' && msg.button === 'left') {
                latestMouseMsg = msg; // Last writer wins
            } else if (msg.type === 'Key') {
                switch (msg.key) {
                    case 'q':
                        quit = true;
                        break;
                    case 'm':
                        toggleMode = true;
                        break;
                    case ' ':
                        togglePause = true;
                        break;
                    case 'escape':
                        escape = true;
                        break;
                    case 'w':
                        netDy -= 1;
                        break;
                    case 's':
                        netDy += 1;
                        break;
                    case 'a':
                        netDx -= 1;
                        break;
                    case 'd':
                        netDx += 1;
                        break;
                }
            }
        }

        // Clear queue immediately
        state.pendingInputs = [];

        if (quit) return [state, tea.quit()];

        // Logic Priority Application
        if (toggleMode) {
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
        }
        if (togglePause) state.paused = !state.paused;
        if (escape) {
            state.manualPath = [];
            state.manualMoveTarget = null;
            state.pathStuckTicks = 0;
        }

        // Apply Coalesced Movement (Manual Mode)
        if (state.gameMode === 'manual') {
            // Clamp to prevent speed hacking via key spam
            const moveDx = Math.sign(netDx);
            const moveDy = Math.sign(netDy);

            if (moveDx !== 0 || moveDy !== 0) {
                // Only calculate physics ONCE per tick
                if (manualKeysMovement(state, state.actors.get(state.activeActorId), moveDx, moveDy)) {
                    // Cancel auto paths if moving manually
                    state.manualPath = [];
                    state.manualMoveTarget = null;
                }
            }

            if (latestMouseMsg) {
                const msg = latestMouseMsg;
                const spaceX = Math.floor((state.width - state.spaceWidth) / 2);
                const clickX = msg.x - spaceX - 1;
                const clickY = msg.y;
                if (clickX >= 0 && clickX < state.spaceWidth && clickY >= 0 && clickY < state.height) {
                    const actor = state.actors.get(state.activeActorId);
                    const ignoreId = actor.heldItem ? actor.heldItem.id : -1;
                    let path = null;

                    // If holding, check neighbors
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
                            // Use new pathfinder
                            const p = findPathManual(state, actor.x, actor.y, n.x, n.y, ignoreId);
                            if (p && p.length > 0) {
                                path = p;
                                break;
                            }
                        }
                    }

                    if (path === null) {
                        path = findPathManual(state, actor.x, actor.y, clickX, clickY, ignoreId);
                    }

                    state.manualMoveTarget = {x: clickX, y: clickY};
                    // Path is inverted stack from findPathManual
                    if (path && path.length > 0) {
                        state.manualPath = path;
                    } else {
                        state.manualPath = [];
                    }
                }
            }
        }

        return [state, null];
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
            getPathInfo,
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