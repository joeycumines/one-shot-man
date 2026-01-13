#!/usr/bin/env osm script

// ============================================================================
// example-04-bt-shooter.js
// A 2D top-down shooter game demonstrating osm:bt + osm:bubbletea integration
// DISCLAIMER: This is turbo-slop.
// ============================================================================

// Top-level error handler - ensures ANY uncaught error triggers non-zero exit
try {
    // Load required modules with error handling
    var bt, tea, lip;
    try {
        bt = require('osm:bt');
    } catch (e) {
        console.error('Error: Failed to load osm:bt module. Make sure you are running with "osm script"');
        throw e;
    }
    try {
        tea = require('osm:bubbletea');
    } catch (e) {
        console.error('Error: Failed to load osm:bubbletea module. Make sure you are running with "osm script"');
        throw e;
    }
    try {
        lip = require('osm:lipgloss');
    } catch (e) {
        console.error('Error: Failed to load osm:lipgloss module. Make sure you are running with "osm script"');
        throw e;
    }

// Game constants
    const SCREEN_WIDTH = 80;
    const SCREEN_HEIGHT = 25;
    const PLAYER_SPEED = 80;  // units/sec - doubled for faster movement (~1.33 chars/frame at 60fps)
    const PLAYER_MAX_HEALTH = 100;
    const PLAYER_INVINCIBILITY_TIME = 2000;
    const PLAYER_SHOT_COOLDOWN = 500;
    const PROJECTILE_SPEED = 50;  // increased to keep relative feel
    const ENEMY_PROJECTILE_DAMAGE = 10;
    const PLAYER_PROJECTILE_DAMAGE = 25;
    const EXPLOSION_PARTICLE_COUNT = 10;
    const EXPLOSION_MAX_AGE = 300;
    const AI_TICK_RATE = 100; // 10 ticks per second

// Enemy type configurations
    const ENEMY_TYPES = {
        grunt: {
            health: 50,
            speed: 25,  // was 8, scaled ~3x to match player speed increase
            damage: 10,
            attackRange: 10,
            shootCooldown: 1500,
            projectileSpeed: 35,  // was 10
            sprite: '◆',
            color: '#FF0000'
        },
        sniper: {
            health: 30,
            speed: 18,  // was 6
            damage: 30,
            minRange: 12,
            maxRange: 25,
            shootCooldown: 2000,
            projectileSpeed: 55,  // was 18
            sprite: '◈',
            color: '#FF00FF'
        },
        pursuer: {
            health: 60,
            speed: 35,  // was 12
            damage: 15,
            dashRange: 15,
            minDashRange: 8,
            dashCooldown: 4000,
            dashSpeed: 70,  // was 25
            sprite: '◉',
            color: '#FFA500'
        },
        tank: {
            health: 150,
            speed: 12,  // was 4
            damage: 8,
            burstCooldown: 1800,
            burstCount: 3,
            shotDelay: 200,
            projectileSpeed: 25,  // was 8
            sprite: '█',
            color: '#8B0000'
        }
    };

// Wave definitions
    const WAVES = [
        [
            {type: 'grunt', count: 3}
        ],
        [
            {type: 'grunt', count: 4},
            {type: 'sniper', count: 1}
        ],
        [
            {type: 'grunt', count: 3},
            {type: 'sniper', count: 2},
            {type: 'pursuer', count: 1}
        ],
        [
            {type: 'grunt', count: 4},
            {type: 'pursuer', count: 2},
            {type: 'tank', count: 1}
        ],
        [
            {type: 'sniper', count: 2},
            {type: 'pursuer', count: 2},
            {type: 'tank', count: 2}
        ]
    ];

// ============================================================================
// Utility Functions (Lines 51-100)
// ============================================================================

    function distance(x1, y1, x2, y2) {
        return Math.sqrt(Math.pow(x2 - x1, 2) + Math.pow(y2 - y1, 2));
    }

    function clamp(value, min, max) {
        return Math.max(min, Math.min(max, value));
    }

    function createExplosion(x, y, count) {
        const colors = ['#FF0000', '#FFA500', '#FFFF00'];
        const particles = [];
        for (let i = 0; i < count; i++) {
            particles.push({
                x: x,
                y: y,
                vx: (Math.random() - 0.5) * 10,
                vy: (Math.random() - 0.5) * 10,
                char: ['*', '+', '×', '·'][Math.floor(Math.random() * 4)],
                color: colors[Math.floor(Math.random() * colors.length)],
                age: 0,
                maxAge: EXPLOSION_MAX_AGE + Math.random() * 200
            });
        }
        return particles;
    }

// Seeded random number generator using sfc32 algorithm
// DISABLED: process.env.SEED not available in osm scripting context
    /*
    function sfc32(seed) {
        let a = parseInt(seed.toString());
        let b = parseInt(seed.toString()) ^ 0x12345678;
        let c = parseInt(seed.toString()) ^ 0xABCDEF01;
        let d = parseInt(seed.toString()) ^ 0xFEDCBA09;

        return function() {
            a >>>= 0; b >>>= 0; c >>>= 0; d >>>= 0;
            let t = (a + b) | 0;
            a = b ^ b >>> 9;
            b = c + (c << 3) | 0;
            c = (c << 21 | c >>> 11);
            d = d + 1 | 0;
            t = t + d | 0;
            c = c + t | 0;
            return (t >>> 0) / 4294967296;
        };
    }

    // Use SEED environment variable for reproducible random values
    const seed = process.env.SEED;
    let rng;
    if (seed) {
        rng = sfc32(seed);
    } else {
        rng = Math.random;
    }
    */

// Always use Math.random for now
    let rng = Math.random;

// ============================================================================
// Entity Constructors (Lines 101-200)
// ============================================================================

    function createPlayer() {
        return {
            x: SCREEN_WIDTH / 2,
            y: SCREEN_HEIGHT - 3,
            vx: 0,
            vy: 0,
            health: PLAYER_MAX_HEALTH,
            maxHealth: PLAYER_MAX_HEALTH,
            invincibleUntil: 0,
            lastShotTime: 0,
            shotCooldown: PLAYER_SHOT_COOLDOWN,
            facing: 'up',  // Direction player is facing: 'up', 'down', 'left', 'right'
            sprite: '▲',   // Dynamic sprite based on facing direction
            color: '#00FF00'
        };
    }

    // Get sprite character based on facing direction
    function getPlayerSprite(facing) {
        switch (facing) {
            case 'up':
                return '▲';
            case 'down':
                return '▼';
            case 'left':
                return '◄';
            case 'right':
                return '►';
            default:
                return '▲';
        }
    }

    // Get projectile velocity based on facing direction
    function getProjectileVelocity(facing, speed) {
        switch (facing) {
            case 'up':
                return {vx: 0, vy: -speed};
            case 'down':
                return {vx: 0, vy: speed};
            case 'left':
                return {vx: -speed, vy: 0};
            case 'right':
                return {vx: speed, vy: 0};
            default:
                return {vx: 0, vy: -speed};
        }
    }

    function createEnemy(type, id) {
        const config = ENEMY_TYPES[type];
        if (!config) {
            throw new Error('Invalid enemy type: ' + type);
        }

        const enemy = {
            id: id,
            type: type,
            x: Math.floor(rng() * (SCREEN_WIDTH - 4)) + 2,
            y: 2,
            health: config.health,
            maxHealth: config.health,
            speed: config.speed,
            state: 'idle',
            sprite: config.sprite,
            color: config.color,
            projectileSpeed: config.projectileSpeed,
            blackboard: new bt.Blackboard(),
            tree: null,
            ticker: null
        };

        // Initialize blackboard with type-specific values
        enemy.blackboard.set('type', type);
        enemy.blackboard.set('id', id);
        enemy.blackboard.set('lastShotTime', 0);

        // CRITICAL: Initialize position and health BEFORE ticker starts!
        // Without this, the AI's first tick has undefined values and checkAlive fails.
        enemy.blackboard.set('x', enemy.x);
        enemy.blackboard.set('y', enemy.y);
        enemy.blackboard.set('health', enemy.health);
        // Set default player position (center-ish) so AI has a target before first syncToBlackboards
        // This will be overwritten by syncToBlackboards with actual player position
        enemy.blackboard.set('playerX', Math.floor(SCREEN_WIDTH / 2));
        enemy.blackboard.set('playerY', Math.floor(SCREEN_HEIGHT - 4));

        try {
            // ALL enemy types need speed for movement
            enemy.blackboard.set('speed', config.speed);

            if (type === 'grunt') {
                enemy.blackboard.set('attackRange', config.attackRange);
                enemy.blackboard.set('shootCooldown', config.shootCooldown);
                enemy.blackboard.set('shootDamage', config.damage);
            } else if (type === 'sniper') {
                enemy.blackboard.set('minRange', config.minRange);
                enemy.blackboard.set('maxRange', config.maxRange);
                enemy.blackboard.set('shootCooldown', config.shootCooldown);
                enemy.blackboard.set('shootDamage', config.damage);
            } else if (type === 'pursuer') {
                enemy.blackboard.set('dashRange', config.dashRange);
                enemy.blackboard.set('minDashRange', config.minDashRange);
                enemy.blackboard.set('dashCooldown', config.dashCooldown);
                enemy.blackboard.set('dashSpeed', config.dashSpeed);
                enemy.blackboard.set('dashDamage', config.damage);
                enemy.blackboard.set('lastDashTime', 0);
                enemy.blackboard.set('dashProgress', 0);
            } else if (type === 'tank') {
                enemy.blackboard.set('burstCooldown', config.burstCooldown);
                enemy.blackboard.set('burstCount', config.burstCount);
                enemy.blackboard.set('shotDelay', config.shotDelay);
                enemy.blackboard.set('shotDamage', config.damage);
                enemy.blackboard.set('lastBurstTime', 0);
                enemy.blackboard.set('burstIndex', 0);
                enemy.blackboard.set('burstInProgress', false);
            }

            // Build and start behavior tree ticker
            enemy.tree = createEnemyTree(type, enemy.blackboard);
            enemy.ticker = bt.newTicker(AI_TICK_RATE, enemy.tree);
        } catch (e) {
            console.error('Error initializing enemy #' + id + ' (' + type + '): ' + e.message);
            throw e;
        }

        return enemy;
    }

    function createProjectile(id, owner, ownerId, x, y, vx, vy, damage, speed) {
        return {
            id: id,
            x: x,
            y: y,
            vx: vx,
            vy: vy,
            damage: damage,
            speed: speed,
            owner: owner,
            ownerId: ownerId,
            age: 0,
            maxAge: 3000,
            sprite: owner === 'player' ? '•' : '○',
            color: owner === 'player' ? '#00FFFF' : '#FF6600'
        };
    }

    function createParticle(x, y) {
        return {
            x: x,
            y: y,
            char: ['*', '+', '×', '·'][Math.floor(rng() * 4)],
            color: ['#FF0000', '#FFA500', '#FFFF00'][Math.floor(rng() * 3)],
            age: 0,
            maxAge: EXPLOSION_MAX_AGE + rng() * 200
        };
    }

// ============================================================================
// Behavior Tree Leaf Functions (Lines 201-400)
// ============================================================================

// Common leaves - NOTE: bb is passed explicitly and captured via closure
// because bt.createBlockingLeafNode doesn't pass context at tick time
    function createCheckAliveLeaf(bb) {
        return bt.createBlockingLeafNode(() => {
            const health = bb.get('health');
            // console.log('CheckAlive: health=' + health);
            return health > 0 ? bt.success : bt.failure;
        });
    }

    function createCheckInRangeLeaf(bb, rangeKey) {
        return bt.createBlockingLeafNode(() => {
            const x = bb.get('x');
            const y = bb.get('y');
            const playerX = bb.get('playerX');
            const playerY = bb.get('playerY');
            const range = bb.get(rangeKey);

            if (x === undefined || y === undefined || playerX === undefined || playerY === undefined) {
                return bt.failure;
            }

            const dist = distance(x, y, playerX, playerY);
            return dist < range ? bt.success : bt.failure;
        });
    }

    function createMoveTowardLeaf(bb, speedKey) {
        return bt.createBlockingLeafNode(() => {
            const x = bb.get('x');
            const y = bb.get('y');
            const playerX = bb.get('playerX');
            const playerY = bb.get('playerY');
            const speed = bb.get(speedKey);

            // DEBUG: Log to see if leaf is being executed
            // console.log('MoveToward: x=' + x + ', y=' + y + ', px=' + playerX + ', py=' + playerY + ', speed=' + speed);

            if (x === undefined || y === undefined || playerX === undefined || playerY === undefined) {
                return bt.failure;
            }

            const dist = distance(x, y, playerX, playerY);
            if (dist < 0.5) {
                return bt.running;
            }

            const dx = (playerX - x) / dist;
            const dy = (playerY - y) / dist;

            const newX = clamp(x + dx * speed * 0.1, 1, SCREEN_WIDTH - 2);
            const newY = clamp(y + dy * speed * 0.1, 1, SCREEN_HEIGHT - 2);
            // console.log('MoveToward: setting newX=' + newX + ', newY=' + newY);
            bb.set('newX', newX);
            bb.set('newY', newY);
            return bt.running;
        });
    }

    function createShootLeaf(bb, cooldownKey, damageKey) {
        return bt.createBlockingLeafNode(() => {
            const now = Date.now();
            const lastShot = bb.get('lastShotTime');
            const cooldown = bb.get(cooldownKey);

            if (now - lastShot < cooldown) {
                return bt.running;
            }

            const playerX = bb.get('playerX');
            const playerY = bb.get('playerY');

            bb.set('fire', true);
            bb.set('fireTargetX', playerX);
            bb.set('fireTargetY', playerY);
            bb.set('lastShotTime', now);
            return bt.success;
        });
    }

// Sniper-specific leaves
    function createCheckSniperRangeLeaf(bb) {
        return bt.createBlockingLeafNode(() => {
            const x = bb.get('x');
            const y = bb.get('y');
            const playerX = bb.get('playerX');
            const playerY = bb.get('playerY');
            const minRange = bb.get('minRange');
            const maxRange = bb.get('maxRange');

            if (x === undefined || y === undefined || playerX === undefined || playerY === undefined) {
                return bt.failure;
            }

            const dist = distance(x, y, playerX, playerY);
            return dist >= minRange && dist <= maxRange ? bt.success : bt.failure;
        });
    }

    function createRetreatLeaf(bb, speedKey) {
        return bt.createBlockingLeafNode(() => {
            const x = bb.get('x');
            const y = bb.get('y');
            const playerX = bb.get('playerX');
            const playerY = bb.get('playerY');
            const speed = bb.get(speedKey);

            if (x === undefined || y === undefined || playerX === undefined || playerY === undefined) {
                return bt.failure;
            }

            const dist = distance(x, y, playerX, playerY);
            const dx = (playerX - x) / dist;
            const dy = (playerY - y) / dist;

            bb.set('newX', clamp(x - dx * speed * 0.1, 1, SCREEN_WIDTH - 2));
            bb.set('newY', clamp(y - dy * speed * 0.1, 1, SCREEN_HEIGHT - 2));
            return bt.running;
        });
    }

    function createAimAndShootLeaf(bb) {
        return bt.createBlockingLeafNode(() => {
            const now = Date.now();
            const lastShot = bb.get('lastShotTime');
            const cooldown = bb.get('shootCooldown');

            if (now - lastShot < cooldown) {
                return bt.running;
            }

            const playerX = bb.get('playerX');
            const playerY = bb.get('playerY');

            bb.set('fire', true);
            bb.set('fireTargetX', playerX);
            bb.set('fireTargetY', playerY);
            bb.set('lastShotTime', now);
            return bt.success;
        });
    }

// Pursuer-specific leaves
    function createCheckTooCloseLeaf(bb, minRange) {
        return bt.createBlockingLeafNode(() => {
            const x = bb.get('x');
            const y = bb.get('y');
            const playerX = bb.get('playerX');
            const playerY = bb.get('playerY');

            if (x === undefined || y === undefined || playerX === undefined || playerY === undefined) {
                return bt.failure;
            }

            const dist = distance(x, y, playerX, playerY);
            return dist < minRange ? bt.success : bt.failure;
        });
    }

    function createCanDashLeaf(bb) {
        return bt.createBlockingLeafNode(() => {
            const now = Date.now();
            const lastDash = bb.get('lastDashTime');
            const cooldown = bb.get('dashCooldown');

            return now - lastDash >= cooldown ? bt.success : bt.failure;
        });
    }

    function createCheckDashRangeLeaf(bb) {
        return bt.createBlockingLeafNode(() => {
            const x = bb.get('x');
            const y = bb.get('y');
            const playerX = bb.get('playerX');
            const playerY = bb.get('playerY');
            const dashRange = bb.get('dashRange');
            const minDashRange = bb.get('minDashRange');

            if (x === undefined || y === undefined || playerX === undefined || playerY === undefined) {
                return bt.failure;
            }

            const dist = distance(x, y, playerX, playerY);
            return dist < dashRange && dist > minDashRange ? bt.success : bt.failure;
        });
    }

    function createExecuteDashLeaf(bb) {
        return bt.createLeafNode(() => {
            const dashProgress = bb.get('dashProgress') || 0;
            const x = bb.get('x');
            const y = bb.get('y');

            if (dashProgress >= 1.0) {
                bb.set('dashing', false);
                bb.delete('dashProgress');
                return bt.success;
            }

            let targetX = bb.get('dashTargetX');
            let targetY = bb.get('dashTargetY');

            if (dashProgress === 0) {
                const playerX = bb.get('playerX');
                const playerY = bb.get('playerY');
                targetX = playerX;
                targetY = playerY;
                bb.set('dashTargetX', targetX);
                bb.set('dashTargetY', targetY);
            }

            const progress = dashProgress + 0.2;
            const newX = x + (targetX - x) * 0.2;
            const newY = y + (targetY - y) * 0.2;

            bb.set('newX', clamp(newX, 1, SCREEN_WIDTH - 2));
            bb.set('newY', clamp(newY, 1, SCREEN_HEIGHT - 2));
            bb.set('dashProgress', progress);
            bb.set('dashing', true);

            if (progress >= 1.0) {
                bb.set('lastDashTime', Date.now());
            }

            return bt.running;
        });
    }

// Tank-specific leaves
    function createSlowChaseLeaf(bb) {
        return bt.createBlockingLeafNode(() => {
            const x = bb.get('x');
            const y = bb.get('y');
            const playerX = bb.get('playerX');
            const playerY = bb.get('playerY');
            const speed = 4;

            if (x === undefined || y === undefined || playerX === undefined || playerY === undefined) {
                return bt.failure;
            }

            const dist = distance(x, y, playerX, playerY);
            if (dist < 5) {
                return bt.running;
            }

            const dx = (playerX - x) / dist;
            const dy = (playerY - y) / dist;

            bb.set('newX', clamp(x + dx * speed * 0.1, 1, SCREEN_WIDTH - 2));
            bb.set('newY', clamp(y + dy * speed * 0.1, 1, SCREEN_HEIGHT - 2));
            return bt.running;
        });
    }

    function createCheckBurstReadyLeaf(bb) {
        return bt.createBlockingLeafNode(() => {
            const now = Date.now();
            const lastBurst = bb.get('lastBurstTime');
            const cooldown = bb.get('burstCooldown');
            const burstInProgress = bb.get('burstInProgress');

            if (burstInProgress) {
                return bt.running;
            }

            return now - lastBurst >= cooldown ? bt.success : bt.failure;
        });
    }

    function createFireBurstLeaf(bb) {
        return bt.createLeafNode(() => {
            const burstIndex = bb.get('burstIndex') || 0;
            const burstCount = bb.get('burstCount');
            const shotDelay = bb.get('shotDelay');
            const lastShotTime = bb.get('lastShotTime') || 0;
            const now = Date.now();

            if (burstIndex >= burstCount) {
                bb.set('burstInProgress', false);
                bb.set('lastBurstTime', now);
                bb.set('burstIndex', 0);
                return bt.success;
            }

            if (burstIndex === 0) {
                bb.set('burstInProgress', true);
            }

            if (now - lastShotTime >= shotDelay) {
                const playerX = bb.get('playerX');
                const playerY = bb.get('playerY');

                bb.set('fire', true);
                bb.set('fireTargetX', playerX);
                bb.set('fireTargetY', playerY);
                bb.set('lastShotTime', now);
                bb.set('burstIndex', burstIndex + 1);
            }

            return bt.running;
        });
    }

// ============================================================================
// Behavior Tree Builders (Lines 401-500)
// ============================================================================

// Uses bt.node(tick, ...children) to build composable trees with Go composites + JS leaves

    function createEnemyTree(type, blackboard) {
        try {
            switch (type) {
                case 'grunt':
                    return buildGruntTree(blackboard);
                case 'sniper':
                    return buildSniperTree(blackboard);
                case 'pursuer':
                    return buildPursuerTree(blackboard);
                case 'tank':
                    return buildTankTree(blackboard);
                default:
                    throw new Error('Unknown enemy type: ' + type);
            }
        } catch (e) {
            console.error('Error creating behavior tree for enemy type "' + type + '": ' + e.message);
            throw e;
        }
    }

// ============================================================================
// SOPHISTICATED AI LEAVES - Advanced behavioral patterns
// ============================================================================

    // Strafe leaf: Move perpendicular to player for evasive maneuvering
    function createStrafeLeaf(bb) {
        return bt.createBlockingLeafNode(() => {
            const x = bb.get('x');
            const y = bb.get('y');
            const playerX = bb.get('playerX');
            const playerY = bb.get('playerY');
            const speed = bb.get('speed');

            if (x === undefined || y === undefined || playerX === undefined || playerY === undefined) {
                return bt.failure;
            }

            const dist = distance(x, y, playerX, playerY);
            if (dist < 0.5) return bt.running;

            // Calculate perpendicular vector (rotate 90 degrees)
            const dx = (playerX - x) / dist;
            const dy = (playerY - y) / dist;

            // Alternate strafe direction based on time (creates weaving pattern)
            const strafeDir = Math.sin(Date.now() / 500) > 0 ? 1 : -1;
            const perpX = -dy * strafeDir;  // Perpendicular vector
            const perpY = dx * strafeDir;

            const newX = clamp(x + perpX * speed * 0.08, 1, SCREEN_WIDTH - 2);
            const newY = clamp(y + perpY * speed * 0.08, 1, SCREEN_HEIGHT - 2);
            bb.set('newX', newX);
            bb.set('newY', newY);
            return bt.running;
        });
    }

    // Flank leaf: Circle around to attack from the side
    function createFlankLeaf(bb) {
        return bt.createBlockingLeafNode(() => {
            const x = bb.get('x');
            const y = bb.get('y');
            const playerX = bb.get('playerX');
            const playerY = bb.get('playerY');
            const speed = bb.get('speed');

            if (x === undefined || y === undefined || playerX === undefined || playerY === undefined) {
                return bt.failure;
            }

            const dist = distance(x, y, playerX, playerY);
            if (dist < 3) return bt.success; // Close enough to flank

            // Move in a spiral pattern (combine forward + perpendicular)
            const dx = (playerX - x) / dist;
            const dy = (playerY - y) / dist;

            // Spiral coefficient: more perpendicular when far, more direct when close
            const spiralFactor = Math.min(0.7, dist / 20);
            const perpX = -dy;
            const perpY = dx;

            const moveX = dx * (1 - spiralFactor) + perpX * spiralFactor;
            const moveY = dy * (1 - spiralFactor) + perpY * spiralFactor;

            const newX = clamp(x + moveX * speed * 0.1, 1, SCREEN_WIDTH - 2);
            const newY = clamp(y + moveY * speed * 0.1, 1, SCREEN_HEIGHT - 2);
            bb.set('newX', newX);
            bb.set('newY', newY);
            return bt.running;
        });
    }

    // Reposition leaf: Find optimal sniping distance
    function createRepositionLeaf(bb) {
        return bt.createBlockingLeafNode(() => {
            const x = bb.get('x');
            const y = bb.get('y');
            const playerX = bb.get('playerX');
            const playerY = bb.get('playerY');
            const speed = bb.get('speed');
            const optimalRange = bb.get('maxRange') - 3; // Prefer slightly closer than max

            if (x === undefined || y === undefined || playerX === undefined || playerY === undefined) {
                return bt.failure;
            }

            const dist = distance(x, y, playerX, playerY);

            // Calculate direction to/from player
            const dx = (playerX - x) / dist;
            const dy = (playerY - y) / dist;

            // Move to optimal range
            let moveDir;
            if (dist < optimalRange - 2) {
                // Too close - back away
                moveDir = -1;
            } else if (dist > optimalRange + 2) {
                // Too far - approach
                moveDir = 1;
            } else {
                // At optimal range - strafe slightly
                const perpX = -dy * (Math.sin(Date.now() / 800) > 0 ? 1 : -1);
                const perpY = dx * (Math.sin(Date.now() / 800) > 0 ? 1 : -1);
                bb.set('newX', clamp(x + perpX * speed * 0.05, 1, SCREEN_WIDTH - 2));
                bb.set('newY', clamp(y + perpY * speed * 0.05, 1, SCREEN_HEIGHT - 2));
                return bt.success;
            }

            const newX = clamp(x + dx * moveDir * speed * 0.08, 1, SCREEN_WIDTH - 2);
            const newY = clamp(y + dy * moveDir * speed * 0.08, 1, SCREEN_HEIGHT - 2);
            bb.set('newX', newX);
            bb.set('newY', newY);
            return bt.running;
        });
    }

    // Telegraph leaf: Brief pause before dash to give player warning
    function createTelegraphLeaf(bb) {
        return bt.createLeafNode(() => {
            const telegraphStart = bb.get('telegraphStart');
            const now = Date.now();
            const TELEGRAPH_DURATION = 300; // 300ms warning

            if (!telegraphStart) {
                // Start telegraph - set visual cue
                bb.set('telegraphStart', now);
                bb.set('telegraphing', true);
                return bt.running;
            }

            if (now - telegraphStart >= TELEGRAPH_DURATION) {
                // Telegraph complete - ready to dash
                bb.delete('telegraphStart');
                bb.set('telegraphing', false);
                return bt.success;
            }

            return bt.running;
        });
    }

    // Area denial: Tank moves to intercept player's likely path
    function createInterceptLeaf(bb) {
        return bt.createBlockingLeafNode(() => {
            const x = bb.get('x');
            const y = bb.get('y');
            const playerX = bb.get('playerX');
            const playerY = bb.get('playerY');
            const lastPlayerX = bb.get('lastPlayerX') || playerX;
            const lastPlayerY = bb.get('lastPlayerY') || playerY;
            const speed = 5; // Tank moves slowly but predictively

            // Calculate player's movement direction
            const playerDx = playerX - lastPlayerX;
            const playerDy = playerY - lastPlayerY;

            // Predict where player will be (lead the target)
            const leadFactor = 3;
            const predictedX = playerX + playerDx * leadFactor;
            const predictedY = playerY + playerDy * leadFactor;

            // Store current player pos for next tick
            bb.set('lastPlayerX', playerX);
            bb.set('lastPlayerY', playerY);

            // Move toward predicted position
            const dist = distance(x, y, predictedX, predictedY);
            if (dist < 0.5) return bt.running;

            const dx = (predictedX - x) / dist;
            const dy = (predictedY - y) / dist;

            const newX = clamp(x + dx * speed * 0.1, 1, SCREEN_WIDTH - 2);
            const newY = clamp(y + dy * speed * 0.1, 1, SCREEN_HEIGHT - 2);
            bb.set('newX', newX);
            bb.set('newY', newY);
            return bt.running;
        });
    }

    // Random chance check - adds unpredictability
    function createRandomChanceLeaf(bb, probability) {
        return bt.createBlockingLeafNode(() => {
            return Math.random() < probability ? bt.success : bt.failure;
        });
    }

// ============================================================================
// SOPHISTICATED BEHAVIOR TREES
// ============================================================================

// Grunt AI: Strafe while chasing, flank when close, shoot when in range
    function buildGruntTree(bb) {
        const checkAlive = createCheckAliveLeaf(bb);
        const checkInRange = createCheckInRangeLeaf(bb, 'attackRange');
        const shoot = createShootLeaf(bb, 'shootCooldown', 'shootDamage');
        const strafe = createStrafeLeaf(bb);
        const flank = createFlankLeaf(bb);
        const moveToward = createMoveTowardLeaf(bb, 'speed');
        const randomStrafe = createRandomChanceLeaf(bb, 0.3);

        // Attack sequence: in range AND shoot
        const attackSequence = bt.node(bt.sequence,
            checkInRange,
            shoot
        );

        // Strafe approach: randomly strafe while moving
        const strafeApproach = bt.node(bt.sequence,
            randomStrafe,
            strafe
        );

        // Approach behavior: try strafe, fallback to direct approach
        const approachBehavior = bt.node(bt.fallback,
            strafeApproach,
            flank,
            moveToward
        );

        // Main behavior: attack if in range, otherwise approach with evasive maneuvers
        const mainBehavior = bt.node(bt.fallback,
            attackSequence,
            approachBehavior
        );

        // Root: check alive, then main behavior
        return bt.node(bt.sequence,
            checkAlive,
            mainBehavior
        );
    }

// Sniper AI: Maintain optimal distance, reposition constantly, precise shots
    function buildSniperTree(bb) {
        const checkAlive = createCheckAliveLeaf(bb);
        const checkTooClose = createCheckTooCloseLeaf(bb, bb.get('minRange'));
        const retreat = createRetreatLeaf(bb, 'speed');
        const checkSniperRange = createCheckSniperRangeLeaf(bb);
        const aimAndShoot = createAimAndShootLeaf(bb);
        const reposition = createRepositionLeaf(bb);
        const moveToward = createMoveTowardLeaf(bb, 'speed');

        // Emergency retreat: too close, must escape
        const emergencyRetreat = bt.node(bt.sequence,
            checkTooClose,
            retreat
        );

        // Sniper attack: in optimal range AND shoot
        const sniperAttack = bt.node(bt.sequence,
            checkSniperRange,
            aimAndShoot
        );

        // Main behavior priority: emergency retreat > attack > reposition > approach
        const mainBehavior = bt.node(bt.fallback,
            emergencyRetreat,
            sniperAttack,
            reposition,
            moveToward
        );

        return bt.node(bt.sequence,
            checkAlive,
            mainBehavior
        );
    }

// Pursuer AI: Telegraph before dash, making attacks more readable
    function buildPursuerTree(bb) {
        const checkAlive = createCheckAliveLeaf(bb);
        const canDash = createCanDashLeaf(bb);
        const checkDashRange = createCheckDashRangeLeaf(bb);
        const telegraph = createTelegraphLeaf(bb);
        const executeDash = createExecuteDashLeaf(bb);
        const flank = createFlankLeaf(bb);
        const moveToward = createMoveTowardLeaf(bb, 'speed');

        // Full dash attack: can dash AND in range AND telegraph warning AND execute
        const dashAttackSequence = bt.node(bt.sequence,
            canDash,
            checkDashRange,
            telegraph,
            executeDash
        );

        // Approach with flanking behavior
        const approachBehavior = bt.node(bt.fallback,
            flank,
            moveToward
        );

        // Main behavior: try dash attack, fallback to flanking approach
        const mainBehavior = bt.node(bt.fallback,
            dashAttackSequence,
            approachBehavior
        );

        return bt.node(bt.sequence,
            checkAlive,
            mainBehavior
        );
    }

// Tank AI: Intercept player path, burst fire while moving
    function buildTankTree(bb) {
        const checkAlive = createCheckAliveLeaf(bb);
        const intercept = createInterceptLeaf(bb);
        const checkBurstReady = createCheckBurstReadyLeaf(bb);
        const fireBurst = createFireBurstLeaf(bb);
        const slowChase = createSlowChaseLeaf(bb);

        // Burst fire sequence
        const burstSequence = bt.node(bt.sequence,
            checkBurstReady,
            fireBurst
        );

        // Movement: prefer interception, fallback to direct chase
        const movementBehavior = bt.node(bt.fallback,
            intercept,
            slowChase
        );

        // Tank can move AND shoot: use fork for parallel behavior
        const combatBehavior = bt.node(bt.fork,
            burstSequence,
            movementBehavior
        );

        // Main behavior: combat actions (shoot while moving)
        const mainBehavior = bt.node(bt.fallback,
            combatBehavior,
            movementBehavior
        );

        return bt.node(bt.sequence,
            checkAlive,
            mainBehavior
        );
    }

// ============================================================================
// Game Logic Functions (Lines 501-700)
// ============================================================================

    function initializeGame() {
        return {
            gameMode: 'menu',
            score: 0,
            lives: 3,
            wave: 0,
            waveState: {
                inProgress: false,
                enemiesSpawned: 0,
                enemiesRemaining: 0,
                complete: false
            },
            player: createPlayer(),
            enemies: new Map(),
            projectiles: new Map(),
            particles: [],
            terminalSize: {width: SCREEN_WIDTH, height: SCREEN_HEIGHT},
            lastTickTime: Date.now(),
            deltaTime: 0,
            nextEntityId: 0,
            nextProjectileId: 0,
            debugMode: false,
            tick: 0
        };
    }

    function updatePlayer(state) {
        const player = state.player;

        // Player position is now updated DIRECTLY in key handler
        // This function just handles invincibility timer

        // Check invincibility
        if (Date.now() > player.invincibleUntil) {
            player.invincible = false;
        } else {
            player.invincible = true;
        }
    }

    function syncToBlackboards(state) {
        state.enemies.forEach((enemy, id) => {
            enemy.blackboard.set('x', enemy.x);
            enemy.blackboard.set('y', enemy.y);
            enemy.blackboard.set('health', enemy.health);
            enemy.blackboard.set('playerX', state.player.x);
            enemy.blackboard.set('playerY', state.player.y);
        });
    }

    function syncFromBlackboards(state) {
        state.enemies.forEach((enemy, id) => {
            // Update position if AI requested movement
            if (enemy.blackboard.has('newX')) {
                enemy.x = enemy.blackboard.get('newX');
                enemy.blackboard.delete('newX');
            }
            if (enemy.blackboard.has('newY')) {
                enemy.y = enemy.blackboard.get('newY');
                enemy.blackboard.delete('newY');
            }

            // Fire projectile if AI requested
            if (enemy.blackboard.get('fire')) {
                const targetX = enemy.blackboard.get('fireTargetX');
                const targetY = enemy.blackboard.get('fireTargetY');
                const config = ENEMY_TYPES[enemy.type];

                const dist = distance(enemy.x, enemy.y, targetX, targetY);
                const vx = ((targetX - enemy.x) / dist) * config.projectileSpeed;
                const vy = ((targetY - enemy.y) / dist) * config.projectileSpeed;

                const projectile = createProjectile(
                    ++state.nextProjectileId,
                    'enemy',
                    enemy.id,
                    enemy.x,
                    enemy.y,
                    vx,
                    vy,
                    config.damage,
                    config.projectileSpeed
                );
                state.projectiles.set(projectile.id, projectile);

                enemy.blackboard.delete('fire');
                enemy.blackboard.delete('fireTargetX');
                enemy.blackboard.delete('fireTargetY');
            }
        });
    }

    function updateEnemies(state) {
        const dt = state.deltaTime / 1000;

        state.enemies.forEach((enemy, id) => {
            // Update state based on blackboard
            if (enemy.blackboard.has('dashing')) {
                enemy.state = 'dashing';
            } else if (enemy.blackboard.get('fire')) {
                enemy.state = 'attacking';
            } else {
                enemy.state = 'chasing';
            }
        });
    }

    function updateProjectiles(state) {
        const toRemove = [];

        state.projectiles.forEach((projectile, id) => {
            const dt = state.deltaTime / 1000;

            // Update position
            projectile.x += projectile.vx * dt;
            projectile.y += projectile.vy * dt;

            // Update age
            projectile.age += state.deltaTime;

            // Remove if too old or out of bounds
            if (projectile.age > projectile.maxAge ||
                projectile.x < 0 || projectile.x > state.terminalSize.width ||
                projectile.y < 0 || projectile.y > state.terminalSize.height) {
                toRemove.push(id);
            }
        });

        toRemove.forEach(id => state.projectiles.delete(id));
    }

    function updateParticles(state) {
        const toRemove = [];

        state.particles = state.particles.filter(particle => {
            particle.age += state.deltaTime;
            particle.x += particle.vx * (state.deltaTime / 1000);
            particle.y += particle.vy * (state.deltaTime / 1000);
            return particle.age < particle.maxAge;
        });
    }

    function checkCollisions(state) {
        const player = state.player;

        // Player projectile vs Enemy
        state.projectiles.forEach((projectile, pid) => {
            if (projectile.owner !== 'player') return;

            state.enemies.forEach((enemy, eid) => {
                if (distance(projectile.x, projectile.y, enemy.x, enemy.y) < 1) {
                    enemy.health -= projectile.damage;
                    state.projectiles.delete(pid);

                    // Create hit particles
                    state.particles.push(...createExplosion(enemy.x, enemy.y, 3));
                }
            });
        });

        // Enemy projectile vs Player
        if (!player.invincible) {
            state.projectiles.forEach((projectile, pid) => {
                if (projectile.owner !== 'enemy') return;

                if (distance(projectile.x, projectile.y, player.x, player.y) < 1) {
                    player.health -= projectile.damage;
                    state.projectiles.delete(pid);
                    player.invincibleUntil = Date.now() + PLAYER_INVINCIBILITY_TIME;
                }
            });
        }

        // Player vs Enemy contact
        if (!player.invincible) {
            state.enemies.forEach((enemy, eid) => {
                if (distance(player.x, player.y, enemy.x, enemy.y) < 1.5) {
                    player.health -= 20;
                    player.invincibleUntil = Date.now() + PLAYER_INVINCIBILITY_TIME;

                    // Push player back
                    const dx = player.x - enemy.x;
                    const dy = player.y - enemy.y;
                    const dist = Math.sqrt(dx * dx + dy * dy) || 1;
                    player.vx = (dx / dist) * 20;
                    player.vy = (dy / dist) * 20;
                }
            });
        }
    }

    function handleDeaths(state) {
        const toRemove = [];

        // Check enemy deaths
        state.enemies.forEach(function (enemy, id) {
            if (enemy.health <= 0) {
                toRemove.push(id);
                state.score += 100;
                state.particles.push(...createExplosion(enemy.x, enemy.y, EXPLOSION_PARTICLE_COUNT));

                // Stop ticker - ensure graceful cleanup on error
                if (enemy.ticker) {
                    try {
                        enemy.ticker.stop();
                    } catch (e) {
                        console.error('Error stopping enemy #' + id + ' ticker during death handling: ' + e.message);
                    }
                }
            }
        });

        toRemove.forEach(function (id) {
            state.enemies.delete(id);
        });

        // Check player death
        if (state.player.health <= 0) {
            state.lives--;
            if (state.lives <= 0) {
                state.gameMode = 'gameOver';
            } else {
                state.player = createPlayer();
            }
        }

        // Update wave state
        state.waveState.enemiesRemaining = state.enemies.size;
        if (state.waveState.inProgress && state.enemies.size === 0) {
            state.waveState.complete = true;
        }
    }

    function spawnWave(state) {
        const waveIndex = state.wave - 1;
        // console.log('spawnWave called: waveIndex=' + waveIndex + ', WAVES.length=' + WAVES.length);
        if (waveIndex < 0 || waveIndex >= WAVES.length) {
            // console.log('spawnWave: invalid wave index');
            return;
        }

        const waveConfig = WAVES[waveIndex];
        // console.log('spawnWave: waveConfig=' + JSON.stringify(waveConfig));
        let count = 0;

        waveConfig.forEach(function (config) {
            for (let i = 0; i < config.count; i++) {
                try {
                    const id = ++state.nextEntityId;
                    // console.log('Creating enemy type=' + config.type + ', id=' + id);
                    const enemy = createEnemy(config.type, id);
                    state.enemies.set(id, enemy);
                    count++;
                    // console.log('Enemy created successfully');
                } catch (e) {
                    console.error('Error spawning enemy type "' + config.type + '": ' + e.message);
                    console.error(e.stack);
                }
            }
        });

        // console.log('spawnWave: spawned ' + count + ' enemies');
        state.waveState.inProgress = true;
        state.waveState.enemiesSpawned = count;
        state.waveState.enemiesRemaining = count;
    }

    function nextWave(state) {
        if (state.wave >= WAVES.length) {
            state.gameMode = 'victory';
        } else {
            state.wave++;
            state.waveState = {
                inProgress: false,
                enemiesSpawned: 0,
                enemiesRemaining: 0,
                complete: false
            };
            spawnWave(state);
        }
    }

// ============================================================================
// Rendering Functions (Lines 701-850)
// ============================================================================

// OPTIMIZATION: Pre-allocate render buffer to avoid per-frame allocations.
// This is CRITICAL for performance - the original implementation allocated
// 2000+ objects per frame and used O(n²) string concatenation.
    let _renderBuffer = null;
    let _renderBufferWidth = 0;
    let _renderBufferHeight = 0;

// Get or create render buffer for given dimensions
    function getRenderBuffer(width, height) {
        if (_renderBuffer === null || _renderBufferWidth !== width || _renderBufferHeight !== height) {
            // Only reallocate if dimensions changed
            _renderBufferWidth = width;
            _renderBufferHeight = height;
            // Use 1D array of chars (much faster than 2D array of objects)
            _renderBuffer = new Array(width * height);
            for (let i = 0; i < _renderBuffer.length; i++) {
                _renderBuffer[i] = ' ';
            }
        }
        return _renderBuffer;
    }

// Fast buffer access helpers
    function bufferIndex(x, y, width) {
        return y * width + x;
    }

    function clearBuffer(buffer, width, height) {
        for (let i = 0; i < buffer.length; i++) {
            buffer[i] = ' ';
        }
    }

    function renderPlayArea(state) {
        const width = state.terminalSize && state.terminalSize.width ? state.terminalSize.width : SCREEN_WIDTH;
        const height = state.terminalSize && state.terminalSize.height ? state.terminalSize.height : SCREEN_HEIGHT;

        // Get pre-allocated buffer (or create if first call/resize)
        const buffer = getRenderBuffer(width, height);
        clearBuffer(buffer, width, height);

        // Render play area boundaries using direct index access
        for (let x = 0; x < width; x++) {
            buffer[bufferIndex(x, 1, width)] = '═';
            buffer[bufferIndex(x, height - 2, width)] = '═';
        }
        for (let y = 1; y < height - 1; y++) {
            buffer[bufferIndex(0, y, width)] = '║';
            buffer[bufferIndex(width - 1, y, width)] = '║';
        }
        buffer[bufferIndex(0, 1, width)] = '╔';
        buffer[bufferIndex(width - 1, 1, width)] = '╗';
        buffer[bufferIndex(0, height - 2, width)] = '╚';
        buffer[bufferIndex(width - 1, height - 2, width)] = '╝';

        // Render particles
        state.particles.forEach(particle => {
            const x = Math.floor(particle.x);
            const y = Math.floor(particle.y);
            if (y > 1 && y < height - 2 && x > 0 && x < width - 1) {
                buffer[bufferIndex(x, y, width)] = particle.char;
            }
        });

        // Render projectiles
        state.projectiles.forEach(projectile => {
            const x = Math.floor(projectile.x);
            const y = Math.floor(projectile.y);
            if (y > 1 && y < height - 2 && x > 0 && x < width - 1) {
                buffer[bufferIndex(x, y, width)] = projectile.sprite;
            }
        });

        // Render enemies with special visual states
        state.enemies.forEach(enemy => {
            const x = Math.floor(enemy.x);
            const y = Math.floor(enemy.y);
            if (y > 1 && y < height - 2 && x > 0 && x < width - 1) {
                // Pursuer telegraph visual: flash when about to dash
                if (enemy.type === 'pursuer' && enemy.blackboard.get('telegraphing')) {
                    // Rapid flash effect (! symbol as warning)
                    buffer[bufferIndex(x, y, width)] = Math.floor(Date.now() / 100) % 2 === 0 ? '!' : enemy.sprite;
                } else if (enemy.blackboard.get('dashing')) {
                    // Dashing visual: show dash trail
                    buffer[bufferIndex(x, y, width)] = '»';
                } else {
                    buffer[bufferIndex(x, y, width)] = enemy.sprite;
                }
            }
        });

        // Render player
        if (state.player && !state.player.invincible || Math.floor(Date.now() / 100) % 2 === 0) {
            const px = Math.floor(state.player.x);
            const py = Math.floor(state.player.y);
            if (py > 1 && py < height - 2 && px > 0 && px < width - 1) {
                buffer[bufferIndex(px, py, width)] = state.player.sprite;
            }
        }

        // OPTIMIZATION: Build output using row arrays and join (O(n) instead of O(n²))
        const rows = [];
        for (let y = 0; y < height; y++) {
            const rowStart = y * width;
            // Slice out the row and join (much faster than += for each char)
            rows.push(buffer.slice(rowStart, rowStart + width).join(''));
        }
        return rows.join('\n');
    }

    function renderHUD(state) {
        const width = state.terminalSize && state.terminalSize.width ? state.terminalSize.width : SCREEN_WIDTH;
        // CRITICAL: Clamp all values to prevent RangeError in repeat() when player is dead/dying
        const healthPercent = Math.max(0, Math.min(100, Math.floor((state.player.health / state.player.maxHealth) * 100)));
        const barLength = Math.max(0, Math.min(10, Math.floor(healthPercent / 10)));
        const healthBar = '[' + '█'.repeat(barLength) + '░'.repeat(10 - barLength) + '] ' + healthPercent + '%';
        // Clamp lives to prevent negative repeat count
        const livesDisplay = Math.max(0, state.lives);

        let header = 'Score: ' + state.score + ' | Wave: ' + state.wave + '/' + WAVES.length +
            ' | Lives: ' + '♥'.repeat(livesDisplay);
        header += ' '.repeat(width - header.length - healthBar.length) + healthBar;

        const footer = 'WASD: Move | SPACE: Shoot | P: Pause | Q: Quit';

        return {header, footer};
    }

    function renderModal(state) {
        // Defensive check for state
        if (!state) {
            return 'State is undefined';
        }

        const width = state.terminalSize && state.terminalSize.width ? state.terminalSize.width : SCREEN_WIDTH;
        const height = state.terminalSize && state.terminalSize.height ? state.terminalSize.height : SCREEN_HEIGHT;
        const gameMode = state.gameMode || 'menu'; // Default to menu if undefined

        let title = '';
        let message = '';
        let prompt = '';

        switch (gameMode) {
            case 'menu':
                title = '=== BT SHOOTER ===';
                message = 'A behavior tree powered shooter game';
                prompt = 'Press SPACE to start';
                break;
            case 'paused':
                title = '=== PAUSED ===';
                message = 'Game is paused';
                prompt = 'Press P to resume | Q to quit';
                break;
            case 'gameOver':
                title = '=== GAME OVER ===';
                message = 'Final Score: ' + (state.score || 0);
                prompt = 'Press R to restart | Q to quit';
                break;
            case 'victory':
                title = '=== VICTORY! ===';
                message = 'All waves cleared! Final Score: ' + (state.score || 0);
                prompt = 'Press R to play again | Q to quit';
                break;
            default:
                title = '=== BT SHOOTER ===';
                message = 'Mode: ' + gameMode;
                prompt = 'Press Q to quit';
        }

        const modalWidth = Math.max(title.length, message.length, prompt.length) + 4;
        const modalX = Math.floor((width - modalWidth) / 2);
        const modalY = Math.floor((height - 5) / 2);

        // Center text in modal
        const centerText = (text, modalW) => {
            const padding = Math.floor((modalW - text.length) / 2);
            return ' '.repeat(padding) + text + ' '.repeat(modalW - text.length - padding);
        };

        // OPTIMIZATION: Use array and join instead of += string concatenation
        const lines = [];
        for (let y = 0; y < height; y++) {
            if (y === modalY) {
                lines.push(' '.repeat(modalX) + '╔' + '═'.repeat(modalWidth - 2) + '╗');
            } else if (y === modalY + 1) {
                lines.push(' '.repeat(modalX) + '║' + centerText(title, modalWidth - 2) + '║');
            } else if (y === modalY + 2) {
                lines.push(' '.repeat(modalX) + '║' + centerText(message, modalWidth - 2) + '║');
            } else if (y === modalY + 3) {
                lines.push(' '.repeat(modalX) + '║' + centerText(prompt, modalWidth - 2) + '║');
            } else if (y === modalY + 4) {
                lines.push(' '.repeat(modalX) + '╚' + '═'.repeat(modalWidth - 2) + '╝');
            } else {
                lines.push(' '.repeat(width));
            }
        }

        return lines.join('\n');
    }

    function renderDebugInfo(state) {
        let output = '\n\n=== DEBUG MODE ===\n';
        output += 'Tick: ' + state.tick + '\n';
        output += 'Enemies: ' + state.enemies.size + '\n';
        output += 'Projectiles: ' + state.projectiles.size + '\n';

        state.enemies.forEach((enemy, id) => {
            const dist = distance(enemy.x, enemy.y, state.player.x, state.player.y);
            const lastShot = enemy.blackboard.get('lastShotTime');
            const timeSinceShot = lastShot ? Math.floor((Date.now() - lastShot) / 1000) + 's' : 'N/A';

            output += `Enemy #${id} [${enemy.type}]: ` +
                `pos=(${enemy.x.toFixed(1)}, ${enemy.y.toFixed(1)}), ` +
                `state=${enemy.state}, ` +
                `health=${enemy.health}, ` +
                `dist=${dist.toFixed(1)}, ` +
                `lastShot=${timeSinceShot} ago\n`;
        });

        // Output parseable JSON for E2E test harness
        // Split across lines so terminal wrapping doesn't break the markers
        const debugJson = getDebugOverlayJSON(state);
        output += '\n__JSON_START__\n' + debugJson + '\n__JSON_END__\n';

        return output;
    }

// Generate structured JSON for E2E test harness scraping
// NOTE: Keep this VERY SHORT to avoid terminal line-wrapping truncation!
// Terminal is typically 80 chars wide. JSON must be < 80 chars.
    function getDebugOverlayJSON(state) {
        // Get first enemy position for movement verification tests
        let ex = -1, ey = -1;
        const enemyIter = state.enemies.values().next();
        if (!enemyIter.done && enemyIter.value) {
            ex = Math.round(enemyIter.value.x);
            ey = Math.round(enemyIter.value.y);
        }

        // ULTRA-compact: Use single-char keys, minimal values
        // ~70 chars: {"m":"p","t":123,"w":1,"e":3,"p":0,"x":40,"y":21,"a":37,"b":2}
        return JSON.stringify({
            m: state.gameMode === 'playing' ? 'p' : state.gameMode.charAt(0),
            t: state.tick,
            w: state.wave,
            e: state.enemies.size,
            p: state.projectiles.size,
            x: Math.round(state.player.x),  // player x
            y: Math.round(state.player.y),  // player y
            a: ex,  // first enemy x
            b: ey   // first enemy y
        });
    }

    function view(state) {
        // Handle all menu-like modes where player may not be active
        if (state.gameMode === 'menu') {
            return renderModal(state);
        }

        // Defensive check: if player is not initialized, return modal
        if (!state || !state.player) {
            return renderModal(state);
        }

        const hud = renderHUD(state);
        const playArea = renderPlayArea(state);
        const lines = playArea.split('\n');

        lines[0] = hud.header;
        lines[lines.length - 1] = hud.footer;

        let output = lines.join('\n');

        if (state.gameMode !== 'playing') {
            output = playArea.split('\n').map((line, y) => {
                const modal = renderModal(state).split('\n');
                if (y < modal.length) {
                    const terminalWidth = state.terminalSize && state.terminalSize.width ? state.terminalSize.width : SCREEN_WIDTH;
                    return line.substring(0, Math.floor((terminalWidth - modal[y].length) / 2)) + modal[y];
                }
                return line;
            }).join('\n');
        }

        if (state.debugMode) {
            output += renderDebugInfo(state);
        }

        return output;
    }

// ============================================================================
// Bubbletea Model (Lines 851-1000)
// ============================================================================

    function init() {
        // Return [state, cmd] like update() does - the Go binding now supports this
        return [initializeGame(), tea.tick(16, 'tick')];
    }

    function update(state, msg) {
        if (msg.type === 'Tick' && msg.id === 'tick') {
            // Calculate delta time
            const now = Date.now();
            state.deltaTime = now - state.lastTickTime;
            state.lastTickTime = now;

            // Increment tick counter for test synchronization
            state.tick++;

            if (state.gameMode === 'playing') {
                // Update player
                updatePlayer(state);

                // Sync state with enemy blackboards
                syncToBlackboards(state);
                syncFromBlackboards(state);

                // Update enemies
                updateEnemies(state);

                // Update projectiles
                updateProjectiles(state);

                // Update particles
                updateParticles(state);

                // Check collisions
                checkCollisions(state);

                // Handle deaths
                handleDeaths(state);

                // Check wave completion
                if (state.waveState.complete) {
                    nextWave(state);
                }
            }

            return [state, tea.tick(16, 'tick')];
        }

        if (msg.type === 'Key') {
            // Defensive check: ensure state is valid
            if (!state) {
                return [state, tea.tick(16, 'tick')];
            }

            switch (state.gameMode) {
                case 'menu':
                    if (msg.key === ' ') {
                        state.wave = 1;
                        spawnWave(state);
                        state.gameMode = 'playing';
                    } else if (msg.key === 'q') {
                        return [state, tea.quit()];
                    }
                    break;

                case 'playing':
                    // Ensure player exists before handling input
                    if (!state.player) {
                        return [state, tea.tick(16, 'tick')];
                    }

                    // DIRECT POSITION MOVEMENT - terminals don't reliably send key-repeat
                    // Each keypress moves player by 1 cell immediately AND updates facing direction
                    switch (msg.key) {
                        case 'w':
                        case 'up':
                            state.player.y = clamp(state.player.y - 1, 1, state.terminalSize.height - 4);
                            state.player.facing = 'up';
                            state.player.sprite = getPlayerSprite('up');
                            break;
                        case 's':
                        case 'down':
                            state.player.y = clamp(state.player.y + 1, 1, state.terminalSize.height - 4);
                            state.player.facing = 'down';
                            state.player.sprite = getPlayerSprite('down');
                            break;
                        case 'a':
                        case 'left':
                            state.player.x = clamp(state.player.x - 1, 1, state.terminalSize.width - 2);
                            state.player.facing = 'left';
                            state.player.sprite = getPlayerSprite('left');
                            break;
                        case 'd':
                        case 'right':
                            state.player.x = clamp(state.player.x + 1, 1, state.terminalSize.width - 2);
                            state.player.facing = 'right';
                            state.player.sprite = getPlayerSprite('right');
                            break;
                        case ' ':
                            const now = Date.now();
                            if (now - state.player.lastShotTime >= state.player.shotCooldown) {
                                // Get projectile velocity based on player's facing direction
                                const vel = getProjectileVelocity(state.player.facing, PROJECTILE_SPEED);
                                const projectile = createProjectile(
                                    ++state.nextProjectileId,
                                    'player',
                                    0,
                                    state.player.x,
                                    state.player.y,
                                    vel.vx,
                                    vel.vy,
                                    PLAYER_PROJECTILE_DAMAGE,
                                    PROJECTILE_SPEED
                                );
                                state.projectiles.set(projectile.id, projectile);
                                state.player.lastShotTime = now;
                            }
                            break;
                        case 'p':
                            state.gameMode = 'paused';
                            break;
                        case 'q':
                            return [state, tea.quit()];
                        case '`':
                        case 'f3':
                            state.debugMode = !state.debugMode;
                            break;
                    }
                    break;

                case 'paused':
                    if (msg.key === 'p') {
                        state.gameMode = 'playing';
                    } else if (msg.key === 'q') {
                        return [state, tea.quit()];
                    }
                    break;

                case 'gameOver':
                case 'victory':
                    if (msg.key === 'r') {
                        return initializeGame();
                    } else if (msg.key === 'q') {
                        return [state, tea.quit()];
                    }
                    break;
            }
        }

        if (msg.type === 'Resize') {
            state.terminalSize = {width: msg.width, height: msg.height};
            if (state.player) {
                state.player.x = Math.min(state.player.x, msg.width - 2);
                state.player.y = Math.min(state.player.y, msg.height - 4);
            }
        }

        return [state, tea.tick(16, 'tick')];
    }

// ============================================================================
// Entry Point (Lines 1001-1020)
// ============================================================================

// Global cleanup function to ensure tickers are stopped on any error
    function cleanupTickers(state) {
        if (state && state.enemies) {
            state.enemies.forEach(function (enemy, id) {
                if (enemy.ticker) {
                    try {
                        enemy.ticker.stop();
                    } catch (e) {
                        console.error('Error stopping enemy #' + id + ' ticker: ' + e.message);
                    }
                }
            });
        }
    }

    const program = tea.newModel({
        init: function () {
            return init();
        },
        update: function (msg, model) {
            return update(model, msg);
        },
        view: function (model) {
            return view(model);
        },
        // Enable render throttling to improve input responsiveness
        // This caches view output and only re-renders after 16ms (60fps)
        // Tick and WindowSize messages always trigger immediate re-render
        renderThrottle: {
            enabled: true,
            minIntervalMs: 16,  // ~60fps cap
            alwaysRenderMsgTypes: ["Tick", "WindowSize"]
        }
    });

// Error tracking for exit code
    var gameError = null;

// Wrap game execution with try/catch for runtime error handling
    try {
        console.log('');
        console.log('═══════════════════════════════════════════════════════════════');
        console.log('                    BT SHOOTER GAME STARTING                          ');
        console.log('═══════════════════════════════════════════════════════════════');
        console.log('');
        console.log('CONTROLS:');
        console.log('  WASD / Arrow Keys: Move player');
        console.log('  SPACE: Fire projectile');
        console.log('  P: Pause game');
        console.log('  D: Toggle debug mode');
        console.log('  Q: Quit game');
        console.log('');
        console.log('Press SPACE to begin!');
        console.log('');

        // Run the game
        tea.run(program, {altScreen: true});

        // Log quit confirmation when game exits cleanly
        if (!gameError) {
            console.log('');
            console.log('Game quit successfully.');
        }
    } catch (e) {
        gameError = e;
        console.error('');
        console.error('FATAL ERROR: ' + e.message);
        console.error('Stack trace: ' + e.stack);

        // Attempt to clean up tickers before re-throwing
        try {
            cleanupTickers(program.init()[0]);
        } catch (cleanupError) {
            console.error('Error during cleanup: ' + cleanupError.message);
        }

        // Re-throw to trigger Go-level error handling (non-zero exit code)
        throw e;
    }

} catch (e) {
    // Top-level error handler - catches module loading errors and runtime errors
    // This ensures any error triggers a non-zero exit code via Go's panic recovery
    console.error('');
    console.error('========================================');
    console.error('GAME STARTUP FAILED');
    console.error('========================================');
    console.error('Error: ' + e.message);
    if (e.stack) {
        console.error('Stack: ' + e.stack);
    }
    console.error('');
    console.error('If this is a module loading error, ensure you are running:');
    console.error('  osm script scripts/example-04-bt-shooter.js');
    console.error('========================================');

    // Throw to trigger Go's panic recovery in engine.ExecuteScript()
    // This results in a non-zero exit code (1) being returned
    throw e;
}
