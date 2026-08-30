/*---
description: goja compat map 10
includes: [assert.js]
---*/
var m=new Map(); m.set('k',10); assert.sameValue(m.get('k'),10,'map 10');
