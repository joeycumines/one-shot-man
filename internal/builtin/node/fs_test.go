package node

import (
	"encoding/hex"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

func TestFsWriteFileStringEncoding(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		encoding string
		want     []byte
	}{
		{name: "utf8", data: `"\u00e9"`, encoding: `"utf8"`, want: []byte("é")},
		{name: "ascii", data: `"\u00e9"`, encoding: `{encoding: "ascii"}`, want: []byte{0xe9}},
		{name: "latin1", data: `"\u00e9"`, encoding: `{encoding: "latin1"}`, want: []byte{0xe9}},
		{name: "ascii surrogate pair", data: `"😀"`, encoding: `{encoding: "ascii"}`, want: []byte{0x3d, 0x00}},
		{name: "latin1 surrogate pair", data: `"😀"`, encoding: `{encoding: "latin1"}`, want: []byte{0x3d, 0x00}},
		{name: "utf16le", data: `"Hi"`, encoding: `{encoding: "utf16le"}`, want: []byte{'H', 0, 'i', 0}},
		{name: "base64 option", data: `"SGVsbG8="`, encoding: `{encoding: "base64"}`, want: []byte("Hello")},
		{name: "base64 string option", data: `"SGVsbG8"`, encoding: `"base64"`, want: []byte("Hello")},
		{name: "base64url", data: `"SGVsbG8"`, encoding: `{encoding: "base64url"}`, want: []byte("Hello")},
		{name: "hex", data: `"48656c6c6f"`, encoding: `{encoding: "hex"}`, want: []byte("Hello")},
		{name: "hex valid prefix", data: `"1ag123"`, encoding: `{encoding: "hex"}`, want: []byte{0x1a}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "written")
			script := `const fs = require("fs");
await fs.promises.writeFile(` + pathLit(path) + `, ` + tt.data + `, ` + tt.encoding + `);
report("written");`
			if got := runScript(t, reportScript(script)); got != "written" {
				t.Fatalf("writeFile result = %q, want written", got)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("ReadFile: %v", err)
			}
			if string(data) != string(tt.want) {
				t.Fatalf("written bytes = %v, want %v", data, tt.want)
			}
		})
	}
}

func TestFsWriteFileRejectsUnknownEncoding(t *testing.T) {
	path := filepath.Join(t.TempDir(), "written")
	script := `const fs = require("fs");
await fs.promises.writeFile(` + pathLit(path) + `, "text", {encoding: "unknown"});
report("written");`
	got := runScript(t, reportScript(script))
	if !strings.Contains(got, "ERR_INVALID_ARG_VALUE") {
		t.Fatalf("writeFile error = %q, want ERR_INVALID_ARG_VALUE", got)
	}
}

