// orchestrator.js — Reusable BT action templates for AI orchestration workflows.
//
// Usage:
//   var templates = require('./scripts/bt-templates/orchestrator.js');
//   var bb = new bt.Blackboard();
//   var node = templates.verifyOutput(bb, 'make test');
//   bt.tick(node);
//
// This module provides:
//   1. Leaf node factories — individual operations as bt.Node
//   2. Workflow composers — sequence common patterns
//   3. PA-BT action library — preconditions/effects for planning
//
// All leaf factories accept a bt.Blackboard as first argument for state sharing.
// Templates use bt.createBlockingLeafNode for sequential workflow semantics.

'use strict';

var bt = require('osm:bt');
var orc = require('osm:claudemux');
var exec = require('osm:exec');

// ---------------------------------------------------------------------------
//  Leaf Node Factories
// ---------------------------------------------------------------------------
// Each function returns a bt.Node that can be composed into larger trees via
// bt.node(bt.sequence, ...) or bt.node(bt.fallback, ...).

// spawnClaude creates a leaf that spawns a Claude Code agent via the provider
// registry and stores the handle on the blackboard.
//
// Blackboard writes:
//   agent         — AgentHandle for the spawned process
//   parser        — PTY output parser instance
//   agentSpawned  — true on success
//   lastError     — error message on failure
//
// Parameters:
//   bb           — bt.Blackboard instance
//   registry     — provider Registry from osm:orchestrator
//   providerName — provider name (default: 'claude-code')
//   spawnOpts    — optional SpawnOpts object {model, dir, rows, cols, env, args}
exports.spawnClaude = function(bb, registry, providerName, spawnOpts) {
    return bt.createBlockingLeafNode(function() {
        try {
            var handle = registry.spawn(providerName || 'claude-code', spawnOpts || {});
            bb.set('agent', handle);
            bb.set('agentSpawned', true);
            bb.set('parser', orc.newParser());
            return bt.success;
        } catch (e) {
            bb.set('lastError', String(e));
            return bt.failure;
        }
    });
};

// sendPrompt creates a leaf that sends a prompt string to the spawned agent.
//
// Blackboard reads:  agent
// Blackboard writes: promptSent (true), lastError
exports.sendPrompt = function(bb, prompt) {
    return bt.createBlockingLeafNode(function() {
        var agent = bb.get('agent');
        if (!agent) {
            bb.set('lastError', 'no agent spawned');
            return bt.failure;
        }
        try {
            agent.send(prompt + '\n');
            bb.set('promptSent', true);
            return bt.success;
        } catch (e) {
            bb.set('lastError', String(e));
            return bt.failure;
        }
    });
};

// waitForResponse creates a leaf that reads agent output, parses events, and
// waits for completion or agent exit.
//
// Security: permission prompts are automatically rejected (sends 'n').
//
// Blackboard reads:  agent, parser
// Blackboard writes: response, responseReceived (true), rateLimited, permissionRejected, lastError
//
// Parameters:
//   bb   — bt.Blackboard instance
//   opts — optional {maxEmptyReads: number} (default: 100)
exports.waitForResponse = function(bb, opts) {
    var maxEmptyReads = (opts && opts.maxEmptyReads) || 100;

    return bt.createBlockingLeafNode(function() {
        var agent = bb.get('agent');
        var parser = bb.get('parser');
        if (!agent || !parser) {
            bb.set('lastError', 'no agent or parser available');
            return bt.failure;
        }

        var output = [];
        var emptyCount = 0;

        while (agent.isAlive() && emptyCount < maxEmptyReads) {
            var data = agent.receive();
            if (data === '') {
                emptyCount++;
                continue;
            }
            emptyCount = 0;

            var lines = data.split('\n');
            for (var i = 0; i < lines.length; i++) {
                if (lines[i] === '') continue;
                output.push(lines[i]);

                var event = parser.parse(lines[i]);

                if (event.type === orc.EVENT_COMPLETION) {
                    bb.set('response', output.join('\n'));
                    bb.set('responseReceived', true);
                    return bt.success;
                }
                if (event.type === orc.EVENT_RATE_LIMIT) {
                    bb.set('rateLimited', true);
                    // Store partial output and signal rate limit
                    bb.set('response', output.join('\n'));
                    return bt.running;
                }
                if (event.type === orc.EVENT_PERMISSION) {
                    // Security: auto-reject permission prompts
                    try { agent.send('n\n'); } catch (e) { /* ignore send errors */ }
                    bb.set('permissionRejected', true);
                }
                if (event.type === orc.EVENT_ERROR) {
                    bb.set('lastError', lines[i]);
                }
            }
        }

        // Agent exited or max empty reads reached
        if (output.length > 0) {
            bb.set('response', output.join('\n'));
            bb.set('responseReceived', true);
            return bt.success;
        }
        bb.set('lastError', 'no output received from agent');
        return bt.failure;
    });
};

