/*---
description: goja compat map 25
includes: [assert.js]
---*/
var m=new Map(); m.set('k',25); assert.sameValue(m.get('k'),25,'map 25');
