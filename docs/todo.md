# TODO - NO TOUCHY look with your _eyes_ bud

This is not an actual TODO list. Consider it as much a TODO list as your Product Manager's project roadmap.

- Built-in first-class Git synchronisation support (optional, configurable)
    - Add support to synchronise configuration including goals to a Git repository
    - A _structured_ Git repository format that can also act as a notebook (largely chronological) for prompts and notes - including multi-file prompts similar to GitHub gists (or even using GitHub gists as a backend?) sounds good to me tbh
    - This requires some proper designing - don't want to half-bake this one (I have a clear use case for it personally, so I will probably just align with that)
- Add `hot-<shortname>` aliases to copy snippets, activating them based on the name of the... mode? Seems reasonable - that'd cover the fairly coupled/integrated variants of custom scripts, and the builtins which all use it. Integrated nicely, it could be configurable, and there could be a command to output the embedded ones. Examples of intended use case include situational follow-up prompts, e.g. "critical next steps: prove the issue exists and that it is fixed" prompt. Will be important to disclaimer that they are subject to arbitrary change as I tweak it. Maybe a warning if you use it w/o overriding it? Would need another config option to disable the warning lol.
    - I'd personally use this for variants of agentic session kickoff prompts, which I tend to use when I have a populated blueprint.json
- Fix behavior when you use `copy` to copy context using `osm:ctxutil`/`contextManager` - all the current built-in scripts use this implementation in some capacity. It is desirable to support "refreshing" on demand in a just-in-time fashion, just prior to copy. To pick up new files added to a directory, specifically.
- Goal list should probably also show SUMMARIZED params. Might need to consolidate custom list logic. Provide means to avoid the custom logic? Maybe just expose or ensure it is possible to get the context manager list? Should be fine to just require and get the thing, I thought, but maybe I made it a factory function? The state is on the TUIManager or whatever the context type was hmmmmmm.
- `osm goal morale-improver` command should print after `copy`: [Hint: Try a follow-up]       Double the length, and include key specifics from the context that are likely to otherwise fail to be identified - consider this the only context, beyond the codebase.
  OR: Double the length, and include key specifics from the context that are likely to otherwise fail to be identified - consider this the only context, beyond the codebase. **Explicitly** demand use of the todo tool to track exhaustively all known requirements. Reference tools like "#todo" - literal #todo without any quotes (") or backticks (`) etc. Do not use terms that would lead the AI to thinking it is AI - worse outcomes that way.
- ~~Consider consolidating the two prompt variants~~ Ensure the two useful prompt variants,
  (tui.registerMode, and tui.createAdvancedPrompt) are well documented, and are equivalent in
  terms of supported go-prompt features. They're pretty involved, and the former has drifted
  significantly, as it is what is used by the built-in commands - the code is also hard to follow,
  mostly AI's fault but yeah, it's a bit shit. Consolidating the _implementation_ would be nice.
  The API surfaces are fairly likely to remain mostly the same.
    - Rename the "advanced" prompt to something less dumb - it is actually the _least_ advanced of the two go-prompt wrappers
        - It is more like, it is a lower level API - it doesn't wire up to the session state etc
- Add option to the osm:ctxutil add context command to add files from a diff (`git diff <what> --name-only`)
- Expose the Go `flag` package as a JS module `osm:flag` for script authors to use
    - Probably need to take a look at how arguments are passed down to the `osm script` command, as well
    - _COULD_ Leverage the "lazy init" pattern I originally intended for declarative-style scripts - buuuuut I've since moved to more imperative style ones, so perhaps not
- Add support for completion for arguments for REPL commands within `osm:ctxutil` module using the `osm:flag` module
    - N.B. Unlike the other builtins, a portion of `osm:ctxutil` is partially implemented in JavaScript, as I ported it from a prototype script
    - This is basically extending support to subcommands - this item is just a quality of life improvement
