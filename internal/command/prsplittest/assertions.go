package prsplittest

// NumVal safely extracts a numeric value from Goja results which may return
// int64 or float64 depending on the JS value. Returns float64 for comparison.
func NumVal(v any) float64 {
	switch n := v.(type) {
	case float64:
		return n
	case int64:
		return float64(n)
	case int:
		return float64(n)
	default:
		return 0
	}
}

// GitMockSetupJS returns JS code that installs a git-focused exec mock.
// The mock matches git subcommands by stripping 'git' and optional '-C dir'
// prefixes, then looking up responses by the remaining args joined with space.
//
// Mock state globals:
//
//	_gitCalls      — array of {argv:[...]} records
//	_gitResponses  — map of subcommand key → response object
//	_gitOk(stdout) — helper: success response
//	_gitFail(stderr) — helper: failure response
//
// Non-git commands (e.g. sh -c from verifySplit) are matched via a
// "!sh" prefix key in _gitResponses.
func GitMockSetupJS() string {
	return `(function() {
    var execMod = require('osm:exec');
    globalThis._gitCalls = [];
    globalThis._gitResponses = {};

    function ok(stdout) {
        return {stdout: stdout || '', stderr: '', code: 0, error: false, message: ''};
    }
    function fail(stderr) {
        return {stdout: '', stderr: stderr || 'error', code: 1, error: true, message: stderr || 'error'};
    }
    globalThis._gitOk = ok;
    globalThis._gitFail = fail;

    execMod.execv = function(argv) {
        globalThis._gitCalls.push({argv: argv.slice()});

        // 'test' command: check _testFileExists set for T24 tests.
        if (argv[0] === 'test' && argv[1] === '-f') {
            var path = argv[2] || '';
            if (globalThis._testFileExists && globalThis._testFileExists[path]) {
                return ok('');
            }
            return fail('');
        }

        // Non-git commands: route via '!' + argv[0] key (e.g. '!sh', '!which').
        if (argv[0] !== 'git') {
            var nonGitKey = '!' + argv[0];
            if (globalThis._gitResponses[nonGitKey] !== undefined) {
                var r = globalThis._gitResponses[nonGitKey];
                if (typeof r === 'function') return r(argv);
                return r;
            }
            return ok('');
        }

        // Strip 'git' and optional '-C dir'.
        var args = argv.slice(1);
        if (args.length >= 2 && args[0] === '-C') args = args.slice(2);

        // Exact match.
        var key = args.join(' ');
        if (globalThis._gitResponses[key] !== undefined) {
            var r = globalThis._gitResponses[key];
            if (typeof r === 'function') return r(argv);
            return r;
        }

        // Prefix matching (longest first).
        for (var i = args.length - 1; i >= 1; i--) {
            var prefix = args.slice(0, i).join(' ');
            if (globalThis._gitResponses[prefix] !== undefined) {
                var r = globalThis._gitResponses[prefix];
                if (typeof r === 'function') return r(argv);
                return r;
            }
        }

        // Default: success with empty output.
        return ok('');
    };

    // execStream delegates to the execv mock but adapts the interface.
    execMod.execStream = function(argv, opts) {
        var r = execMod.execv(argv);
        opts = opts || {};
        if (opts.onStdout && r.stdout) opts.onStdout(r.stdout);
        if (opts.onStderr && r.stderr) opts.onStderr(r.stderr);
        return {code: r.code, error: r.error, message: r.message};
    };

    // Mock exec.spawn to route through the same mock dispatcher as execv.
    execMod.spawn = function(cmd, args) {
        var fullArgv = [cmd].concat(args || []);
        var r = execMod.execv(fullArgv);
        var stdoutRead = false;
        var stderrRead = false;
        return {
            stdout: {
                read: function() {
                    if (!stdoutRead) {
                        stdoutRead = true;
                        if (r.stdout) return Promise.resolve({done: false, value: r.stdout});
                    }
                    return Promise.resolve({done: true});
                }
            },
            stderr: {
                read: function() {
                    if (!stderrRead) {
                        stderrRead = true;
                        if (r.stderr) return Promise.resolve({done: false, value: r.stderr});
                    }
                    return Promise.resolve({done: true});
                }
            },
            wait: function() { return Promise.resolve({code: r.code}); },
            kill: function() {}
        };
    };
})();`
}
