package command

import (
	userk8smod "github.com/joeycumines/one-shot-man/internal/builtin/userk8s"
	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
)

// userK8sOpts resolves the userk8s.* configuration into the engine option that
// supplies the osm:userk8s module. Values are read through the schema
// resolver, so the documented EnvVar mappings (OSM_USERK8S_*) override the
// configuration file exactly as they do for every other option.
func userK8sOpts(cfg *config.Config) []scripting.EngineOption {
	if cfg == nil {
		return nil
	}
	return []scripting.EngineOption{scripting.WithUserK8sOptions(userK8sConfig(cfg))}
}

// userK8sConfig resolves the userk8s.* options for one configuration.
func userK8sConfig(cfg *config.Config) userk8smod.Options {
	values := make(map[string]string)
	for _, resolved := range config.DefaultSchema().ResolveAll(cfg) {
		values[resolved.Key] = resolved.Value
	}

	options := userk8smod.Options{
		Source:     values["userk8s.source"],
		Namespace:  values["userk8s.namespace"],
		Kubeconfig: values["userk8s.kubeconfig"],
		Context:    values["userk8s.context"],
	}
	if options.Source == "" {
		options.Source = userk8smod.SourceFiles
	}
	if artifacts := values["userk8s.artifacts"]; artifacts != "" {
		options.Artifacts = parsePathList(artifacts)
	}
	return options
}
