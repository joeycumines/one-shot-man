/*---
description: goja compat map 49
includes: [assert.js]
---*/
var m=new Map(); m.set('k',49); assert.sameValue(m.get('k'),49,'map 49');
