/*---
description: goja compat map 24
includes: [assert.js]
---*/
var m=new Map(); m.set('k',24); assert.sameValue(m.get('k'),24,'map 24');
