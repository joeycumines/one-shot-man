/*---
description: goja compat map 5
includes: [assert.js]
---*/
var m=new Map(); m.set('k',5); assert.sameValue(m.get('k'),5,'map 5');
