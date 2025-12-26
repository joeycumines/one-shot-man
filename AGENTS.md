**This is `./AGENTS.md`. Your equivalent is `./WIP.md` which must track your plan. KEEP THE PLAN UP TO DATE AS YOU EVOLVE THE CODE. DEVIATIONS TO THE PLAN MUST BE LOGGED WITHIN THE PLAN. THE PLAN MUST BE REASSESSED HOLISTICALLY AFTER ANY CHANGE OF ANY SIZE.**

**ON TOOLS: Use `config.mk` to create custom targets, and the `make` tool to run targets. ALWAYS use custom targets that *limit* the amount of output you receive. For example, piping through tail, with FEW lines output. Prior to tail, pipe to tee. The file ./build.log in the root of the project is gitignored, so use that. That way you can *search* the output. To be clear, timing dependent tests are BANNED. As are those that take too long to run. Testing retries, for example, MUST be done in a way that supports avoiding running afoul of those CRITICAL rules. Abide. OBEY.**

**ON CHECKS: All checks MUST pass AT ALL TIMES. DO NOT PROCEED IF CHECKS ARE FAILING. FIX THE CHECKS IMMEDIATELY. You are SOLELY responsible for ALL checks and ALL behavior. Declaring "it was broken before" is NOT an excuse. FIX IT.**

DO NOT MODIFY THIS FILE. This is MY file. You are ONLY allowed to READ this file.

Current focus: `osm super-document` UI and command behavior fixes.

**N.B. The designs are AUTHORITATIVE but this document takes precedence over them where conflicts exist.**

**ALL the below are MANDATORY changes that MUST be completed IMMEDIATELY.**

- There is a bug in SCENARIO B if the text of the document in the list wraps, it breaks the zone detection for the delete button.
- The scrollbar in the textarea for document add/edit _should remain_ to support a CAPPED (large/long/tall) textedit, but the textedit MUST be allowed to grow up to that size - for which reason, the _entire_ edit document page (INCLUDING buttons, EXCEPT the hints down the bottom and title header) should _scroll_, with a scrollbar, much like the main document list page
- The cursor / line highlight is just a black void - idk if the text is black on black or what but it's impossible to see where the cursor is or what the content is
    - It doesn't seem to obey any of the styles? It might just be an oversight. Idk. Like, text is always black on the buttons, too. Maybe ok? Idk how it is mean to work lol.
- The buttons on the document list page STILL don't REMOTELY match the designs - the buttons which exist, how they are laid out, EVERYTHING NEEDS TO BE AS DESIGNED IN THE ASCII ART AT THE TOP OF super_document_script.js
    - The designs are AUTHORITATIVE although this document takes precedence over them where conflicts exist
    - Still need to fix the LAYOUT in SCENARIO B
- Document add/edit page: The navigation in the content textarea is terrible - clicking and moving the cursor should really work, as should navigating via page up and down, and all the other standard keys (this is likely related to "fix the event API")
- Document list page: It isn't possible to page down to the TOP (bottom ok) of the _viewport_ (the scrollbar seems to wired up to the document list itself, or maybe it doesn't handle the fractional component properly?)
- Document list page: It isn't possible to navigate to the TOP (bottom ok) of the page using the arrow keys - it should be possible to de-highlight everything and "get to the top" (there's nothing but the document count up there, but otherwise you can't reach the top)

**ALL the above are MANDATORY changes that MUST be completed IMMEDIATELY.**

**ON TOOLS: Use `config.mk` to create custom targets, and the `make` tool to run targets. ALWAYS use custom targets that *limit* the amount of output you receive. For example, piping through tail, with FEW lines output. Prior to tail, pipe to tee. The file ./build.log in the root of the project is gitignored, so use that. That way you can *search* the output. To be clear, timing dependent tests are BANNED. As are those that take too long to run. Testing retries, for example, MUST be done in a way that supports avoiding running afoul of those CRITICAL rules. Abide. OBEY.**

**ON CHECKS: All checks MUST pass AT ALL TIMES. DO NOT PROCEED IF CHECKS ARE FAILING. FIX THE CHECKS IMMEDIATELY. You are SOLELY responsible for ALL checks and ALL behavior. Declaring "it was broken before" is NOT an excuse. FIX IT.**
