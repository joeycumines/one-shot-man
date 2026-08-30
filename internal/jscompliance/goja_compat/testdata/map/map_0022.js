/*---
description: goja compat map 22
includes: [assert.js]
---*/
var m=new Map(); m.set('k',22); assert.sameValue(m.get('k'),22,'map 22');
