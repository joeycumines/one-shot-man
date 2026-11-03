package userk8s

import (
	"strings"
	"testing"
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
