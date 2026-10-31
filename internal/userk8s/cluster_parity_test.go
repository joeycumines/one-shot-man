package userk8s

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/goccy/go-yaml"
	"github.com/joeycumines/one-shot-man/internal/userk8s/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	dynamicfake "k8s.io/client-go/dynamic/fake"
	ktesting "k8s.io/client-go/testing"
)

// valueRunner resolves every command, so both backends can be compared on a
// successful resolution rather than only on a miss.
type valueRunner struct{}

func (valueRunner) Run(context.Context, []string, time.Duration) (string, error) {
	return "resolved-value", nil
}

func seededClusterWithRunner(t *testing.T, namespace string, objects *Objects, runner CommandRunner) (*ClusterBackend, *dynamicfake.FakeDynamicClient) {
	t.Helper()
	seeded := make([]runtime.Object, 0)
	for i := range objects.Providers {
		seeded = append(seeded, toUnstructured(t, &objects.Providers[i])...)
	}
	for i := range objects.Accesses {
		seeded = append(seeded, toUnstructured(t, &objects.Accesses[i])...)
	}
	for i := range objects.Models {
		seeded = append(seeded, toUnstructured(t, &objects.Models[i])...)
	}
	for i := range objects.Servings {
		seeded = append(seeded, toUnstructured(t, &objects.Servings[i])...)
	}
	for i := range objects.Selections {
		seeded = append(seeded, toUnstructured(t, &objects.Selections[i])...)
	}
	for i := range objects.Routes {
		seeded = append(seeded, toUnstructured(t, &objects.Routes[i])...)
	}
	for i := range objects.Tools {
		seeded = append(seeded, toUnstructured(t, &objects.Tools[i])...)
	}
	for i := range objects.Bindings {
		seeded = append(seeded, toUnstructured(t, &objects.Bindings[i])...)
	}
	client := dynamicfake.NewSimpleDynamicClientWithCustomListKinds(runtime.NewScheme(), clusterListKinds(), seeded...)
	return NewClusterBackendFromDynamic(client, namespace, runner), client
}

// TestClusterResourceNamesMatchCommittedCRDs pins the resource plurals the
// backend lists against the CRDs that actually ship, because the fake client
// would happily accept a plural the real server does not serve.
func TestClusterResourceNamesMatchCommittedCRDs(t *testing.T) {
	tests := []struct {
		resource schema.GroupVersionResource
		want     string
		scope    string
	}{
		{clusterProviders, "modelproviders", "Cluster"},
		{clusterAccesses, "modelaccesses", "Cluster"},
		{clusterModels, "models", "Cluster"},
		{clusterServings, "modelservings", "Cluster"},
		{clusterSelections, "modelselections", "Namespaced"},
		{clusterRoutes, "modelroutes", "Namespaced"},
		{clusterTools, "tools", "Cluster"},
		{clusterBindings, "localsecretbindings", "Namespaced"},
	}
	for _, test := range tests {
		if test.resource.Resource != test.want {
			t.Errorf("resource %q: want %q", test.resource.Resource, test.want)
		}
		if test.resource.Group != v1alpha1.Group || test.resource.Version != v1alpha1.Version {
			t.Errorf("resource %q: group/version %s/%s, want %s/%s", test.resource.Resource, test.resource.Group, test.resource.Version, v1alpha1.Group, v1alpha1.Version)
		}

		path := filepath.Join("api", "config", "crd", v1alpha1.Group+"_"+test.resource.Resource+".yaml")
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading the shipped CRD %s: %v", path, err)
		}
		var crd struct {
			Spec struct {
				Scope string `json:"scope"`
				Names struct {
					Plural string `json:"plural"`
					Kind   string `json:"kind"`
				} `json:"names"`
			} `json:"spec"`
		}
		if err := yaml.Unmarshal(data, &crd); err != nil {
			t.Fatalf("decoding the shipped CRD %s: %v", path, err)
		}
		if crd.Spec.Names.Plural != test.resource.Resource {
			t.Errorf("CRD %s declares plural %q but the backend lists %q", path, crd.Spec.Names.Plural, test.resource.Resource)
		}
		if crd.Spec.Scope != test.scope {
			t.Errorf("CRD %s declares scope %q, want %q", path, crd.Spec.Scope, test.scope)
		}
	}
}

