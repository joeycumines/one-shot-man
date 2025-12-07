# TODO - NOT for AI, for people only

- Fix duplicate log lines for purged sessions etc?
- Consider tightening up the concept of "logging" / the half-baked API exposed to scripts
- Consider making commands and subcommands correctly fail upon receiving unexpected arguments or options
- Consider persisting scripts as modes in the session state, to allow quick swapping between them - actually make modes useful
- Add ability to include JS modules directly
- Implement behavior tree JS wrapper - wrap go-behavior-tree most likely (otherwise a proper event loop will be necessary)
- Implement partially-compliant fetch API backed by the Go http client
- Add support for https://code.visualstudio.com/docs/copilot/customization/prompt-files ?
- Document refiner - stalled for now (assessment: fairly low value, easy to refine manually)
- Code review splitter - prompts seem particularly LLM dependent, stalled
