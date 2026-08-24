/*---
description: goja compat map 11
includes: [assert.js]
---*/
var m=new Map(); m.set('k',11); assert.sameValue(m.get('k'),11,'map 11');
