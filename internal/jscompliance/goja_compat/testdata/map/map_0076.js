/*---
description: goja compat map 76
includes: [assert.js]
---*/
var m=new Map(); m.set('k',76); assert.sameValue(m.get('k'),76,'map 76');
