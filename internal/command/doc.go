// Package command implements the CLI commands registered by osm.
// Each command implements the Command interface and is registered
// in the command registry during startup. Commands range from
// interactive prompt builders (code-review, prompt-flow) to
// utility commands (config, session, sync) and the embedded
// JavaScript scripting engine (script).
package command
