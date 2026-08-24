/*---
description: Synthetic test262 case 255 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(255,255), 510, 'fn 255');
