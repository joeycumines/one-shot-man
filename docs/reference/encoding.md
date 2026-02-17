# osm:encoding — Base64 and Hex Encoding/Decoding

A native module providing encoding and decoding functions for base64 (standard and URL-safe) and hexadecimal formats.

```js
const encoding = require('osm:encoding');
```

---

## Functions

### `encoding.base64Encode(input)`

Encodes input to standard base64 (RFC 4648 §4) with `+/` characters and `=` padding.

**Parameters:**

| Name    | Type                | Required | Description                                   |
|---------|---------------------|----------|-----------------------------------------------|
| `input` | string \| Uint8Array | No       | Data to encode. `null`/`undefined` → empty bytes → `""`. |

**Returns:** `string` — base64-encoded string.

**Examples:**

```js
encoding.base64Encode('hello');        // → "aGVsbG8="
encoding.base64Encode('');             // → ""
encoding.base64Encode(null);           // → ""
encoding.base64Encode(undefined);      // → ""
```

---

### `encoding.base64Decode(encoded)`

Decodes a standard base64 string.

**Parameters:**

| Name      | Type   | Required | Description                                   |
|-----------|--------|----------|-----------------------------------------------|
| `encoded` | string | No       | Base64 string to decode. `null`/`undefined` → `""` → `""`. |

**Returns:** `string` — decoded content.

**Throws:** `Error` with message `"base64Decode: ..."` if input is not valid base64.

**Examples:**

```js
encoding.base64Decode('aGVsbG8=');     // → "hello"
encoding.base64Decode('');             // → ""
encoding.base64Decode(null);           // → ""

// Invalid input throws
try {
  encoding.base64Decode('!!!not-valid!!!');
} catch (e) {
  log.error(e.message); // "base64Decode: illegal base64 data..."
}
```

---

### `encoding.base64URLEncode(input)`

Encodes input to URL-safe base64 (RFC 4648 §5) using `-_` instead of `+/`, **without padding**.

**Parameters:**

| Name    | Type                | Required | Description                                   |
|---------|---------------------|----------|-----------------------------------------------|
| `input` | string \| Uint8Array | No       | Data to encode. `null`/`undefined` → `""`.    |

**Returns:** `string` — URL-safe base64 string (no `+`, `/`, or `=` characters).

**Examples:**

```js
encoding.base64URLEncode('hello');
// → "aGVsbG8" (no padding)

// Standard base64 may contain +, /, = characters
encoding.base64Encode('subjects?_d');
// → "c3ViamVjdHM/X2Q="

encoding.base64URLEncode('subjects?_d');
// → "c3ViamVjdHM_X2Q" (URL-safe, no padding)
```

---

### `encoding.base64URLDecode(encoded)`

Decodes a URL-safe base64 string (without padding).

**Parameters:**

| Name      | Type   | Required | Description                                   |
|-----------|--------|----------|-----------------------------------------------|
| `encoded` | string | No       | URL-safe base64 string. `null`/`undefined` → `""`. |

**Returns:** `string` — decoded content.

**Throws:** `Error` with message `"base64URLDecode: ..."` if input is invalid.

**Examples:**

```js
encoding.base64URLDecode('aGVsbG8');   // → "hello"
encoding.base64URLDecode('');          // → ""
```

---

### `encoding.hexEncode(input)`

Encodes input to lowercase hexadecimal.

**Parameters:**

| Name    | Type                | Required | Description                                   |
|---------|---------------------|----------|-----------------------------------------------|
| `input` | string \| Uint8Array | No       | Data to encode. `null`/`undefined` → `""`.    |

**Returns:** `string` — lowercase hex string.

**Examples:**

```js
encoding.hexEncode('hello');           // → "68656c6c6f"
encoding.hexEncode('');                // → ""
encoding.hexEncode(null);             // → ""
```

---

### `encoding.hexDecode(encoded)`

Decodes a hexadecimal string. Case-insensitive.

**Parameters:**

| Name      | Type   | Required | Description                                   |
|-----------|--------|----------|-----------------------------------------------|
| `encoded` | string | No       | Hex string to decode. `null`/`undefined` → `""`. |

**Returns:** `string` — decoded content.

**Throws:** `Error` with message `"hexDecode: ..."` for invalid hex characters or odd-length input.

**Examples:**

```js
encoding.hexDecode('68656c6c6f');      // → "hello"
encoding.hexDecode('48454C4C4F');      // → "HELLO" (case-insensitive)
encoding.hexDecode('');                // → ""
encoding.hexDecode(null);             // → ""

// Invalid input throws
try {
  encoding.hexDecode('xyz');
} catch (e) {
  log.error(e.message); // "hexDecode: encoding/hex: invalid byte..."
}

try {
  encoding.hexDecode('abc');  // odd length
} catch (e) {
  log.error(e.message); // "hexDecode: encoding/hex: odd length hex string"
}
```

---

## Roundtrip Examples

```js
var encoding = require('osm:encoding');

// Base64 roundtrip
var original = 'Hello, World!';
var encoded = encoding.base64Encode(original);
var decoded = encoding.base64Decode(encoded);
// decoded === original

// URL-safe base64 roundtrip
var urlEncoded = encoding.base64URLEncode(original);
var urlDecoded = encoding.base64URLDecode(urlEncoded);
// urlDecoded === original

// Hex roundtrip
var hexed = encoding.hexEncode(original);
var unhexed = encoding.hexDecode(hexed);
// unhexed === original

// Unicode roundtrip
var unicode = '日本語テスト';
encoding.base64Decode(encoding.base64Encode(unicode));
// → '日本語テスト'
```

---

## Notes

- All functions are synchronous.
- Input accepts both strings (converted to UTF-8 bytes) and `Uint8Array`/byte arrays.
- `null` and `undefined` inputs are treated as empty — they encode to `""` and decode from `""` to `""`.
- Decode errors throw JavaScript `Error` objects — never return `null` or `undefined`.
- Standard base64 (`base64Encode`/`base64Decode`) uses RFC 4648 §4 with `=` padding.
- URL-safe base64 (`base64URLEncode`/`base64URLDecode`) uses RFC 4648 §5 with `-_` characters and **no padding**.
- Hex output is always **lowercase**; hex input is **case-insensitive**.
