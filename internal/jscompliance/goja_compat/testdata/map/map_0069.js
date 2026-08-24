/*---
description: goja compat map 69
includes: [assert.js]
---*/
var m=new Map(); m.set('k',69); assert.sameValue(m.get('k'),69,'map 69');
