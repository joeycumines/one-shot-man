package command

import (
	"strings"
	"testing"

	"github.com/joeycumines/one-shot-man/internal/config"
)

func TestUserK8sConfigResolvesEnvironmentOverrides(t *testing.T) {
	cfg := config.NewConfig()
	cfg.SetGlobalOption("userk8s.source", "files")
	cfg.SetGlobalOption("userk8s.artifacts", "/from/config.yaml")
	cfg.SetGlobalOption("userk8s.namespace", "from-config")

	t.Setenv("OSM_USERK8S_ARTIFACTS", "/from/env.yaml:/also/env.yaml")
	t.Setenv("OSM_USERK8S_NAMESPACE", "from-env")

	options := userK8sConfig(cfg)
	if options.Source != "files" {
		t.Fatalf("source: got %q, want the configured files backend", options.Source)
	}
	if len(options.Artifacts) != 2 || options.Artifacts[0] != "/from/env.yaml" || options.Artifacts[1] != "/also/env.yaml" {
		t.Fatalf("artifacts: got %v, want both environment paths", options.Artifacts)
	}
	if options.Namespace != "from-env" {
		t.Fatalf("namespace: got %q, want the environment value", options.Namespace)
	}

	if len(userK8sOpts(nil)) != 0 {
		t.Fatal("userK8sOpts(nil): want no options")
	}
	if len(userK8sOpts(cfg)) != 1 {
		t.Fatalf("userK8sOpts: got %d options, want exactly one", len(userK8sOpts(cfg)))
	}
	if !strings.Contains(userK8sConfig(config.NewConfig()).Source, "files") {
		t.Fatalf("userK8sConfig: want the files default, got %q", userK8sConfig(config.NewConfig()).Source)
	}
}
