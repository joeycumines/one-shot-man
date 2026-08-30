/*---
description: goja compat map 39
includes: [assert.js]
---*/
var m=new Map(); m.set('k',39); assert.sameValue(m.get('k'),39,'map 39');
