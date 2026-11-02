package userk8s

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/userk8s/api/v1alpha1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	dynamicfake "k8s.io/client-go/dynamic/fake"
)

// clusterListKinds maps each catalog resource to its list kind, which the
// dynamic fake requires in order to answer List calls.
func clusterListKinds() map[schema.GroupVersionResource]string {
	return map[schema.GroupVersionResource]string{
		clusterProviders:  "ModelProviderList",
		clusterAccesses:   "ModelAccessList",
		clusterModels:     "ModelList",
		clusterServings:   "ModelServingList",
		clusterSelections: "ModelSelectionList",
		clusterRoutes:     "ModelRouteList",
		clusterTools:      "ToolList",
		clusterBindings:   "LocalSecretBindingList",
	}
}

func toUnstructured(t *testing.T, objects ...any) []runtime.Object {
	t.Helper()
	converted := make([]runtime.Object, 0, len(objects))
	for _, object := range objects {
		switch typed := object.(type) {
		case *v1alpha1.ModelProvider:
			typed.SetGroupVersionKind(v1alpha1.GroupVersion.WithKind("ModelProvider"))
		case *v1alpha1.ModelAccess:
			typed.SetGroupVersionKind(v1alpha1.GroupVersion.WithKind("ModelAccess"))
		case *v1alpha1.Model:
			typed.SetGroupVersionKind(v1alpha1.GroupVersion.WithKind("Model"))
		case *v1alpha1.ModelServing:
			typed.SetGroupVersionKind(v1alpha1.GroupVersion.WithKind("ModelServing"))
		case *v1alpha1.ModelSelection:
			typed.SetGroupVersionKind(v1alpha1.GroupVersion.WithKind("ModelSelection"))
		case *v1alpha1.ModelRoute:
			typed.SetGroupVersionKind(v1alpha1.GroupVersion.WithKind("ModelRoute"))
		case *v1alpha1.Tool:
			typed.SetGroupVersionKind(v1alpha1.GroupVersion.WithKind("Tool"))
		case *v1alpha1.LocalSecretBinding:
			typed.SetGroupVersionKind(v1alpha1.GroupVersion.WithKind("LocalSecretBinding"))
		}
		content, err := runtime.DefaultUnstructuredConverter.ToUnstructured(object)
		if err != nil {
			t.Fatalf("ToUnstructured: %v", err)
		}
		converted = append(converted, &unstructured.Unstructured{Object: content})
	}
	return converted
}

// seededCluster serves the fixture's catalog objects from a fake API server,
// so both backends answer the same questions about the same facts.
func seededCluster(t *testing.T, namespace string, objects *Objects) *ClusterBackend {
	t.Helper()
	seeded := make([]runtime.Object, 0, len(objects.Providers)+len(objects.Accesses)+len(objects.Models)+len(objects.Tools)+len(objects.Bindings))
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
	return NewClusterBackendFromDynamic(client, namespace, &fakeRunner{})
}

