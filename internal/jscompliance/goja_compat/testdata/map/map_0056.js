/*---
description: goja compat map 56
includes: [assert.js]
---*/
var m=new Map(); m.set('k',56); assert.sameValue(m.get('k'),56,'map 56');
