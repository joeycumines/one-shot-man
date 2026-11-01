package v1alpha1

import (
	"bytes"
	"context"
	"go/format"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"unicode/utf8"

	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	celpkg "k8s.io/apiextensions-apiserver/pkg/apiserver/schema/cel"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	sigsyaml "sigs.k8s.io/yaml"
)

// This file is the executable gate over the shipped CRD manifests. The sibling
// crd_test.go proves the right files exist and that selected rules appear as
// text; it cannot notice that a rule does not compile. A CEL rule the API server
// cannot compile makes the whole custom resource unusable, so the manifests are
// checked here with the same structural-schema builder and CEL validator the API
// server runs.
//
// The defect that motivated this file: a rule read `self.id == ”`, where the
// right-hand operand was U+201D (a typographic quotation mark) instead of two
// ASCII apostrophes. The text-based test passed, because the rule was present
// and its message matched; every ModelRoute object then failed validation. The
// ASCII scan below is the cheap version of that guard, and the compile pass is
// the real one.

// shippedCRDFiles returns the committed CRD manifests in a stable order.
func shippedCRDFiles(t *testing.T) []string {
	t.Helper()
	entries, err := os.ReadDir(crdDir)
	if err != nil {
		t.Fatalf("read %s: %v", crdDir, err)
	}
	files := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			files = append(files, entry.Name())
		}
	}
	sort.Strings(files)
	return files
}

// ruleLiterals walks a decoded CRD document and returns every
// x-kubernetes-validations rule string it contains, wherever it sits.
func ruleLiterals(node any) []string {
	var out []string
	switch typed := node.(type) {
	case map[string]any:
		if raw, ok := typed["x-kubernetes-validations"]; ok {
			if rules, ok := raw.([]any); ok {
				for _, rule := range rules {
					if entry, ok := rule.(map[string]any); ok {
						if text, ok := entry["rule"].(string); ok {
							out = append(out, text)
						}
					}
				}
			}
		}
		keys := make([]string, 0, len(typed))
		for key := range typed {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			out = append(out, ruleLiterals(typed[key])...)
		}
	case []any:
		for _, value := range typed {
			out = append(out, ruleLiterals(value)...)
		}
	}
	return out
}

