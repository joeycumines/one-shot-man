/*---
description: goja compat map 20
includes: [assert.js]
---*/
var m=new Map(); m.set('k',20); assert.sameValue(m.get('k'),20,'map 20');
