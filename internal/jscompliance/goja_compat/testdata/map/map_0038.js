/*---
description: goja compat map 38
includes: [assert.js]
---*/
var m=new Map(); m.set('k',38); assert.sameValue(m.get('k'),38,'map 38');
