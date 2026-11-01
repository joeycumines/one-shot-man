package userk8s

import (
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/userk8s/api/v1alpha1"
)

const offerProvider = `apiVersion: one-shot-man/v1alpha1
kind: ModelProvider
metadata:
  name: alpha
  annotations:
    one-shot-man/registry-name: alpha:cloud
spec:
  display_name: Alpha
`

const offerModel = `apiVersion: one-shot-man/v1alpha1
kind: Model
metadata:
  name: alpha-model
spec:
  canonical_id: alpha/alpha-model
  display_name: Alpha Model
  context_window: 1000
  max_output_tokens: 10
`

const offerAccess = `apiVersion: one-shot-man/v1alpha1
kind: ModelAccess
metadata:
  name: alpha-chat
spec:
  provider: alpha
  protocol: chat
`

func offerServing(name string, deprecated bool) string {
	serving := `apiVersion: one-shot-man/v1alpha1
kind: ModelServing
metadata:
  name: ` + name + `
spec:
  provider: alpha
  model: alpha-model
  provider_model_id: ` + name + `
`
	if deprecated {
		serving += "  deprecated: true\n"
	}
	return serving
}

func offerRoute(name string, offers string) string {
	if offers == "" {
		offers = "  offers: []\n"
	} else {
		offers = "  offers:\n" + offers
	}
	return `apiVersion: one-shot-man/v1alpha1
kind: ModelRoute
metadata:
  name: ` + name + `
  namespace: test
spec:
  access: alpha-chat
` + offers + `  surfaces:
  - chat
  endpoints:
    chat: http://127.0.0.1:11239/` + name + `/v1
  naming: served
  transport:
    kind: shaper
    upstream: https://alpha.example
`
}

// TestRouteOffersMatchesProviderByRegistryName proves providers are compared as
// resolved objects. The access names the provider by metadata name and the
// serving by its registry name; raw string equality would treat them as
// different providers and reject a perfectly valid catalog.
func TestRouteOffersMatchesProviderByRegistryName(t *testing.T) {
	serving := `apiVersion: one-shot-man/v1alpha1
kind: ModelServing
metadata:
  name: alpha-serving
spec:
  provider: alpha:cloud
  model: alpha-model
  provider_model_id: alpha-wire
`
	objects := documents(t, offerProvider, offerModel, offerAccess, serving, offerRoute("alpha-local", ""))
	offers, err := objects.routeOffers(&objects.Routes[0])
	if err != nil {
		t.Fatalf("routeOffers: %v", err)
	}
	if len(offers) != 1 || offers[0].Serving.Name != "alpha-serving" {
		t.Fatalf("offers: got %+v, want the serving whose provider is spelled by its registry name", offers)
	}
}

// TestRouteOffersHonoursExplicitlyPinnedDeprecatedServing pins the documented
// contract: "a deprecated Model hides every serving unless one is explicitly
// pinned". An explicit route offer is a pin; the defaulted inventory still
// filters deprecated entries.
func TestRouteOffersHonoursExplicitlyPinnedDeprecatedServing(t *testing.T) {
	objects := documents(t,
		offerProvider,
		`apiVersion: one-shot-man/v1alpha1
kind: Model
metadata:
  name: alpha-model
spec:
  canonical_id: alpha/alpha-model
  display_name: Alpha Model
  context_window: 1000
  max_output_tokens: 10
  deprecated: true
`,
		offerAccess,
		offerServing("alpha-retired", false),
		offerServing("alpha-live", false),
		offerRoute("alpha-pinned", "  - serving: alpha-retired\n"),
		offerRoute("alpha-defaulted", ""),
	)

	pinned, err := objects.routeOffers(lookupRouteOrFail(t, objects, "alpha-pinned"))
	if err != nil {
		t.Fatalf("routeOffers(pinned): %v", err)
	}
	if len(pinned) != 1 || pinned[0].Serving.Name != "alpha-retired" {
		t.Fatalf("pinned offers: got %+v, want the explicitly listed deprecated Model serving", pinned)
	}

	defaulted, err := objects.routeOffers(lookupRouteOrFail(t, objects, "alpha-defaulted"))
	if err != nil {
		t.Fatalf("routeOffers(defaulted): %v", err)
	}
	if len(defaulted) != 0 {
		t.Fatalf("defaulted offers: got %+v, want none — a deprecated Model hides its servings unless pinned", defaulted)
	}
}

