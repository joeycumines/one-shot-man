package userk8s

import (
	"context"
	"strings"
	"testing"
)

// renderedPersonalProfile is a byte-exact copy of the personal profile that
// `gmake render-model-profiles` produces in the joeyc-ai repository, refreshed
// 2026-09-23. It exercises the real registry vocabulary: sanitized
// metadata names carrying one-shot-man/registry-name annotations,
// equivalent_to references written in raw registry spelling, cluster-scoped
// catalog kinds alongside the Namespace and Secret, and 15 selector-matched
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
		{"providers", len(objects.Providers), 13},
		{"accesses", len(objects.Accesses), 15},
		{"models", len(objects.Models), 156},
		{"tools", len(objects.Tools), 9},
		{"bindings", len(objects.Bindings), 15},
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

	for _, access := range objects.Accesses {
		resolution, err := backend.ResolveCredential(ctx, access.Name)
		if err != nil {
			t.Fatalf("ResolveCredential(%s): %v", access.Name, err)
		}
		if resolution.Status != StatusMissingCredentials {
			t.Fatalf("ResolveCredential(%s): status %q, want %q with no configured source (reason %q)", access.Name, resolution.Status, StatusMissingCredentials, resolution.Reason)
		}
		if len(resolution.Credentials) != 0 {
			t.Fatalf("ResolveCredential(%s): resolved credentials without any configured source: %+v", access.Name, resolution.Credentials)
		}
		for _, slot := range access.Spec.Auth.RequiredEnv {
			if !strings.Contains(resolution.Reason, `"`+slot+`"`) {
				t.Fatalf("ResolveCredential(%s): reason %q does not name the uncovered slot %q", access.Name, resolution.Reason, slot)
			}
		}
		if !strings.Contains(resolution.Reason, "attempts:") {
			t.Fatalf("ResolveCredential(%s): reason %q carries no resolver failure classes", access.Name, resolution.Reason)
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
	for _, model := range objects.Models {
		if !model.Spec.Deprecated {
			nonDeprecated++
		}
	}
	for _, tool := range objects.Tools {
		if tool.Spec.BudgetProfile == "" {
			t.Fatalf("Tool %q declares no budget profile", tool.Name)
		}
		for i := range objects.Models {
			model := &objects.Models[i]
			if model.Spec.Deprecated {
				continue
			}
			attempted++
			projection, err := backend.ProjectModel(ctx, tool.Name, model)
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
				t.Fatalf("Project(%s,%s,%s): %v", tool.Name, model.Spec.Provider, model.Name, err)
			}
			projected++
			if projection.BudgetProfile != tool.Spec.BudgetProfile {
				t.Fatalf("Project(%s,%s,%s): budget profile %q, want %q", tool.Name, model.Spec.Provider, model.Name, projection.BudgetProfile, tool.Spec.BudgetProfile)
			}
			if projection.ProviderSlug == "" || projection.ModelSlug == "" {
				t.Fatalf("Project(%s,%s,%s): empty slug in %+v", tool.Name, model.Spec.Provider, model.Name, projection)
			}
			contextWindow, ok := projection.Settings["context_window"].(int64)
			if !ok || contextWindow <= 0 {
				t.Fatalf("Project(%s,%s,%s): settings.context_window missing or non-positive: %v", tool.Name, model.Spec.Provider, model.Name, projection.Settings["context_window"])
			}
		}
	}
	if projected == 0 {
		t.Fatal("Project: no tool/selection pair projected successfully")
	}
	if attempted != len(objects.Tools)*nonDeprecated {
		t.Fatalf("Project: sweep attempted %d pairs, want every tool x non-deprecated model (%d x %d); the sweep is not exercising the real catalog", attempted, len(objects.Tools), nonDeprecated)
	}
	if projected+grammarRejections != attempted {
		t.Fatalf("Project: %d projected + %d grammar rejections != %d attempted", projected, grammarRejections, attempted)
	}
	if grammarRejections > 300 {
		t.Fatalf("Project: %d selections are inexpressible; derivation regressed", grammarRejections)
	}
}
