package scripting

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestComputePathLCA(t *testing.T) {
	t.Run("EmptyPaths", func(t *testing.T) {
		if got := computePathLCA(nil); got != "" {
			t.Fatalf("expected empty LCA for nil paths, got %q", got)
		}
		if got := computePathLCA([]string{}); got != "" {
			t.Fatalf("expected empty LCA for empty paths, got %q", got)
		}
	})

	t.Run("SingleFileNoDir", func(t *testing.T) {
		// file.go has no directory component → LCA is ""
		if got := computePathLCA([]string{"file.go"}); got != "" {
			t.Fatalf("expected empty LCA for root-level file, got %q", got)
		}
	})

	t.Run("SingleFileWithDir", func(t *testing.T) {
		got := computePathLCA([]string{filepath.Join("src", "main.go")})
		if got != "src" {
			t.Fatalf("expected LCA 'src', got %q", got)
		}
	})

	t.Run("TwoFilesCommonPrefix", func(t *testing.T) {
		paths := []string{
			filepath.Join("src", "internal", "api", "handlers.go"),
			filepath.Join("src", "internal", "db", "models.go"),
		}
		got := computePathLCA(paths)
		want := filepath.Join("src", "internal")
		if got != want {
			t.Fatalf("expected LCA %q, got %q", want, got)
		}
	})

	t.Run("TwoFilesDivergent", func(t *testing.T) {
		paths := []string{
			filepath.Join("frontend", "src", "App.tsx"),
			filepath.Join("backend", "src", "main.go"),
		}
		got := computePathLCA(paths)
		if got != "" {
			t.Fatalf("expected empty LCA for divergent paths, got %q", got)
		}
	})

	t.Run("MixedDepths", func(t *testing.T) {
		paths := []string{
			filepath.Join("pkg", "api", "v1", "handler.go"),
			filepath.Join("pkg", "util.go"),
			filepath.Join("pkg", "api", "router.go"),
		}
		got := computePathLCA(paths)
		// util.go is directly in pkg/ with no subdir → its dir is "pkg"
		// Others are pkg/api/... → common is "pkg"
		if got != "pkg" {
			t.Fatalf("expected LCA 'pkg', got %q", got)
		}
	})

	t.Run("AllRootLevel", func(t *testing.T) {
		paths := []string{"a.go", "b.go", "c.go"}
		got := computePathLCA(paths)
		if got != "" {
			t.Fatalf("expected empty LCA for root-level files, got %q", got)
		}
	})

	t.Run("DeepCommonPrefix", func(t *testing.T) {
		paths := []string{
			filepath.Join("a", "b", "c", "d", "x.go"),
			filepath.Join("a", "b", "c", "d", "y.go"),
			filepath.Join("a", "b", "c", "e", "z.go"),
		}
		got := computePathLCA(paths)
		want := filepath.Join("a", "b", "c")
		if got != want {
			t.Fatalf("expected LCA %q, got %q", want, got)
		}
	})
}

