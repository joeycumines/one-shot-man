package userk8s

import (
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/userk8s/api/v1alpha1"
)

const routeValidationProvider = `apiVersion: one-shot-man/v1alpha1
kind: ModelProvider
metadata:
  name: alpha
spec:
  display_name: Alpha
`

const routeValidationModel = `apiVersion: one-shot-man/v1alpha1
kind: Model
metadata:
  name: alpha-model
spec:
  canonical_id: alpha/alpha-model
  display_name: Alpha Model
  context_window: 128000
  max_output_tokens: 16384
  input_modalities:
  - text
  output_modalities:
  - text
  tool_call: true
`

const routeValidationServing = `apiVersion: one-shot-man/v1alpha1
kind: ModelServing
metadata:
  name: alpha-model-alpha
spec:
  provider: alpha
  model: alpha-model
  provider_model_id: alpha-wire
`

func routeValidationDocuments(extra ...string) [][]byte {
	documents := [][]byte{
		[]byte(routeValidationProvider),
		[]byte(`apiVersion: one-shot-man/v1alpha1
kind: ModelAccess
metadata:
  name: alpha-chat
  labels:
    one-shot-man/provider: alpha
spec:
  provider: alpha
  protocol: chat
`),
		[]byte(routeValidationModel),
		[]byte(routeValidationServing),
	}
	for _, document := range extra {
		documents = append(documents, []byte(document))
	}
	return documents
}

func routeValidationRoute(name, access, credential, authMode string) string {
	route := `apiVersion: one-shot-man/v1alpha1
kind: ModelRoute
metadata:
  name: ` + name + `
  namespace: test
spec:
  access: ` + access + `
  offers:
  - serving: alpha-model-alpha
`
	if credential != "" {
		route += "  credential: " + credential + "\n"
	}
	route += `  surfaces:
  - chat
  endpoints:
    chat: http://127.0.0.1:1/` + name + `/v1
  naming: served
  transport:
    kind: shaper
    upstream: https://alpha.example
`
	if authMode != "" {
		route += "    shaper:\n      auth_mode: " + authMode + "\n"
	}
	return route
}

func TestRouteValidationRefusesUndeclaredCredential(t *testing.T) {
	_, err := loadDocuments(routeValidationDocuments(
		routeValidationRoute("alpha-local", "alpha-chat", "missing-binding", ""),
	)...)
	if err == nil || !strings.Contains(err.Error(), `references undeclared LocalSecretBinding "missing-binding"`) {
		t.Fatalf("error: got %v", err)
	}
}

func TestRouteValidationRefusesCredentialAuthModeWithoutCredential(t *testing.T) {
	_, err := loadDocuments(routeValidationDocuments(
		routeValidationRoute("alpha-local", "alpha-chat", "", "bearer"),
	)...)
	if err == nil || !strings.Contains(err.Error(), "names no credential") {
		t.Fatalf("error: got %v", err)
	}
}

func TestRouteValidationRefusesCredentialAuthModeWithNoneScheme(t *testing.T) {
	binding := `apiVersion: one-shot-man/v1alpha1
kind: LocalSecretBinding
metadata:
  name: none-binding
  namespace: test
spec:
  selector:
    matchLabels:
      one-shot-man/provider: alpha
  secretRef:
    name: none
    key: NONE
  scheme: none
  envVar: UNUSED
`
	_, err := loadDocuments(routeValidationDocuments(
		binding,
		routeValidationRoute("alpha-local", "alpha-chat", "none-binding", "bearer"),
	)...)
	if err == nil || !strings.Contains(err.Error(), "names no credential") {
		t.Fatalf("error: got %v", err)
	}
}

func TestRouteValidationAllowsNoAuthModeWithoutCredential(t *testing.T) {
	_, err := loadDocuments(routeValidationDocuments(
		routeValidationRoute("alpha-local", "alpha-chat", "", "none"),
	)...)
	if err != nil {
		t.Fatalf("loadDocuments: %v", err)
	}
}

