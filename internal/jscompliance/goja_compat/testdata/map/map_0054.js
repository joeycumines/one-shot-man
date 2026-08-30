/*---
description: goja compat map 54
includes: [assert.js]
---*/
var m=new Map(); m.set('k',54); assert.sameValue(m.get('k'),54,'map 54');
