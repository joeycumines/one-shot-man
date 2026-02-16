package encodingmod

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
	_ = vm.Set("enc", module.Get("exports"))
	return vm
}

// --- base64Encode / base64Decode ---

func TestBase64Encode_Simple(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`enc.base64Encode("hello world")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "aGVsbG8gd29ybGQ=" {
		t.Fatalf("expected 'aGVsbG8gd29ybGQ=', got %q", v.String())
	}
}

func TestBase64Decode_Simple(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`enc.base64Decode("aGVsbG8gd29ybGQ=")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "hello world" {
		t.Fatalf("expected 'hello world', got %q", v.String())
	}
}

func TestBase64_Roundtrip(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var original = "The quick brown fox jumps over the lazy dog!";
		var encoded = enc.base64Encode(original);
		var decoded = enc.base64Decode(encoded);
		if (decoded !== original) throw new Error("roundtrip failed: " + decoded);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestBase64Encode_EmptyString(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`enc.base64Encode("")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "" {
		t.Fatalf("expected empty string, got %q", v.String())
	}
}

func TestBase64Decode_EmptyString(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`enc.base64Decode("")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "" {
		t.Fatalf("expected empty string, got %q", v.String())
	}
}

func TestBase64Decode_InvalidInput(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`enc.base64Decode("!!!not-valid-base64!!!")`)
	if err == nil {
		t.Fatal("expected error for invalid base64")
	}
}

func TestBase64Encode_Null(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`enc.base64Encode(null)`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "" {
		t.Fatalf("expected empty string for null input, got %q", v.String())
	}
}

func TestBase64Encode_Undefined(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`enc.base64Encode(undefined)`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "" {
		t.Fatalf("expected empty string for undefined input, got %q", v.String())
	}
}

func TestBase64Encode_Unicode(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var original = "\u65e5\u672c\u8a9e\u30c6\u30b9\u30c8";
		var encoded = enc.base64Encode(original);
		var decoded = enc.base64Decode(encoded);
		if (decoded !== original) throw new Error("unicode roundtrip failed: " + decoded);
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestBase64Encode_BinaryLike(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var encoded = enc.base64Encode("\x00\x01\x02\xff");
		var decoded = enc.base64Decode(encoded);
		if (decoded !== "\x00\x01\x02\xff") throw new Error("binary roundtrip failed");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

// --- base64URLEncode / base64URLDecode ---

func TestBase64URLEncode_Simple(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`enc.base64URLEncode("hello world")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "aGVsbG8gd29ybGQ" {
		t.Fatalf("expected 'aGVsbG8gd29ybGQ', got %q", v.String())
	}
}

func TestBase64URLDecode_Simple(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`enc.base64URLDecode("aGVsbG8gd29ybGQ")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "hello world" {
		t.Fatalf("expected 'hello world', got %q", v.String())
	}
}

func TestBase64URL_Roundtrip(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var original = "subjects?_d=1&sort=asc";
		var encoded = enc.base64URLEncode(original);
		var decoded = enc.base64URLDecode(encoded);
		if (decoded !== original) throw new Error("url roundtrip failed: " + decoded);
		if (encoded.indexOf("+") !== -1 || encoded.indexOf("/") !== -1) {
			throw new Error("URL encoding contains unsafe chars: " + encoded);
		}
		if (encoded.indexOf("=") !== -1) {
			throw new Error("URL encoding contains padding: " + encoded);
		}
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestBase64URLEncode_EmptyString(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`enc.base64URLEncode("")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "" {
		t.Fatalf("expected empty string, got %q", v.String())
	}
}

func TestBase64URLDecode_InvalidInput(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`enc.base64URLDecode("!!!invalid!!!")`)
	if err == nil {
		t.Fatal("expected error for invalid base64url")
	}
}

// --- hexEncode / hexDecode ---

func TestHexEncode_Simple(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`enc.hexEncode("hello")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "68656c6c6f" {
		t.Fatalf("expected '68656c6c6f', got %q", v.String())
	}
}

func TestHexDecode_Simple(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`enc.hexDecode("68656c6c6f")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "hello" {
		t.Fatalf("expected 'hello', got %q", v.String())
	}
}

func TestHex_Roundtrip(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`
		var original = "binary test \x00\x01\xff";
		var encoded = enc.hexEncode(original);
		var decoded = enc.hexDecode(encoded);
		if (decoded !== original) throw new Error("hex roundtrip failed");
	`)
	if err != nil {
		t.Fatal(err)
	}
}

func TestHexEncode_EmptyString(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`enc.hexEncode("")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "" {
		t.Fatalf("expected empty string, got %q", v.String())
	}
}

func TestHexDecode_EmptyString(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`enc.hexDecode("")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "" {
		t.Fatalf("expected empty string, got %q", v.String())
	}
}

func TestHexDecode_InvalidInput(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`enc.hexDecode("xyz")`)
	if err == nil {
		t.Fatal("expected error for invalid hex")
	}
}

func TestHexDecode_OddLength(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	_, err := vm.RunString(`enc.hexDecode("abc")`)
	if err == nil {
		t.Fatal("expected error for odd-length hex")
	}
}

func TestHexEncode_Null(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`enc.hexEncode(null)`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "" {
		t.Fatalf("expected empty string for null, got %q", v.String())
	}
}

func TestHexDecode_UpperCase(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`enc.hexDecode("48454C4C4F")`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "HELLO" {
		t.Fatalf("expected 'HELLO', got %q", v.String())
	}
}

// --- Cross-function ---

func TestBase64Decode_NullInput(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`enc.base64Decode(null)`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "" {
		t.Fatalf("expected empty string for null, got %q", v.String())
	}
}

func TestHexDecode_NullInput(t *testing.T) {
	t.Parallel()
	vm := setup(t)
	v, err := vm.RunString(`enc.hexDecode(null)`)
	if err != nil {
		t.Fatal(err)
	}
	if v.String() != "" {
		t.Fatalf("expected empty string for null, got %q", v.String())
	}
}