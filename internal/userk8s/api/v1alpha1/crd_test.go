package v1alpha1

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/goccy/go-yaml"
)

const crdDir = "../config/crd"

func loadCRD(t *testing.T, file string) map[string]any {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(crdDir, file))
	if err != nil {
		t.Fatalf("read %s: %v", file, err)
	}
	var doc map[string]any
	if err := yaml.Unmarshal(data, &doc); err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	return doc
}

func at(t *testing.T, value any, path ...string) any {
	t.Helper()
	current := value
	for _, step := range path {
		mapping, ok := current.(map[string]any)
		if !ok {
			t.Fatalf("at %q: not a mapping (%T)", step, current)
		}
		next, ok := mapping[step]
		if !ok {
			t.Fatalf("at %q: key missing", step)
		}
		current = next
	}
	return current
}

func asString(t *testing.T, value any) string {
	t.Helper()
	text, ok := value.(string)
	if !ok {
		t.Fatalf("not a string (%T)", value)
	}
	return text
}

func asList(t *testing.T, value any) []any {
	t.Helper()
	list, ok := value.([]any)
	if !ok {
		t.Fatalf("not a list (%T)", value)
	}
	return list
}

func asMap(t *testing.T, value any) map[string]any {
	t.Helper()
	mapping, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("not a mapping (%T)", value)
	}
	return mapping
}

func asInt(t *testing.T, value any) int {
	t.Helper()
	switch number := value.(type) {
	case int:
		return number
	case int64:
		return int(number)
	case uint64:
		return int(number)
	case float64:
		return int(number)
	default:
		t.Fatalf("not a number (%T)", value)
		return 0
	}
}

func rules(t *testing.T, schema any) []string {
	t.Helper()
	mapping := asMap(t, schema)
	raw, ok := mapping["x-kubernetes-validations"]
	if !ok {
		return nil
	}
	var out []string
	for _, entry := range asList(t, raw) {
		out = append(out, asString(t, asMap(t, entry)["rule"]))
	}
	return out
}

func normalizeRuleWhitespace(rule string) string {
	return strings.Join(strings.Fields(rule), " ")
}

func assertRule(t *testing.T, file string, got []string, fragment string) {
	t.Helper()
	want := normalizeRuleWhitespace(fragment)
	for _, rule := range got {
		if strings.Contains(normalizeRuleWhitespace(rule), want) {
			return
		}
	}
	t.Fatalf("%s: no validation rule contains %q; got %v", file, fragment, got)
}

func assertStringField(t *testing.T, file string, schema any, key, want string) {
	t.Helper()
	got, ok := asMap(t, schema)[key]
	if !ok {
		t.Fatalf("%s: %s missing", file, key)
	}
	if asString(t, got) != want {
		t.Fatalf("%s: %s = %q, want %q", file, key, got, want)
	}
}

func assertBoolField(t *testing.T, file string, schema any, key string, want bool) {
	t.Helper()
	got, ok := asMap(t, schema)[key]
	if !ok {
		t.Fatalf("%s: %s missing", file, key)
	}
	value, ok := got.(bool)
	if !ok || value != want {
		t.Fatalf("%s: %s = %v, want %v", file, key, got, want)
	}
}

func assertEnum(t *testing.T, file string, schema any, want ...string) {
	t.Helper()
	got := asList(t, asMap(t, schema)["enum"])
	if len(got) != len(want) {
		t.Fatalf("%s: enum = %v, want %v", file, got, want)
	}
	for i, item := range got {
		if asString(t, item) != want[i] {
			t.Fatalf("%s: enum = %v, want %v", file, got, want)
		}
	}
}

func rootSchema(t *testing.T, doc map[string]any) map[string]any {
	t.Helper()
	versions := asList(t, at(t, doc, "spec", "versions"))
	if len(versions) != 1 {
		t.Fatalf("expected one version, got %d", len(versions))
	}
	version := asMap(t, versions[0])
	if got := asString(t, version["name"]); got != Version {
		t.Fatalf("version = %q, want %q", got, Version)
	}
	if version["served"] != true || version["storage"] != true {
		t.Fatalf("version must be served and stored: %v", version)
	}
	return asMap(t, at(t, version, "schema", "openAPIV3Schema"))
}

type crdCase struct {
	file     string
	plural   string
	kind     string
	scope    string
	specKeys []string
}