func TestRouteValidationRefusesSurfaceTheProtocolCannotServe(t *testing.T) {
	messagesAccess := `apiVersion: one-shot-man/v1alpha1
kind: ModelAccess
metadata:
  name: alpha-messages
  labels:
    one-shot-man/provider: alpha
spec:
  provider: alpha
  protocol: messages
`
	route := `apiVersion: one-shot-man/v1alpha1
kind: ModelRoute
metadata:
  name: alpha-local
  namespace: test
spec:
  access: alpha-messages
  offers:
  - serving: alpha-model-alpha
  surfaces:
  - chat
  endpoints:
    chat: http://127.0.0.1:1/alpha/v1
  naming: served
  transport:
    kind: shaper
    upstream: https://alpha.example
`
	_, err := loadDocuments(routeValidationDocuments(messagesAccess, route)...)
	if err == nil || !strings.Contains(err.Error(), `publishes surface "chat", which protocol "messages" cannot serve`) {
		t.Fatalf("error: got %v", err)
	}
}

// TestRouteValidationAllowsTwoShaperRoutesForOneAccess pins the documented
// multi-account design: ModelRoute.metadata.name is the shaper section identity,
// so several routes (accounts, regions, protocols) may dial the same logical
// ModelAccess. Each route here has a distinct mount prefix, which is what the
// shaper actually needs to keep them apart.
func TestRouteValidationAllowsTwoShaperRoutesForOneAccess(t *testing.T) {
	_, err := loadDocuments(routeValidationDocuments(
		routeValidationRoute("alpha-one", "alpha-chat", "", ""),
		routeValidationRoute("alpha-two", "alpha-chat", "", ""),
	)...)
	if err != nil {
		t.Fatalf("two routes over one access with distinct mounts must load: %v", err)
	}
}

func TestRouteValidationNormalizesSurfaceAPIPathsToOneMount(t *testing.T) {
	route := `apiVersion: one-shot-man/v1alpha1
kind: ModelRoute
metadata:
  name: alpha-local
  namespace: test
spec:
  access: alpha-chat
  offers:
  - serving: alpha-model-alpha
  surfaces:
  - anthropic
  - chat
  - responses
  endpoints:
    anthropic: http://127.0.0.1:11239/alpha/anthropic
    chat: http://127.0.0.1:11239/alpha/openai/v1
    responses: http://127.0.0.1:11239/alpha/openai/v1
  naming: served
  transport:
    kind: shaper
    upstream: https://alpha.example
`
	_, err := loadDocuments(routeValidationDocuments(route)...)
	if err != nil {
		t.Fatalf("surface API paths should resolve to the shared /alpha mount: %v", err)
	}
}

func TestRouteValidationDetectsDuplicateMountWithSurfaceAPIPaths(t *testing.T) {
	first := `apiVersion: one-shot-man/v1alpha1
kind: ModelRoute
metadata:
  name: alpha-api-paths
  namespace: test
spec:
  access: alpha-chat
  offers:
  - serving: alpha-model-alpha
  surfaces:
  - anthropic
  - chat
  endpoints:
    anthropic: http://127.0.0.1:11239/shared/anthropic
    chat: http://127.0.0.1:11239/shared/openai/v1
  naming: served
  transport:
    kind: shaper
    upstream: https://alpha.example
`
	second := `apiVersion: one-shot-man/v1alpha1
kind: ModelRoute
metadata:
  name: alpha-legacy-path
  namespace: test
spec:
  access: alpha-chat
  offers:
  - serving: alpha-model-alpha
  surfaces:
  - chat
  endpoints:
    chat: http://127.0.0.1:11239/shared/v1
  naming: served
  transport:
    kind: shaper
    upstream: https://alpha.example
`
	_, err := loadDocuments(routeValidationDocuments(first, second)...)
	if err == nil || !strings.Contains(err.Error(), "both mount the shaper at prefix \"/shared\"") {
		t.Fatalf("error: got %v, want the normalized /shared mount collision", err)
	}
}

