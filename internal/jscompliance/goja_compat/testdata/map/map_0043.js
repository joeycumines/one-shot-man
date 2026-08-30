/*---
description: goja compat map 43
includes: [assert.js]
---*/
var m=new Map(); m.set('k',43); assert.sameValue(m.get('k'),43,'map 43');
