**This is `./AGENTS.md`. Your equivalent is `./WIP.md` which must track your plan. KEEP THE PLAN UP TO DATE AS YOU EVOLVE THE CODE. DEVIATIONS TO THE PLAN MUST BE LOGGED WITHIN THE PLAN. THE PLAN MUST BE REASSESSED HOLISTICALLY AFTER ANY CHANGE OF ANY SIZE.**

**ON TOOLS: Use `config.mk` to create custom targets, and the `make` tool to run targets. ALWAYS use custom targets that *limit* the amount of output you receive. For example, piping through tail, with FEW lines output. Prior to tail, pipe to tee. The file ./build.log in the root of the project is gitignored, so use that. That way you can *search* the output. To be clear, timing dependent tests are BANNED. As are those that take too long to run. Testing retries, for example, MUST be done in a way that supports avoiding running afoul of those CRITICAL rules. Abide. OBEY.**

**ON CHECKS: All checks MUST pass AT ALL TIMES. DO NOT PROCEED IF CHECKS ARE FAILING. FIX THE CHECKS IMMEDIATELY. You are SOLELY responsible for ALL checks and ALL behavior. Declaring "it was broken before" is NOT an excuse. FIX IT.**

DO NOT MODIFY THIS FILE. This is MY file. You are ONLY allowed to READ this file.

Current focus: `osm super-document` UI and command behavior fixes.

**N.B. The designs are AUTHORITATIVE but this document takes precedence over them where conflicts exist.**

**ALL the below are MANDATORY changes that MUST be completed IMMEDIATELY.**

- A review button is required
- The `--repl` flag needs to be renamed to `--shell` to match the naming (repl was in use but shell was decided on as being less confusing)
- The scrolling behavior of the document list page should _include_ the buttons - those buttons are ONLY USEFUL for mouse users, which have access to easy scrolling. The height of the button stack is problematic in small terminal windows.
- The scrollbar in the textarea for document add/edit _should remain_ to support a CAPPED (large/long/tall) textedit, but the textedit MUST be allowed to grow up to that size - for which reason, the _entire_ edit document page (INCLUDING buttons, EXCEPT the hints down the bottom and title header) should _scroll_, with a scrollbar, much like the main document list page
- The only page which correctly handles mouse input calculations when the viewport is smaller than the content is the main document list view - the edit document view etc still have BROKEN mouse input calculations
    - **THIS NEEDS TO WORK WITH SCROLLING! Testing also obviously needs to support scrolling the viewport.**
- There doesn't need to be a "title edit only" mode to the edit document view - just use the normal edit document view for both title and content editing, and auto-focus the appropriate input field based on what was clicked to open the view (title or content - note: there's only one hotkey to edit, and it should probably go to the content. **Also, the "edit document" button is redundant, the button should be removed (it can't even be used with keyboard lol)**)
- The view document button / mode is also redundant - a "full screen" edit view would be much better, accessible from within the edit document view (e.g. a button to "expand" to full screen, or a hotkey). The view document mode should be removed, along with button and shortcuts.
- nil pointer dereference on CLICKING the **submit** button without having first highlighted it with tab (CLICKING IT WORKS EVEN WITH MOUSE if and only if you first highlight it with tab - otherwise it crashes)
- The cursor / line highlight is just a black void - idk if the text is black on black or what but it's impossible to see where the cursor is or what the content is
    - This is still poor - **PROPER STYLE CONFIGURATION WIRING IS REQUIRED**
- Remove the dumbass Generate button as it is POINTLESS (IT DOES NOTHING) - DO NOT use / remove it, follow the designs in super_document_script.js
- The buttons on the document list page STILL don't REMOTELY match the designs - the buttons which exist, how they are laid out, EVERYTHING NEEDS TO BE AS DESIGNED IN THE ASCII ART AT THE TOP OF super_document_script.js
    - The designs are AUTHORITATIVE although this document takes precedence over them where conflicts exist
- The terminal state resetting behavior is BROKEN - the terminal mode is now munted on exit e.g. via `q`
- The `doc-list` command is silly - although the `osm:ctxutil` contextManager commands _are_ and MUST remain standard behavior wise, `list` is and SHOULD list any OTHER context, as well, alongside the core set of context i.e. diffs files etc that are supported by most commands in this project
    - TO BE CLEAR, this was MEANT to be consolidating `doc-list` into JUST `list`
- Fix the event API - implement an internal helper package (for now) to support proper encoding of ALL event types, retaining the type of the event, then replace various switch cases littered throughout the codebase (there are FAR too many pointless switch cases - not that they are bad in and of themselves, but there are many that are literally like "yes this looks like a key press" - bubbletea has metadata for them, but it might take a PR to expose a parser - GOOD HIGH QUALITY REDESIGNS ONLY)

**ALL the above are MANDATORY changes that MUST be completed IMMEDIATELY.**

**ON TOOLS: Use `config.mk` to create custom targets, and the `make` tool to run targets. ALWAYS use custom targets that *limit* the amount of output you receive. For example, piping through tail, with FEW lines output. Prior to tail, pipe to tee. The file ./build.log in the root of the project is gitignored, so use that. That way you can *search* the output. To be clear, timing dependent tests are BANNED. As are those that take too long to run. Testing retries, for example, MUST be done in a way that supports avoiding running afoul of those CRITICAL rules. Abide. OBEY.**

**ON CHECKS: All checks MUST pass AT ALL TIMES. DO NOT PROCEED IF CHECKS ARE FAILING. FIX THE CHECKS IMMEDIATELY. You are SOLELY responsible for ALL checks and ALL behavior. Declaring "it was broken before" is NOT an excuse. FIX IT.**
