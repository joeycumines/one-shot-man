/*---
description: goja compat map 57
includes: [assert.js]
---*/
var m=new Map(); m.set('k',57); assert.sameValue(m.get('k'),57,'map 57');