func crdCases() []crdCase {
	return []crdCase{
		{"one-shot-man_modelproviders.yaml", "modelproviders", "ModelProvider", "Cluster", []string{"display_name", "country", "deprecated", "labels"}},
		{"one-shot-man_modelaccesses.yaml", "modelaccesses", "ModelAccess", "Cluster", []string{"provider", "protocol", "deprecated", "labels"}},
		{"one-shot-man_models.yaml", "models", "Model", "Cluster", []string{"canonical_id", "family", "display_name", "aliases", "context_window", "max_output_tokens", "reasoning", "input_modalities", "output_modalities", "tool_call", "structured_output", "deprecated", "labels", "toolSettings"}},
		{"one-shot-man_modelservings.yaml", "modelservings", "ModelServing", "Cluster", []string{"provider", "model", "provider_model_id", "priority", "default", "deprecated", "overrides", "labels"}},
		{"one-shot-man_modelselections.yaml", "modelselections", "ModelSelection", "Namespaced", []string{"display_name", "strategy", "candidates"}},
		{"one-shot-man_modelroutes.yaml", "modelroutes", "ModelRoute", "Namespaced", []string{"access", "credential", "offers", "surfaces", "endpoints", "naming", "order", "transport"}},
		{"one-shot-man_localsecretbindings.yaml", "localsecretbindings", "LocalSecretBinding", "Namespaced", []string{"selector", "secretRef", "scheme", "envVar", "resolvers"}},
		{"one-shot-man_tools.yaml", "tools", "Tool", "Cluster", []string{"display_name", "surfaces", "credential_channels", "identifiers", "budget_profile"}},
	}
}

func TestCRDFileSet(t *testing.T) {
	entries, err := os.ReadDir(crdDir)
	if err != nil {
		t.Fatalf("read %s: %v", crdDir, err)
	}
	var got []string
	for _, entry := range entries {
		if !entry.IsDir() {
			got = append(got, entry.Name())
		}
	}
	var want []string
	for _, tc := range crdCases() {
		want = append(want, tc.file)
	}
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("crd file set = %v, want %v", got, want)
	}
}

func TestCatalogCRDManifests(t *testing.T) {
	for _, tc := range crdCases() {
		t.Run(tc.kind, func(t *testing.T) {
			doc := loadCRD(t, tc.file)
			if got := asString(t, doc["apiVersion"]); got != "apiextensions.k8s.io/v1" {
				t.Fatalf("apiVersion = %q", got)
			}
			if got := asString(t, doc["kind"]); got != "CustomResourceDefinition" {
				t.Fatalf("kind = %q", got)
			}
			if got := asString(t, at(t, doc, "metadata", "name")); got != tc.plural+"."+Group {
				t.Fatalf("metadata.name = %q", got)
			}
			if got := asString(t, at(t, doc, "spec", "group")); got != Group {
				t.Fatalf("spec.group = %q", got)
			}
			if got := asString(t, at(t, doc, "spec", "scope")); got != tc.scope {
				t.Fatalf("spec.scope = %q, want %q", got, tc.scope)
			}
			if got := asString(t, at(t, doc, "spec", "names", "kind")); got != tc.kind {
				t.Fatalf("spec.names.kind = %q", got)
			}
			if got := asString(t, at(t, doc, "spec", "names", "listKind")); got != tc.kind+"List" {
				t.Fatalf("spec.names.listKind = %q", got)
			}
			schema := rootSchema(t, doc)
			assertRule(t, tc.file, rules(t, schema),
				"self.metadata.name.matches('^[a-z0-9]([-a-z0-9]*[a-z0-9])?$')")
			specProps := asMap(t, at(t, schema, "properties", "spec", "properties"))
			for _, key := range tc.specKeys {
				if _, ok := specProps[key]; !ok {
					t.Errorf("%s: spec.%s missing from schema", tc.file, key)
				}
			}
		})
	}
}

func TestModelProviderSchema(t *testing.T) {
	doc := loadCRD(t, "one-shot-man_modelproviders.yaml")
	spec := asMap(t, at(t, rootSchema(t, doc), "properties", "spec"))
	display := asMap(t, at(t, spec, "properties", "display_name"))
	if got := asInt(t, display["minLength"]); got != 1 {
		t.Fatalf("display_name.minLength = %d, want 1", got)
	}
	if got := asMap(t, at(t, spec, "properties", "country")); asInt(t, got["minLength"]) != 2 || asInt(t, got["maxLength"]) != 3 {
		t.Fatalf("country must be 2..3 characters")
	}
	assertBoolField(t, "modelproviders", at(t, spec, "properties", "deprecated"), "default", false)
}

