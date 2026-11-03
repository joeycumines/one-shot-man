package userk8s

import (
	"context"
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/userk8s/api/v1alpha1"
)

// renderedPersonalProfile is a byte-exact copy of the personal profile that
// `gmake render-model-profiles` produces in the joeyc-ai repository. It
// exercises the real registry vocabulary: sanitized
// metadata names carrying one-shot-man/registry-name annotations,
// equivalent_to references written in raw registry spelling, cluster-scoped
// catalog kinds alongside the Namespace and Secret, and 13 selector-matched
// bindings whose local chains are command (1Password, optional) + env only —
// no file resolvers, per the local no-secret-files policy.
const renderedPersonalProfile = "testdata/rendered-personal-profile.yaml"

func TestLoadRealRenderedProfile(t *testing.T) {
	objects, err := LoadPaths(renderedPersonalProfile)
	if err != nil {
		t.Fatalf("LoadPaths(%s): %v", renderedPersonalProfile, err)
	}
	for _, want := range []struct {
		name string
		got  int
		want int
	}{
		{"providers", len(objects.Providers), 16},
		{"accesses", len(objects.Accesses), 15},
		{"models", len(objects.Models), 172},
		{"servings", len(objects.Servings), 239},
		{"selections", len(objects.Selections), 1},
		{"routes", len(objects.Routes), 14},
		{"tools", len(objects.Tools), 9},
		{"bindings", len(objects.Bindings), 13},
		{"skipped", objects.Skipped, 2},
	} {
		if want.got != want.want {
			t.Errorf("%s: got %d, want %d", want.name, want.got, want.want)
		}
	}
	if len(objects.Artifacts) != 1 {
		t.Errorf("artifacts: got %v, want one entry", objects.Artifacts)
	}
}

func TestRealRenderedProfileResolvesWithoutError(t *testing.T) {
	objects, err := LoadPaths(renderedPersonalProfile)
	if err != nil {
		t.Fatalf("LoadPaths(%s): %v", renderedPersonalProfile, err)
	}
	backend := mustBackend(t, objects, &fakeRunner{})
	ctx := context.Background()

	// Isolate the resolution inputs so the assertions below cannot depend on
	// the developer's shell: every declared environment slot is emptied and
	// HOME points at an empty directory, so file resolvers miss too.
	t.Setenv("HOME", t.TempDir())
	for _, binding := range objects.Bindings {
		for _, step := range binding.Spec.Resolvers {
			if step.Env != nil {
				t.Setenv(step.Env.Name, "")
			}
		}
	}

	for i := range objects.Bindings {
		binding := &objects.Bindings[i]
		resolution, err := backend.ResolveCredential(ctx, binding.Name)
		if err != nil {
			t.Fatalf("ResolveCredential(%s): %v", binding.Name, err)
		}
		if binding.Spec.Scheme == "" || binding.Spec.Scheme == v1alpha1.CredentialNone {
			if resolution.Status != StatusResolved {
				t.Fatalf("ResolveCredential(%s): status %q, want resolved for a credential-less binding", binding.Name, resolution.Status)
			}
			continue
		}
		if resolution.Status != StatusMissingCredentials {
			t.Fatalf("ResolveCredential(%s): status %q, want %q with no configured source (reason %q)", binding.Name, resolution.Status, StatusMissingCredentials, resolution.Reason)
		}
		if len(resolution.Credentials) != 0 {
			t.Fatalf("ResolveCredential(%s): resolved credentials without any configured source: %+v", binding.Name, resolution.Credentials)
		}
		if !strings.Contains(resolution.Reason, "did not resolve") {
			t.Fatalf("ResolveCredential(%s): reason %q does not report the failed binding", binding.Name, resolution.Reason)
		}
	}
}

func TestRealRenderedProfileProjectsEveryToolSelection(t *testing.T) {
	objects, err := LoadPaths(renderedPersonalProfile)
	if err != nil {
		t.Fatalf("LoadPaths(%s): %v", renderedPersonalProfile, err)
	}
	backend := mustBackend(t, objects, &fakeRunner{})
	ctx := context.Background()

	projected := 0
	grammarRejections := 0
	attempted := 0
	nonDeprecated := 0
	for i := range objects.Servings {
		if !objects.Servings[i].Spec.Deprecated {
			nonDeprecated++
		}
	}
	for _, tool := range objects.Tools {
		if tool.Spec.BudgetProfile == "" {
			t.Fatalf("Tool %q declares no budget profile", tool.Name)
		}
		for i := range objects.Servings {
			serving := &objects.Servings[i]
			if serving.Spec.Deprecated {
				continue
			}
			attempted++
			projection, err := backend.ProjectServing(ctx, tool.Name, serving)
			if err != nil {
				// A registry slug that a tool's grammar cannot express is a
				// legitimate incompatibility; anything else is a defect. (The
				// historical example, codex's "~openai/gpt-latest", left the
				// catalog on 2026-09-23 when it was removed as unserveable
				// through the shaper; the guard below now pins sweep coverage
				// directly instead of depending on an inexpressible pair
				// existing.)
				if strings.Contains(err.Error(), "cannot derive") {
					grammarRejections++
					continue
				}
				t.Fatalf("Project(%s,%s,%s): %v", tool.Name, serving.Spec.Provider, serving.Name, err)
			}
			projected++
			if projection.BudgetProfile != tool.Spec.BudgetProfile {
				t.Fatalf("Project(%s,%s,%s): budget profile %q, want %q", tool.Name, serving.Spec.Provider, serving.Name, projection.BudgetProfile, tool.Spec.BudgetProfile)
			}
			if projection.ProviderSlug == "" || projection.ModelSlug == "" {
				t.Fatalf("Project(%s,%s,%s): empty slug in %+v", tool.Name, serving.Spec.Provider, serving.Name, projection)
			}
			contextWindow, ok := projection.Settings["context_window"].(int64)
			if !ok || contextWindow <= 0 {
				t.Fatalf("Project(%s,%s,%s): settings.context_window missing or non-positive: %v", tool.Name, serving.Spec.Provider, serving.Name, projection.Settings["context_window"])
			}
		}
	}
	if projected == 0 {
		t.Fatal("Project: no tool/selection pair projected successfully")
	}
	if attempted != len(objects.Tools)*nonDeprecated {
		t.Fatalf("Project: sweep attempted %d pairs, want every tool x non-deprecated serving (%d x %d); the sweep is not exercising the real catalog", attempted, len(objects.Tools), nonDeprecated)
	}
	if projected+grammarRejections != attempted {
		t.Fatalf("Project: %d projected + %d grammar rejections != %d attempted", projected, grammarRejections, attempted)
	}
	if grammarRejections > 300 {
		t.Fatalf("Project: %d selections are inexpressible; derivation regressed", grammarRejections)
	}
}
