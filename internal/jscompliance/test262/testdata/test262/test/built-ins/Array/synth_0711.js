/*---
description: Synthetic test262 case 711 - ES5.1 baseline
includes: [assert.js]
flags: []
---*/
assert.sameValue((function(a,b){return a+b})(711,711), 1422, 'fn 711');