func TestModelAccessSchema(t *testing.T) {
	doc := loadCRD(t, "one-shot-man_modelaccesses.yaml")
	spec := asMap(t, at(t, rootSchema(t, doc), "properties", "spec"))
	props := asMap(t, spec["properties"])

	assertEnum(t, "modelaccesses protocol", props["protocol"], "chat", "messages", "responses")
	assertBoolField(t, "modelaccesses", props["deprecated"], "default", false)
	required := map[string]bool{}
	for _, item := range asList(t, spec["required"]) {
		required[asString(t, item)] = true
	}
	for _, key := range []string{"provider", "protocol"} {
		if !required[key] {
			t.Errorf("spec.%s must be required", key)
		}
	}
	for _, gone := range []string{"auth", "endpoints", "mode", "shaper", "surfaces", "preferred_surface", "order"} {
		if _, ok := props[gone]; ok {
			t.Errorf("spec.%s must not exist: credentials and transports are entity facts", gone)
		}
	}
}

func TestModelServingSchema(t *testing.T) {
	doc := loadCRD(t, "one-shot-man_modelservings.yaml")
	spec := asMap(t, at(t, rootSchema(t, doc), "properties", "spec"))
	props := asMap(t, spec["properties"])

	required := map[string]bool{}
	for _, item := range asList(t, spec["required"]) {
		required[asString(t, item)] = true
	}
	for _, key := range []string{"provider", "model", "provider_model_id"} {
		if !required[key] {
			t.Errorf("spec.%s must be required", key)
		}
	}
	priority := asMap(t, props["priority"])
	if asString(t, priority["format"]) != "int32" || asInt(t, priority["default"]) != 100 {
		t.Fatalf("priority must default to 100: %v", priority)
	}
	assertBoolField(t, "modelservings", props["default"], "default", false)
	assertBoolField(t, "modelservings", props["deprecated"], "default", false)

	overrides := asMap(t, asMap(t, props["overrides"])["properties"])
	for _, key := range []string{"context_window", "max_output_tokens", "input_modalities", "output_modalities", "reasoning", "tool_call", "structured_output"} {
		if _, ok := overrides[key]; !ok {
			t.Errorf("overrides.%s missing", key)
		}
	}
}

func TestModelSelectionSchema(t *testing.T) {
	doc := loadCRD(t, "one-shot-man_modelselections.yaml")
	spec := asMap(t, at(t, rootSchema(t, doc), "properties", "spec"))
	props := asMap(t, spec["properties"])

	assertEnum(t, "modelselections strategy", props["strategy"], "ordered", "priority", "dynamic")
	assertStringField(t, "modelselections strategy", props["strategy"], "default", "ordered")
	candidates := asMap(t, props["candidates"])
	if asInt(t, candidates["minItems"]) != 1 {
		t.Fatalf("candidates must require at least one entry")
	}
	got := rules(t, asMap(t, candidates["items"]))
	assertRule(t, "modelselections candidate", got, "(has(self.serving) ? 1 : 0) + (has(self.model) ? 1 : 0)")
}

func TestModelRouteSchema(t *testing.T) {
	doc := loadCRD(t, "one-shot-man_modelroutes.yaml")
	spec := asMap(t, at(t, rootSchema(t, doc), "properties", "spec"))
	props := asMap(t, spec["properties"])

	assertEnum(t, "modelroutes naming", props["naming"], "served", "wire", "registry", "canonical")
	order := asMap(t, props["order"])
	if asString(t, order["format"]) != "int32" || asInt(t, order["default"]) != 100 {
		t.Fatalf("order must default to 100: %v", order)
	}
	transport := asMap(t, props["transport"])
	assertEnum(t, "modelroutes transport.kind", at(t, transport, "properties", "kind"), "direct", "shaper", "remote")
	surfaces := asMap(t, props["surfaces"])
	assertEnum(t, "modelroutes surfaces", surfaces["items"], "anthropic", "chat", "responses")
	if asInt(t, surfaces["minItems"]) != 1 || surfaces["uniqueItems"] != true {
		t.Fatalf("surfaces must be a non-empty unique array")
	}
	offers := asMap(t, props["offers"])
	offerProps := asMap(t, asMap(t, offers["items"])["properties"])
	for _, key := range []string{"serving", "id"} {
		if _, ok := offerProps[key]; !ok {
			t.Errorf("offers.items.%s missing", key)
		}
	}

	got := rules(t, spec)
	assertRule(t, "modelroutes", got, "self.endpoints.size() == self.surfaces.size()")
	assertRule(t, "modelroutes", got, "self.transport.kind != 'shaper' || self.naming == 'served'")
	assertRule(t, "modelroutes", got, "self.transport.kind != 'direct' || self.naming == 'wire'")
	assertRule(t, "modelroutes", got, "self.transport.kind == 'shaper' || self.transport.kind == 'direct' || self.offers.size() > 0")
}