// verifyOutput creates a leaf that runs a shell command and checks its exit code.
//
// Blackboard writes: verifyCode, verifyStdout, verifyStderr, verified (true), lastError
exports.verifyOutput = function(bb, command) {
    return bt.createBlockingLeafNode(function() {
        var result = exec.exec('sh', '-c', command);
        bb.set('verifyCode', result.code);
        bb.set('verifyStdout', result.stdout);
        bb.set('verifyStderr', result.stderr);
        if (result.code === 0) {
            bb.set('verified', true);
            return bt.success;
        }
        bb.set('lastError', 'verify failed: exit ' + result.code);
        return bt.failure;
    });
};

// runTests creates a leaf that executes a test command.
//
// Blackboard writes: testCode, testStdout, testsPassed (true), lastError
exports.runTests = function(bb, command) {
    command = command || 'make test';
    return bt.createBlockingLeafNode(function() {
        var result = exec.exec('sh', '-c', command);
        bb.set('testCode', result.code);
        bb.set('testStdout', result.stdout);
        if (result.code === 0) {
            bb.set('testsPassed', true);
            return bt.success;
        }
        bb.set('lastError', 'tests failed: exit ' + result.code);
        return bt.failure;
    });
};

// commitChanges creates a leaf that stages all changes and commits.
//
// Blackboard writes: commitOutput, committed (true), lastError
exports.commitChanges = function(bb, message) {
    return bt.createBlockingLeafNode(function() {
        var addResult = exec.exec('git', 'add', '-A');
        if (addResult.code !== 0) {
            bb.set('lastError', 'git add failed: ' + addResult.stderr);
            return bt.failure;
        }
        var commitResult = exec.exec('git', 'commit', '-m', message);
        if (commitResult.code !== 0) {
            bb.set('lastError', 'git commit failed: ' + commitResult.stderr);
            return bt.failure;
        }
        bb.set('commitOutput', commitResult.stdout.trim());
        bb.set('committed', true);
        return bt.success;
    });
};

// splitBranch creates a leaf that creates and checks out a new git branch.
//
// Blackboard writes: currentBranch, branchCreated (true), lastError
exports.splitBranch = function(bb, branchName) {
    return bt.createBlockingLeafNode(function() {
        var result = exec.exec('git', 'checkout', '-b', branchName);
        if (result.code !== 0) {
            bb.set('lastError', 'branch creation failed: ' + result.stderr);
            return bt.failure;
        }
        bb.set('currentBranch', branchName);
        bb.set('branchCreated', true);
        return bt.success;
    });
};

// ---------------------------------------------------------------------------
//  Workflow Composers
// ---------------------------------------------------------------------------
// Higher-level functions that compose leaf templates into common patterns.

// spawnAndPrompt creates a sequence: spawn → send prompt → wait for response.
//
// Parameters:
//   bb       — bt.Blackboard
//   registry — provider Registry
//   config   — {provider?, spawnOpts?, prompt, maxEmptyReads?}
exports.spawnAndPrompt = function(bb, registry, config) {
    config = config || {};
    var provName = config.provider || 'claude-code';
    var spawnOpts = config.spawnOpts || {};
    var prompt = config.prompt || '';

    return bt.node(bt.sequence,
        exports.spawnClaude(bb, registry, provName, spawnOpts),
        exports.sendPrompt(bb, prompt),
        exports.waitForResponse(bb, config)
    );
};

