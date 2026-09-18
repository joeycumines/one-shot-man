package gateway

import (
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/userk8s/api/v1alpha1"
)

func surfaceEndpoints(endpoints map[string]string) map[string]v1alpha1.SurfaceEndpoint {
	converted := make(map[string]v1alpha1.SurfaceEndpoint, len(endpoints))
	for surface, endpoint := range endpoints {
		converted[surface] = v1alpha1.SurfaceEndpoint(endpoint)
	}
	return converted
}

func shaperAccess(name, provider string, endpoints map[string]string, required ...string) v1alpha1.ModelAccess {
	return v1alpha1.ModelAccess{
		Name: name,
		Spec: v1alpha1.ModelAccessSpec{
			Provider:  provider,
			Mode:      "shaper",
			Auth:      &v1alpha1.ModelAccessAuth{Scheme: v1alpha1.AuthSchemeBearer, RequiredEnv: required},
			Endpoints: surfaceEndpoints(endpoints),
		},
	}
}

func TestMountsMatchTheLiveShaperArguments(t *testing.T) {
	// The expected set mirrors the running shaper's own arguments: prefixes and
	// SHAPER_PROVIDER_<X>_API_KEY variables observed from the live process.
	accesses := []v1alpha1.ModelAccess{
		shaperAccess("camel-shaper", "camel", map[string]string{"anthropic": "http://127.0.0.1:11239/camel", "chat": "http://127.0.0.1:11239/camel/v1", "responses": "http://127.0.0.1:11239/camel/v1"}, "CAMEL_API_KEY"),
		shaperAccess("dialagram-shaper", "dialagram", map[string]string{"responses": "http://127.0.0.1:11239/dialagram/v1"}, "DIALAGRAM_API_KEY"),
		shaperAccess("electronhub-shaper", "electronhub", map[string]string{"anthropic": "http://127.0.0.1:11239/electronhub"}, "ELECTRONHUB_API_KEY"),
		shaperAccess("umans-shaper", "umans", map[string]string{"anthropic": "http://127.0.0.1:11239/umans"}, "UMANS_API_KEY"),
		shaperAccess("verboo-shaper", "verboo", map[string]string{"responses": "http://127.0.0.1:11239/verboo/v1"}, "VERBOO_API_KEY"),
		shaperAccess("yolo-auto-shaper", "yolo-auto", map[string]string{"responses": "http://127.0.0.1:11239/yolo/v1"}, "YOLO_API_KEY"),
		shaperAccess("zen-shaper", "zen", map[string]string{"responses": "http://127.0.0.1:11239/zen/v1"}, "OPENCODE_API_KEY"),
		// Three accesses of one shaper prefix collapse into a single mount.
		shaperAccess("opencode-go-shaper-chat", "opencode-go", map[string]string{"chat": "http://127.0.0.1:11239/zen/go/v1"}, "OPENCODE_API_KEY"),
		shaperAccess("opencode-go-shaper-responses", "opencode-go", map[string]string{"responses": "http://127.0.0.1:11239/zen/go/v1"}, "OPENCODE_API_KEY"),
		shaperAccess("opencode-go-shaper-anthropic", "opencode-go", map[string]string{"anthropic": "http://127.0.0.1:11239/zen/go"}, "OPENCODE_API_KEY"),
	}

	mounts := Mounts(accesses)
	want := map[string]string{
		"/camel":       "SHAPER_PROVIDER_CAMEL_API_KEY",
		"/dialagram":   "SHAPER_PROVIDER_DIALAGRAM_API_KEY",
		"/electronhub": "SHAPER_PROVIDER_ELECTRONHUB_API_KEY",
		"/umans":       "SHAPER_PROVIDER_UMANS_API_KEY",
		"/verboo":      "SHAPER_PROVIDER_VERBOO_API_KEY",
		"/yolo":        "SHAPER_PROVIDER_YOLO_API_KEY",
		"/zen":         "SHAPER_PROVIDER_OPENCODE_API_KEY",
		"/zen/go":      "SHAPER_PROVIDER_OPENCODE_API_KEY",
	}
	if len(mounts) != len(want) {
		t.Fatalf("mounts: got %d (%v), want %d", len(mounts), mounts, len(want))
	}
	for i, mount := range mounts {
		expected, ok := want[mount.Prefix]
		if !ok {
			t.Errorf("unexpected mount %+v", mount)
			continue
		}
		if mount.Credential != expected {
			t.Errorf("mount %s credential: got %q, want %q", mount.Prefix, mount.Credential, expected)
		}
		if i > 0 && mounts[i-1].Prefix >= mount.Prefix {
			t.Errorf("mounts are not ordered by prefix: %q then %q", mounts[i-1].Prefix, mount.Prefix)
		}
	}

	// A direct-mode access and a shaper access without a declared credential
	// contribute nothing.
	direct := v1alpha1.ModelAccess{Name: "zhipu-direct", Spec: v1alpha1.ModelAccessSpec{
		Provider: "zhipu", Mode: "direct", Endpoints: surfaceEndpoints(map[string]string{"chat": "https://api.z.ai/api/coding/paas/v4"}),
	}}
	noCredential := shaperAccess("mystery-shaper", "mystery", map[string]string{"chat": "http://127.0.0.1:11239/mystery/v1"})
	if extra := Mounts([]v1alpha1.ModelAccess{direct, noCredential}); len(extra) != 0 {
		t.Fatalf("mounts from unusable accesses: got %v, want none", extra)
	}
}

