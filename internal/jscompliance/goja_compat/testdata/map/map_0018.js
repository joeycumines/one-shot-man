/*---
description: goja compat map 18
includes: [assert.js]
---*/
var m=new Map(); m.set('k',18); assert.sameValue(m.get('k'),18,'map 18');
