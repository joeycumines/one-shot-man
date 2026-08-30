/*---
description: goja compat map 2
includes: [assert.js]
---*/
var m=new Map(); m.set('k',2); assert.sameValue(m.get('k'),2,'map 2');