// TestRouteValidationRefusesTwoShaperRoutesOnOneMount is the hazard the
// per-access ban was standing in for: two shaper sections claiming the same
// local path would serve whichever route bound it first.
func TestRouteValidationRefusesTwoShaperRoutesOnOneMount(t *testing.T) {
	first := `apiVersion: one-shot-man/v1alpha1
kind: ModelRoute
metadata:
  name: alpha-one
  namespace: test
spec:
  access: alpha-chat
  offers:
  - serving: alpha-model-alpha
  surfaces:
  - chat
  endpoints:
    chat: http://127.0.0.1:11239/shared/v1
  naming: served
  transport:
    kind: shaper
    upstream: https://alpha.example
`
	second := `apiVersion: one-shot-man/v1alpha1
kind: ModelRoute
metadata:
  name: alpha-two
  namespace: test
spec:
  access: alpha-chat
  offers:
  - serving: alpha-model-alpha
  surfaces:
  - chat
  endpoints:
    chat: http://127.0.0.1:11239/shared/v1
  naming: served
  transport:
    kind: shaper
    upstream: https://alpha.example
`
	_, err := loadDocuments(routeValidationDocuments(first, second)...)
	if err == nil ||
		!strings.Contains(err.Error(), "both mount the shaper at prefix") ||
		!strings.Contains(err.Error(), "alpha-one") ||
		!strings.Contains(err.Error(), "alpha-two") {
		t.Fatalf("error: got %v, want both routes and the shared prefix named", err)
	}
}

// TestRouteValidationRefusesSurfaceWithoutMountPrefix covers the sentinel bug:
// a surface whose endpoint is just /v1 (or /) contributes no mount prefix, and
// a later surface must not be allowed to define one on its behalf.
func TestRouteValidationRefusesSurfaceWithoutMountPrefix(t *testing.T) {
	noPrefix := `apiVersion: one-shot-man/v1alpha1
kind: ModelRoute
metadata:
  name: alpha-one
  namespace: test
spec:
  access: alpha-chat
  offers:
  - serving: alpha-model-alpha
  surfaces:
  - anthropic
  endpoints:
    anthropic: http://127.0.0.1:11239/v1
  naming: served
  transport:
    kind: shaper
    upstream: https://alpha.example
`
	_, err := loadDocuments(routeValidationDocuments(noPrefix)...)
	if err == nil || !strings.Contains(err.Error(), "carries no mount prefix") {
		t.Fatalf("error: got %v, want a refusal naming the prefixless surface", err)
	}
}

func TestRouteValidationRefusesAPIPathWithoutMountPrefix(t *testing.T) {
	noPrefix := `apiVersion: one-shot-man/v1alpha1
kind: ModelRoute
metadata:
  name: alpha-one
  namespace: test
spec:
  access: alpha-chat
  offers:
  - serving: alpha-model-alpha
  surfaces:
  - chat
  endpoints:
    chat: http://127.0.0.1:11239/openai/v1
  naming: served
  transport:
    kind: shaper
    upstream: https://alpha.example
`
	_, err := loadDocuments(routeValidationDocuments(noPrefix)...)
	if err == nil || !strings.Contains(err.Error(), "carries no mount prefix") {
		t.Fatalf("error: got %v, want an API-only path refused", err)
	}
}

func TestRouteValidationRefusesSurfaceWithMultipleSlashesBeforeV1(t *testing.T) {
	multiSlash := `apiVersion: one-shot-man/v1alpha1
kind: ModelRoute
metadata:
  name: alpha-one
  namespace: test
spec:
  access: alpha-chat
  offers:
  - serving: alpha-model-alpha
  surfaces:
  - anthropic
  endpoints:
    anthropic: http://127.0.0.1:11239///v1
  naming: served
  transport:
    kind: shaper
    upstream: https://alpha.example
`
	_, err := loadDocuments(routeValidationDocuments(multiSlash)...)
	if err == nil || !strings.Contains(err.Error(), "carries no mount prefix") {
		t.Fatalf("error: got %v, want a refusal naming the prefixless surface", err)
	}
}

