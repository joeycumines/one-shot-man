/*---
description: goja compat map 28
includes: [assert.js]
---*/
var m=new Map(); m.set('k',28); assert.sameValue(m.get('k'),28,'map 28');
