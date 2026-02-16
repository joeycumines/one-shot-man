package cryptomod

import (
	"testing"

	"github.com/dop251/goja"
)

func setup(t *testing.T) *goja.Runtime {
	t.Helper()
	vm := goja.New()
	module := vm.NewObject()
	exports := vm.NewObject()
	_ = module.Set("exports", exports)
	Require(vm, module)
	_ = vm.Set("crypto", module.Get("exports"))
	return vm
}

// --- sha256 ---

func TestSHA256Hello(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`crypto.sha256("hello")`)
	if err != nil {
		t.Fatal(err)
	}
	expected := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestSHA256Empty(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`crypto.sha256("")`)
	if err != nil {
		t.Fatal(err)
	}
	// SHA-256 of empty string
	expected := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestSHA256KnownVector(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	// NIST test vector: SHA-256("abc")
	v, err := vm.RunString(`crypto.sha256("abc")`)
	if err != nil {
		t.Fatal(err)
	}
	expected := "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestSHA256Undefined(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`crypto.sha256()`)
	if err != nil {
		t.Fatal(err)
	}
	// undefined -> empty bytes -> same as sha256("")
	expected := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestSHA256Null(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`crypto.sha256(null)`)
	if err != nil {
		t.Fatal(err)
	}
	expected := "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestSHA256LongString(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	// SHA-256 of "abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq" (NIST)
	v, err := vm.RunString(`crypto.sha256("abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq")`)
	if err != nil {
		t.Fatal(err)
	}
	expected := "248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

// --- sha1 ---

func TestSHA1Hello(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`crypto.sha1("hello")`)
	if err != nil {
		t.Fatal(err)
	}
	expected := "aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestSHA1Empty(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`crypto.sha1("")`)
	if err != nil {
		t.Fatal(err)
	}
	expected := "da39a3ee5e6b4b0d3255bfef95601890afd80709"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestSHA1KnownVector(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	// SHA-1("abc") -- NIST FIPS 180-4
	v, err := vm.RunString(`crypto.sha1("abc")`)
	if err != nil {
		t.Fatal(err)
	}
	expected := "a9993e364706816aba3e25717850c26c9cd0d89d"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestSHA1Undefined(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`crypto.sha1()`)
	if err != nil {
		t.Fatal(err)
	}
	expected := "da39a3ee5e6b4b0d3255bfef95601890afd80709"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

// --- md5 ---

func TestMD5Hello(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`crypto.md5("hello")`)
	if err != nil {
		t.Fatal(err)
	}
	expected := "5d41402abc4b2a76b9719d911017c592"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestMD5Empty(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`crypto.md5("")`)
	if err != nil {
		t.Fatal(err)
	}
	// MD5 of empty string -- RFC 1321
	expected := "d41d8cd98f00b204e9800998ecf8427e"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestMD5KnownVector(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	// MD5("abc") -- RFC 1321
	v, err := vm.RunString(`crypto.md5("abc")`)
	if err != nil {
		t.Fatal(err)
	}
	expected := "900150983cd24fb0d6963f7d28e17f72"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestMD5Null(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`crypto.md5(null)`)
	if err != nil {
		t.Fatal(err)
	}
	expected := "d41d8cd98f00b204e9800998ecf8427e"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestMD5MessageDigest(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	// MD5("message digest") -- RFC 1321 test vector
	v, err := vm.RunString(`crypto.md5("message digest")`)
	if err != nil {
		t.Fatal(err)
	}
	expected := "f96b697d7cb7938d525a2f31aaf161d0"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

// --- hmacSHA256 ---

func TestHMACSHA256Known(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	// RFC 4231 Test Case 2: key = "Jefe", data = "what do ya want for nothing?"
	v, err := vm.RunString(`crypto.hmacSHA256("Jefe", "what do ya want for nothing?")`)
	if err != nil {
		t.Fatal(err)
	}
	expected := "5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestHMACSHA256EmptyMessage(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	// HMAC-SHA256 with key "key" and empty message
	v, err := vm.RunString(`crypto.hmacSHA256("key", "")`)
	if err != nil {
		t.Fatal(err)
	}
	expected := "5d5d139563c95b5967b9bd9a8c9b233a9dedb45072794cd232dc1b74832607d0"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestHMACSHA256EmptyKey(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	// HMAC-SHA256 with empty key and message "hello"
	v, err := vm.RunString(`crypto.hmacSHA256("", "hello")`)
	if err != nil {
		t.Fatal(err)
	}
	expected := "4352b26e33fe0d769a8922a6ba29004109f01688e26acc9e6cb347e5a5afc4da"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestHMACSHA256BothEmpty(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	// HMAC-SHA256 with empty key and empty message
	v, err := vm.RunString(`crypto.hmacSHA256("", "")`)
	if err != nil {
		t.Fatal(err)
	}
	expected := "b613679a0814d9ec772f95d778c35fc5ff1697c493715653c6c712144292c5ad"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestHMACSHA256UndefinedArgs(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	// Both undefined -> empty key, empty message
	v, err := vm.RunString(`crypto.hmacSHA256()`)
	if err != nil {
		t.Fatal(err)
	}
	expected := "b613679a0814d9ec772f95d778c35fc5ff1697c493715653c6c712144292c5ad"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

// --- hmacSHA1 ---

func TestHMACSHA1Known(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	// RFC 2202 Test Case 2: key = "Jefe", data = "what do ya want for nothing?"
	v, err := vm.RunString(`crypto.hmacSHA1("Jefe", "what do ya want for nothing?")`)
	if err != nil {
		t.Fatal(err)
	}
	expected := "effcdf6ae5eb2fa2d27416d5f184df9c259a7c79"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestHMACSHA1EmptyMessage(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`crypto.hmacSHA1("key", "")`)
	if err != nil {
		t.Fatal(err)
	}
	expected := "f42bb0eeb018ebbd4597ae7213711ec60760843f"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestHMACSHA1EmptyKey(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`crypto.hmacSHA1("", "hello")`)
	if err != nil {
		t.Fatal(err)
	}
	expected := "63cce3559126764fd2581f05878c6791065c0d06"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestHMACSHA1BothEmpty(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`crypto.hmacSHA1("", "")`)
	if err != nil {
		t.Fatal(err)
	}
	expected := "fbdb1d1b18aa6c08324b7d64b71fb76370690e1d"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestHMACSHA1UndefinedArgs(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`crypto.hmacSHA1()`)
	if err != nil {
		t.Fatal(err)
	}
	expected := "fbdb1d1b18aa6c08324b7d64b71fb76370690e1d"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

// --- Byte array input ---

func TestSHA256ByteArray(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	// Create a Uint8Array with bytes for "hello" and pass to sha256
	v, err := vm.RunString(`
		var bytes = new Uint8Array([104, 101, 108, 108, 111]); // "hello"
		crypto.sha256(bytes);
	`)
	if err != nil {
		t.Fatal(err)
	}
	expected := "2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestMD5ByteArray(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`
		var bytes = new Uint8Array([104, 101, 108, 108, 111]); // "hello"
		crypto.md5(bytes);
	`)
	if err != nil {
		t.Fatal(err)
	}
	expected := "5d41402abc4b2a76b9719d911017c592"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestHMACSHA256ByteArrayKey(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	// "Jefe" as byte array
	v, err := vm.RunString(`
		var key = new Uint8Array([74, 101, 102, 101]); // "Jefe"
		crypto.hmacSHA256(key, "what do ya want for nothing?");
	`)
	if err != nil {
		t.Fatal(err)
	}
	expected := "5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

// --- Output format ---

func TestOutputIsLowercaseHex(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var hashes = [
			crypto.sha256("test"),
			crypto.sha1("test"),
			crypto.md5("test"),
			crypto.hmacSHA256("key", "test"),
			crypto.hmacSHA1("key", "test"),
		];
		for (var i = 0; i < hashes.length; i++) {
			var h = hashes[i];
			if (typeof h !== "string") throw new Error("hash " + i + " is not a string: " + typeof h);
			if (h !== h.toLowerCase()) throw new Error("hash " + i + " is not lowercase: " + h);
			if (!/^[0-9a-f]+$/.test(h)) throw new Error("hash " + i + " contains non-hex chars: " + h);
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestHashLengths(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var s256 = crypto.sha256("x");
		if (s256.length !== 64) throw new Error("sha256 length should be 64, got " + s256.length);
		var s1 = crypto.sha1("x");
		if (s1.length !== 40) throw new Error("sha1 length should be 40, got " + s1.length);
		var m = crypto.md5("x");
		if (m.length !== 32) throw new Error("md5 length should be 32, got " + m.length);
		var hs256 = crypto.hmacSHA256("k", "x");
		if (hs256.length !== 64) throw new Error("hmacSHA256 length should be 64, got " + hs256.length);
		var hs1 = crypto.hmacSHA1("k", "x");
		if (hs1.length !== 40) throw new Error("hmacSHA1 length should be 40, got " + hs1.length);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

// --- Determinism ---

func TestSHA256Deterministic(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var a = crypto.sha256("determinism");
		var b = crypto.sha256("determinism");
		if (a !== b) throw new Error("sha256 not deterministic: " + a + " vs " + b);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestHMACSHA256Deterministic(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var a = crypto.hmacSHA256("key", "msg");
		var b = crypto.hmacSHA256("key", "msg");
		if (a !== b) throw new Error("hmacSHA256 not deterministic: " + a + " vs " + b);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

// --- toBytes edge cases ---

func TestToBytesNumberFallback(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	// Number input -> converted via .String() -> hashed as string "42"
	v, err := vm.RunString(`crypto.sha256(42)`)
	if err != nil {
		t.Fatal(err)
	}
	// SHA-256("42")
	expected := "73475cb40a568e8da8a045ced110137e159f890ac4da883b6b17dc651b3a8049"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}

func TestSHA256UnicodeString(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString("crypto.sha256(\"\\u65E5\\u672C\\u8A9E\")")
	if err != nil {
		t.Fatal(err)
	}
	expected := "77710aedc74ecfa33685e33a6c7df5cc83004da1bdcef7fb280f5c2b2e97e0a5"
	if v.String() != expected {
		t.Fatalf("expected %q, got %q", expected, v.String())
	}
}