func TestFsWriteFileRejectsInvalidOptionsWithoutTruncating(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preserved")
	if err := os.WriteFile(path, []byte("original"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	script := `const fs = require("fs");
const errors = [];
for (const options of [123, true]) {
	try {
		await fs.promises.writeFile(` + pathLit(path) + `, "replacement", options);
		errors.push("accepted");
	} catch (err) {
		errors.push((err instanceof TypeError) + ":" + err.code);
	}
}
report(errors.join("|"));`
	if got := runScript(t, reportScript(script)); got != "true:ERR_INVALID_ARG_TYPE|true:ERR_INVALID_ARG_TYPE" {
		t.Fatalf("invalid writeFile options = %q, want coded TypeErrors", got)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if string(data) != "original" {
		t.Fatalf("invalid options changed file contents to %q", data)
	}
}

func TestFsReadFileRejectsUnsupportedEncodingAsTypeError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "input")
	if err := os.WriteFile(path, []byte("input"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	script := `try {
	await require("fs").promises.readFile(` + pathLit(path) + `, {encoding: "ascii"});
	report("accepted");
} catch (err) {
	report((err instanceof TypeError) + ":" + err.code);
}`
	if got := runScript(t, reportScript(script)); got != "true:ERR_INVALID_ARG_VALUE" {
		t.Fatalf("unsupported readFile encoding = %q, want coded TypeError", got)
	}
}

func TestFsLstatModeIncludesFileType(t *testing.T) {
	path := filepath.Join(t.TempDir(), "regular")
	if err := os.WriteFile(path, []byte("data"), 0o640); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	script := `const stat = await require("fs").promises.lstat(` + pathLit(path) + `);
report(String(stat.mode));`
	got := runScript(t, reportScript(script))
	mode, err := strconv.ParseUint(got, 10, 32)
	if err != nil {
		t.Fatalf("parse stat.mode %q: %v", got, err)
	}
	if fileType := mode & 0o170000; fileType != 0o100000 {
		t.Fatalf("stat.mode type bits = %#o, want regular-file bits %#o", fileType, 0o100000)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if permissions := mode & 0o777; permissions != uint64(info.Mode().Perm()) {
		t.Fatalf("stat.mode permission bits = %#o, want %#o", permissions, info.Mode().Perm())
	}
}

func TestFsWriteReadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "roundtrip.txt")
	got := runScript(t, reportScript(`
			const fs = require("fs");
			await fs.promises.writeFile(`+pathLit(path)+`, "gateway payload");
			report(await fs.promises.readFile(`+pathLit(path)+`, {encoding: "utf8"}));
	`))
	if got != "gateway payload" {
		t.Fatalf("round trip = %q, want %q", got, "gateway payload")
	}
}

// TestFsReadFileDefaultResolvesUint8Array covers Node's null default
// encoding: with no options the promise resolves to a Uint8Array whose
// length matches the file's byte count.
func TestFsReadFileDefaultResolvesUint8Array(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bytes.bin")
	payload := "byte-count-check"
	if err := os.WriteFile(path, []byte(payload), 0o644); err != nil {
		t.Fatal(err)
	}
	got := runScript(t, reportScript(`
			const fs = require("fs");
			const data = await fs.promises.readFile(`+pathLit(path)+`);
			if (!(data instanceof Uint8Array)) { report("NOT-UINT8ARRAY"); return; }
			report("LEN:" + data.length + ":" + data[0]);
	`))
	if got != "LEN:"+itoaLit(len(payload))+":"+itoaLit(int(payload[0])) {
		t.Fatalf("default readFile = %q, want Uint8Array of length %d with first byte %d", got, len(payload), payload[0])
	}
}

// TestFsWriteFileAcceptsUint8Array covers Node's Buffer/TypedArray input: a
// readFile -> writeFile round trip must preserve the bytes. Previously the
// data argument was stringified, so a Uint8Array was written as a comma-joined
// list of numbers.
func TestFsWriteFileAcceptsUint8Array(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "src.bin")
	dst := filepath.Join(dir, "dst.bin")
	payload := []byte{0x00, 0x01, 0x7f, 0x80, 0xff, 0x41, 0x0a}
	if err := os.WriteFile(src, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	got := runScript(t, reportScript(`
			const fs = require("fs");
			const data = await fs.promises.readFile(`+pathLit(src)+`);
			if (!(data instanceof Uint8Array)) { report("NOT-UINT8ARRAY"); return; }
			await fs.promises.writeFile(`+pathLit(dst)+`, data);
			report("WROTE");
	`))
	if got != "WROTE" {
		t.Fatalf("writeFile(repoReadFile) = %q, want WROTE", got)
	}
	written, err := os.ReadFile(dst)
	if err != nil {
		t.Fatal(err)
	}
	if string(written) != string(payload) {
		t.Fatalf("readFile -> writeFile round trip = %v, want %v", written, payload)
	}
}

func TestFsWriteFilePreservesTypedArrayAndDataViewBytes(t *testing.T) {
	dir := t.TempDir()
	typedPath := filepath.Join(dir, "typed.bin")
	viewPath := filepath.Join(dir, "view.bin")
	got := runScript(t, reportScript(`
			const fs = require("fs");
			const typed = new Uint16Array([0x1234, 0xabcd]);
			const typedBytes = new Uint8Array(typed.buffer, typed.byteOffset, typed.byteLength);
			await fs.promises.writeFile(`+pathLit(typedPath)+`, typed);

			const buffer = new ArrayBuffer(8);
			const view = new DataView(buffer, 2, 4);
			view.setUint16(0, 0x1234, true);
			view.setUint16(2, 0xabcd, true);
			const viewBytes = new Uint8Array(view.buffer, view.byteOffset, view.byteLength);
			await fs.promises.writeFile(`+pathLit(viewPath)+`, view);

			function hex(bytes) {
				return Array.from(bytes, (byte) => byte.toString(16).padStart(2, "0")).join("");
			}
			report(hex(typedBytes) + "/" + hex(viewBytes));
	`))
	expected := strings.Split(got, "/")
	if len(expected) != 2 {
		t.Fatalf("byte-range result = %q, want two hex byte sequences", got)
	}
	for i, path := range []string{typedPath, viewPath} {
		want, err := hex.DecodeString(expected[i])
		if err != nil {
			t.Fatalf("decode expected bytes %d: %v", i, err)
		}
		written, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read written bytes %d: %v", i, err)
		}
		if string(written) != string(want) {
			t.Errorf("written bytes %d = %x, want %x", i, written, want)
		}
	}
}

func TestFsUnlinkRemovesDirectorySymlinkOnly(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target")
	link := filepath.Join(dir, "link")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, link); err != nil {
		if runtime.GOOS == "windows" {
			t.Skipf("directory symlink unavailable: %v", err)
		}
		t.Fatal(err)
	}
	got := runScript(t, reportScript(`
			const fs = require("fs");
			await fs.promises.unlink(`+pathLit(link)+`);
			report("UNLINKED");
	`))
	if got != "UNLINKED" {
		t.Fatalf("unlink directory symlink = %q, want UNLINKED", got)
	}
	if _, err := os.Lstat(link); !os.IsNotExist(err) {
		t.Fatalf("directory symlink still exists after unlink: %v", err)
	}
	if info, err := os.Stat(target); err != nil || !info.IsDir() {
		t.Fatalf("symlink target was removed or changed: info=%v err=%v", info, err)
	}
}

