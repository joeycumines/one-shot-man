package userk8s

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/joeycumines/one-shot-man/internal/userk8s/api/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/tools/clientcmd"
)

// ClusterBackend serves the same Catalog interface as the files backend by
// listing the catalog kinds from a Kubernetes API server once, then reusing
// the shared catalog logic over the objects it read. There is no watch and no
// informer: the catalog is read when first needed and then cached, so a
// long-running consumer observes the catalog as of its first read. Status
// reports no artifacts, because artifacts name files and a cluster read has
// none.
type ClusterBackend struct {
	client    dynamic.Interface
	namespace string
	runner    CommandRunner

	// mu guards the one-time catalog read, so concurrent callers list once
	// instead of racing to build the cached backend.
	mu    sync.Mutex
	inner *FilesBackend
}

// Compile-time proof that the cluster backend satisfies the shared interface.
var _ Catalog = (*ClusterBackend)(nil)

// ClusterBackendOptions configures a ClusterBackend.
type ClusterBackendOptions struct {
	// Namespace scopes the namespaced kinds (LocalSecretBinding). Empty uses
	// the namespace of the selected kubeconfig context.
	Namespace string

	// Kubeconfig is an explicit kubeconfig path; empty follows KUBECONFIG and
	// then the default loading rules.
	Kubeconfig string

	// Context selects a kubeconfig context; empty uses the current context.
	Context string

	// Runner executes command resolvers; tests inject a fake.
	Runner CommandRunner
}

// NewClusterBackend builds a backend against the cluster the kubeconfig
// describes, without contacting it: the catalog is listed on first use.
func NewClusterBackend(options ClusterBackendOptions) (*ClusterBackend, error) {
	loadingRules := clientcmd.NewDefaultClientConfigLoadingRules()
	if options.Kubeconfig != "" {
		loadingRules.ExplicitPath = options.Kubeconfig
	}
	overrides := &clientcmd.ConfigOverrides{}
	if options.Context != "" {
		overrides.CurrentContext = options.Context
	}
	loading := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loadingRules, overrides)

	restConfig, err := loading.ClientConfig()
	if err != nil {
		return nil, fmt.Errorf("userk8s: resolving the cluster connection: %w", err)
	}
	namespace := options.Namespace
	if namespace == "" {
		if fromContext, _, err := loading.Namespace(); err == nil {
			namespace = fromContext
		}
	}
	client, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, fmt.Errorf("userk8s: creating the cluster client: %w", err)
	}
	return NewClusterBackendFromDynamic(client, namespace, options.Runner), nil
}

// Namespace reports the namespace namespaced kinds are read from.
func (c *ClusterBackend) Namespace() string { return c.namespace }

// NewClusterBackendFromDynamic is NewClusterBackend over an injected client, so
// tests can drive the backend with a fake clientset.
func NewClusterBackendFromDynamic(client dynamic.Interface, namespace string, runner CommandRunner) *ClusterBackend {
	return &ClusterBackend{client: client, namespace: namespace, runner: runner}
}

// GroupVersion identifies the catalog API the cluster backend reads.
var clusterGroupVersion = schema.GroupVersion{Group: v1alpha1.Group, Version: v1alpha1.Version}

// ClusterBackend reads five kinds; the resources are the lowercase plurals of
// the kind names.
var (
	clusterProviders = clusterGroupVersion.WithResource("modelproviders")
	clusterAccesses  = clusterGroupVersion.WithResource("modelaccesses")
	clusterModels    = clusterGroupVersion.WithResource("models")
	clusterTools     = clusterGroupVersion.WithResource("tools")
	clusterBindings  = clusterGroupVersion.WithResource("localsecretbindings")
)

// catalog lists the catalog kinds once and reuses the shared catalog logic.
func (c *ClusterBackend) catalog(ctx context.Context) (*FilesBackend, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.inner != nil {
		return c.inner, nil
	}
	if c.client == nil {
		return nil, errors.New("userk8s: the cluster backend has no client")
	}

	objects := &Objects{}
	for _, kind := range []struct {
		resource   schema.GroupVersionResource
		namespaced bool
		add        func(unstructured.Unstructured) error
	}{
		{clusterProviders, false, func(u unstructured.Unstructured) error {
			var provider v1alpha1.ModelProvider
			if err := fromUnstructured(&u, &provider); err != nil {
				return err
			}
			objects.Providers = append(objects.Providers, provider)
			return nil
		}},
		{clusterAccesses, false, func(u unstructured.Unstructured) error {
			var access v1alpha1.ModelAccess
			if err := fromUnstructured(&u, &access); err != nil {
				return err
			}
			objects.Accesses = append(objects.Accesses, access)
			return nil
		}},
		{clusterModels, false, func(u unstructured.Unstructured) error {
			var model v1alpha1.Model
			if err := fromUnstructured(&u, &model); err != nil {
				return err
			}
			objects.Models = append(objects.Models, model)
			return nil
		}},
		{clusterTools, false, func(u unstructured.Unstructured) error {
			var tool v1alpha1.Tool
			if err := fromUnstructured(&u, &tool); err != nil {
				return err
			}
			objects.Tools = append(objects.Tools, tool)
			return nil
		}},
		{clusterBindings, true, func(u unstructured.Unstructured) error {
			var binding v1alpha1.LocalSecretBinding
			if err := fromUnstructured(&u, &binding); err != nil {
				return err
			}
			objects.Bindings = append(objects.Bindings, binding)
			return nil
		}},
	} {
		list, err := c.list(ctx, kind.resource, kind.namespaced)
		if err != nil {
			return nil, err
		}
		for i := range list.Items {
			if err := kind.add(list.Items[i]); err != nil {
				return nil, fmt.Errorf("userk8s: decoding %s: %w", kind.resource.Resource, err)
			}
		}
	}

	// The shared cross-reference checks must hold for cluster objects too.
	if err := objects.index(); err != nil {
		return nil, err
	}
	inner, err := newBackendFromObjects(objects, c.runner, SourceCluster)
	if err != nil {
		return nil, err
	}
	c.inner = inner
	return inner, nil
}

