package termmux_test

import (
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestModuleIsolation enforces the module-extraction contract: the termmux
// package tree (internal/termmux/...) must have zero imports from osm-internal
// packages outside the termmux subtree. This enables extracting termmux as a
// standalone Go module without dragging in osm-specific code.
//
// If this test fails, a new import was added that couples termmux to the
// broader osm codebase. Either remove the import or create a follow-up task
// to refactor the dependency.
func TestModuleIsolation(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping module isolation test in short mode")
	}
	t.Parallel()

	// Locate the module root via go env GOMOD.
	gomod, err := exec.Command("go", "env", "GOMOD").CombinedOutput()
	if err != nil {
		t.Fatalf("go env GOMOD failed: %v\n%s", err, gomod)
	}
	moduleRoot := filepath.Dir(strings.TrimSpace(string(gomod)))
	if moduleRoot == "" || moduleRoot == "." {
		t.Fatal("could not determine module root from go env GOMOD")
	}

	// Determine the module path from go list -m.
	modPath, err := exec.Command("go", "list", "-m").CombinedOutput()
	if err != nil {
		t.Fatalf("go list -m failed: %v\n%s", err, modPath)
	}
	moduleName := strings.TrimSpace(string(modPath))

	// List all transitive dependencies of the termmux package tree.
	cmd := exec.Command("go", "list", "-deps", "./internal/termmux/...")
	cmd.Dir = moduleRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("go list -deps failed: %v\n%s", err, out)
	}

	// The only allowed internal imports are within the termmux subtree itself.
	internalPrefix := moduleName + "/internal/"
	allowedPrefix := moduleName + "/internal/termmux"

	var violations []string
	for _, dep := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if dep == "" {
			continue
		}
		if strings.HasPrefix(dep, internalPrefix) && !strings.HasPrefix(dep, allowedPrefix) {
			violations = append(violations, dep)
		}
	}

	if len(violations) > 0 {
		t.Errorf("termmux has %d forbidden internal dependencies (must be extractable as standalone module):", len(violations))
		for _, v := range violations {
			t.Errorf("  - %s", v)
		}
	}
}
