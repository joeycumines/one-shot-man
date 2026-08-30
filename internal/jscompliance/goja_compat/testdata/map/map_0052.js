/*---
description: goja compat map 52
includes: [assert.js]
---*/
var m=new Map(); m.set('k',52); assert.sameValue(m.get('k'),52,'map 52');
