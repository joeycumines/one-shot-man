/*---
description: goja compat map 68
includes: [assert.js]
---*/
var m=new Map(); m.set('k',68); assert.sameValue(m.get('k'),68,'map 68');
