package userk8s

// ResolutionStatus describes how completely an access's credentials resolved.
type ResolutionStatus string

const (
	// StatusResolved means every required environment slot was covered.
	StatusResolved ResolutionStatus = "resolved"
	// StatusMissingCredentials means at least one required slot had no value.
	StatusMissingCredentials ResolutionStatus = "missingCredentials"
	// StatusUnsupportedScheme means the access's scheme is modeled but not
	// launchable yet; the reason names the scheme.
	StatusUnsupportedScheme ResolutionStatus = "unsupportedScheme"
)

// Credential is one resolved environment variable slot. Values never appear
// in logs; Provenance names the resolver that produced the value without
// disclosing it.
type Credential struct {
	EnvVar     string `json:"envVar"`
	Value      string `json:"value"`
	Provenance string `json:"provenance"`
}

// Resolution is the credential state of one access.
type Resolution struct {
	Status      ResolutionStatus `json:"status"`
	Reason      string           `json:"reason,omitempty"`
	Credentials []Credential     `json:"credentials,omitempty"`
}

// Projection is what a tool adapter receives for one selection. BudgetProfile
// is the tool's named derivation rule (derivation itself is adapter code);
// Access and Surfaces name how the selection is reached, so no consumer has to
// re-derive routing facts.
type Projection struct {
	ProviderSlug  string         `json:"providerSlug"`
	ModelSlug     string         `json:"modelSlug"`
	ProviderID    string         `json:"providerId"`
	ModelID       string         `json:"modelId"`
	BudgetProfile string         `json:"budgetProfile"`
	Access        string         `json:"access,omitempty"`
	Surfaces      []string       `json:"surfaces,omitempty"`
	Settings      map[string]any `json:"settings"`
}

// BackendStatus reports what the backend loaded.
type BackendStatus struct {
	Source    string   `json:"source"`
	Artifacts []string `json:"artifacts"`
	Providers int      `json:"providers"`
	Accesses  int      `json:"accesses"`
	Models    int      `json:"models"`
	Tools     int      `json:"tools"`
	Bindings  int      `json:"bindings"`
}
