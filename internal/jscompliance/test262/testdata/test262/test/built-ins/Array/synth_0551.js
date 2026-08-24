/*---
description: Synthetic test262 case 551 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(551,551), 1102, 'fn 551');
