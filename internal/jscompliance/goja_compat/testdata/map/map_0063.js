/*---
description: goja compat map 63
includes: [assert.js]
---*/
var m=new Map(); m.set('k',63); assert.sameValue(m.get('k'),63,'map 63');