// verifyAndCommit creates a sequence: run tests → [verify] → commit.
//
// Parameters:
//   bb   — bt.Blackboard
//   opts — {testCommand?, verifyCommand?, message}
exports.verifyAndCommit = function(bb, opts) {
    opts = opts || {};
    var testCmd = opts.testCommand || 'make test';
    var verifyCmd = opts.verifyCommand || null;
    var commitMsg = opts.message || 'Automated commit';

    if (verifyCmd) {
        return bt.node(bt.sequence,
            exports.runTests(bb, testCmd),
            exports.verifyOutput(bb, verifyCmd),
            exports.commitChanges(bb, commitMsg)
        );
    }
    return bt.node(bt.sequence,
        exports.runTests(bb, testCmd),
        exports.commitChanges(bb, commitMsg)
    );
};

// ---------------------------------------------------------------------------
//  PA-BT Action Library — Planning on Failure with Remediation
// ---------------------------------------------------------------------------
// Creates PA-BT action templates with preconditions and effects that enable
// the PABT planner to synthesize workflows automatically via backchaining.
//
// Usage:
//   var pabt = require('osm:pabt');
//   var actions = templates.createPlanningActions(pabt, bb, registry, config);
//   var state = pabt.newState(bb);
//   Object.keys(actions).forEach(function(name) {
//       state.registerAction(name, actions[name]);
//   });
//   var plan = pabt.newPlan(state, [{key: 'committed', match: function(v) { return v === true; }}]);
//
// The planner backchains from the goal:
//   committed=true → CommitChanges (needs testsPassed)
//     → RunTests (needs responseReceived)
//       → WaitForResponse (needs promptSent)
//         → SendPrompt (needs agentSpawned)
//           → SpawnClaude (no preconditions)
//
// If RunTests fails and codeReady stays false, the planner's PPA structure
// re-evaluates and can re-run the prompt→test cycle (remediation).

