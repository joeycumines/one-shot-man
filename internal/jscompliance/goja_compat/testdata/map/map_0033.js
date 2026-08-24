/*---
description: goja compat map 33
includes: [assert.js]
---*/
var m=new Map(); m.set('k',33); assert.sameValue(m.get('k'),33,'map 33');
