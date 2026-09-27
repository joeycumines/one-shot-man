package node

import (
	"testing"
)

func TestCryptoRandomBytesValidatesSize(t *testing.T) {
	got := runScript(t, reportScript(`
		const crypto = require("crypto");
		async function classify(fn) {
			try {
				return "accepted:" + (await fn()).length;
			} catch (err) {
				const kind = err instanceof RangeError ? "RangeError" :
					err instanceof TypeError ? "TypeError" : "Error";
				return kind + ":" + err.code;
			}
		}
		report([
			await classify(() => crypto.randomBytes()),
			await classify(() => crypto.randomBytes(undefined)),
			await classify(() => crypto.randomBytes(null)),
			await classify(() => crypto.randomBytes("32")),
			await classify(() => crypto.randomBytes(true)),
			await classify(() => crypto.randomBytes({})),
			await classify(() => crypto.randomBytes(-1)),
			await classify(() => crypto.randomBytes(2147483633)),
			await classify(() => crypto.randomBytes(0)),
			await classify(() => crypto.randomBytes(1.5)),
		].join("|"));
	`))
	want := "TypeError:ERR_INVALID_ARG_TYPE|TypeError:ERR_INVALID_ARG_TYPE|TypeError:ERR_INVALID_ARG_TYPE|" +
		"TypeError:ERR_INVALID_ARG_TYPE|TypeError:ERR_INVALID_ARG_TYPE|TypeError:ERR_INVALID_ARG_TYPE|" +
		"RangeError:ERR_OUT_OF_RANGE|RangeError:ERR_OUT_OF_RANGE|accepted:0|accepted:1"
	if got != want {
		t.Fatalf("randomBytes validation = %q, want %q", got, want)
	}
}

func TestCryptoRandomBytesResolvesNBytes(t *testing.T) {
	got := runScript(t, reportScript(`
			const crypto = require("crypto");
			const buf = await crypto.randomBytes(32);
			report("LEN:" + buf.length);
	`))
	if got != "LEN:32" {
		t.Fatalf("randomBytes(32).length = %q, want LEN:32", got)
	}
}

func TestCryptoRandomBytesAreDistinctAndHexable(t *testing.T) {
	got := runScript(t, reportScript(`
			const crypto = require("crypto");
			const a = await crypto.randomBytes(16);
			const b = await crypto.randomBytes(16);
			if (a.every((v, i) => v === b[i])) { report("ERROR: identical"); return; }
			let hex = "";
			for (const byte of a) { hex += byte.toString(16).padStart(2, "0"); }
			report("HEXLEN:" + hex.length);
	`))
	if got != "HEXLEN:32" {
		t.Fatalf("random bytes hex = %q, want HEXLEN:32 with distinct draws", got)
	}
}
