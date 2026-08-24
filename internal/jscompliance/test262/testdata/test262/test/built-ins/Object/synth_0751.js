/*---
description: Synthetic test262 case 751 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(751,751), 1502, 'fn 751');
