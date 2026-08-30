/*---
description: goja compat map 13
includes: [assert.js]
---*/
var m=new Map(); m.set('k',13); assert.sameValue(m.get('k'),13,'map 13');