func TestClusterBackendListAndResolutionParity(t *testing.T) {
	objects, err := LoadPaths(fixture)
	if err != nil {
		t.Fatalf("LoadPaths: %v", err)
	}
	t.Setenv("HOME", t.TempDir())
	for _, binding := range objects.Bindings {
		for _, step := range binding.Spec.Resolvers {
			if step.Env != nil {
				t.Setenv(step.Env.Name, "")
			}
		}
	}
	files, err := newBackendFromObjects(objects, valueRunner{}, SourceFiles)
	if err != nil {
		t.Fatalf("newBackendFromObjects: %v", err)
	}
	cluster, _ := seededClusterWithRunner(t, "model-access-test", objects, valueRunner{})
	ctx := context.Background()

	compare := func(name string, filesValue, clusterValue any) {
		t.Helper()
		filesJSON, err := json.Marshal(filesValue)
		if err != nil {
			t.Fatalf("%s: marshalling the files value: %v", name, err)
		}
		clusterJSON, err := json.Marshal(clusterValue)
		if err != nil {
			t.Fatalf("%s: marshalling the cluster value: %v", name, err)
		}
		if string(filesJSON) != string(clusterJSON) {
			t.Fatalf("%s diverged:\n files: %s\ncluster: %s", name, filesJSON, clusterJSON)
		}
		if string(filesJSON) == "[]" || string(filesJSON) == "null" {
			t.Fatalf("%s: the fixture is empty, so parity proves nothing", name)
		}
	}

	filesProviders, err := files.ListProviders(ctx)
	if err != nil {
		t.Fatalf("files ListProviders: %v", err)
	}
	clusterProviders, err := cluster.ListProviders(ctx)
	if err != nil {
		t.Fatalf("cluster ListProviders: %v", err)
	}
	compare("providers", filesProviders, clusterProviders)

	filesAccesses, err := files.ListAccesses(ctx)
	if err != nil {
		t.Fatalf("files ListAccesses: %v", err)
	}
	clusterAccesses, err := cluster.ListAccesses(ctx)
	if err != nil {
		t.Fatalf("cluster ListAccesses: %v", err)
	}
	compare("accesses", filesAccesses, clusterAccesses)

	filesModels, err := files.ListModels(ctx)
	if err != nil {
		t.Fatalf("files ListModels: %v", err)
	}
	clusterModelsList, err := cluster.ListModels(ctx)
	if err != nil {
		t.Fatalf("cluster ListModels: %v", err)
	}
	compare("models", filesModels, clusterModelsList)

	filesServings, err := files.ListServings(ctx)
	if err != nil {
		t.Fatalf("files ListServings: %v", err)
	}
	clusterServingsList, err := cluster.ListServings(ctx)
	if err != nil {
		t.Fatalf("cluster ListServings: %v", err)
	}
	compare("servings", filesServings, clusterServingsList)

	filesSelections, err := files.ListSelections(ctx)
	if err != nil {
		t.Fatalf("files ListSelections: %v", err)
	}
	clusterSelectionsList, err := cluster.ListSelections(ctx)
	if err != nil {
		t.Fatalf("cluster ListSelections: %v", err)
	}
	compare("selections", filesSelections, clusterSelectionsList)

	filesRoutes, err := files.ListRoutes(ctx)
	if err != nil {
		t.Fatalf("files ListRoutes: %v", err)
	}
	clusterRoutesList, err := cluster.ListRoutes(ctx)
	if err != nil {
		t.Fatalf("cluster ListRoutes: %v", err)
	}
	compare("routes", filesRoutes, clusterRoutesList)

	filesTools, err := files.ListTools(ctx)
	if err != nil {
		t.Fatalf("files ListTools: %v", err)
	}
	clusterToolsList, err := cluster.ListTools(ctx)
	if err != nil {
		t.Fatalf("cluster ListTools: %v", err)
	}
	compare("tools", filesTools, clusterToolsList)

	for i := range objects.Bindings {
		binding := objects.Bindings[i].Name
		filesResolution, err := files.ResolveCredential(ctx, binding)
		if err != nil {
			t.Fatalf("files ResolveCredential(%s): %v", binding, err)
		}
		clusterResolution, err := cluster.ResolveCredential(ctx, binding)
		if err != nil {
			t.Fatalf("cluster ResolveCredential(%s): %v", binding, err)
		}
		compare("resolution "+binding, filesResolution, clusterResolution)
	}
	resolved, err := files.ResolveCredential(ctx, "bind-alpha")
	if err != nil {
		t.Fatalf("files ResolveCredential: %v", err)
	}
	if resolved.Status != StatusResolved || len(resolved.Credentials) != 1 {
		t.Fatalf("resolution: got %+v, want one resolved credential", resolved)
	}
	if resolved.Credentials[0].Value != "resolved-value" {
		t.Fatalf("resolution value: got %q, want the runner value", resolved.Credentials[0].Value)
	}

	projected := 0
	for i := range clusterToolsList {
		for j := range clusterServingsList {
			serving := &clusterServingsList[j]
			filesProjection, filesErr := files.Project(ctx, clusterToolsList[i].Name, serving.Spec.Provider, serving.Name)
			clusterProjection, clusterErr := cluster.Project(ctx, clusterToolsList[i].Name, serving.Spec.Provider, serving.Name)
			if (filesErr == nil) != (clusterErr == nil) {
				t.Fatalf("Project(%s,%s): files err=%v cluster err=%v", clusterToolsList[i].Name, serving.Name, filesErr, clusterErr)
			}
			if filesErr != nil {
				continue
			}
			projected++
			compare("projection "+clusterToolsList[i].Name+"/"+serving.Name, filesProjection, clusterProjection)
		}
	}
	if projected == 0 {
		t.Fatal("projection parity: no selection projected, so the comparison proves nothing")
	}
}