func TestToTxtar_LCAComment(t *testing.T) {
	t.Run("IncludesContextRoot", func(t *testing.T) {
		dir := t.TempDir()
		f := filepath.Join(dir, "file.go")
		if err := os.WriteFile(f, []byte("package main\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		cm, err := NewContextManager(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := cm.AddPath(f); err != nil {
			t.Fatal(err)
		}

		archive := cm.ToTxtar()
		comment := string(archive.Comment)
		if !strings.Contains(comment, "context root:") {
			t.Fatalf("expected 'context root:' in comment, got: %q", comment)
		}
		// The root should be a slash-normalized path
		if !strings.Contains(comment, filepath.ToSlash(dir)) {
			t.Fatalf("expected base path in comment, got: %q", comment)
		}
	})

	t.Run("IncludesCommonPathWhenPresent", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite := func(p, s string) {
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
				t.Fatal(err)
			}
		}
		mustWrite(filepath.Join(dir, "src", "internal", "a.go"), "a")
		mustWrite(filepath.Join(dir, "src", "internal", "b.go"), "b")

		cm, err := NewContextManager(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := cm.AddPath(filepath.Join(dir, "src", "internal", "a.go")); err != nil {
			t.Fatal(err)
		}
		if err := cm.AddPath(filepath.Join(dir, "src", "internal", "b.go")); err != nil {
			t.Fatal(err)
		}

		archive := cm.ToTxtar()
		comment := string(archive.Comment)
		if !strings.Contains(comment, "common path: src/internal") {
			t.Fatalf("expected 'common path: src/internal' in comment, got: %q", comment)
		}
	})

	t.Run("OmitsCommonPathWhenAllRootLevel", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "a.go"), []byte("a"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "b.go"), []byte("b"), 0o644); err != nil {
			t.Fatal(err)
		}

		cm, err := NewContextManager(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := cm.AddPath(filepath.Join(dir, "a.go")); err != nil {
			t.Fatal(err)
		}
		if err := cm.AddPath(filepath.Join(dir, "b.go")); err != nil {
			t.Fatal(err)
		}

		archive := cm.ToTxtar()
		comment := string(archive.Comment)
		if strings.Contains(comment, "common path:") {
			t.Fatalf("expected no 'common path:' for root-level-only files, got: %q", comment)
		}
	})

	t.Run("IncludesTrackedDirectories", func(t *testing.T) {
		dir := t.TempDir()
		subdir := filepath.Join(dir, "mydir")
		if err := os.MkdirAll(subdir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(subdir, "x.go"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}

		cm, err := NewContextManager(dir)
		if err != nil {
			t.Fatal(err)
		}
		// Add directory (not just file)
		if err := cm.AddPath(subdir); err != nil {
			t.Fatal(err)
		}

		archive := cm.ToTxtar()
		comment := string(archive.Comment)
		if !strings.Contains(comment, "tracked directories:") {
			t.Fatalf("expected 'tracked directories:' in comment, got: %q", comment)
		}
		if !strings.Contains(comment, "mydir/") {
			t.Fatalf("expected 'mydir/' in tracked directories, got: %q", comment)
		}
	})

	t.Run("EmptyContextHasRootOnly", func(t *testing.T) {
		dir := t.TempDir()
		cm, err := NewContextManager(dir)
		if err != nil {
			t.Fatal(err)
		}

		archive := cm.ToTxtar()
		comment := string(archive.Comment)
		if !strings.Contains(comment, "context root:") {
			t.Fatalf("expected 'context root:' even for empty context, got: %q", comment)
		}
		if strings.Contains(comment, "common path:") {
			t.Fatalf("expected no 'common path:' for empty context, got: %q", comment)
		}
		if strings.Contains(comment, "tracked directories:") {
			t.Fatalf("expected no 'tracked directories:' for empty context, got: %q", comment)
		}
	})
}

