# TODO - NO TOUCHY look with your _eyes_ bud

This is not an actual TODO list. Consider it as much a TODO list as your Product Manager's project roadmap.

- Take bubbletea/lipgloss for a spin: Implement a form-based "code review merge" (native in Go to kick the tyres?)
- QoL improvements to prompt-flow, e.g. allow `use` without `goal` or `generate` (i.e. add one-step mode), add `footer` support for the second prompt
- Add `exec` context builder command as part of `contextManager`
- Consider tightening up the concept of "logging" / the half-baked API exposed to scripts
- Consider making commands and subcommands correctly fail upon receiving unexpected arguments or options
- ~~Consider consolidating the two prompt variants~~ Ensure the two useful prompt variants,
  (tui.registerMode, and tui.createAdvancedPrompt) are well documented, and are equivalent in
  terms of supported go-prompt features. They're pretty involved, and the former has drifted
  significantly, as it is what is used by the built-in commands - the code is also hard to follow,
  mostly AI's fault but yeah, it's a bit shit. Consolidating the _implementation_ would be nice.
  The API surfaces are fairly likely to remain mostly the same.
- Investigate/fix/implement `osm config <key> <value>` to persist config changes to disk
- Consider integrating git diff completion support into the diff `contextManager` command
- Add ability to include JS modules directly
- Implement behavior tree JS wrapper - wrap go-behavior-tree most likely (otherwise a proper event loop will be necessary)
- Implement partially-compliant fetch API backed by the Go http client
- Add support for https://code.visualstudio.com/docs/copilot/customization/prompt-files ?
- Document refiner - stalled for now (assessment: fairly low value, easy to refine manually)
- Code review splitter - prompts seem particularly LLM dependent, stalled
- Refine "goal" and "script" autodiscovery mechanisms (currently prototype status/needs attention)
- Investigate implementing Anthropic prompt library (https://platform.claude.com/docs/en/resources/prompt-library/library)
- Iterate on configuration model for better extensibility and consistency (feels undercooked)
- Evaluate potential integration with `github.com/joeycumines/MacosUseSDK`
- Enhance definitions and integration with `github.com/joeycumines/go-prompt` implementation
- Review `tview`/`tcell` support for refinement or removal
- Explore in-JS support for `bubbletea` and `lipgloss`
- Plan system-style logging (file output, tailing) - likely deferred
- Fix duplicate log lines for purged sessions etc?
