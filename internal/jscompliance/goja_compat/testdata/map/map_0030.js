/*---
description: goja compat map 30
includes: [assert.js]
---*/
var m=new Map(); m.set('k',30); assert.sameValue(m.get('k'),30,'map 30');
