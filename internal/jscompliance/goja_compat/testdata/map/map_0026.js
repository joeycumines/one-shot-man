/*---
description: goja compat map 26
includes: [assert.js]
---*/
var m=new Map(); m.set('k',26); assert.sameValue(m.get('k'),26,'map 26');
