/*---
description: Synthetic test262 case 175 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(175,175), 350, 'fn 175');
