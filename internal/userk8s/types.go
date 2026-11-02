package userk8s

// ResolutionStatus describes how completely a binding's credentials resolved.
type ResolutionStatus string

const (
	// StatusResolved means every required environment slot was covered.
	StatusResolved ResolutionStatus = "resolved"
	// StatusMissingCredentials means the binding's resolver chain produced no
	// value.
	StatusMissingCredentials ResolutionStatus = "missingCredentials"
	// StatusUnsupportedScheme means the binding's scheme is modeled but not
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

// Resolution is the credential state of one LocalSecretBinding.
type Resolution struct {
	Status      ResolutionStatus `json:"status"`
	Reason      string           `json:"reason,omitempty"`
	Credentials []Credential     `json:"credentials,omitempty"`
}

// Projection is what a tool adapter receives for one selection. BudgetProfile
// is the tool's named derivation rule (derivation itself is adapter code).
type Projection struct {
	ProviderSlug string `json:"providerSlug"`
	ModelSlug    string `json:"modelSlug"`
	ProviderID   string `json:"providerId"`
	ModelID      string `json:"modelId"`
	// ProviderModelID is the id the PROVIDER calls this model. It differs from
	// ModelID (the catalog-wide unique registry key) exactly when the key
	// carries a disambiguating namespace the provider does not use. Empty means
	// the two agree.
	ProviderModelID string         `json:"providerModelId,omitempty"`
	BudgetProfile   string         `json:"budgetProfile"`
	Settings        map[string]any `json:"settings"`
}

// BackendStatus reports what the backend loaded across all eight catalog kinds.
type BackendStatus struct {
	Source     string   `json:"source"`
	Artifacts  []string `json:"artifacts"`
	Providers  int      `json:"providers"`
	Accesses   int      `json:"accesses"`
	Models     int      `json:"models"`
	Servings   int      `json:"servings"`
	Selections int      `json:"selections"`
	Routes     int      `json:"routes"`
	Tools      int      `json:"tools"`
	Bindings   int      `json:"bindings"`
}
