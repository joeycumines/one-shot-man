/*---
description: goja compat map 37
includes: [assert.js]
---*/
var m=new Map(); m.set('k',37); assert.sameValue(m.get('k'),37,'map 37');