func TestClusterBackendParityWithFilesBackend(t *testing.T) {
	objects, err := LoadPaths(fixture)
	if err != nil {
		t.Fatalf("LoadPaths: %v", err)
	}
	files, err := newBackendFromObjects(objects, &fakeRunner{}, SourceFiles)
	if err != nil {
		t.Fatalf("newBackendFromObjects: %v", err)
	}
	cluster := seededCluster(t, "model-access-test", objects)
	ctx := context.Background()

	filesStatus, err := files.Status(ctx)
	if err != nil {
		t.Fatalf("files Status: %v", err)
	}
	clusterStatus, err := cluster.Status(ctx)
	if err != nil {
		t.Fatalf("cluster Status: %v", err)
	}
	if clusterStatus.Source != SourceCluster || filesStatus.Source != SourceFiles {
		t.Fatalf("sources: files=%q cluster=%q", filesStatus.Source, clusterStatus.Source)
	}
	for _, counts := range []struct {
		name           string
		files, cluster int
	}{
		{"providers", filesStatus.Providers, clusterStatus.Providers},
		{"accesses", filesStatus.Accesses, clusterStatus.Accesses},
		{"models", filesStatus.Models, clusterStatus.Models},
		{"servings", filesStatus.Servings, clusterStatus.Servings},
		{"selections", filesStatus.Selections, clusterStatus.Selections},
		{"routes", filesStatus.Routes, clusterStatus.Routes},
		{"tools", filesStatus.Tools, clusterStatus.Tools},
		{"bindings", filesStatus.Bindings, clusterStatus.Bindings},
	} {
		if counts.files != counts.cluster {
			t.Errorf("%s: files=%d cluster=%d", counts.name, counts.files, counts.cluster)
		}
		if counts.files == 0 {
			t.Errorf("%s: the fixture seeded nothing, so parity proves nothing", counts.name)
		}
	}

	t.Setenv("HOME", t.TempDir())
	for i := range objects.Bindings {
		filesResolution, err := files.ResolveCredential(ctx, objects.Bindings[i].Name)
		if err != nil {
			t.Fatalf("files ResolveCredential(%s): %v", objects.Bindings[i].Name, err)
		}
		clusterResolution, err := cluster.ResolveCredential(ctx, objects.Bindings[i].Name)
		if err != nil {
			t.Fatalf("cluster ResolveCredential(%s): %v", objects.Bindings[i].Name, err)
		}
		if filesResolution.Status != clusterResolution.Status || filesResolution.Reason != clusterResolution.Reason {
			t.Errorf("ResolveCredential(%s): files=%v/%q cluster=%v/%q", objects.Bindings[i].Name, filesResolution.Status, filesResolution.Reason, clusterResolution.Status, clusterResolution.Reason)
		}
	}

	tools, err := files.ListTools(ctx)
	if err != nil {
		t.Fatalf("files ListTools: %v", err)
	}
	projected := 0
	for i := range tools {
		for j := range objects.Servings {
			serving := &objects.Servings[j]
			filesProjection, filesErr := files.ProjectServing(ctx, tools[i].Name, serving)
			clusterProjection, clusterErr := cluster.ProjectServing(ctx, tools[i].Name, serving)
			if (filesErr == nil) != (clusterErr == nil) {
				t.Fatalf("Project(%s,%s): files err=%v cluster err=%v", tools[i].Name, serving.Name, filesErr, clusterErr)
			}
			if filesErr != nil {
				continue
			}
			projected++
			filesJSON, _ := json.Marshal(filesProjection)
			clusterJSON, _ := json.Marshal(clusterProjection)
			if string(filesJSON) != string(clusterJSON) {
				t.Fatalf("Project(%s,%s): files=%s cluster=%s", tools[i].Name, serving.Name, filesJSON, clusterJSON)
			}
		}
	}
	if projected == 0 {
		t.Fatal("Project: no selection projected from both backends")
	}
}

func TestClusterBackendNamespaceScoping(t *testing.T) {
	objects, err := LoadPaths(fixture)
	if err != nil {
		t.Fatalf("LoadPaths: %v", err)
	}
	ctx := context.Background()

	inNamespace := seededCluster(t, "model-access-test", objects)
	status, err := inNamespace.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if status.Bindings != len(objects.Bindings) {
		t.Fatalf("bindings in the fixture namespace: got %d, want %d", status.Bindings, len(objects.Bindings))
	}

	elsewhere := seededCluster(t, "some-other-namespace", objects)
	otherStatus, err := elsewhere.Status(ctx)
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if otherStatus.Bindings != 0 {
		t.Fatalf("bindings outside the fixture namespace: got %d, want none", otherStatus.Bindings)
	}
	if _, err := elsewhere.ResolveCredential(ctx, "bind-alpha"); err == nil {
		t.Fatal("ResolveCredential: a binding outside the backend namespace must not resolve")
	}
}

