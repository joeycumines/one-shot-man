/*---
description: goja compat map 32
includes: [assert.js]
---*/
var m=new Map(); m.set('k',32); assert.sameValue(m.get('k'),32,'map 32');
