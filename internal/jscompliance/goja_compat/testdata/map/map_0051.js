/*---
description: goja compat map 51
includes: [assert.js]
---*/
var m=new Map(); m.set('k',51); assert.sameValue(m.get('k'),51,'map 51');