// TestShaperMountPrefixNormalization pins the endpoint->mount derivation for
// every shape the renderer emits and every repeated-slash shape it does not.
// The guard the function exists for is that no surface may reduce to an empty
// prefix: repeated slashes between a surface API suffix and "/v1" used to leave
// the suffix in place, so "/openai//v1" survived as the phantom mount "/openai"
// and accepted an API-only path.
func TestShaperMountPrefixNormalization(t *testing.T) {
	route := func(surface, endpoint string) *v1alpha1.ModelRouteSpec {
		return &v1alpha1.ModelRouteSpec{
			Surfaces:  []string{surface},
			Endpoints: map[string]v1alpha1.SurfaceEndpoint{surface: v1alpha1.SurfaceEndpoint(endpoint)},
		}
	}
	for _, test := range []struct {
		name     string
		surface  string
		endpoint string
		want     string
		wantErr  string
	}{
		{name: "legacy chat", surface: "chat", endpoint: "http://127.0.0.1:11239/alpha/v1", want: "/alpha"},
		{name: "legacy responses", surface: "responses", endpoint: "http://127.0.0.1:11239/alpha/v1", want: "/alpha"},
		{name: "legacy anthropic mount", surface: "anthropic", endpoint: "http://127.0.0.1:11239/alpha", want: "/alpha"},
		{name: "anthropic api suffix", surface: "anthropic", endpoint: "http://127.0.0.1:11239/alpha/anthropic", want: "/alpha"},
		{name: "anthropic api suffix and v1", surface: "anthropic", endpoint: "http://127.0.0.1:11239/alpha/anthropic/v1", want: "/alpha"},
		{name: "trailing slash", surface: "chat", endpoint: "http://127.0.0.1:11239/alpha/v1/", want: "/alpha"},
		{name: "openai path", surface: "chat", endpoint: "http://127.0.0.1:11239/alpha/openai/v1", want: "/alpha"},
		{name: "openai path responses", surface: "responses", endpoint: "http://127.0.0.1:11239/alpha/openai/v1", want: "/alpha"},
		{name: "doubled api segment", surface: "chat", endpoint: "http://127.0.0.1:11239/openai/openai/v1", want: "/openai"},
		{name: "doubled api segment anthropic", surface: "anthropic", endpoint: "http://127.0.0.1:11239/anthropic/anthropic", want: "/anthropic"},
		// A mount whose last segment is one the surface strips (/v1 included)
		// is expressed by repeating it.
		{name: "doubled v1 mount chat", surface: "chat", endpoint: "http://127.0.0.1:11239/api/v1/v1", want: "/api/v1"},
		{name: "doubled v1 mount anthropic", surface: "anthropic", endpoint: "http://127.0.0.1:11239/api/v1/v1", want: "/api/v1"},
		{name: "openai mount on anthropic surface", surface: "anthropic", endpoint: "http://127.0.0.1:11239/openai", want: "/openai"},
		{name: "chat without v1 keeps the final segment", surface: "chat", endpoint: "http://127.0.0.1:11239/alpha/openai", want: "/alpha/openai"},
		// Repeated slashes may not manufacture a mount, and must normalize to
		// the same mount as their canonical spelling.
		{name: "repeat slash after openai", surface: "chat", endpoint: "http://127.0.0.1:11239/openai//v1", wantErr: "carries no mount prefix"},
		{name: "repeat slash after anthropic", surface: "anthropic", endpoint: "http://127.0.0.1:11239/anthropic//v1", wantErr: "carries no mount prefix"},
		{name: "repeat slash before v1", surface: "anthropic", endpoint: "http://127.0.0.1:11239///v1", wantErr: "carries no mount prefix"},
		{name: "repeat slash inside a mount", surface: "chat", endpoint: "http://127.0.0.1:11239/alpha/openai//v1", want: "/alpha"},
		{name: "repeat slash inside an anthropic mount", surface: "anthropic", endpoint: "http://127.0.0.1:11239/alpha/anthropic//v1", want: "/alpha"},
		// Dot segments are resolved the same way a router resolves them, so a
		// path cannot spell one mount two ways.
		{name: "current directory segment", surface: "chat", endpoint: "http://127.0.0.1:11239/alpha/./openai/v1", want: "/alpha"},
		{name: "parent directory segment", surface: "chat", endpoint: "http://127.0.0.1:11239/beta/../alpha/openai/v1", want: "/alpha"},
		{name: "trailing current directory", surface: "chat", endpoint: "http://127.0.0.1:11239/alpha/.", want: "/alpha"},
		{name: "dot only", surface: "chat", endpoint: "http://127.0.0.1:11239/.", wantErr: "carries no mount prefix"},
		{name: "parent above root", surface: "anthropic", endpoint: "http://127.0.0.1:11239/../v1", wantErr: "carries no mount prefix"},
		{name: "api only path", surface: "chat", endpoint: "http://127.0.0.1:11239/openai/v1", wantErr: "carries no mount prefix"},
		{name: "api only path responses", surface: "responses", endpoint: "http://127.0.0.1:11239/openai/v1", wantErr: "carries no mount prefix"},
		// A chat/responses endpoint that is the bare API segment carries no
		// mount either. Accepting it would name the mount after the segment the
		// surface owns, and would collide with a real mount called "/openai"
		// (which is expressed doubled). This is the check the gated strip must
		// not weaken.
		{name: "bare api segment", surface: "chat", endpoint: "http://127.0.0.1:11239/openai", wantErr: "carries no mount prefix"},
		{name: "bare api segment trailing slash", surface: "chat", endpoint: "http://127.0.0.1:11239/openai/", wantErr: "carries no mount prefix"},
		{name: "bare api segment repeated slash", surface: "responses", endpoint: "http://127.0.0.1:11239//openai", wantErr: "carries no mount prefix"},
		{name: "api only path anthropic", surface: "anthropic", endpoint: "http://127.0.0.1:11239/anthropic", wantErr: "carries no mount prefix"},
		{name: "api only path anthropic and v1", surface: "anthropic", endpoint: "http://127.0.0.1:11239/anthropic/v1", wantErr: "carries no mount prefix"},
		{name: "root only", surface: "chat", endpoint: "http://127.0.0.1:11239/", wantErr: "carries no mount prefix"},
	} {
		t.Run(test.name, func(t *testing.T) {
			got, err := shaperMountPrefix("alpha-local", route(test.surface, test.endpoint))
			if test.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), test.wantErr) {
					t.Fatalf("shaperMountPrefix(%q) error: got %v (mount %q), want %q", test.endpoint, err, got, test.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("shaperMountPrefix(%q): %v", test.endpoint, err)
			}
			if got != test.want {
				t.Fatalf("shaperMountPrefix(%q) = %q, want %q", test.endpoint, got, test.want)
			}
		})
	}
}