// TestShippedCRDRulesAreASCIIText is the cheapest guard against the defect that
// shipped: a CEL rule whose string literal was a typographic quotation mark. CEL
// accepts only ASCII quotes as string delimiters, so any non-ASCII byte in a
// rule is a mistake.
func TestShippedCRDRulesAreASCIIText(t *testing.T) {
	checked := 0
	for _, file := range shippedCRDFiles(t) {
		for _, rule := range ruleLiterals(loadCRD(t, file)) {
			checked++
			for i := 0; i < len(rule); i++ {
				if rule[i] >= utf8.RuneSelf {
					t.Errorf("%s: rule contains a non-ASCII byte 0x%02x at offset %d: %s", file, rule[i], i, rule)
					break
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no validation rules found in any shipped CRD, so this check proves nothing")
	}
}

// TestShippedCRDRulesCompile is the real contract: every rule in every shipped
// manifest compiles in the environment the API server uses.
//
// The validator compiles a node's rules lazily, at the moment it visits that
// node, so validating an empty object would never reach a nested rule (the
// offers.items rule that shipped broken lives below an absent array). The
// sampleObject below materializes every declared property, so each rule is
// compiled. Only "rule compile error" entries are treated as failures: ordinary
// schema violations are expected for a synthetic object and are irrelevant to
// this assertion.
func TestShippedCRDRulesCompile(t *testing.T) {
	scheme := runtime.NewScheme()
	if err := apiextensions.AddToScheme(scheme); err != nil {
		t.Fatalf("registering the internal apiextensions types: %v", err)
	}
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("registering the v1 apiextensions types: %v", err)
	}
	if err := apiextensionsv1.RegisterConversions(scheme); err != nil {
		t.Fatalf("registering the apiextensions conversions: %v", err)
	}

	const perCallLimit = uint64(1 << 20)
	const validationLimit = int64(1 << 20)
	compiled := 0
	for _, file := range shippedCRDFiles(t) {
		t.Run(file, func(t *testing.T) {
			root := loadCRDRootSchema(t, file)
			internal := &apiextensions.JSONSchemaProps{}
			if err := scheme.Convert(&root, internal, nil); err != nil {
				t.Fatalf("converting the schema: %v", err)
			}
			structural, err := schema.NewStructural(internal)
			if err != nil {
				t.Fatalf("building the structural schema: %v", err)
			}
			validator := celpkg.NewValidator(structural, true, perCallLimit)
			if validator == nil {
				return
			}
			compiled++
			sample := sampleObject(structural)
			// Pass sample as both obj and oldObj so transition rules referencing oldSelf
			// are compiled and checked alongside standard validation rules.
			errs, _ := validator.Validate(context.Background(), field.NewPath(""), structural, sample, sample, validationLimit)
			for _, entry := range errs {
				if strings.Contains(entry.Detail, "rule compile error") {
					t.Errorf("%s: %v", file, entry)
				}
			}
		})
	}
	if compiled == 0 {
		t.Fatal("no shipped CRD carries validation rules, so this check proves nothing")
	}
}

// sampleObject produces a value that instantiates every property of a
// structural schema, so a lazily compiled rule is reached. Scalars get a
// placeholder of their type; a map or array gets exactly one entry, which is
// enough to compile the rules that iterate them.
func sampleObject(s *schema.Structural) any {
	switch s.Type {
	case "object":
		if s.AdditionalProperties != nil && s.AdditionalProperties.Structural != nil {
			return map[string]any{"sample": sampleObject(s.AdditionalProperties.Structural)}
		}
		out := make(map[string]any, len(s.Properties))
		keys := make([]string, 0, len(s.Properties))
		for key := range s.Properties {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			property := s.Properties[key]
			out[key] = sampleObject(&property)
		}
		return out
	case "array":
		if s.Items == nil {
			return []any{}
		}
		return []any{sampleObject(s.Items)}
	case "string":
		return "sample"
	case "integer":
		return int64(1)
	case "number":
		return float64(1)
	case "boolean":
		return true
	default:
		return map[string]any{}
	}
}

// TestShippedModelRouteRulesBehave drives the two ModelRoute rules that shipped
// broken (or absent) against real objects, so a rule that compiles but has the
// wrong shape still fails: an optional offer id must be accepted when omitted or
// empty, rejected when malformed, and a remote route must declare an offers
// inventory rather than dereferencing an absent one.
func TestShippedModelRouteRulesBehave(t *testing.T) {
	spec := shippedSpecSchema(t, "one-shot-man_modelroutes.yaml")
	structural, err := schema.NewStructural(spec)
	if err != nil {
		t.Fatalf("building the structural schema: %v", err)
	}
	validator := celpkg.NewValidator(structural, false, 1<<20)
	if validator == nil {
		t.Fatal("the shipped model route spec declares no validation rules")
	}

	shaper := func() map[string]any {
		return map[string]any{
			"access":    "alpha-chat",
			"surfaces":  []any{"chat"},
			"endpoints": map[string]any{"chat": "http://127.0.0.1:1/alpha/v1"},
			"naming":    "served",
			"transport": map[string]any{"kind": "shaper", "upstream": "https://alpha.example"},
			"offers":    []any{map[string]any{"serving": "alpha-model-alpha"}},
		}
	}
	valid := func(mutate func(map[string]any)) map[string]any {
		value := shaper()
		mutate(value)
		return value
	}

	tests := []struct {
		name  string
		spec  map[string]any
		valid bool
		why   string
	}{
		{
			name:  "offer id omitted",
			spec:  shaper(),
			valid: true,
			why:   "an omitted optional id must not be dereferenced",
		},
		{
			name: "offer id empty",
			spec: valid(func(m map[string]any) {
				m["offers"] = []any{map[string]any{"serving": "alpha-model-alpha", "id": ""}}
			}),
			valid: true,
			why:   "an empty id means 'default to the serving name'",
		},
		{
			name: "offer id valid RFC-1123",
			spec: valid(func(m map[string]any) {
				m["offers"] = []any{map[string]any{"serving": "alpha-model-alpha", "id": "alias"}}
			}),
			valid: true,
		},
		{
			name: "offer id malformed",
			spec: valid(func(m map[string]any) {
				m["offers"] = []any{map[string]any{"serving": "alpha-model-alpha", "id": "Not_An_ID"}}
			}),
			valid: false,
			why:   "the id becomes a served model id, which is an RFC-1123 name",
		},
		{
			name: "remote route omits offers",
			spec: valid(func(m map[string]any) {
				delete(m, "offers")
				delete(m, "access")
				m["transport"] = map[string]any{"kind": "remote"}
			}),
			valid: false,
			why:   "a remote inventory must be declared, not inferred",
		},
		{
			name: "remote route declares one offer",
			spec: valid(func(m map[string]any) {
				delete(m, "access")
				m["transport"] = map[string]any{"kind": "remote"}
			}),
			valid: true,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			errs, _ := validator.Validate(context.Background(), field.NewPath("spec"), structural, test.spec, nil, 1<<20)
			if got := len(errs) == 0; got != test.valid {
				t.Fatalf("valid=%v, want %v (%s): %v", got, test.valid, test.why, errs.ToAggregate())
			}
		})
	}
}

// TestCRDTypeSourcesAreFormatterStable is the guard that keeps the ASCII rule
// text from regressing through the toolchain rather than through a hand edit.
//
// Go's doc-comment formatter treats a run of two ASCII apostrophes as the legacy
// spelling of a right double quotation mark and rewrites it to U+201D. Because
// every CEL marker is a doc comment, running gofmt (or any editor that formats
// on save) over a rule that compares a field to that empty-string literal
// silently turns it into the exact defect this file exists to catch: the rule
// stops compiling and every object of that kind is rejected. Expressing
// emptiness as `size(self.id) == 0` avoids the construct entirely; this test
// proves the sources stay as written, so a reformat cannot reintroduce the
// defect behind a green ASCII scan.
//
// This comment itself must not spell the offending pair, for the same reason.
func TestCRDTypeSourcesAreFormatterStable(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package directory: %v", err)
	}
	checked := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		source, err := os.ReadFile(name)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		// The same formatter gofmt runs, so "unchanged" means a reformat is a
		// no-op for this file.
		formatted, err := format.Source(source)
		if err != nil {
			t.Fatalf("%s does not parse: %v", name, err)
		}
		checked++
		if !bytes.Equal(formatted, source) {
			t.Errorf("%s is not formatter-stable: running gofmt would change it. "+
				"A doc comment containing '' is rewritten to U+201D, which corrupts a CEL rule.", name)
		}
	}
	if checked == 0 {
		t.Fatal("no sources were checked, so this guard proves nothing")
	}
}

