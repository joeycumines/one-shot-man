/*---
description: goja compat map 67
includes: [assert.js]
---*/
var m=new Map(); m.set('k',67); assert.sameValue(m.get('k'),67,'map 67');
