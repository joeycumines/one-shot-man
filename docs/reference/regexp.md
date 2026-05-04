# osm:regexp — Go RE2 Regular Expressions

A native module exposing Go's `regexp` package (RE2 syntax) to JavaScript. Provides deterministic, linear-time regular expression matching — no catastrophic backtracking.

```js
const re = require('osm:regexp');
```

---

## Module-Level Functions

These functions compile the pattern on each call. For repeated use of the same pattern, prefer [`compile()`](#recompilepattern) for better performance.

### `re.match(pattern, str)`

Tests whether `str` matches `pattern`.

**Parameters:**

| Name      | Type   | Required | Description                          |
|-----------|--------|----------|--------------------------------------|
| `pattern` | string | Yes      | RE2 pattern. `null`/`undefined` throws `"pattern is required"`. |
| `str`     | string | No       | String to test. `null`/`undefined` → `""`. |

**Returns:** `boolean`

**Throws:** `Error` if pattern is `null`/`undefined` or invalid RE2.

**Examples:**

```js
re.match('\\d+', 'abc123');     // → true
re.match('^\\d+$', 'abc123');   // → false
re.match('', 'anything');        // → true (empty pattern matches everything)
re.match('^$', '');              // → true
```

---

### `re.find(pattern, str)`

Returns the first match, or `null` if no match.

**Parameters:**

| Name      | Type   | Required | Description     |
|-----------|--------|----------|-----------------|
| `pattern` | string | Yes      | RE2 pattern.    |
| `str`     | string | No       | String to search. `null`/`undefined` → `""`. |

**Returns:** `string | null`

**Behavior:**

- Returns `null` when there is no match.
- Returns `""` (empty string) when the pattern matches an empty string (e.g., `x*` on `"yyy"` — the regex engine matches `""` at position 0).

**Examples:**

```js
re.find('\\d+', 'abc 123 def 456');   // → "123"
re.find('\\d+', 'no numbers');         // → null
re.find('x*', 'yyy');                  // → "" (empty match, not null)
```

---

### `re.findAll(pattern, str, n?)`

Returns all matches.

**Parameters:**

| Name      | Type   | Required | Description                                   |
|-----------|--------|----------|-----------------------------------------------|
| `pattern` | string | Yes      | RE2 pattern.                                  |
| `str`     | string | No       | String to search.                             |
| `n`       | number | No       | Maximum number of matches. Default: `-1` (unlimited). |

**Returns:** `string[]` — empty array if no matches.

**Examples:**

```js
re.findAll('\\d+', '1 22 333');        // → ["1", "22", "333"]
re.findAll('\\d+', '1 22 333', 2);     // → ["1", "22"]
re.findAll('\\d+', 'no numbers');      // → []
```

---

### `re.findSubmatch(pattern, str)`

Returns the first match with capture groups.

**Parameters:**

| Name      | Type   | Required | Description     |
|-----------|--------|----------|-----------------|
| `pattern` | string | Yes      | RE2 pattern with capture groups. |
| `str`     | string | No       | String to search. |

**Returns:** `string[] | null` — `result[0]` is the full match, `result[1..n]` are capture groups. Returns `null` if no match.

**Examples:**

```js
re.findSubmatch('(\\w+)@(\\w+)', 'user@host');
// → ["user@host", "user", "host"]

re.findSubmatch('(\\d+)', 'no match');
// → null
```

---

### `re.findAllSubmatch(pattern, str, n?)`

Returns all matches with capture groups.

**Parameters:**

| Name      | Type   | Required | Description                                   |
|-----------|--------|----------|-----------------------------------------------|
| `pattern` | string | Yes      | RE2 pattern with capture groups.              |
| `str`     | string | No       | String to search.                             |
| `n`       | number | No       | Maximum number of matches. Default: `-1` (unlimited). |

**Returns:** `string[][]` — array of match arrays. Empty array if no matches.

**Examples:**

```js
re.findAllSubmatch('(\\w+)=(\\w+)', 'a=1 b=2');
// → [["a=1", "a", "1"], ["b=2", "b", "2"]]

re.findAllSubmatch('(\\d+)', 'none');
// → []
```

---

### `re.replace(pattern, str, repl)`

Replaces the **first** match only.

**Parameters:**

| Name      | Type   | Required | Description                                   |
|-----------|--------|----------|-----------------------------------------------|
| `pattern` | string | Yes      | RE2 pattern.                                  |
| `str`     | string | No       | Input string.                                 |
| `repl`    | string | No       | Replacement string. Supports `$1`, `$2`, etc. for backreferences. |

**Returns:** `string` — the modified string. Returns original if no match.

**Examples:**

```js
re.replace('\\d+', 'abc123def456', 'X');
// → "abcXdef456"

re.replace('(\\w+)@(\\w+)', 'user@host rest', '$2@$1');
// → "host@user rest"

re.replace('\\d+', 'no numbers', 'X');
// → "no numbers" (unchanged)
```

---

### `re.replaceAll(pattern, str, repl)`

Replaces **all** matches.

**Parameters:**

| Name      | Type   | Required | Description                                   |
|-----------|--------|----------|-----------------------------------------------|
| `pattern` | string | Yes      | RE2 pattern.                                  |
| `str`     | string | No       | Input string.                                 |
| `repl`    | string | No       | Replacement string. Supports backreferences.  |

**Returns:** `string` — the modified string. Returns original if no match.

**Examples:**

```js
re.replaceAll('\\d', 'a1b2c3', '');
// → "abc"

re.replaceAll('(\\w+)', 'hello world', '[$1]');
// → "[hello] [world]"
```

---

### `re.split(pattern, str, n?)`

Splits `str` by `pattern`.

**Parameters:**

| Name      | Type   | Required | Description                                   |
|-----------|--------|----------|-----------------------------------------------|
| `pattern` | string | Yes      | RE2 pattern.                                  |
| `str`     | string | No       | String to split.                              |
| `n`       | number | No       | Maximum number of substrings. Default: `-1` (unlimited). The last element contains the unsplit remainder. |

**Returns:** `string[]`

**Examples:**

```js
re.split(',', 'a,b,c');
// → ["a", "b", "c"]

re.split('\\s+', 'a b c d e', 3);
// → ["a", "b", "c d e"]

re.split(',', '');
// → [""]

re.split('X', 'no match');
// → ["no match"]
```

---

### `re.compile(pattern)`

Pre-compiles a pattern for repeated use. Invalid patterns throw immediately.

**Parameters:**

| Name      | Type   | Required | Description     |
|-----------|--------|----------|-----------------|
| `pattern` | string | Yes      | RE2 pattern.    |

**Returns:** `RegexpObject` — a compiled regex with bound methods.

**Throws:** `Error` if pattern is invalid RE2.

**Example:**

```js
var emailRe = re.compile('(\\w+)@([\\w.]+)');
emailRe.match('user@example.com');               // → true
emailRe.findSubmatch('From: user@example.com');   // → ["user@example.com", "user", "example.com"]
emailRe.replaceAll('a@b c@d', 'REDACTED');        // → "REDACTED REDACTED"
```

---

## RegexpObject

Returned by `compile()`. Has the same methods as the module-level functions, but without the `pattern` parameter (it's pre-compiled).

| Method                       | Signature                              |
|------------------------------|----------------------------------------|
| `pattern`                    | `string` — the compiled pattern        |
| `match(str)`                 | `boolean`                              |
| `find(str)`                  | `string \| null`                       |
| `findAll(str, n?)`           | `string[]`                             |
| `findSubmatch(str)`          | `string[] \| null`                     |
| `findAllSubmatch(str, n?)`   | `string[][]`                           |
| `replace(str, repl)`         | `string`                               |
| `replaceAll(str, repl)`      | `string`                               |
| `split(str, n?)`             | `string[]`                             |

---

## Unicode Support

RE2 supports Unicode character classes and properties:

```js
re.match('日本', '日本語テスト');           // → true
re.find('[\\p{Han}]+', 'hello 世界 test'); // → "世界"
re.match('\\p{L}+', 'café');               // → true
```

---

## RE2 vs JavaScript RegExp

| Feature                       | `osm:regexp` (RE2)       | JavaScript RegExp    |
|-------------------------------|--------------------------|----------------------|
| Backreferences in patterns    | ❌ Not supported         | ✅ `\1`, `\2`        |
| Lookahead/lookbehind          | ❌ Not supported         | ✅                   |
| Catastrophic backtracking     | ❌ Impossible (linear)   | ⚠️ Possible          |
| Named groups `(?P<name>...)`  | ✅ Go syntax             | ✅ `(?<name>...)`    |
| Unicode properties `\p{...}`  | ✅                       | ✅ (with `/u` flag)  |
| Replacement backreferences    | ✅ `$1`, `$2`            | ✅ `$1`, `$2`        |

---

## Notes

- All functions are synchronous.
- Invalid patterns always throw a JavaScript `Error` — never return `null` or `undefined`.
- The `null`/`undefined` pattern argument is treated as a missing required parameter and throws `"pattern is required"`.
- The `null`/`undefined` `str` argument is coerced to `""` (empty string).
- RE2 guarantees linear-time matching, making it safe for untrusted input patterns.
