package userk8s

import (
	"context"
	"os"
	"testing"

	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	celpkg "k8s.io/apiextensions-apiserver/pkg/apiserver/schema/cel"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	sigsyaml "sigs.k8s.io/yaml"
)

// TestShippedToolSettingsRulesRejectUnwaivedOverrides enforces the promise that
// a per-tool override may exceed a model fact only when it carries a waive
// reason, using the rules the model CRD actually ships and the same CEL
// validator the API server uses.
func TestShippedToolSettingsRulesRejectUnwaivedOverrides(t *testing.T) {
	specSchema := shippedSpecSchema(t)
	rules := specSchema.XValidations
	if len(rules) == 0 {
		t.Fatal("the shipped model CRD declares no spec validation rules")
	}
	structural, err := schema.NewStructural(specSchema)
	if err != nil {
		t.Fatalf("building the structural schema: %v", err)
	}
	validator := celpkg.NewValidator(structural, false, 1<<20)
	ctx := context.Background()

	spec := func(contextWindow, maxOutput int64, override map[string]any) map[string]any {
		return map[string]any{
			"context_window":    contextWindow,
			"max_output_tokens": maxOutput,
			"toolSettings":      map[string]any{"claude": override},
		}
	}

	tests := []struct {
		name      string
		spec      map[string]any
		field     string
		wantValid bool
	}{
		{
			name:      "an output override above the fact without a waive reason is rejected",
			spec:      spec(100000, 1000, map[string]any{"max_output_tokens": int64(2000)}),
			field:     "max_output_tokens",
			wantValid: false,
		},
		{
			name: "the same override with a waive reason is accepted",
			spec: spec(100000, 1000, map[string]any{
				"max_output_tokens": int64(2000),
				"waive":             "the provider enforces a higher cap than the registry records",
			}),
			field:     "max_output_tokens",
			wantValid: true,
		},
		{
			name:      "a context override above the fact without a waive reason is rejected",
			spec:      spec(100000, 1000, map[string]any{"context_window": int64(200000)}),
			field:     "context_window",
			wantValid: false,
		},
		{
			name:      "an auto-compact override above the fact without a waive reason is rejected",
			spec:      spec(100000, 1000, map[string]any{"auto_compact_window": int64(150000)}),
			field:     "auto_compact_window",
			wantValid: false,
		},
		{
			name:      "an override within the facts needs no waive reason",
			spec:      spec(100000, 1000, map[string]any{"max_output_tokens": int64(500)}),
			field:     "max_output_tokens",
			wantValid: true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if !rulesMention(rules, test.field) {
				t.Fatalf("no shipped rule mentions %s, so this check would be vacuous", test.field)
			}
			errs, _ := validator.Validate(ctx, field.NewPath("spec"), structural, test.spec, nil, 1<<20)
			if gotValid := len(errs) == 0; gotValid != test.wantValid {
				t.Fatalf("valid=%v, want %v (errors: %v)", gotValid, test.wantValid, errs.ToAggregate())
			}
		})
	}
}

// rulesMention reports whether any shipped rule names a field, so a rule that
// silently disappeared cannot make a negative case pass.
func rulesMention(rules apiextensions.ValidationRules, field string) bool {
	for _, rule := range rules {
		for i := 0; i+len(field) <= len(rule.Rule); i++ {
			if rule.Rule[i:i+len(field)] == field {
				return true
			}
		}
	}
	return false
}

// shippedSpecSchema decodes the spec schema of the shipped model CRD into the
// internal apiextensions type the structural builder needs. The CRD is decoded
// as the versioned type because only that type carries the json tag for
// x-kubernetes-validations, and is then converted.
func shippedSpecSchema(t *testing.T) *apiextensions.JSONSchemaProps {
	t.Helper()
	var crd struct {
		Spec struct {
			Versions []struct {
				Schema struct {
					OpenAPIV3Schema struct {
						Properties map[string]apiextensionsv1.JSONSchemaProps `json:"properties"`
					} `json:"openAPIV3Schema"`
				} `json:"schema"`
			} `json:"versions"`
		} `json:"spec"`
	}
	data, err := os.ReadFile("api/config/crd/one-shot-man_models.yaml")
	if err != nil {
		t.Fatalf("reading the shipped model CRD: %v", err)
	}
	if err := sigsyaml.Unmarshal(data, &crd); err != nil {
		t.Fatalf("decoding the shipped model CRD: %v", err)
	}
	if len(crd.Spec.Versions) == 0 {
		t.Fatal("the shipped model CRD declares no versions")
	}
	versioned, ok := crd.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"]
	if !ok {
		t.Fatal("the shipped model CRD declares no spec schema")
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
		t.Fatalf("converting the spec schema: %v", err)
	}
	return internal
}