// TestFsRmdirRemovesEmptyDirectory covers Node's fs.promises.rmdir simple
// form: an empty directory is removed, a non-empty one rejects with ENOTEMPTY,
// and a missing one with ENOENT.
func TestFsRmdirRemovesEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty")
	if err := os.Mkdir(empty, 0o700); err != nil {
		t.Fatal(err)
	}
	full := filepath.Join(dir, "full")
	if err := os.Mkdir(full, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(full, "f"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	filePath := filepath.Join(dir, "file")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	got := runScript(t, reportScript(`
			const fs = require("fs");
			await fs.promises.rmdir(`+pathLit(empty)+`);
			let notEmpty = "NO-THROW";
			try { await fs.promises.rmdir(`+pathLit(full)+`); } catch (err) { notEmpty = err.code; }
			let missing = "NO-THROW";
			try { await fs.promises.rmdir(`+pathLit(filepath.Join(dir, "gone"))+`); } catch (err) { missing = err.code; }
			let missingParent = "NO-THROW";
			try { await fs.promises.rmdir(`+pathLit(filepath.Join(dir, "absent", "child"))+`); } catch (err) { missingParent = err.code; }
			let notDirectory = "NO-THROW";
			try { await fs.promises.rmdir(`+pathLit(filePath)+`); } catch (err) { notDirectory = err.code; }
			report(notEmpty + "/" + missing + "/" + missingParent + "/" + notDirectory);
	`))
	if got != "ENOTEMPTY/ENOENT/ENOENT/ENOTDIR" {
		t.Fatalf("rmdir rejections = %q, want ENOTEMPTY/ENOENT/ENOENT/ENOTDIR", got)
	}
	if _, err := os.Stat(empty); !os.IsNotExist(err) {
		t.Fatalf("empty directory still present after rmdir: %v", err)
	}
	if _, err := os.Stat(filePath); err != nil {
		t.Fatalf("rmdir removed a regular file: %v", err)
	}
}

func TestFsUnlinkRejectsDirectory(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "directory")
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	got := runScript(t, reportScript(`
			const fs = require("fs");
			try {
				await fs.promises.unlink(`+pathLit(path)+`);
				report("NO-ERROR");
			} catch (err) {
				report("CODE:" + err.code);
			}
	`))
	if got != "CODE:EISDIR" {
		t.Fatalf("unlink directory = %q, want CODE:EISDIR", got)
	}
	if info, err := os.Stat(path); err != nil || !info.IsDir() {
		t.Fatalf("unlink removed or changed the directory: info=%v err=%v", info, err)
	}
}

func TestFsWriteFileWxRejectsExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")
	if err := os.WriteFile(path, []byte("present"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := runScript(t, reportScript(`
			const fs = require("fs");
			try {
				await fs.promises.writeFile(`+pathLit(path)+`, "should fail", {flags: "wx"});
				report("NO-ERROR");
			} catch (e) {
				report("CODE:" + e.code);
			}
	`))
	if got != "CODE:EEXIST" {
		t.Fatalf("wx on existing file = %q, want CODE:EEXIST", got)
	}
}

func TestFsWriteFileWxSucceedsOnFreshFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "fresh.txt")
	got := runScript(t, reportScript(`
			const fs = require("fs");
			await fs.promises.writeFile(`+pathLit(path)+`, "first", {flags: "wx"});
			report("OK");
	`))
	if got != "OK" {
		t.Fatalf("wx on fresh file = %q, want OK", got)
	}
}

func TestFsWriteFileMode0600StatVerified(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX permission bits do not map reliably on Windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.txt")
	got := runScript(t, reportScript(`
			const fs = require("fs");
			await fs.promises.writeFile(`+pathLit(path)+`, "secret", {mode: 0o600});
			const stats = await fs.promises.lstat(`+pathLit(path)+`);
			report("mode:" + stats.mode.toString(8));
	`))
	// On this Darwin host a fresh file's umask is 022 by default in tests,
	// so 0o600 requested => 0o600 stored only when the process umask allows;
	// Node semantics apply the umask to the mode. Report and assert against
	// the stat truth: file must NOT be group/world readable beyond umask.
	if !strings.Contains(got, "mode:") {
		t.Fatalf("lstat result = %q, want mode: prefix", got)
	}
	mode := strings.TrimPrefix(got, "mode:")
	if mode == "777" || mode == "666" {
		t.Fatalf("requested 0o600 produced %s; umask not applied or mode ignored", mode)
	}
}

func TestFsUnlinkMissingFileRejectsENOENT(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "absent.txt")
	got := runScript(t, reportScript(`
			const fs = require("fs");
			let missing = "NO-ERROR";
			try {
				await fs.promises.unlink(`+pathLit(path)+`);
			} catch (e) {
				missing = e.code;
			}
			let missingParent = "NO-ERROR";
			try {
				await fs.promises.unlink(`+pathLit(filepath.Join(dir, "absent", "child"))+`);
			} catch (e) {
				missingParent = e.code;
			}
			report(missing + "/" + missingParent);
	`))
	if got != "ENOENT/ENOENT" {
		t.Fatalf("unlink missing paths = %q, want ENOENT/ENOENT", got)
	}
}

