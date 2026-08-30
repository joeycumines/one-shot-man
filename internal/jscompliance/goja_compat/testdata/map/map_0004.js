/*---
description: goja compat map 4
includes: [assert.js]
---*/
var m=new Map(); m.set('k',4); assert.sameValue(m.get('k'),4,'map 4');
