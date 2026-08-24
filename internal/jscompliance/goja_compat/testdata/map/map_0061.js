/*---
description: goja compat map 61
includes: [assert.js]
---*/
var m=new Map(); m.set('k',61); assert.sameValue(m.get('k'),61,'map 61');