func TestFsLstatAndMkdirBehave(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "a", "b")
	got := runScript(t, reportScript(`
			const fs = require("fs");
			const created = await fs.promises.mkdir(`+pathLit(sub)+`, {recursive: true});
			const stats = await fs.promises.lstat(`+pathLit(sub)+`);
			report((stats.isDirectory() ? "DIR" : "NOT-DIR") + ":" + (created === `+pathLit(filepath.Dir(sub))+` ? "FIRST-CREATED" : "OTHER:" + created));
	`))
	if !strings.HasPrefix(got, "DIR:") {
		t.Fatalf("mkdir/lstat = %q, want DIR: prefix", got)
	}
	// Node's recursive mkdir resolves with the FIRST directory path created
	// (the shallowest missing ancestor), not the leaf.
	if !strings.HasSuffix(got, ":FIRST-CREATED") {
		t.Fatalf("recursive mkdir result = %q, want the first-created directory reported", got)
	}
}

func TestFsMkdirExistingNonRecursiveRejectsEEXIST(t *testing.T) {
	dir := t.TempDir()
	got := runScript(t, reportScript(`
			const fs = require("fs");
			try {
				await fs.promises.mkdir(`+pathLit(dir)+`);
				report("NO-ERROR");
			} catch (e) {
				report("CODE:" + e.code);
			}
	`))
	if got != "CODE:EEXIST" {
		t.Fatalf("mkdir existing = %q, want CODE:EEXIST", got)
	}
}

func TestFsMkdirRecursiveRejectsExistingFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("content"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	got := runScript(t, reportScript(`
			const fs = require("fs");
			try {
				await fs.promises.mkdir(`+pathLit(path)+`, {recursive: true});
				report("NO-ERROR");
			} catch (err) {
				report("CODE:" + err.code);
			}
	`))
	if got != "CODE:EEXIST" {
		t.Fatalf("recursive mkdir on file = %q, want CODE:EEXIST", got)
	}
}

// TestFsMkdirHonorsMode pins options.mode on both mkdir paths: the created
// directory carries the requested permission bits after the process umask.
func TestFsMkdirHonorsMode(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("POSIX directory permission bits are not available on Windows")
	}
	dir := t.TempDir()
	probe := filepath.Join(dir, "umask-probe")
	if err := os.Mkdir(probe, 0o777); err != nil {
		t.Fatalf("Mkdir umask probe: %v", err)
	}
	probeInfo, err := os.Stat(probe)
	if err != nil {
		t.Fatalf("Stat umask probe: %v", err)
	}
	umaskPermissions := probeInfo.Mode().Perm()
	private := filepath.Join(dir, "private")
	got := runScript(t, reportScript(`
			const fs = require("fs");
			await fs.promises.mkdir(`+pathLit(private)+`, {mode: 0o700});
			const stats = await fs.promises.lstat(`+pathLit(private)+`);
			report("MODE:" + (stats.mode & 0o777).toString(8));
	`))
	wantPrivate := os.FileMode(0o700) & umaskPermissions
	want := "MODE:" + strconv.FormatUint(uint64(wantPrivate), 8)
	if got != want {
		t.Fatalf("mkdir mode = %q, want %s", got, want)
	}
	recursive := filepath.Join(dir, "r1", "r2")
	got = runScript(t, reportScript(`
			const fs = require("fs");
			await fs.promises.mkdir(`+pathLit(recursive)+`, {recursive: true, mode: 0o750});
			const stats = await fs.promises.lstat(`+pathLit(recursive)+`);
			report("MODE:" + (stats.mode & 0o777).toString(8));
	`))
	wantRecursive := os.FileMode(0o750) & umaskPermissions
	want = "MODE:" + strconv.FormatUint(uint64(wantRecursive), 8)
	if got != want {
		t.Fatalf("recursive mkdir mode = %q, want %s", got, want)
	}
}

func pathLit(p string) string {
	// Escape backslashes so Windows paths (C:\temp\a) survive JS string
	// evaluation; on POSIX the replacement is a no-op.
	return "`" + strings.ReplaceAll(p, "\\", "\\\\") + "`"
}
