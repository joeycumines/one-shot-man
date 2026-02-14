package command

import (
	"github.com/joeycumines/one-shot-man/internal/config"
	"github.com/joeycumines/one-shot-man/internal/scripting"
)

// configHotSnippetsForJS converts config.HotSnippets to a JS-compatible
// slice of maps. Returns nil when cfg is nil or contains no snippets,
// which Goja will expose as JavaScript undefined.
func configHotSnippetsForJS(cfg *config.Config) []map[string]interface{} {
	if cfg == nil || len(cfg.HotSnippets) == 0 {
		return nil
	}
	result := make([]map[string]interface{}, len(cfg.HotSnippets))
	for i, s := range cfg.HotSnippets {
		m := map[string]interface{}{
			"name": s.Name,
			"text": s.Text,
		}
		if s.Description != "" {
			m["description"] = s.Description
		}
		result[i] = m
	}
	return result
}

// injectConfigHotSnippets sets the CONFIG_HOT_SNIPPETS global on the engine
// if cfg contains any hot-snippets, allowing JS scripts to pass them to
// contextManager. Safe to call with nil cfg.
func injectConfigHotSnippets(engine *scripting.Engine, cfg *config.Config) {
	snippets := configHotSnippetsForJS(cfg)
	if snippets != nil {
		engine.SetGlobal("CONFIG_HOT_SNIPPETS", snippets)
	}
}