// list reads every page of one resource. An empty result is not an error: a
// cluster that simply has no objects of a kind still serves the other kinds. A
// resource the server does not know is an error that names the CRD
// prerequisite, because that is a deployment fault rather than an empty
// catalog.
func (c *ClusterBackend) list(ctx context.Context, resource schema.GroupVersionResource, namespaced bool) (*unstructured.UnstructuredList, error) {
	if namespaced && c.namespace == "" {
		return nil, fmt.Errorf("userk8s: reading %s requires a namespace (set userk8s.namespace or select a context that carries one)", resource.Resource)
	}

	combined := &unstructured.UnstructuredList{}
	options := metav1.ListOptions{}
	for page := 0; ; page++ {
		if page >= maxCatalogPages {
			return nil, fmt.Errorf("userk8s: listing %s did not finish after %d pages", resource.Resource, maxCatalogPages)
		}
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		response, err := c.listPage(ctx, resource, namespaced, options)
		if err != nil {
			// A resource the server does not know arrives either as NotFound or
			// as a no-kind-match error, depending on the client path.
			if apierrors.IsNotFound(err) || meta.IsNoMatchError(err) {
				return nil, fmt.Errorf("userk8s: the cluster does not serve %s.%s: install the catalog CRDs from the one-shot-man repository before selecting the cluster backend: %w", resource.Resource, resource.Group, err)
			}
			return nil, fmt.Errorf("userk8s: listing %s: %w", resource.Resource, err)
		}
		combined.Items = append(combined.Items, response.Items...)
		if len(combined.Items) > maxCatalogItems {
			return nil, fmt.Errorf("userk8s: listing %s exceeded %d objects", resource.Resource, maxCatalogItems)
		}
		// The server pages large collections; ignoring the continuation token
		// would silently truncate the catalog, and a server that repeats a
		// token must not be able to spin this loop forever.
		if response.GetContinue() == "" {
			return combined, nil
		}
		options.Continue = response.GetContinue()
	}
}

// maxCatalogPages bounds how many pages one resource may span, so a server that
// repeats a continuation token cannot make the engine loop without end; the
// item bound keeps the buffered catalog finite even when a page is enormous.
const (
	maxCatalogPages = 1000
	maxCatalogItems = 100000
)

// listPage reads one page of a resource.
func (c *ClusterBackend) listPage(ctx context.Context, resource schema.GroupVersionResource, namespaced bool, options metav1.ListOptions) (*unstructured.UnstructuredList, error) {
	if namespaced {
		return c.client.Resource(resource).Namespace(c.namespace).List(ctx, options)
	}
	return c.client.Resource(resource).List(ctx, options)
}

// fromUnstructured converts one API object into its typed form.
func fromUnstructured(source *unstructured.Unstructured, target any) error {
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(source.Object, target); err != nil {
		return fmt.Errorf("%s %q: %w", source.GetKind(), source.GetName(), err)
	}
	return nil
}

// ListProviders returns every provider, ordered by name.
func (c *ClusterBackend) ListProviders(ctx context.Context) ([]v1alpha1.ModelProvider, error) {
	inner, err := c.catalog(ctx)
	if err != nil {
		return nil, err
	}
	return inner.ListProviders(ctx)
}

// ListAccesses returns every access, ordered by name.
func (c *ClusterBackend) ListAccesses(ctx context.Context) ([]v1alpha1.ModelAccess, error) {
	inner, err := c.catalog(ctx)
	if err != nil {
		return nil, err
	}
	return inner.ListAccesses(ctx)
}

// ListModels returns every model, ordered by name.
func (c *ClusterBackend) ListModels(ctx context.Context) ([]v1alpha1.Model, error) {
	inner, err := c.catalog(ctx)
	if err != nil {
		return nil, err
	}
	return inner.ListModels(ctx)
}

// ListTools returns every tool, ordered by name.
func (c *ClusterBackend) ListTools(ctx context.Context) ([]v1alpha1.Tool, error) {
	inner, err := c.catalog(ctx)
	if err != nil {
		return nil, err
	}
	return inner.ListTools(ctx)
}

// ResolveCredential reuses the shared resolution protocol.
func (c *ClusterBackend) ResolveCredential(ctx context.Context, accessRef string) (Resolution, error) {
	inner, err := c.catalog(ctx)
	if err != nil {
		return Resolution{}, err
	}
	return inner.ResolveCredential(ctx, accessRef)
}

// Project reuses the shared projection.
func (c *ClusterBackend) Project(ctx context.Context, tool, provider, model string) (Projection, error) {
	inner, err := c.catalog(ctx)
	if err != nil {
		return Projection{}, err
	}
	return inner.Project(ctx, tool, provider, model)
}

// ProjectModel reuses the shared projection for an already-resolved model.
func (c *ClusterBackend) ProjectModel(ctx context.Context, tool string, model *v1alpha1.Model) (Projection, error) {
	inner, err := c.catalog(ctx)
	if err != nil {
		return Projection{}, err
	}
	return inner.ProjectModel(ctx, tool, model)
}

// Status reports what the cluster read.
func (c *ClusterBackend) Status(ctx context.Context) (BackendStatus, error) {
	inner, err := c.catalog(ctx)
	if err != nil {
		return BackendStatus{}, err
	}
	return inner.Status(ctx)
}