// TestRouteValidationAllowsMixedSurfaceLayouts pins the renderer's real
// convention: a legacy-protocol access publishes <mount>/v1 for chat while the
// same route publishes a bare <mount> for anthropic, and an openai-family
// access publishes <mount>/openai/v1 and <mount>/anthropic. Both are one mount.
func TestRouteValidationAllowsMixedSurfaceLayouts(t *testing.T) {
	for _, test := range []struct {
		name     string
		endpoint map[string]string
	}{
		{
			name: "legacy protocol",
			endpoint: map[string]string{
				"anthropic": "http://127.0.0.1:11239/alpha",
				"chat":      "http://127.0.0.1:11239/alpha/v1",
				"responses": "http://127.0.0.1:11239/alpha/v1",
			},
		},
		{
			name: "openai family",
			endpoint: map[string]string{
				"anthropic": "http://127.0.0.1:11239/alpha/anthropic",
				"chat":      "http://127.0.0.1:11239/alpha/openai/v1",
				"responses": "http://127.0.0.1:11239/alpha/openai/v1",
			},
		},
		// The renderer emits both layouts for one mount depending on the
		// access protocol, so a route may legitimately mix them.
		{
			name: "legacy chat with openai anthropic",
			endpoint: map[string]string{
				"anthropic": "http://127.0.0.1:11239/alpha/anthropic",
				"chat":      "http://127.0.0.1:11239/alpha/v1",
				"responses": "http://127.0.0.1:11239/alpha/v1",
			},
		},
		{
			name: "openai chat with bare anthropic",
			endpoint: map[string]string{
				"anthropic": "http://127.0.0.1:11239/alpha",
				"chat":      "http://127.0.0.1:11239/alpha/openai/v1",
				"responses": "http://127.0.0.1:11239/alpha/openai/v1",
			},
		},
		{
			name: "openai chat with anthropic v1",
			endpoint: map[string]string{
				"anthropic": "http://127.0.0.1:11239/alpha/anthropic/v1",
				"chat":      "http://127.0.0.1:11239/alpha/openai/v1",
				"responses": "http://127.0.0.1:11239/alpha/openai/v1",
			},
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			route := `apiVersion: one-shot-man/v1alpha1
kind: ModelRoute
metadata:
  name: alpha-local
  namespace: test
spec:
  access: alpha-chat
  offers:
  - serving: alpha-model-alpha
  surfaces:
  - anthropic
  - chat
  - responses
  endpoints:
    anthropic: ` + test.endpoint["anthropic"] + `
    chat: ` + test.endpoint["chat"] + `
    responses: ` + test.endpoint["responses"] + `
  naming: served
  transport:
    kind: shaper
    upstream: https://alpha.example
`
			if _, err := loadDocuments(routeValidationDocuments(route)...); err != nil {
				t.Fatalf("mixed surface layouts share one mount and must load: %v", err)
			}
		})
	}
}

