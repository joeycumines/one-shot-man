# TODO - NO TOUCHY look with your _eyes_ bud

This is not an actual TODO list. Consider it as much a TODO list as your Product Manager's project roadmap.

- Add `hot-<shortname>` aliases to copy snippets, activating them based on the name of the... mode? Seems reasonable - that'd cover the fairly coupled/integrated variants of custom scripts, and the builtins which all use it. Integrated nicely, it could be configurable, and there could be a command to output the embedded ones. Examples of intended use case include situational follow-up prompts, e.g. "critical next steps: prove the issue exists and that it is fixed" prompt. Will be important to disclaimer that they are subject to arbitrary change as I tweak it. Maybe a warning if you use it w/o overriding it? Would need another config option to disable the warning lol.
- Fix behavior when you use `copy` to copy context using `osm:ctxutil`/`contextManager` - all the current built-in scripts use this implementation in some capacity. It is desirable to support "refreshing" on demand in a just-in-time fashion, just prior to copy. To pick up new files added to a directory, specifically.
- Fix string -> bubbletea key mapping conversion not covering all the cases - make PR to bubbletea is dumb it doesn't expose an API for it
- Take bubbletea/lipgloss for a spin: Implement a form-based "code review merge" (native in Go to kick the tyres?)
- Add summarization of params to goal banner
- Goal list should probably also show summarized params. Might need to consolidate custom list logic. Provide means to avoid the custom logic? Maybe just expose or ensure it is possible to get the context manager list? Should be fine to just require and get the thing, I thought, but maybe I made it a factory function? The state is on the TUIManager or whatever the context type was hmmmmmm.
- Goal command should print after `copy`: [Hint: Try a follow-up]       Double the length, and include key specifics from the context that are likely to otherwise fail to be identified - consider this the only context, beyond the codebase.
  OR: Double the length, and include key specifics from the context that are likely to otherwise fail to be identified - consider this the only context, beyond the codebase. **Explicitly** demand use of the todo tool to track exhaustively all known requirements. Do not use terms that would lead the AI to thinking it is AI - worse outcomes that way.
- Rename the "advanced" prompt to something less dumb - it is actually the _least_ advanced of the two go-prompt wrappers
- Add option to the osm:ctxutil add context command to add files from a diff (`git diff <what> --name-only`)
- Expose the Go `flag` package as a JS module `osm:flag` for script authors to use
- Add support for completion for arguments for REPL commands within `osm:ctxutil` module using the `osm:flag` module
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
- Code review splitter - prompts seem particularly LLM dependent, stalled
- Refine "goal" and "script" autodiscovery mechanisms (currently prototype status/needs attention)
- Investigate implementing Anthropic prompt library (https://platform.claude.com/docs/en/resources/prompt-library/library)
- Iterate on configuration model for better extensibility and consistency (feels undercooked)
- Evaluate potential integration with `github.com/joeycumines/MacosUseSDK`
- Enhance definitions and integration with `github.com/joeycumines/go-prompt` implementation
- Review `tview`/`tcell` support for refinement or removal
- Plan system-style logging (file output, tailing) - likely deferred
- Fix duplicate log lines for purged sessions etc?