func TestClusterBackendRequiresNamespaceForBindings(t *testing.T) {
	objects, err := LoadPaths(fixture)
	if err != nil {
		t.Fatalf("LoadPaths: %v", err)
	}
	unscoped := seededCluster(t, "", objects)
	_, err = unscoped.Status(context.Background())
	if err == nil {
		t.Fatal("Status: want an error when a namespaced kind is read without a namespace")
	}
	if !strings.Contains(err.Error(), "namespace") {
		t.Fatalf("error: got %v, want it to name the missing namespace", err)
	}
}

func TestNewClusterBackendHonoursKubeconfigAndContext(t *testing.T) {
	dir := t.TempDir()
	path := dir + "/config"
	kubeconfig := `apiVersion: v1
kind: Config
current-context: alpha
contexts:
- name: alpha
  context:
    cluster: alpha
    namespace: alpha-namespace
- name: beta
  context:
    cluster: alpha
    namespace: beta-namespace
clusters:
- name: alpha
  cluster:
    server: https://127.0.0.1:6443
users:
- name: alpha
  user:
    token: placeholder
`
	if err := os.WriteFile(path, []byte(kubeconfig), 0o600); err != nil {
		t.Fatalf("writing the kubeconfig: %v", err)
	}

	fromCurrent := mustClusterBackend(t, ClusterBackendOptions{Kubeconfig: path})
	if fromCurrent.Namespace() != "alpha-namespace" {
		t.Fatalf("namespace from the current context: got %q, want alpha-namespace", fromCurrent.Namespace())
	}

	fromContext := mustClusterBackend(t, ClusterBackendOptions{Kubeconfig: path, Context: "beta"})
	if fromContext.Namespace() != "beta-namespace" {
		t.Fatalf("namespace from the selected context: got %q, want beta-namespace", fromContext.Namespace())
	}

	explicit := mustClusterBackend(t, ClusterBackendOptions{Kubeconfig: path, Context: "beta", Namespace: "explicit"})
	if explicit.Namespace() != "explicit" {
		t.Fatalf("explicit namespace: got %q, want the configured value to win", explicit.Namespace())
	}

	t.Setenv("KUBECONFIG", path)
	fromEnv := mustClusterBackend(t, ClusterBackendOptions{})
	if fromEnv.Namespace() != "alpha-namespace" {
		t.Fatalf("namespace from KUBECONFIG: got %q, want alpha-namespace", fromEnv.Namespace())
	}

	if _, err := NewClusterBackend(ClusterBackendOptions{Kubeconfig: dir + "/missing"}); err == nil {
		t.Fatal("NewClusterBackend: want an error for a missing kubeconfig")
	}
}

func mustClusterBackend(t *testing.T, options ClusterBackendOptions) *ClusterBackend {
	t.Helper()
	backend, err := NewClusterBackend(options)
	if err != nil {
		t.Fatalf("NewClusterBackend: %v", err)
	}
	return backend
}

func TestClusterBackendReadsOnceUnderConcurrency(t *testing.T) {
	objects, err := LoadPaths(fixture)
	if err != nil {
		t.Fatalf("LoadPaths: %v", err)
	}
	cluster := seededCluster(t, "model-access-test", objects)

	var wg sync.WaitGroup
	statuses := make([]BackendStatus, 8)
	for i := range statuses {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			status, err := cluster.Status(context.Background())
			if err != nil {
				t.Errorf("Status: %v", err)
				return
			}
			statuses[index] = status
		}(i)
	}
	wg.Wait()

	for i := range statuses {
		if statuses[i].Source != SourceCluster || statuses[i].Models != len(objects.Models) {
			t.Fatalf("concurrent status %d: got %+v", i, statuses[i])
		}
		if i > 0 && statuses[i].Models != statuses[0].Models {
			t.Fatalf("concurrent reads disagreed: %+v then %+v", statuses[0], statuses[i])
		}
	}
}