func TestShaperEnvVarMapping(t *testing.T) {
	tests := map[string]string{
		"UMANS_API_KEY":    "SHAPER_PROVIDER_UMANS_API_KEY",
		"OPENCODE_API_KEY": "SHAPER_PROVIDER_OPENCODE_API_KEY",
		"YOLO_API_KEY":     "SHAPER_PROVIDER_YOLO_API_KEY",
		"WEIRD":            "SHAPER_PROVIDER_WEIRD_API_KEY",
	}
	for declared, want := range tests {
		if got := ShaperEnvVar(declared); got != want {
			t.Errorf("ShaperEnvVar(%q): got %q, want %q", declared, got, want)
		}
	}
}

func TestChildEnvironmentCarriesOnlyTheCredentials(t *testing.T) {
	mounts := Mounts([]v1alpha1.ModelAccess{
		shaperAccess("umans-shaper", "umans", map[string]string{"anthropic": "http://127.0.0.1:11239/umans"}, "UMANS_API_KEY"),
		shaperAccess("camel-shaper", "camel", map[string]string{"chat": "http://127.0.0.1:11239/camel/v1"}, "CAMEL_API_KEY"),
	})
	environment, err := ChildEnvironment(mounts, map[string]string{
		"UMANS_API_KEY": "umans-secret-value",
		"CAMEL_API_KEY": "camel-secret-value",
	})
	if err != nil {
		t.Fatalf("ChildEnvironment: %v", err)
	}
	if len(environment) != 2 {
		t.Fatalf("environment: got %v, want exactly the two credentials", environment)
	}
	joined := strings.Join(environment, "\n")
	for _, expected := range []string{
		"SHAPER_PROVIDER_UMANS_API_KEY=umans-secret-value",
		"SHAPER_PROVIDER_CAMEL_API_KEY=camel-secret-value",
	} {
		if !strings.Contains(joined, expected) {
			t.Errorf("environment is missing %q", expected)
		}
	}
	// The declared names themselves must not leak into the child's environment:
	// the shaper reads only the SHAPER_PROVIDER_ spelling.
	if strings.Contains(joined, "\nUMANS_API_KEY=") || strings.HasPrefix(joined, "UMANS_API_KEY=") {
		t.Errorf("environment carries the declared name as well: %v", environment)
	}

	if _, err := ChildEnvironment(mounts, map[string]string{"UMANS_API_KEY": "only-one"}); err == nil {
		t.Fatal("ChildEnvironment: want an error when a credential is missing")
	}
	if _, err := ChildEnvironment(mounts, map[string]string{"UMANS_API_KEY": "  ", "CAMEL_API_KEY": "x"}); err == nil {
		t.Fatal("ChildEnvironment: want an error when a credential is blank")
	}
}
