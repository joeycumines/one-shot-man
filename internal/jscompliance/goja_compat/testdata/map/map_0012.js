/*---
description: goja compat map 12
includes: [assert.js]
---*/
var m=new Map(); m.set('k',12); assert.sameValue(m.get('k'),12,'map 12');
