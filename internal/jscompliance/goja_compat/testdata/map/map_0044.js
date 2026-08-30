/*---
description: goja compat map 44
includes: [assert.js]
---*/
var m=new Map(); m.set('k',44); assert.sameValue(m.get('k'),44,'map 44');
