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

func TestRouteValidationRefusesTwoShaperRoutesForOneAccess(t *testing.T) {
	_, err := loadDocuments(routeValidationDocuments(
		routeValidationRoute("alpha-one", "alpha-chat", "", ""),
		routeValidationRoute("alpha-two", "alpha-chat", "", ""),
	)...)
	if err == nil ||
		!strings.Contains(err.Error(), "both mount access") ||
		!strings.Contains(err.Error(), "alpha-one") ||
		!strings.Contains(err.Error(), "alpha-two") {
		t.Fatalf("error: got %v", err)
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
