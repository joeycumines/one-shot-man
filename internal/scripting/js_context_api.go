package scripting

// JavaScript API functions for context management

// jsContextAddPath adds a path to the context manager.
func (e *Engine) jsContextAddPath(path string) error {
	return e.contextManager.AddPath(path)
}

// jsContextRemovePath removes a path from the context manager.
func (e *Engine) jsContextRemovePath(path string) error {
	return e.contextManager.RemovePath(path)
}

// jsContextListPaths returns all tracked paths.
func (e *Engine) jsContextListPaths() []string {
	return e.contextManager.ListPaths()
}

// jsContextGetPath returns information about a tracked path.
func (e *Engine) jsContextGetPath(path string) interface{} {
	contextPath, exists := e.contextManager.GetPath(path)
	if !exists {
		return nil
	}
	return contextPath
}

// jsContextRefreshPath refreshes a tracked path.
func (e *Engine) jsContextRefreshPath(path string) error {
	return e.contextManager.RefreshPath(path)
}

// jsContextToTxtar returns the context as a txtar-formatted string.
func (e *Engine) jsContextToTxtar() string {
	return e.contextManager.GetTxtarString()
}

// jsContextFromTxtar loads context from a txtar-formatted string.
func (e *Engine) jsContextFromTxtar(data string) error {
	return e.contextManager.LoadFromTxtarString(data)
}

// jsContextGetStats returns context statistics.
func (e *Engine) jsContextGetStats() map[string]interface{} {
	return e.contextManager.GetStats()
}

// jsContextFilterPaths returns paths matching a pattern.
func (e *Engine) jsContextFilterPaths(pattern string) ([]string, error) {
	return e.contextManager.FilterPaths(pattern)
}

// jsContextGetFilesByExtension returns files with a given extension.
func (e *Engine) jsContextGetFilesByExtension(ext string) []string {
	return e.contextManager.GetFilesByExtension(ext)
}