// TestRouteOffersReportsMisconfigurationOnDeprecatedEntries makes sure the
// deprecation filter no longer masks real errors: a provider mismatch and a
// duplicate served id are both reported even when the entries are deprecated.
func TestRouteOffersReportsMisconfigurationOnDeprecatedEntries(t *testing.T) {
	mismatch := `apiVersion: one-shot-man/v1alpha1
kind: ModelProvider
metadata:
  name: beta
spec:
  display_name: Beta
`
	t.Run("provider mismatch on a deprecated serving", func(t *testing.T) {
		_, err := loadDocuments(
			[]byte(offerProvider), []byte(mismatch), []byte(offerModel), []byte(offerAccess),
			[]byte(`apiVersion: one-shot-man/v1alpha1
kind: ModelServing
metadata:
  name: beta-serving
spec:
  provider: beta
  model: alpha-model
  provider_model_id: beta-wire
  deprecated: true
`),
			[]byte(offerRoute("alpha-local", "  - serving: beta-serving\n")),
		)
		if err == nil || !strings.Contains(err.Error(), "belongs to provider") {
			t.Fatalf("error: got %v, want a provider-mismatch refusal", err)
		}
	})

	t.Run("duplicate id on deprecated servings", func(t *testing.T) {
		_, err := loadDocuments(
			[]byte(offerProvider), []byte(offerModel), []byte(offerAccess),
			[]byte(offerServing("alpha-one", true)),
			[]byte(offerServing("alpha-two", true)),
			[]byte(offerRoute("alpha-local", "  - serving: alpha-one\n    id: shared\n  - serving: alpha-two\n    id: shared\n")),
		)
		if err == nil || !strings.Contains(err.Error(), "more than once") {
			t.Fatalf("error: got %v, want a duplicate-id refusal", err)
		}
	})
}

// lookupRouteOrFail resolves a route by name in a test catalog.
func lookupRouteOrFail(t *testing.T, o *Objects, name string) *v1alpha1.ModelRoute {
	t.Helper()
	route, ok := o.lookupRoute(name)
	if !ok {
		t.Fatalf("no route %q", name)
	}
	return route
}

// TestRetiredRouteDoesNotClaimItsMountPrefix proves a route whose defaulted
// inventory has no live offering cannot block a live route on the same mount
// path: the gateway never spawns a section for it, so reserving its prefix would
// refuse a working catalog. The retired route's inventory must be DEFAULTED — an
// explicitly listed deprecated serving is a pin, and a pinned route really does
// serve.
func TestRetiredRouteDoesNotClaimItsMountPrefix(t *testing.T) {
	beta := `apiVersion: one-shot-man/v1alpha1
kind: ModelProvider
metadata:
  name: beta
spec:
  display_name: Beta
`
	betaAccess := `apiVersion: one-shot-man/v1alpha1
kind: ModelAccess
metadata:
  name: beta-chat
spec:
  provider: beta
  protocol: chat
`
	deprecatedServing := `apiVersion: one-shot-man/v1alpha1
kind: ModelServing
metadata:
  name: alpha-retired
spec:
  provider: alpha
  model: alpha-model
  provider_model_id: alpha-retired-wire
  deprecated: true
`
	// Only the alpha serving is deprecated; beta's is live.
	liveBetaServing := `apiVersion: one-shot-man/v1alpha1
kind: ModelServing
metadata:
  name: beta-live
spec:
  provider: beta
  model: alpha-model
  provider_model_id: beta-live-wire
`
	sharedMount := func(name, access string) string {
		return `apiVersion: one-shot-man/v1alpha1
kind: ModelRoute
metadata:
  name: ` + name + `
  namespace: test
spec:
  access: ` + access + `
  offers: []
  surfaces:
  - chat
  endpoints:
    chat: http://127.0.0.1:11239/shared/v1
  naming: served
  transport:
    kind: shaper
    upstream: https://` + name + `.example
`
	}
	_, err := loadDocuments(toBytes([]string{
		offerProvider, beta, offerModel, offerAccess, betaAccess,
		deprecatedServing, liveBetaServing,
		sharedMount("alpha-retired-route", "alpha-chat"),
		sharedMount("beta-live-route", "beta-chat"),
	})...)
	if err != nil {
		t.Fatalf("a route with no live offering must not block a live route on the same mount: %v", err)
	}
}
