package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// LocalSecretBinding is the local bridge between a Kubernetes Secret
// reference and the environment variable slots an access requires.
// Selector-matched to ModelAccess labels, it names the Secret key that holds
// the value in-cluster and the ordered resolver chain that produces the value
// locally (environment lookup, file read, or command execution — for example
// the 1Password CLI). Namespaced: credential material is machine- or
// tenant-local.
//
// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Namespaced
// +kubebuilder:validation:XValidation:rule="self.metadata.name.matches('^[a-z0-9]([-a-z0-9]*[a-z0-9])?$')",message="metadata.name must match ^[a-z0-9]([-a-z0-9]*[a-z0-9])?$ — emitted resource names are sanitized; the raw registry name lives in the one-shot-man/registry-name annotation"
type LocalSecretBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec LocalSecretBindingSpec `json:"spec,omitempty"`
}

// SecretKeyRef references one key of one Secret in the binding's namespace.
type SecretKeyRef struct {
	// Name is the Secret name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Key is the data key holding the credential value.
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
}

// EnvResolver reads a credential from one environment variable.
type EnvResolver struct {
	// Name is the environment variable name.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// FileResolver reads a credential from the first line of one file.
type FileResolver struct {
	// Path is the credential file path.
	// +kubebuilder:validation:MinLength=1
	Path string `json:"path"`
}

// CommandResolver produces a credential by executing one command.
type CommandResolver struct {
	// Argv is the exact argument vector, executed without a shell.
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:MinLength=1
	Argv []string `json:"argv"`

	// Timeout bounds one execution attempt (e.g. "60s"). The default must
	// outlast a human Touch ID / desktop approval on a command resolver.
	// +kubebuilder:default="60s"
	// +optional
	Timeout *metav1.Duration `json:"timeout,omitempty"`
}

// LocalSecretResolver is one step of an ordered resolver chain. Exactly one
// of Env, File, or Command is set per step; the first step that yields a
// non-empty value wins.
type LocalSecretResolver struct {
	// +optional
	Env *EnvResolver `json:"env,omitempty"`

	// +optional
	File *FileResolver `json:"file,omitempty"`

	// +optional
	Command *CommandResolver `json:"command,omitempty"`
}

// LocalSecretBindingSpec binds one environment variable slot to one Secret
// key plus an ordered local resolver chain.
//
// +kubebuilder:validation:XValidation:rule="(has(self.selector.matchLabels) && self.selector.matchLabels.size() > 0) || (has(self.selector.matchExpressions) && self.selector.matchExpressions.size() > 0)",message="spec.selector must declare a non-empty matchLabels or matchExpressions — an empty selector matches every access"
// +kubebuilder:validation:XValidation:rule="!has(self.resolvers) || self.resolvers.all(r, (has(r.env) ? 1 : 0) + (has(r.file) ? 1 : 0) + (has(r.command) ? 1 : 0) == 1)",message="each resolver step must set exactly one of env, file, or command"
type LocalSecretBindingSpec struct {
	// Selector matches the ModelAccess labels this binding serves. Empty
	// selectors match everything and are rejected.
	Selector metav1.LabelSelector `json:"selector"`

	// SecretRef names the in-cluster source of the value.
	SecretRef SecretKeyRef `json:"secretRef"`

	// EnvVar is the environment variable slot this binding covers; an access
	// is resolvable only when every requiredEnv name has a matched binding.
	// +kubebuilder:validation:Pattern=`^[A-Za-z_][A-Za-z0-9_]*$`
	EnvVar string `json:"envVar"`

	// Resolvers is the ordered local chain executed when the value is needed
	// outside a cluster; first success wins. Absent means the binding is
	// usable only with the Secret present.
	// +optional
	Resolvers []LocalSecretResolver `json:"resolvers,omitempty"`
}

// LocalSecretBindingList is the list form of LocalSecretBinding.
//
// +kubebuilder:object:root=true
type LocalSecretBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []LocalSecretBinding `json:"items"`
}
