package gateway

import (
	"fmt"
	"sort"
	"strings"

	"github.com/joeycumines/one-shot-man/internal/userk8s/api/v1alpha1"
)

// Mount is one shaper-fronted access the gateway serves: the prefix it is
// exposed at and the variable the shaper reads its provider credential from.
type Mount struct {
	Access     string
	Provider   string
	Prefix     string
	Credential string // the variable the shaper reads, e.g. SHAPER_PROVIDER_UMANS_API_KEY
	Required   string // the variable the catalog declared, e.g. UMANS_API_KEY
}

// shaperEnvPrefix and shaperEnvSuffix spell the variable the shaper reads, which
// differs from the name the catalog declares for the same credential: the
// registry records the tool-facing name (YOLO_API_KEY) while the shaper spells
// it SHAPER_PROVIDER_YOLO_API_KEY.
const (
	shaperEnvPrefix = "SHAPER_PROVIDER_"
	shaperEnvSuffix = "_API_KEY"
)

// Mounts derives the served mounts from the catalog: one per shaper-mode access
// that declares a prefix, deduplicated by prefix and ordered by it so the
// gateway's command line is deterministic.
func Mounts(accesses []v1alpha1.ModelAccess) []Mount {
	seen := map[string]bool{}
	mounts := make([]Mount, 0, len(accesses))
	for i := range accesses {
		access := &accesses[i]
		if access.Spec.Mode != "shaper" {
			continue
		}
		prefix := mountPrefix(access)
		if prefix == "" || prefix == "/" || seen[prefix] {
			continue
		}
		seen[prefix] = true
		required := ""
		if access.Spec.Auth != nil && len(access.Spec.Auth.RequiredEnv) > 0 {
			required = access.Spec.Auth.RequiredEnv[0]
		}
		if required == "" {
			continue
		}
		mounts = append(mounts, Mount{
			Access:     access.Name,
			Provider:   access.Spec.Provider,
			Prefix:     prefix,
			Credential: ShaperEnvVar(required),
			Required:   required,
		})
	}
	sort.Slice(mounts, func(i, j int) bool { return mounts[i].Prefix < mounts[j].Prefix })
	return mounts
}

// ShaperEnvVar maps a declared credential variable to the one the shaper reads.
func ShaperEnvVar(declared string) string {
	suffix := strings.TrimSuffix(declared, shaperEnvSuffix)
	return shaperEnvPrefix + suffix + shaperEnvSuffix
}

// mountPrefix is the path an access is exposed at: its alphabetically first
// declared endpoint path with a trailing /v1 removed, so
// http://127.0.0.1:11239/zen/go/v1 mounts at /zen/go and the anthropic-shaped
// http://127.0.0.1:11239/umans mounts at /umans.
func mountPrefix(access *v1alpha1.ModelAccess) string {
	surfaces := make([]string, 0, len(access.Spec.Endpoints))
	for surface := range access.Spec.Endpoints {
		surfaces = append(surfaces, surface)
	}
	if len(surfaces) == 0 {
		return ""
	}
	sort.Strings(surfaces)
	path := string(access.Spec.Endpoints[surfaces[0]])
	if i := strings.Index(path, "://"); i >= 0 {
		rest := path[i+3:]
		if slash := strings.IndexByte(rest, '/'); slash >= 0 {
			path = rest[slash:]
		} else {
			return ""
		}
	}
	path = strings.TrimRight(path, "/")
	return strings.TrimSuffix(path, "/v1")
}

// ChildEnvironment is the environment the shaper child runs with: the resolved
// credentials under the names the shaper reads, and nothing else. This is the
// only place a credential value is placed, which is what keeps every file, log
// and diagnostic free of them.
func ChildEnvironment(mounts []Mount, resolved map[string]string) ([]string, error) {
	environment := make([]string, 0, len(mounts))
	for _, mount := range mounts {
		value, ok := resolved[mount.Required]
		if !ok || strings.TrimSpace(value) == "" {
			return nil, fmt.Errorf("no credential resolved for %s (%s)", mount.Access, mount.Required)
		}
		environment = append(environment, mount.Credential+"="+value)
	}
	return environment, nil
}
