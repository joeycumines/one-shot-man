package userk8s

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/goccy/go-yaml"
	apiextensions "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema"
	"k8s.io/apiextensions-apiserver/pkg/apiserver/schema/defaulting"
	"k8s.io/apimachinery/pkg/runtime"
	sigsyaml "sigs.k8s.io/yaml"
)

// crdFile is the committed manifest for one catalog kind.
func crdFile(kind string) string {
	names := map[string]string{
		"Model":              "one-shot-man_models.yaml",
		"ModelServing":       "one-shot-man_modelservings.yaml",
		"ModelRoute":         "one-shot-man_modelroutes.yaml",
		"ModelSelection":     "one-shot-man_modelselections.yaml",
		"LocalSecretBinding": "one-shot-man_localsecretbindings.yaml",
	}
	return filepath.Join("api", "config", "crd", names[kind])
}

// defaultedByAPIServer applies the API server's own defaulting algorithm to a
// document, driven by the CRD manifest that ships for its kind, and returns the
// typed object with those defaults. This is the oracle: while applyDefaults is
// hand-written, this function is the algorithm the server actually runs, so a
// divergence between the two is a real backend bug rather than a test artifact.
func defaultedByAPIServer(t *testing.T, kind, document string) any {
	t.Helper()

	var converted struct {
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
	data, err := os.ReadFile(crdFile(kind))
	if err != nil {
		t.Fatalf("reading the CRD for %s: %v", kind, err)
	}
	if err := sigsyaml.Unmarshal(data, &converted); err != nil {
		t.Fatalf("decoding the CRD for %s: %v", kind, err)
	}
	versioned, ok := converted.Spec.Versions[0].Schema.OpenAPIV3Schema.Properties["spec"]
	if !ok {
		t.Fatalf("the CRD for %s declares no spec schema", kind)
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
		t.Fatalf("converting the spec schema for %s: %v", kind, err)
	}
	structural, err := schema.NewStructural(internal)
	if err != nil {
		t.Fatalf("building the structural schema for %s: %v", kind, err)
	}

	var object map[string]any
	if err := yaml.Unmarshal([]byte(document), &object); err != nil {
		t.Fatalf("decoding the document for %s: %v", kind, err)
	}
	spec, _ := object["spec"].(map[string]any)
	if spec == nil {
		spec = map[string]any{}
		object["spec"] = spec
	}
	defaulting.Default(spec, structural)
	object["spec"] = spec

	target, err := newTarget(kind)
	if err != nil {
		t.Fatalf("target for %s: %v", kind, err)
	}
	// Round-trip through JSON rather than YAML: the API server's stored form is
	// JSON, and types such as metav1.Duration decode through their json
	// unmarshaller. Using the YAML decoder here would reject or mistype the very
	// values under comparison.
	encoded, err := sigsyaml.Marshal(object)
	if err != nil {
		t.Fatalf("re-encoding the defaulted document for %s: %v", kind, err)
	}
	if err := sigsyaml.Unmarshal(encoded, target); err != nil {
		t.Fatalf("decoding the defaulted document for %s: %v", kind, err)
	}
	return target
}

// catalogSupport is the minimal set of prerequisites a single document of one
// kind needs to pass the loader's cross-resource checks. Each case names its own
// object so the prerequisites never collide with it.
const catalogSupport = `apiVersion: one-shot-man/v1alpha1
kind: ModelProvider
metadata:
  name: alpha
spec:
  display_name: Alpha
---
apiVersion: one-shot-man/v1alpha1
kind: ModelAccess
metadata:
  name: alpha-chat
spec:
  provider: alpha
  protocol: chat
---
apiVersion: one-shot-man/v1alpha1
kind: Model
metadata:
  name: prereq-model
spec:
  canonical_id: alpha/prereq-model
  display_name: Prereq Model
  context_window: 1000
  max_output_tokens: 10
---
apiVersion: one-shot-man/v1alpha1
kind: ModelServing
metadata:
  name: prereq-serving
spec:
  provider: alpha
  model: prereq-model
  provider_model_id: prereq-wire
`

// TestFilesDefaultingMatchesAPIServerDefaulting is the non-circular check that
// the files backend applies exactly the defaults a cluster would. For each kind,
// one document omits every defaulted field; the object the loader produces from
// the raw text must equal the object the API server's defaulting algorithm
// produces from the same text, field for field.
func TestFilesDefaultingMatchesAPIServerDefaulting(t *testing.T) {
	tests := []struct {
		kind     string
		document string
	}{
		{
			kind: "Model",
			document: `apiVersion: one-shot-man/v1alpha1
kind: Model
metadata:
  name: alpha-model-target
spec:
  canonical_id: alpha/alpha-model-target
  display_name: Alpha Model Target
  context_window: 1000
  max_output_tokens: 10
`,
		},
		{
			kind: "ModelServing",
			document: `apiVersion: one-shot-man/v1alpha1
kind: ModelServing
metadata:
  name: alpha-serving-target
spec:
  provider: alpha
  model: prereq-model
  provider_model_id: alpha-wire
`,
		},
		{
			kind: "ModelRoute",
			document: `apiVersion: one-shot-man/v1alpha1
kind: ModelRoute
metadata:
  name: alpha-local
  namespace: test
spec:
  access: alpha-chat
  offers: []
  surfaces:
    - chat
  endpoints:
    chat: http://127.0.0.1:1/alpha/v1
  naming: served
  transport:
    kind: shaper
    upstream: https://alpha.example
`,
		},
		{
			kind: "ModelSelection",
			document: `apiVersion: one-shot-man/v1alpha1
kind: ModelSelection
metadata:
  name: alpha-default
  namespace: test
spec:
  display_name: Alpha default
  candidates:
    - serving: prereq-serving
`,
		},
		{
			kind: "LocalSecretBinding",
			document: `apiVersion: one-shot-man/v1alpha1
kind: LocalSecretBinding
metadata:
  name: bind-alpha
  namespace: test
spec:
  selector:
    matchLabels:
      one-shot-man/provider: alpha
  secretRef:
    name: creds
    key: ALPHA_API_KEY
  envVar: ALPHA_API_KEY
  resolvers:
    - command:
        argv:
          - fake-op
`,
		},
	}

	for _, test := range tests {
		t.Run(test.kind, func(t *testing.T) {
			// Load the document alongside the prerequisites it references, then
			// compare only the kind under test.
			source := catalogSupport + "---\n" + test.document
			objects, err := loadDocuments([]byte(source))
			if err != nil {
				t.Fatalf("loadDocuments: %v", err)
			}
			loaded := onlyObject(t, objects, test.kind, test.document)

			oracle := defaultedByAPIServer(t, test.kind, test.document)

			loadedJSON, err := json.Marshal(loaded)
			if err != nil {
				t.Fatalf("marshalling the loaded object: %v", err)
			}
			oracleJSON, err := json.Marshal(oracle)
			if err != nil {
				t.Fatalf("marshalling the API-server-defaulted object: %v", err)
			}
			if string(loadedJSON) != string(oracleJSON) {
				t.Fatalf("files defaulting diverged from the API server for %s:\n files: %s\noracle: %s", test.kind, loadedJSON, oracleJSON)
			}
		})
	}
}

// onlyObject returns the object of one kind whose name matches the document
// under test, so a prerequisite of the same kind is not mistaken for it.
func onlyObject(t *testing.T, objects *Objects, kind, document string) any {
	t.Helper()
	var meta struct {
		Metadata struct {
			Name string `json:"name"`
		} `json:"metadata"`
	}
	if err := yaml.Unmarshal([]byte(document), &meta); err != nil {
		t.Fatalf("reading the document name: %v", err)
	}
	name := meta.Metadata.Name
	pick := func(candidates []any, names []string) any {
		for i, candidateName := range names {
			if candidateName == name {
				return candidates[i]
			}
		}
		t.Fatalf("no %s named %q among %v", kind, name, names)
		return nil
	}
	switch kind {
	case "Model":
		names := make([]string, len(objects.Models))
		values := make([]any, len(objects.Models))
		for i := range objects.Models {
			names[i] = objects.Models[i].Name
			values[i] = objects.Models[i]
		}
		return pick(values, names)
	case "ModelServing":
		names := make([]string, len(objects.Servings))
		values := make([]any, len(objects.Servings))
		for i := range objects.Servings {
			names[i] = objects.Servings[i].Name
			values[i] = objects.Servings[i]
		}
		return pick(values, names)
	case "ModelRoute":
		names := make([]string, len(objects.Routes))
		values := make([]any, len(objects.Routes))
		for i := range objects.Routes {
			names[i] = objects.Routes[i].Name
			values[i] = objects.Routes[i]
		}
		return pick(values, names)
	case "ModelSelection":
		names := make([]string, len(objects.Selections))
		values := make([]any, len(objects.Selections))
		for i := range objects.Selections {
			names[i] = objects.Selections[i].Name
			values[i] = objects.Selections[i]
		}
		return pick(values, names)
	case "LocalSecretBinding":
		names := make([]string, len(objects.Bindings))
		values := make([]any, len(objects.Bindings))
		for i := range objects.Bindings {
			names[i] = objects.Bindings[i].Name
			values[i] = objects.Bindings[i]
		}
		return pick(values, names)
	default:
		t.Fatalf("no object accessor for kind %s", kind)
		return nil
	}
}