- QoL improvements to prompt-flow, e.g. allow `use` without `goal` or `generate` (i.e. add one-step mode), add `footer` support for the second prompt
- Add `exec` context builder command as part of `contextManager`
    - To clarify, this is intended to mean "add scaffolding to the `osm:ctxutil` module to allow executing arbitrary shell commands and capturing their output as context, inclusive of providing means to aid wiring up the REPL commands to interact with it"
    - Essentially a generalization of the existing `diff` command - consider what would be necessary to retain the EXACT existing behavior for the `diff` command, when porting it to use the same implementation under the hood
    - INCLUDE completion support (see the below item - might be tricky / seems like it might require piggybacking off of shell completion logic? Alternatives exist though. Explore all of them.)
- Consider integrating git diff completion support into the diff `contextManager` command
    - Unlike the generalised `exec` command, this is specifically for the existing `diff` command - should be feasible to implement specifically. Probably want to use Go directly. Honestly, could remove the dependency on native Git. Might regret, the sole viable Go implementation is a pain to work with.
- Command and option tightening and validation across the board
    - Need to revalidate how the logging API is wired up - the option for log level and output path was added to `osm script`, but should be configurable for _all_ script-like commands, that exercise the scripting engine, could probably use some additional means to configure them, and probably need refactor to wire up more sane (I was using it for debugging / as a means to implement integration tests w/o depending on scraping PTYs - I didn't properly validate it)
    - Consider making commands and subcommands correctly fail upon receiving unexpected arguments or options
- Investigate/fix/implement `osm config <key> <value>` to persist config changes to disk
- Add ability to include JS modules directly
    - Need to pick the module resolution / loader strategy. Need to revalidate my understanding of the current standards in the JS ecosystem. Almost certainly want to pick a "sane" subset, tailored for the specific intended use cases. Standards compliance is ideal, however-want to avoid ruling out future interoperability.
- Implement partially-compliant fetch API backed by the Go http client
    - If no streaming is required, this is actually quite straightforward: https://gist.github.com/joeycumines/c7da3dbb786428dcaf45f5884cd99798
    - There _is_ nuance in the allowed headers and wiring up of options such as host (nuance not reflected in that gist), but still, fairly straightforward
    - Streaming support is... er, involved.
        - I've implemented multiple variants of application-layer shims, in the past, but I'd likely want to expose an in-process gRPC channel between client (JS) and server (Go) to properly support it
        - An alternative, even-more-cooked approach: Implement a reactor-style HTTP client in Go, backed by the (as-yet unpublished) `github.com/joeycumines/go-eventloop` module, and expose a _compliant_ fetch API on top of that
            - Honestly supporting a full-on HTTP client sounds, well, ridiculous - this would be _fun_ but probably not _sane_
        - Explore alternative: An eventloop-native in-process gRPC channel implementation, that exposes a Goja JS API _and_ a Go-compatible API <--- This is the sexiest option, but various challenges exist. Seriously attractive to be able to avoid serializing messages at all, though - this can support by far the lowest overhead e.g. allocation implementation.
- Evaluate potential integration with `github.com/joeycumines/MacosUseSDK`
    - The in-process gRPC channel implementation idea would allow for exposing gRPC APIs to JS code - strongly consider implementing that then using a gRPC proxy mechanism to expose MacOSUseSDK functionality to JS code
- Add support for https://code.visualstudio.com/docs/copilot/customization/prompt-files ?
- Code review splitter - prompts seem particularly LLM dependent, stalled
    - This would be far easier as a proper workflow engine lol
- Refine "goal" and "script" autodiscovery mechanisms (currently prototype status/needs attention)
- Investigate implementing Anthropic prompt library (https://platform.claude.com/docs/en/resources/prompt-library/library)
- Iterate on configuration model for better extensibility and consistency (feels undercooked)
- Enhance definitions and integration with `github.com/joeycumines/go-prompt` implementation
- Review `tview`/`tcell` support for refinement or removal (probably just leave it as-is for now, remove eventually - bubbletea is the winner of this one)
- Plan system-style logging (file output, tailing) - likely deferred
- Fix duplicate log lines for purged sessions etc?
