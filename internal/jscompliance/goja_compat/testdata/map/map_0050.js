/*---
description: goja compat map 50
includes: [assert.js]
---*/
var m=new Map(); m.set('k',50); assert.sameValue(m.get('k'),50,'map 50');