func TestClusterBackendReadsTheCatalogOnce(t *testing.T) {
	objects, err := LoadPaths(fixture)
	if err != nil {
		t.Fatalf("LoadPaths: %v", err)
	}
	cluster, client := seededClusterWithRunner(t, "model-access-test", objects, &fakeRunner{})
	ctx := context.Background()

	before, err := cluster.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}

	added := &v1alpha1.Model{
		APIVersion: v1alpha1.GroupVersion.String(), Kind: "Model",
		Name: "added-after-the-first-read",
		Spec: v1alpha1.ModelSpec{CanonicalID: "test/added", DisplayName: "Added", ContextWindow: 1000, MaxOutputTokens: 10},
	}
	content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(added)
	if err != nil {
		t.Fatalf("ToUnstructured: %v", err)
	}
	if _, err := client.Resource(clusterModels).Create(ctx, &unstructured.Unstructured{Object: content}, metav1.CreateOptions{}); err != nil {
		t.Fatalf("creating a model after the first read: %v", err)
	}

	after, err := cluster.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if after.Models != before.Models {
		t.Fatalf("models: got %d after adding one, want the cached %d", after.Models, before.Models)
	}
}

func TestClusterBackendKubeconfigErrorNamesTheMissingFile(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "no-such-kubeconfig")
	_, err := NewClusterBackend(ClusterBackendOptions{Kubeconfig: missing})
	if err == nil {
		t.Fatal("NewClusterBackend: want an error for a missing kubeconfig")
	}
	if !strings.Contains(err.Error(), "no-such-kubeconfig") {
		t.Fatalf("error: got %v, want it to name the missing file", err)
	}
}

