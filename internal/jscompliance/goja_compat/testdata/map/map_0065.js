/*---
description: goja compat map 65
includes: [assert.js]
---*/
var m=new Map(); m.set('k',65); assert.sameValue(m.get('k'),65,'map 65');
