/*---
description: Synthetic test262 case 743 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(743,743), 1486, 'fn 743');
