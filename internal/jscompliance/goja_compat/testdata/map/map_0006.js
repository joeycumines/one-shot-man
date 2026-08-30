/*---
description: goja compat map 6
includes: [assert.js]
---*/
var m=new Map(); m.set('k',6); assert.sameValue(m.get('k'),6,'map 6');
