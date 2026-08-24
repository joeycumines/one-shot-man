/*---
description: goja compat map 34
includes: [assert.js]
---*/
var m=new Map(); m.set('k',34); assert.sameValue(m.get('k'),34,'map 34');
