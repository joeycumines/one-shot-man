/*---
description: goja compat map 60
includes: [assert.js]
---*/
var m=new Map(); m.set('k',60); assert.sameValue(m.get('k'),60,'map 60');