func TestModelSchema(t *testing.T) {
	doc := loadCRD(t, "one-shot-man_models.yaml")
	spec := asMap(t, at(t, rootSchema(t, doc), "properties", "spec"))
	props := asMap(t, spec["properties"])

	for _, gone := range []string{"provider", "access", "provider_model_id", "default", "equivalent_to"} {
		if _, ok := props[gone]; ok {
			t.Errorf("spec.%s must not exist: a Model is a provider-independent capability definition", gone)
		}
	}
	required := map[string]bool{}
	for _, item := range asList(t, spec["required"]) {
		required[asString(t, item)] = true
	}
	for _, key := range []string{"canonical_id", "display_name", "context_window", "max_output_tokens"} {
		if !required[key] {
			t.Errorf("spec.%s must be required", key)
		}
	}
	contextWindow := asMap(t, props["context_window"])
	if asInt(t, contextWindow["minimum"]) != 1 || asString(t, contextWindow["format"]) != "int64" {
		t.Fatalf("context_window must be a positive int64")
	}
	inputs := asMap(t, props["input_modalities"])
	assertEnum(t, "models input_modalities", inputs["items"], "text", "image", "audio")
	outputs := asMap(t, props["output_modalities"])
	assertEnum(t, "models output_modalities", outputs["items"], "text", "image", "audio")
	assertBoolField(t, "models", props["tool_call"], "default", true)
	assertBoolField(t, "models", props["structured_output"], "default", false)
	assertBoolField(t, "models", props["deprecated"], "default", false)

	reasoning := asMap(t, props["reasoning"])
	reasoningProps := asMap(t, reasoning["properties"])
	assertBoolField(t, "models reasoning", reasoningProps["supported"], "default", false)
	assertBoolField(t, "models reasoning", reasoningProps["toggle"], "default", false)
	efforts := asMap(t, reasoningProps["efforts"])
	assertEnum(t, "models reasoning.efforts", efforts["items"], "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra")

	got := rules(t, spec)
	assertRule(t, "models", got, "self.context_window > 0")
	assertRule(t, "models", got, "self.max_output_tokens > 0")
	assertRule(t, "models", got, "self.input_modalities.all(m, m in ['text', 'image', 'audio'])")
	assertRule(t, "models", got, "v.context_window <= self.context_window")
	assertRule(t, "models", got, "v.effort_level in self.reasoning.efforts")
}

func TestLocalSecretBindingSchema(t *testing.T) {
	doc := loadCRD(t, "one-shot-man_localsecretbindings.yaml")
	spec := asMap(t, at(t, rootSchema(t, doc), "properties", "spec"))
	props := asMap(t, spec["properties"])

	assertEnum(t, "localsecretbindings scheme", props["scheme"], "none", "bearer", "api-key", "x-api-key", "awsSigV4")
	assertStringField(t, "localsecretbindings scheme", props["scheme"], "default", "bearer")
	got := rules(t, spec)
	assertRule(t, "localsecretbindings", got, "self.selector.matchLabels.size() > 0")
	assertRule(t, "localsecretbindings", got, "has(r.env) ? 1 : 0")
	assertRule(t, "localsecretbindings", got, "has(r.command) ? 1 : 0")
	required := map[string]bool{}
	for _, item := range asList(t, spec["required"]) {
		required[asString(t, item)] = true
	}
	for _, key := range []string{"selector", "secretRef", "envVar"} {
		if !required[key] {
			t.Errorf("spec.%s must be required", key)
		}
	}
}

func TestToolSchema(t *testing.T) {
	doc := loadCRD(t, "one-shot-man_tools.yaml")
	spec := asMap(t, at(t, rootSchema(t, doc), "properties", "spec"))
	props := asMap(t, spec["properties"])
	surfaces := asMap(t, props["surfaces"])
	assertEnum(t, "tools surfaces", surfaces["items"], "anthropic", "chat", "responses")
	if asInt(t, surfaces["minItems"]) != 1 || surfaces["uniqueItems"] != true {
		t.Fatalf("surfaces must be a non-empty unique array")
	}
	channels := asMap(t, props["credential_channels"])
	assertEnum(t, "tools credential_channels", channels["items"], "gateway", "envRef", "env")
	if asInt(t, channels["minItems"]) != 1 || channels["uniqueItems"] != true {
		t.Fatalf("credential_channels must be a non-empty unique array")
	}
}

func TestRegistryNameAnnotation(t *testing.T) {
	if RegistryNameAnnotation != "one-shot-man/registry-name" {
		t.Fatalf("RegistryNameAnnotation = %q", RegistryNameAnnotation)
	}
}
