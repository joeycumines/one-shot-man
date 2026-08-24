/*---
description: goja compat map 77
includes: [assert.js]
---*/
var m=new Map(); m.set('k',77); assert.sameValue(m.get('k'),77,'map 77');
