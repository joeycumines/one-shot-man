/*---
description: goja compat map 53
includes: [assert.js]
---*/
var m=new Map(); m.set('k',53); assert.sameValue(m.get('k'),53,'map 53');