// createPlanningActions builds a map of PA-BT actions for orchestration.
//
// Parameters:
//   pabt     — the osm:pabt module
//   bb       — bt.Blackboard for state
//   registry — provider Registry
//   config   — {provider?, spawnOpts?, prompt?, testCommand?}
//
// Returns: object with named PA-BT action objects ready for state.registerAction()
exports.createPlanningActions = function(pabt, bb, registry, config) {
    config = config || {};
    var providerName = config.provider || 'claude-code';
    var testCommand = config.testCommand || 'make test';
    var prompt = config.prompt || '';

    return {
        SpawnClaude: pabt.newAction('SpawnClaude',
            [], // no preconditions — entry point
            [{key: 'agentSpawned', value: true}],
            bt.createBlockingLeafNode(function() {
                try {
                    var handle = registry.spawn(providerName, config.spawnOpts || {});
                    bb.set('agent', handle);
                    bb.set('agentSpawned', true);
                    bb.set('parser', orc.newParser());
                    return bt.success;
                } catch (e) {
                    bb.set('lastError', String(e));
                    return bt.failure;
                }
            })
        ),

        SendPrompt: pabt.newAction('SendPrompt',
            [{key: 'agentSpawned', match: function(v) { return v === true; }}],
            [{key: 'promptSent', value: true}],
            bt.createBlockingLeafNode(function() {
                var agent = bb.get('agent');
                if (!agent) {
                    bb.set('lastError', 'no agent spawned');
                    return bt.failure;
                }
                try {
                    agent.send(prompt + '\n');
                    bb.set('promptSent', true);
                    return bt.success;
                } catch (e) {
                    bb.set('lastError', String(e));
                    return bt.failure;
                }
            })
        ),

        WaitForResponse: pabt.newAction('WaitForResponse',
            [{key: 'promptSent', match: function(v) { return v === true; }}],
            [{key: 'responseReceived', value: true}],
            bt.createBlockingLeafNode(function() {
                var agent = bb.get('agent');
                var parser = bb.get('parser');
                if (!agent || !parser) {
                    bb.set('lastError', 'no agent or parser');
                    return bt.failure;
                }
                var output = [];
                var emptyCount = 0;
                var maxEmpty = 100;
                while (agent.isAlive() && emptyCount < maxEmpty) {
                    var data = agent.receive();
                    if (data === '') { emptyCount++; continue; }
                    emptyCount = 0;
                    var lines = data.split('\n');
                    for (var i = 0; i < lines.length; i++) {
                        if (lines[i] === '') continue;
                        output.push(lines[i]);
                        var event = parser.parse(lines[i]);
                        if (event.type === orc.EVENT_COMPLETION) {
                            bb.set('response', output.join('\n'));
                            bb.set('responseReceived', true);
                            return bt.success;
                        }
                        if (event.type === orc.EVENT_PERMISSION) {
                            try { agent.send('n\n'); } catch (e) { /* ignore */ }
                            bb.set('permissionRejected', true);
                        }
                    }
                }
                if (output.length > 0) {
                    bb.set('response', output.join('\n'));
                    bb.set('responseReceived', true);
                    return bt.success;
                }
                return bt.failure;
            })
        ),

        RunTests: pabt.newAction('RunTests',
            [{key: 'responseReceived', match: function(v) { return v === true; }}],
            [{key: 'testsPassed', value: true}],
            bt.createBlockingLeafNode(function() {
                var result = exec.exec('sh', '-c', testCommand);
                bb.set('testCode', result.code);
                bb.set('testStdout', result.stdout);
                if (result.code === 0) {
                    bb.set('testsPassed', true);
                    return bt.success;
                }
                bb.set('lastError', 'tests failed: exit ' + result.code);
                return bt.failure;
            })
        ),

        CommitChanges: pabt.newAction('CommitChanges',
            [{key: 'testsPassed', match: function(v) { return v === true; }}],
            [{key: 'committed', value: true}],
            bt.createBlockingLeafNode(function() {
                var addResult = exec.exec('git', 'add', '-A');
                if (addResult.code !== 0) {
                    bb.set('lastError', 'git add failed: ' + addResult.stderr);
                    return bt.failure;
                }
                var commitResult = exec.exec('git', 'commit', '-m',
                    config.commitMessage || 'Automated commit');
                if (commitResult.code !== 0) {
                    bb.set('lastError', 'git commit failed: ' + commitResult.stderr);
                    return bt.failure;
                }
                bb.set('commitOutput', commitResult.stdout.trim());
                bb.set('committed', true);
                return bt.success;
            })
        ),

        SplitBranch: pabt.newAction('SplitBranch',
            [{key: 'committed', match: function(v) { return v === true; }}],
            [{key: 'branchCreated', value: true}],
            bt.createBlockingLeafNode(function() {
                var branchName = bb.get('targetBranch') || config.branchName || 'split-branch';
                var result = exec.exec('git', 'checkout', '-b', branchName);
                if (result.code !== 0) {
                    bb.set('lastError', 'branch creation failed: ' + result.stderr);
                    return bt.failure;
                }
                bb.set('currentBranch', branchName);
                bb.set('branchCreated', true);
                return bt.success;
            })
        ),

        VerifyOutput: pabt.newAction('VerifyOutput',
            [{key: 'testsPassed', match: function(v) { return v === true; }}],
            [{key: 'verified', value: true}],
            bt.createBlockingLeafNode(function() {
                var command = config.verifyCommand || 'echo ok';
                var result = exec.exec('sh', '-c', command);
                bb.set('verifyCode', result.code);
                bb.set('verifyStdout', result.stdout);
                if (result.code === 0) {
                    bb.set('verified', true);
                    return bt.success;
                }
                bb.set('lastError', 'verify failed: exit ' + result.code);
                return bt.failure;
            })
        )
    };
};

// ---------------------------------------------------------------------------
//  Module version
// ---------------------------------------------------------------------------
exports.VERSION = '1.0.0';
