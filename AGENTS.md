**This is `./AGENTS.md`. Your equivalent is `./WIP.md` which must track your plan. KEEP THE PLAN UP TO DATE AS YOU EVOLVE THE CODE. DEVIATIONS TO THE PLAN MUST BE LOGGED WITHIN THE PLAN. THE PLAN MUST BE REASSESSED HOLISTICALLY AFTER ANY CHANGE OF ANY SIZE.**

**ON TOOLS: Use `config.mk` to create custom targets, and the `make` tool to run targets. ALWAYS use custom targets that *limit* the amount of output you receive. For example, piping through tail, with FEW lines output. Prior to tail, pipe to tee. The file ./build.log in the root of the project is gitignored, so use that. That way you can *search* the output. To be clear, timing dependent tests are BANNED. As are those that take too long to run. Testing retries, for example, MUST be done in a way that supports avoiding running afoul of those CRITICAL rules. Abide. OBEY.**

**ON CHECKS: All checks MUST pass AT ALL TIMES. DO NOT PROCEED IF CHECKS ARE FAILING. FIX THE CHECKS IMMEDIATELY. You are SOLELY responsible for ALL checks and ALL behavior. Declaring "it was broken before" is NOT an excuse. FIX IT.**

DO NOT MODIFY THIS FILE. This is MY file. You are ONLY allowed to READ this file.

- nil pointer dereference on CLICKING the **submit** button without having first highlighted it with tab (CLICKING IT WORKS EVEN WITH MOUSE if and only if you first highlight it with tab - otherwise it crashes)
- The cursor / line highlight is just a black void - idk if the text is black on black or what but it's impossible to see where the cursor is or what the content is
- The fucking buttons STILL don't REMOTELY match the designs (the ascii art and description of behavior is authorative, NOT your code): There's a useless "Rename" button, the style and layout of the buttons don't align, nor do the text.
- Fix the event API - implement proper encoding of ALL event types retaining the type of the event, then replace the dumbass switch cases that just forward the event (this is a follow on from the backtab one - backtab does now work, the implementation is just pathetic)
- The idea of a Generate button is POINTLESS (IT DOES NOTHING) - DO NOT use / remove it, follow the designs in super_document_script.js
- The `--repl` flag needs to be renamed to `--shell` to match the naming (repl was in use but shell was decided on as being less confusing)
- The `doc-list` command is silly - although the `osm:ctxutil` contextManager commands _are_ and MUST remain standard behavior wise, `list` is and SHOULD list any OTHER context, as well, alongside the core set of context i.e. diffs files etc that are supported by most commands in this project
- You have failed to implement scrolling support of any kind - the designs for the main page / document list view EXPLICITLY indicate scrolling behavior - this is MANDATORY
- If the window is smaller than the content, clicking location is miscalculated (e.g. clicking on the buttons result in clicking the documents - despite the fact that the top of the page is cut off, i.e. the clicks are registered higher visually than they actually are)
    - An attempted fix was made, and it doesn't necessarily seem to have made things worse, but it is still an issue
    - **THIS NEEDS TO WORK WITH SCROLLING! Testing obviously needs to support scrolling the viewport.**

**Execution Order**:

This ordering is strict. Deviating will cause merge conflicts or invalid tests.
These plans are located within `.github/prompts/`. They are **STRICTLY** to be executed in the following order.

8. **`plan-fixCursorHighlight.prompt.md`** (Visual Fix)
10. **`plan-removeSuperDocumentGenerateUi.prompt.md`** (Cleanup)
11. **`plan-renameReplToShell.prompt.md`** (Cleanup/Rename)
15. **`plan-docListIncludeOtherContexts.prompt.md`** (Command Fix)

**ON TOOLS: Use `config.mk` to create custom targets, and the `make` tool to run targets. ALWAYS use custom targets that *limit* the amount of output you receive. For example, piping through tail, with FEW lines output. Prior to tail, pipe to tee. The file ./build.log in the root of the project is gitignored, so use that. That way you can *search* the output. To be clear, timing dependent tests are BANNED. As are those that take too long to run. Testing retries, for example, MUST be done in a way that supports avoiding running afoul of those CRITICAL rules. Abide. OBEY.**

**ON CHECKS: All checks MUST pass AT ALL TIMES. DO NOT PROCEED IF CHECKS ARE FAILING. FIX THE CHECKS IMMEDIATELY. You are SOLELY responsible for ALL checks and ALL behavior. Declaring "it was broken before" is NOT an excuse. FIX IT.**