// pagedClient serves a fixed sequence of pages for one resource, which the
// upstream fake cannot express on its own.
type pagedClient struct {
	dynamic.Interface
	resource schema.GroupVersionResource
	pages    []*unstructured.UnstructuredList
	calls    int
	options  []metav1.ListOptions
}

func (p *pagedClient) Resource(resource schema.GroupVersionResource) dynamic.NamespaceableResourceInterface {
	inner := p.Interface.Resource(resource)
	if resource == p.resource {
		return &pagedResource{NamespaceableResourceInterface: inner, client: p}
	}
	return inner
}

type pagedResource struct {
	dynamic.NamespaceableResourceInterface
	client *pagedClient
}

func (r *pagedResource) Namespace(string) dynamic.ResourceInterface { return r }

func (r *pagedResource) List(_ context.Context, options metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	r.client.options = append(r.client.options, options)
	if r.client.calls >= len(r.client.pages) {
		return &unstructured.UnstructuredList{}, nil
	}
	page := r.client.pages[r.client.calls]
	r.client.calls++
	return page, nil
}

func modelPage(provider, name, continueToken string) *unstructured.UnstructuredList {
	item := unstructured.Unstructured{Object: map[string]any{
		"apiVersion": v1alpha1.GroupVersion.String(),
		"kind":       "Model",
		"metadata":   map[string]any{"name": name},
		"spec": map[string]any{
			"canonical_id":      "test/" + name,
			"display_name":      name,
			"context_window":    int64(1000000),
			"max_output_tokens": int64(1000000),
			"input_modalities":  []any{"text", "image"},
			"output_modalities": []any{"text"},
			"reasoning":         map[string]any{"supported": true, "efforts": []any{"low", "high"}},
			"tool_call":         true,
		},
	}}
	list := &unstructured.UnstructuredList{
		Object: map[string]any{"apiVersion": v1alpha1.GroupVersion.String(), "kind": "ModelList"},
		Items:  []unstructured.Unstructured{item},
	}
	if continueToken != "" {
		list.Object["metadata"] = map[string]any{"continue": continueToken}
	}
	return list
}

func TestClusterBackendFollowsPagination(t *testing.T) {
	objects, err := LoadPaths(fixture)
	if err != nil {
		t.Fatalf("LoadPaths: %v", err)
	}
	_, client := seededClusterWithRunner(t, "model-access-test", objects, &fakeRunner{})
	provider := objects.Providers[0].Name
	paged := &pagedClient{
		Interface: client,
		resource:  clusterModels,
		pages:     []*unstructured.UnstructuredList{modelPage(provider, "alpha-model", "page-two"), modelPage(provider, "beta-model", "")},
	}
	cluster := NewClusterBackendFromDynamic(paged, "model-access-test", &fakeRunner{})

	models, err := cluster.ListModels(context.Background())
	if err != nil {
		t.Fatalf("ListModels: %v", err)
	}
	if paged.calls != 2 {
		t.Fatalf("pages read: got %d, want both pages", paged.calls)
	}
	if len(paged.options) != 2 || paged.options[0].Continue != "" || paged.options[1].Continue != "page-two" {
		t.Fatalf("list options: got %+v, want the server's continuation token propagated", paged.options)
	}
	if len(models) != 2 || models[0].Name != "alpha-model" || models[1].Name != "beta-model" {
		t.Fatalf("models: got %+v, want one model from each page", models)
	}
}