// TestRouteValidationRefusesMismatchedMountPrefix catches the exact bypass the
// sentinel allowed: the first surface trims to nothing, so a later surface's
// different prefix used to be accepted without ever being compared.
func TestRouteValidationRefusesMismatchedMountPrefix(t *testing.T) {
	mismatched := `apiVersion: one-shot-man/v1alpha1
kind: ModelRoute
metadata:
  name: alpha-one
  namespace: test
spec:
  access: alpha-chat
  offers:
  - serving: alpha-model-alpha
  surfaces:
  - chat
  - responses
  endpoints:
    chat: http://127.0.0.1:11239/v1
    responses: http://127.0.0.1:11239/mount/v1
  naming: served
  transport:
    kind: shaper
    upstream: https://alpha.example
`
	_, err := loadDocuments(routeValidationDocuments(mismatched)...)
	if err == nil || !strings.Contains(err.Error(), "carries no mount prefix") {
		t.Fatalf("error: got %v, want the prefixless surface refused rather than silently compared", err)
	}
}

// TestRouteValidationRefusesEndpointsThatDisagreeOnTheMount is the comparison
// the previous test cannot reach: both endpoints carry a real mount, so the
// only thing wrong is that they name different ones.
func TestRouteValidationRefusesEndpointsThatDisagreeOnTheMount(t *testing.T) {
	disagreeing := `apiVersion: one-shot-man/v1alpha1
kind: ModelRoute
metadata:
  name: alpha-one
  namespace: test
spec:
  access: alpha-chat
  offers:
  - serving: alpha-model-alpha
  surfaces:
  - chat
  - responses
  endpoints:
    chat: http://127.0.0.1:11239/alpha/openai/v1
    responses: http://127.0.0.1:11239/beta/v1
  naming: served
  transport:
    kind: shaper
    upstream: https://alpha.example
`
	_, err := loadDocuments(routeValidationDocuments(disagreeing)...)
	if err == nil ||
		!strings.Contains(err.Error(), "endpoints disagree on the mount prefix") ||
		!strings.Contains(err.Error(), `"/alpha"`) ||
		!strings.Contains(err.Error(), `"/beta"`) {
		t.Fatalf("error: got %v, want both competing mount prefixes named", err)
	}
}

