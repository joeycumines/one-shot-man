/*---
description: goja compat map 62
includes: [assert.js]
---*/
var m=new Map(); m.set('k',62); assert.sameValue(m.get('k'),62,'map 62');
