/*---
description: Synthetic test262 case 575 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(575,575), 1150, 'fn 575');