// TestRouteValidationDetectsRepeatedSlashMountCollision pins that repeated
// slashes cannot hide a duplicate mount: the canonical and the repeated-slash
// spelling of one mount are the same local path, so two shaper sections must
// still collide.
func TestRouteValidationDetectsRepeatedSlashMountCollision(t *testing.T) {
	canonical := `apiVersion: one-shot-man/v1alpha1
kind: ModelRoute
metadata:
  name: alpha-canonical
  namespace: test
spec:
  access: alpha-chat
  offers:
  - serving: alpha-model-alpha
  surfaces:
  - chat
  endpoints:
    chat: http://127.0.0.1:11239/alpha/openai/v1
  naming: served
  transport:
    kind: shaper
    upstream: https://alpha.example
`
	repeated := `apiVersion: one-shot-man/v1alpha1
kind: ModelRoute
metadata:
  name: alpha-repeated
  namespace: test
spec:
  access: alpha-chat
  offers:
  - serving: alpha-model-alpha
  surfaces:
  - chat
  endpoints:
    chat: http://127.0.0.1:11239/alpha//openai//v1
  naming: served
  transport:
    kind: shaper
    upstream: https://alpha.example
`
	_, err := loadDocuments(routeValidationDocuments(canonical, repeated)...)
	if err == nil || !strings.Contains(err.Error(), `both mount the shaper at prefix "/alpha"`) {
		t.Fatalf("error: got %v, want the repeated-slash spelling to collide on /alpha", err)
	}
}

func TestRouteValidationAllowsRetiredInventoryBesideLiveRoute(t *testing.T) {
	retiredServing := `apiVersion: one-shot-man/v1alpha1
kind: ModelServing
metadata:
  name: alpha-retired-alpha
spec:
  provider: alpha
  model: alpha-model
  provider_model_id: alpha-retired
  deprecated: true
`
	retiredRoute := `apiVersion: one-shot-man/v1alpha1
kind: ModelRoute
metadata:
  name: alpha-retired
  namespace: test
spec:
  access: alpha-chat
  offers:
  - serving: alpha-retired-alpha
  surfaces:
  - chat
  endpoints:
    chat: http://127.0.0.1:1/retired/v1
  naming: served
  transport:
    kind: shaper
    upstream: https://alpha.example
`
	_, err := loadDocuments(routeValidationDocuments(
		retiredServing,
		retiredRoute,
		routeValidationRoute("alpha-live", "alpha-chat", "", ""),
	)...)
	if err != nil {
		t.Fatalf("a retired inventory must not claim the access or block the live route: %v", err)
	}
}

func TestRouteValidationRequiresRemoteOfferInventory(t *testing.T) {
	remote := `apiVersion: one-shot-man/v1alpha1
kind: ModelRoute
metadata:
  name: alpha-remote
  namespace: test
spec:
  offers: []
  surfaces:
  - chat
  endpoints:
    chat: https://remote.example/v1
  naming: served
  transport:
    kind: remote
`
	_, err := loadDocuments(routeValidationDocuments(remote)...)
	if err == nil || !strings.Contains(err.Error(), "must declare its offers inventory") {
		t.Fatalf("error: got %v", err)
	}
}
