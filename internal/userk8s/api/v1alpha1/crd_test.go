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

func assertRule(t *testing.T, file string, got []string, fragment string) {
	t.Helper()
	for _, rule := range got {
		if strings.Contains(rule, fragment) {
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
		{"one-shot-man_modelaccesses.yaml", "modelaccesses", "ModelAccess", "Cluster", []string{"provider", "mode", "user_agent", "anonymous", "country", "auth", "endpoints", "order", "deprecated", "labels"}},
		{"one-shot-man_models.yaml", "models", "Model", "Cluster", []string{"provider", "access", "context_window", "max_output_tokens", "can_reason", "input_modalities", "reasoning_efforts", "default", "deprecated", "equivalent_to", "labels", "toolSettings"}},
		{"one-shot-man_localsecretbindings.yaml", "localsecretbindings", "LocalSecretBinding", "Namespaced", []string{"selector", "secretRef", "envVar", "resolvers"}},
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
	required := map[string]bool{}
	for _, item := range asList(t, spec["required"]) {
		required[asString(t, item)] = true
	}
	if !required["display_name"] {
		t.Fatalf("spec.display_name must be required")
	}
	assertBoolField(t, "modelproviders", at(t, spec, "properties", "deprecated"), "default", false)
}

func TestModelAccessSchema(t *testing.T) {
	doc := loadCRD(t, "one-shot-man_modelaccesses.yaml")
	spec := asMap(t, at(t, rootSchema(t, doc), "properties", "spec"))
	props := asMap(t, spec["properties"])

	auth := asMap(t, props["auth"])
	required := asList(t, auth["required"])
	if len(required) != 1 || asString(t, required[0]) != "scheme" {
		t.Fatalf("auth.required = %v, want [scheme]", required)
	}
	assertEnum(t, "modelaccesses auth.scheme", at(t, auth, "properties", "scheme"), "none", "bearer", "awsSigV4", "toolManaged")
	requiredEnv := asMap(t, asMap(t, auth["properties"])["requiredEnv"])
	if asString(t, requiredEnv["type"]) != "array" || requiredEnv["uniqueItems"] != true {
		t.Fatalf("requiredEnv must be a unique-items array")
	}
	assertStringField(t, "modelaccesses requiredEnv.items", asMap(t, requiredEnv["items"]), "pattern", `^[A-Za-z_][A-Za-z0-9_]*$`)

	got := rules(t, spec)
	assertRule(t, "modelaccesses", got, "self.auth.scheme != 'bearer'")
	assertRule(t, "modelaccesses", got, "self.auth.scheme != 'awsSigV4'")
	assertRule(t, "modelaccesses", got, "self.auth.scheme != 'none'")
	assertRule(t, "modelaccesses", got, "self.endpoints.size() > 0")
	assertRule(t, "modelaccesses", got, "self.endpoints.all(k, v, k in ['responses', 'chat', 'anthropic'])")

	endpoints := asMap(t, props["endpoints"])
	additional := asMap(t, endpoints["additionalProperties"])
	if asString(t, additional["type"]) != "string" || asInt(t, additional["minLength"]) != 1 {
		t.Fatalf("endpoints values must be non-empty strings: %v", additional)
	}

	assertEnum(t, "modelaccesses mode", at(t, props, "mode"), "direct", "shaper", "proxy")
	assertStringField(t, "modelaccesses mode", at(t, props, "mode"), "default", "direct")
	order := asMap(t, props["order"])
	if asString(t, order["format"]) != "int32" || asInt(t, order["default"]) != 100 {
		t.Fatalf("order must default to 100: %v", order)
	}
	assertBoolField(t, "modelaccesses", props["anonymous"], "default", false)
	assertBoolField(t, "modelaccesses", props["deprecated"], "default", false)

	header := asMap(t, asMap(t, props["user_agent"])["properties"])
	assertStringField(t, "modelaccesses user_agent.value", header["value"], "default", "")
	assertBoolField(t, "modelaccesses user_agent.force", header["force"], "default", false)
	assertBoolField(t, "modelaccesses user_agent.auto", header["auto"], "default", true)
}

func TestModelSchema(t *testing.T) {
	doc := loadCRD(t, "one-shot-man_models.yaml")
	spec := asMap(t, at(t, rootSchema(t, doc), "properties", "spec"))
	props := asMap(t, spec["properties"])

	access := asMap(t, props["access"])
	if asInt(t, access["minLength"]) != 1 {
		t.Fatalf("access.minLength = %v, want 1", access["minLength"])
	}
	contextWindow := asMap(t, props["context_window"])
	if asInt(t, contextWindow["minimum"]) != 1 || asString(t, contextWindow["format"]) != "int64" {
		t.Fatalf("context_window must be a positive int64: %v", contextWindow)
	}
	maxOutput := asMap(t, props["max_output_tokens"])
	if asInt(t, maxOutput["minimum"]) != 1 {
		t.Fatalf("max_output_tokens.minimum = %v, want 1", maxOutput["minimum"])
	}

	modalities := asMap(t, props["input_modalities"])
	assertEnum(t, "models input_modalities", modalities["items"], "text", "image", "audio")
	if modalities["uniqueItems"] != true {
		t.Fatalf("input_modalities must be unique")
	}
	if defaults := asList(t, modalities["default"]); len(defaults) != 1 || asString(t, defaults[0]) != "text" {
		t.Fatalf("input_modalities must default to [text]: %v", defaults)
	}
	efforts := asMap(t, props["reasoning_efforts"])
	assertEnum(t, "models reasoning_efforts", efforts["items"], "none", "minimal", "low", "medium", "high", "xhigh", "max", "ultra")
	if efforts["uniqueItems"] != true {
		t.Fatalf("reasoning_efforts must be unique")
	}
	assertBoolField(t, "models", props["can_reason"], "default", false)
	assertBoolField(t, "models", props["default"], "default", false)
	assertBoolField(t, "models", props["deprecated"], "default", false)

	got := rules(t, spec)
	assertRule(t, "models", got, "self.context_window > 0")
	assertRule(t, "models", got, "self.max_output_tokens > 0")
	assertRule(t, "models", got, "self.input_modalities.all(m, m in ['text', 'image', 'audio'])")
	assertRule(t, "models", got, "self.reasoning_efforts.all(e, e in ['none', 'minimal', 'low', 'medium', 'high', 'xhigh', 'max', 'ultra'])")
	assertRule(t, "models", got, "v.context_window <= self.context_window")
	assertRule(t, "models", got, "v.max_output_tokens <= self.max_output_tokens")
	assertRule(t, "models", got, "v.auto_compact_window <= self.context_window")
	assertRule(t, "models", got, "v.effort_level in self.reasoning_efforts")
	assertRule(t, "models", got, "v.waive.size() > 0")

	settings := asMap(t, at(t, props, "toolSettings", "additionalProperties"))
	settingsProps := asMap(t, settings["properties"])
	for _, key := range []string{"context_window", "max_output_tokens", "auto_compact_window", "effort_level", "flags", "waive"} {
		if _, ok := settingsProps[key]; !ok {
			t.Errorf("toolSettings.%s missing", key)
		}
	}
	required := map[string]bool{}
	for _, item := range asList(t, spec["required"]) {
		required[asString(t, item)] = true
	}
	for _, key := range []string{"provider", "context_window", "max_output_tokens"} {
		if !required[key] {
			t.Errorf("spec.%s must be required", key)
		}
	}
}

func TestLocalSecretBindingSchema(t *testing.T) {
	doc := loadCRD(t, "one-shot-man_localsecretbindings.yaml")
	spec := asMap(t, at(t, rootSchema(t, doc), "properties", "spec"))
	props := asMap(t, spec["properties"])

	got := rules(t, spec)
	assertRule(t, "localsecretbindings", got, "self.selector.matchLabels.size() > 0")
	assertRule(t, "localsecretbindings", got, "self.selector.matchExpressions.size() > 0")
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

	secretRef := asMap(t, props["secretRef"])
	for _, key := range []string{"name", "key"} {
		entry := asMap(t, at(t, secretRef, "properties", key))
		if asInt(t, entry["minLength"]) != 1 {
			t.Errorf("secretRef.%s.minLength = %v, want 1", key, entry["minLength"])
		}
	}

	envVar := asMap(t, props["envVar"])
	assertStringField(t, "localsecretbindings envVar", envVar, "pattern", `^[A-Za-z_][A-Za-z0-9_]*$`)

	resolver := asMap(t, at(t, props, "resolvers", "items"))
	if asString(t, resolver["type"]) != "object" {
		t.Fatalf("resolver must be an object")
	}
	resolverProps := asMap(t, resolver["properties"])
	command := asMap(t, resolverProps["command"])
	argv := asMap(t, asMap(t, command["properties"])["argv"])
	if asInt(t, argv["minItems"]) != 1 {
		t.Fatalf("resolver argv minItems = %v, want 1", argv["minItems"])
	}
	if asInt(t, asMap(t, argv["items"])["minLength"]) != 1 {
		t.Fatalf("resolver argv items must be non-empty")
	}
	timeout := asMap(t, at(t, command, "properties", "timeout"))
	if asString(t, timeout["type"]) != "string" || asString(t, timeout["default"]) != "5s" {
		t.Fatalf("resolver timeout = %v, want string default 5s", timeout)
	}
	env := asMap(t, resolverProps["env"])
	if asInt(t, asMap(t, asMap(t, env["properties"])["name"])["minLength"]) != 1 {
		t.Fatalf("env resolver name must be non-empty")
	}
	file := asMap(t, resolverProps["file"])
	if asInt(t, asMap(t, asMap(t, file["properties"])["path"])["minLength"]) != 1 {
		t.Fatalf("file resolver path must be non-empty")
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

	required := map[string]bool{}
	for _, item := range asList(t, spec["required"]) {
		required[asString(t, item)] = true
	}
	for _, key := range []string{"display_name", "surfaces", "credential_channels", "identifiers", "budget_profile"} {
		if !required[key] {
			t.Errorf("spec.%s must be required", key)
		}
	}

	identifiers := asMap(t, props["identifiers"])
	for _, key := range []string{"provider_slug", "model_slug"} {
		grammar := asMap(t, at(t, identifiers, "properties", key))
		pattern := asMap(t, at(t, grammar, "properties", "pattern"))
		if asInt(t, pattern["minLength"]) != 1 {
			t.Errorf("identifiers.%s.pattern.minLength = %v, want 1", key, pattern["minLength"])
		}
	}
}

func TestRegistryNameAnnotation(t *testing.T) {
	if RegistryNameAnnotation != "one-shot-man/registry-name" {
		t.Fatalf("RegistryNameAnnotation = %q", RegistryNameAnnotation)
	}
}