func TestClusterBackendStopsOnARepeatedContinuationToken(t *testing.T) {
	objects, err := LoadPaths(fixture)
	if err != nil {
		t.Fatalf("LoadPaths: %v", err)
	}
	_, client := seededClusterWithRunner(t, "model-access-test", objects, &fakeRunner{})
	provider := objects.Providers[0].Name
	repeating := make([]*unstructured.UnstructuredList, maxCatalogPages+1)
	for i := range repeating {
		repeating[i] = modelPage(provider, "repeating", "always-the-same")
	}
	paged := &pagedClient{Interface: client, resource: clusterModels, pages: repeating}
	cluster := NewClusterBackendFromDynamic(paged, "model-access-test", &fakeRunner{})

	if _, err := cluster.ListModels(context.Background()); err == nil {
		t.Fatal("ListModels: want an error when a server repeats its continuation token forever")
	}
	if paged.calls > maxCatalogPages {
		t.Fatalf("pages read: got %d, want at most %d", paged.calls, maxCatalogPages)
	}
}
func TestClusterBackendReportsAMissingResourceAsACRDPermission(t *testing.T) {
	objects, err := LoadPaths(fixture)
	if err != nil {
		t.Fatalf("LoadPaths: %v", err)
	}
	cluster, client := seededClusterWithRunner(t, "model-access-test", objects, &fakeRunner{})
	client.PrependReactor("list", "tools", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, apierrors.NewNotFound(schema.GroupResource{Group: v1alpha1.Group, Resource: "tools"}, "")
	})

	_, err = cluster.Status(context.Background())
	if err == nil {
		t.Fatal("Status: want an error when a catalog resource is absent")
	}
	if !strings.Contains(err.Error(), "install the catalog CRDs") {
		t.Fatalf("error: got %v, want it to name the CRD prerequisite", err)
	}
}

func TestClusterBackendReportsAnUnknownResourceKindAsACRDPermission(t *testing.T) {
	objects, err := LoadPaths(fixture)
	if err != nil {
		t.Fatalf("LoadPaths: %v", err)
	}
	cluster, client := seededClusterWithRunner(t, "model-access-test", objects, &fakeRunner{})
	client.PrependReactor("list", "tools", func(ktesting.Action) (bool, runtime.Object, error) {
		return true, nil, &meta.NoKindMatchError{
			GroupKind:        schema.GroupKind{Group: v1alpha1.Group, Kind: "Tool"},
			SearchedVersions: []string{v1alpha1.Version},
		}
	})

	_, err = cluster.Status(context.Background())
	if err == nil {
		t.Fatal("Status: want an error when the server knows no such kind")
	}
	if !strings.Contains(err.Error(), "install the catalog CRDs") {
		t.Fatalf("error: got %v, want the no-kind-match path to name the CRD prerequisite", err)
	}
}

func TestStatusArtifactShapeMatchesBetweenBackends(t *testing.T) {
	objects, err := LoadPaths(fixture)
	if err != nil {
		t.Fatalf("LoadPaths: %v", err)
	}
	files, err := newBackendFromObjects(objects, &fakeRunner{}, SourceFiles)
	if err != nil {
		t.Fatalf("newBackendFromObjects: %v", err)
	}
	cluster, _ := seededClusterWithRunner(t, "model-access-test", objects, &fakeRunner{})
	ctx := context.Background()

	clusterStatus, err := cluster.Status(ctx)
	if err != nil {
		t.Fatalf("cluster Status: %v", err)
	}
	if clusterStatus.Artifacts == nil {
		t.Fatal("cluster Status: artifacts must be an empty list, not nil")
	}
	clusterJSON, err := json.Marshal(clusterStatus)
	if err != nil {
		t.Fatalf("marshalling the cluster status: %v", err)
	}
	if !strings.Contains(string(clusterJSON), `"artifacts":[]`) {
		t.Fatalf("cluster status JSON: got %s, want an empty artifacts array", clusterJSON)
	}

	filesStatus, err := files.Status(ctx)
	if err != nil {
		t.Fatalf("files Status: %v", err)
	}
	if len(filesStatus.Artifacts) == 0 {
		t.Fatal("files Status: want the fixture path recorded")
	}
}
