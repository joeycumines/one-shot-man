/*---
description: goja compat map 46
includes: [assert.js]
---*/
var m=new Map(); m.set('k',46); assert.sameValue(m.get('k'),46,'map 46');
