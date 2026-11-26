package command

import (
	"fmt"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/command/prsplittest"
)

func TestVerificationHelpersRejectIncompleteNativeSuccess(t *testing.T) {
	if testing.Short() {
		t.Skip("creates a JavaScript engine")
	}
	eval := prsplittest.NewChunkEngine(t, nil, "00_core", "06_verification")
	for _, async := range []bool{false, true} {
		for _, operation := range []string{"tree", "diff", "cleanup_checkout", "cleanup_delete", "setup", "verify"} {
			for _, fault := range []string{"tree", "capture", "tree_capture", "reject"} {
				t.Run(fmt.Sprintf("async=%t/%s/%s", async, operation, fault), func(t *testing.T) {
					got, err := eval(fmt.Sprintf(`(async function() {
						var asynchronous = %t, operation = %q, fault = %q;
						var exec = prSplit._modules.exec, oldSpawn = exec.spawn, oldExecv = exec.execv;
						var calls = [], failedIndex = -1, created = '';
						function nativeResult(command, args) {
							var phase = command === 'git' ? '' : 'verify', stdout = '';
							if (command === 'git') {
								if (args.indexOf('rev-parse') >= 0) {
									phase = 'tree'; stdout = args.join(' ').indexOf('source') >= 0 ? 'source-tree' : 'split-tree';
								} else if (args.indexOf('diff') >= 0) { phase = 'diff'; stdout = 'a.go'; }
								else if (args.indexOf('checkout') >= 0) phase = 'cleanup_checkout';
								else if (args.indexOf('branch') >= 0) phase = 'cleanup_delete';
								else if (args.indexOf('worktree') >= 0) {
									if (args.indexOf('add') >= 0) { phase = 'setup'; created = args[args.length - 2]; }
									else phase = 'remove';
								}
							}
							calls.push(phase);
							var hit = phase === operation && failedIndex < 0;
							if (hit) {
								failedIndex = calls.length - 1;
								if (fault === 'reject') throw Object.assign(new Error('native rollback failed'), {treeCleanupError:'tree close denied'});
							}
							return {code:0, stdout:stdout, stderr:'', error:hit,
								message:hit ? 'native capture or cleanup failed' : '',
								truncated:hit && (fault === 'capture' || fault === 'tree_capture'),
								treeCleanupError:hit && (fault === 'tree' || fault === 'tree_capture') ? 'tree close denied' : undefined};
						}
						exec.execv = async function(argv) { return nativeResult(argv[0], argv.slice(1)); };
						exec.spawn = async function(command, args) {
							var result = nativeResult(command, args);
							function stream(text) {
								return {read:async function() {
									if (text) { var value = text; text = ''; return {value:value, done:false}; }
									return {done:true, truncated:result.truncated};
								}};
							}
							return {stdout:stream(result.stdout), stderr:stream(''),
								wait:async function() { return {code:0, signal:null, treeCleanupError:result.treeCleanupError}; },
								kill:async function() {}};
						};
						try {
							var suffix = asynchronous ? 'Async' : '';
							var plan = {dir:'.', baseBranch:'main', sourceBranch:'source', splits:[{name:'split/1'}, {name:'split/2'}]};
							var result;
							if (operation === 'tree' || operation === 'diff') {
								var fn = operation === 'diff' ? 'verifyEquivalenceDetailed' : 'verifyEquivalence';
								result = await prSplit[fn + suffix](plan);
								if (result.equivalent || !(result.error || result.diffError)) return 'failed query produced successful evidence: ' + JSON.stringify(result);
							} else if (operation.indexOf('cleanup_') === 0) {
								result = await prSplit['cleanupBranches' + suffix](plan);
								if (!result.errors.length || result.deleted.length || calls.length !== failedIndex + 1) return 'cleanup continued after unconfirmed command: ' + JSON.stringify({result:result, calls:calls});
							} else {
								result = await prSplit['verifySplit' + suffix]('split/1', {dir:'.', verifyCommand:'verify-command'});
								if (result.passed || !result.error) return 'unknown setup/verification passed';
								if (operation === 'setup' && calls.indexOf('verify') >= 0) return 'verification followed failed setup';
								if (fault !== 'capture') {
									if (calls.indexOf('remove') >= 0 || !result.worktreeRetained || result.worktreeDir !== created) return 'known live tree lost worktree ownership';
								} else if (calls.filter(function(phase) { return phase === 'remove'; }).length !== 1) return 'safe cleanup was not attempted once';
								if (fault === 'tree_capture' && !result.truncated) return 'tree failure hid incomplete output capture';
							}
							if (failedIndex < 0) return 'fault was never exercised';
							return 'OK';
						} finally { exec.spawn = oldSpawn; exec.execv = oldExecv; }
					})()`, async, operation, fault))
					if err != nil {
						t.Fatal(err)
					}
					if got != "OK" {
						t.Fatal(got)
					}
				})
			}
		}
	}
}