func TestToTxtar_FullPathPreservation(t *testing.T) {
	t.Run("CollidingBasenamesPreserveFullRelativePath", func(t *testing.T) {
		// Key test case for the false proximity bug fix.
		// Two files share basename but have a common prefix that would
		// be stripped by minimal-suffix disambiguation.
		dir := t.TempDir()
		mustWrite := func(p, s string) {
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		// src/internal/api/handlers.go and src/external/api/handlers.go
		// With minimal suffix: "internal/api/handlers.go" vs "external/api/handlers.go"
		//   (strips "src/" prefix — looks like top-level dirs)
		// With full path: "src/internal/api/handlers.go" vs "src/external/api/handlers.go"
		//   (preserves common "src/" context)
		f1 := filepath.Join(dir, "src", "internal", "api", "handlers.go")
		f2 := filepath.Join(dir, "src", "external", "api", "handlers.go")
		mustWrite(f1, "package internal_api\n")
		mustWrite(f2, "package external_api\n")

		cm, err := NewContextManager(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := cm.AddPath(f1); err != nil {
			t.Fatal(err)
		}
		if err := cm.AddPath(f2); err != nil {
			t.Fatal(err)
		}

		txt := cm.GetTxtarString()

		// Should use FULL relative paths, not minimal suffixes
		if !strings.Contains(txt, "-- src/internal/api/handlers.go --") {
			t.Fatalf("expected full relative path src/internal/api/handlers.go, got: %s", txt)
		}
		if !strings.Contains(txt, "-- src/external/api/handlers.go --") {
			t.Fatalf("expected full relative path src/external/api/handlers.go, got: %s", txt)
		}

		// Verify the LCA comment includes the common path
		if !strings.Contains(txt, "common path: src") {
			t.Fatalf("expected 'common path: src' in comment, got: %s", txt)
		}
	})

	t.Run("DeepCollidingWithCommonPrefix", func(t *testing.T) {
		// Files:
		//   app/frontend/components/Button.tsx
		//   app/backend/components/Button.tsx
		// Minimal suffix would give: frontend/components/Button.tsx, backend/components/Button.tsx
		// Full path preserves the app/ prefix
		dir := t.TempDir()
		mustWrite := func(p, s string) {
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		f1 := filepath.Join(dir, "app", "frontend", "components", "Button.tsx")
		f2 := filepath.Join(dir, "app", "backend", "components", "Button.tsx")
		mustWrite(f1, "export default Button\n")
		mustWrite(f2, "export default Button\n")

		cm, err := NewContextManager(dir)
		if err != nil {
			t.Fatal(err)
		}
		if err := cm.AddPath(f1); err != nil {
			t.Fatal(err)
		}
		if err := cm.AddPath(f2); err != nil {
			t.Fatal(err)
		}

		txt := cm.GetTxtarString()

		if !strings.Contains(txt, "-- app/frontend/components/Button.tsx --") {
			t.Fatalf("expected full path with app/ prefix, got: %s", txt)
		}
		if !strings.Contains(txt, "-- app/backend/components/Button.tsx --") {
			t.Fatalf("expected full path with app/ prefix, got: %s", txt)
		}
	})

	t.Run("NonCollidingBasenamesPreserveFullPath", func(t *testing.T) {
		dir := t.TempDir()
		mustWrite := func(p, s string) {
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		f1 := filepath.Join(dir, "src", "main.go")
		f2 := filepath.Join(dir, "docs", "README.md")
		f3 := filepath.Join(dir, "config", "app.yaml")
		mustWrite(f1, "package main\n")
		mustWrite(f2, "# docs\n")
		mustWrite(f3, "app: test\n")

		cm, err := NewContextManager(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range []string{f1, f2, f3} {
			if err := cm.AddPath(f); err != nil {
				t.Fatal(err)
			}
		}

		txt := cm.GetTxtarString()

		// Non-colliding basenames should use full relative paths
		if !strings.Contains(txt, "-- src/main.go --") {
			t.Fatalf("expected 'src/main.go', got: %s", txt)
		}
		if !strings.Contains(txt, "-- docs/README.md --") {
			t.Fatalf("expected 'docs/README.md', got: %s", txt)
		}
		if !strings.Contains(txt, "-- config/app.yaml --") {
			t.Fatalf("expected 'config/app.yaml', got: %s", txt)
		}
	})

	t.Run("MixedCollidingAndNonColliding", func(t *testing.T) {
		// Verify that colliding basenames using full paths don't create
		// false proximity with non-colliding files
		dir := t.TempDir()
		mustWrite := func(p, s string) {
			if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(p, []byte(s), 0o644); err != nil {
				t.Fatal(err)
			}
		}

		// handlers.go collides; utils.go is unique
		f1 := filepath.Join(dir, "src", "api", "handlers.go")
		f2 := filepath.Join(dir, "src", "web", "handlers.go")
		f3 := filepath.Join(dir, "src", "util", "helpers.go")
		mustWrite(f1, "package api\n")
		mustWrite(f2, "package web\n")
		mustWrite(f3, "package util\n")

		cm, err := NewContextManager(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, f := range []string{f1, f2, f3} {
			if err := cm.AddPath(f); err != nil {
				t.Fatal(err)
			}
		}

		txt := cm.GetTxtarString()

		// Colliding: full paths preserving src/ prefix
		if !strings.Contains(txt, "-- src/api/handlers.go --") {
			t.Fatalf("expected 'src/api/handlers.go', got: %s", txt)
		}
		if !strings.Contains(txt, "-- src/web/handlers.go --") {
			t.Fatalf("expected 'src/web/handlers.go', got: %s", txt)
		}
		// Non-colliding: also full path
		if !strings.Contains(txt, "-- src/util/helpers.go --") {
			t.Fatalf("expected 'src/util/helpers.go', got: %s", txt)
		}
	})
}
