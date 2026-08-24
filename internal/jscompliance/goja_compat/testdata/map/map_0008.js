/*---
description: goja compat map 8
includes: [assert.js]
---*/
var m=new Map(); m.set('k',8); assert.sameValue(m.get('k'),8,'map 8');