// loadCRDRootSchema decodes the openAPIV3Schema of one CRD's single version as
// the versioned apiextensions type, which is the only shape that carries the
// x-kubernetes-validations json tag.
func loadCRDRootSchema(t *testing.T, file string) apiextensionsv1.JSONSchemaProps {
	t.Helper()
	var crd struct {
		Spec struct {
			Versions []struct {
				Schema struct {
					OpenAPIV3Schema apiextensionsv1.JSONSchemaProps `json:"openAPIV3Schema"`
				} `json:"schema"`
			} `json:"versions"`
		} `json:"spec"`
	}
	data, err := os.ReadFile(filepath.Join(crdDir, file))
	if err != nil {
		t.Fatalf("reading %s: %v", file, err)
	}
	if err := sigsyaml.Unmarshal(data, &crd); err != nil {
		t.Fatalf("decoding %s: %v", file, err)
	}
	if len(crd.Spec.Versions) != 1 {
		t.Fatalf("%s: expected one version, got %d", file, len(crd.Spec.Versions))
	}
	return crd.Spec.Versions[0].Schema.OpenAPIV3Schema
}

// shippedSpecSchema returns the spec sub-schema of one shipped CRD in the
// internal apiextensions form the structural builder and CEL validator consume.
func shippedSpecSchema(t *testing.T, file string) *apiextensions.JSONSchemaProps {
	t.Helper()
	root := loadCRDRootSchema(t, file)
	versioned, ok := root.Properties["spec"]
	if !ok {
		t.Fatalf("%s: the schema declares no spec", file)
	}

	scheme := runtime.NewScheme()
	if err := apiextensions.AddToScheme(scheme); err != nil {
		t.Fatalf("registering the internal apiextensions types: %v", err)
	}
	if err := apiextensionsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("registering the v1 apiextensions types: %v", err)
	}
	if err := apiextensionsv1.RegisterConversions(scheme); err != nil {
		t.Fatalf("registering the apiextensions conversions: %v", err)
	}

	internal := &apiextensions.JSONSchemaProps{}
	if err := scheme.Convert(&versioned, internal, nil); err != nil {
		t.Fatalf("%s: converting the spec schema: %v", file, err)
	}
	return internal
}
