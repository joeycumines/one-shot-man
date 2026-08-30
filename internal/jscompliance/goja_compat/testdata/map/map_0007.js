/*---
description: goja compat map 7
includes: [assert.js]
---*/
var m=new Map(); m.set('k',7); assert.sameValue(m.get('k'),7,'map 7');
