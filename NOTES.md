# Notes for AI that APPEARS TO BE MISSING THE FUCKING POINT

## The instructions

> <directive>
> **Implement fully-fledged, well-architected, _production ready_ (exhaustively unit and integration tested) support for both `github.com/charmbracelet/bubbletea` and `github.com/charmbracelet/lipgloss`, and use them in a new built-in command (i.e. alongside `code-review` etc) named `super-document`: An easy to use, mouse-integration-supporting TUI, which is somewhat like a form where you can attach documents, and add or remove documents at will. The documents must have contents that is able to be updated via TUI, or be specified via file (again in the TUI), and must support (for example) one-click removal for a given document document. There must be an easy single-click button for `copy`, too. That said, keyboard navigation and functionality must also be supported.**
> </directive>
> 
> <attachments>
> You don't need to spend any time thinking about prompting, just follow my specific instructions (re: building the final copyable prompt)
> 
> 1. Lead with the following
> ```
> Implement a super-document that is INTERNALLY CONSISTENT based on a quorum or carefully-weighed analysis of the attached documents.
> 
> Your goal is to coalesce AS MUCH INFORMATION AS POSSIBLE, in as raw form as possible, while **preserving internal consistency**.
> 
> All information or content that you DON'T manage to coalesce will be discarded, making it critical that you output as much as the requirement of internal consistency allows.
> ```
> 
> 2. Attach context per the established pattern (or nothing) - use the existing context manager (actually, you'll need to implement the REPL-style support too - like "drop into the shell" as an option, hmmmm - this is very nice to have, but lesser priority than the lipgloss/bubbletea based interface)
> 
> 3. Attach the documents, sourcing the text via one of the two noted ways (the state will be managed separately from but similarly too the context manager pattern), ensuring you "fence" the contents in markdown code blocks _safely_, and formatting it like:
> ````
> Document 1:
> ```
> {{DOCUMENT 1 CONTENTS}}
> ```
> 
> Document 2:
> ```
> {{DOCUMENT 2 CONTENTS}}
> ```
> ````
> etc
> </attachments>
> 
> <guidance>
> To start with, you need to understand the why: This is a tool to merge documents. A one-shot facilitating time saving mechanism. The integration and API is equally important as the tool. This is an _extensible_ implementation, and a core guiding principle - north star, if you will - is that ALL functionality MUST be exposed to the JavaScript scripting environment, preferably as `require`-able modules that are implemented in native Go (naturally, that is strictly necessary, in this case).
> 
> The implementation must follow the established patterns. These patterns are well-established and exist for very good reasons. There are too many to list, so a critical part of your task will be _discovery_ and _planning_.
> 
> Here's just a few relevant patterns:
> 
> 1. ALL functionality MUST be exposed to JavaScript
> 2. Global state is BANNED
> 3. Non-deterministic testing and timing-dependent tests are BANNED
> 4. Modular packages and modules are preferred - you MUST align with the structure of the actual Go modules, and expose INTELLIGENTLY-DESIGNED, deeply-considered forward-thinking API surfaces - you _don't_ need to implement _everything_ but there **MUST** be a clear pathway to "full support", which you MUST document
> 5. `./internal/builtin/tview` is a prototype for a similar tool, investigate it and learn from it - it is exposed as a TUI/REPL command as part of `prompt-flow` (what it does isnt relevant, im just telling you where to look)
> 6. **To be clear, I expect to be able to `require('osm:bubbletea')` and `require('osm:lipgloss')`, and build an equivalent / similar app using a variety of components (not just the ones you used) ENTIRELY WITHIN JAVASCRIPT after I merge your PR** - make the magic happen DO NOT let it just be a facade. Your past attempts have been PATHETIC so step up your effort and commitment - show me you aren't incompetent lazy or useless, that you can _deliver on what you claim to be able to - IN FULL_
> </guidance>

## My previous update

># Implementation & Remediation Plan: osm:bubbletea, osm:lipgloss, super-document
>
>**STATUS:** CRITICALLY INCOMPLETE.
>**DIRECTIVE:** Execute full remediation and implementation in ONE session.
>**CONSTRAINT:** Strict adherence to original requirements. No fabrication.
>
>## 1. SCOPE & MANDATE
>
>Implement fully-fledged, production-ready support for `github.com/charmbracelet/bubbletea` and `github.com/charmbracelet/lipgloss`. Expose as `osm:bubbletea` and `osm:lipgloss`. Implement `super-document` command.
>
>### 1.1 Core Requirements (Per Original Prompt)
>> Implement a super-document that is INTERNALLY CONSISTENT based on a quorum or carefully-weighed analysis of the attached documents.
>>
>> Your goal is to coalesce AS MUCH INFORMATION AS POSSIBLE, in as raw form as possible, while **preserving internal consistency**.
>>
>> All information or content that you DON'T manage to coalesce will be discarded, making it critical that you output as much as the requirement of internal consistency allows.
>>
>> **Implement fully-fledged, well-architected, _production ready_ (exhaustively unit and integration tested) support for both `github.com/charmbracelet/bubbletea` and `github.com/charmbracelet/lipgloss`, and use them in a new built-in command (i.e. alongside `code-review` etc) named `super-document`: An easy to use, mouse-integration-supporting TUI, which is somewhat like a form where you can attach documents, and add or remove documents at will. The documents must have contents that is able to be updated via TUI, or be specified via file (again in the TUI), and must support (for example) one-click removal for a given document document. There must be an easy single-click button for `copy`, too. That said, keyboard navigation and functionality must also be supported.**
>
>### 1.2 Integration Standards
>1.  **Exposition:** `require('osm:bubbletea')` and `require('osm:lipgloss')` must function identically to native Go modules.
>2.  **State:** Global state is BANNED.
>3.  **Testing:** Non-deterministic/timing-dependent tests are BANNED.
>4.  **Architecture:** Modular packages aligned with Go module structure.
>5.  **Capability:** Users must be able to build the `super-document` tool entirely within JavaScript using the exposed bindings.
>
>---
>
> <section snipped - the issue is youve completely missed the point of what the expected behavior is>
>
>---
>
>## 4. SUPER-DOCUMENT IMPLEMENTATION
>
>**Objective:** Combine `osm:bubbletea` and `osm:lipgloss` into the `super-document` TUI tool.
>
>### 4.1 Functional Logic
>1.  **Document CRUD:** Implement Add, Remove, Edit, View, List, Clear.
>2.  **Content Loading:** Support `load <path>` and `add --file <path>`.
>3.  **Context Integration:** Integrate `osm:ctxutil` for shared context.
>4.  **Output Generation:** Implement fence-safe markdown generation.
>    * *Pattern:*
>        ```markdown
>        Document 1:
>        ```
>        {{CONTENT}}
>        ```
>        ```
>
>### 4.2 User Interface
>1.  **Navigation:** Keyboard and Mouse support (as per requirements).
>2.  **Actions:** Single-click `copy` button.
>3.  **Visuals:** Utilize `osm:lipgloss` for styling (borders, padding, colors).
>
>---
>
>## 5. VALIDATION & VERIFICATION
>
>**Constraint:** Session must include extensive validation prior to cessation.
>
>### 5.1 Static Analysis
>* Run `staticcheck`. Fix all reported issues.
>
>### 5.2 Unit Testing
>* **Lipgloss:** Test immutability (Base style must remain unchanged when deriving new style).
>* **Bubbletea:** Test `Update` cycle with `tea.Msg`.
>* **Bubbletea:** Test `valueToCmd` batch/sequence execution.
>* **Bubbletea:** Test `MouseMsg` and `WindowSizeMsg` conversion.
>
>### 5.3 Functional/Integration Testing
>* **Scripting:** Verify `super-document` script executes without `ReferenceError` on globals.
>* **Logic:** Test `addDocument`, `removeDocument`, `updateDocument`.
>* **Output:** Verify `buildFinalPrompt` generates correctly fenced markdown.
>* **End-to-End:** Simulate a TUI session (mock input) and verify state changes.
>
>---
>
>## 6. EXECUTION ORDER
>
>1.  **Refactor Core:** Fix Mutex, WaitGroup, IO types.
>2.  **Fix Logic:** Lipgloss Immutability, KeyMsg Ctrl.
>3.  **Implement TUI:** Complete `super-document` logic and script injection.
>4.  **Verify:** Run suite of new behavioral and functional tests.

## The Point

This is meant be a GUI TOOL! A GUI TOOL! The REPL-style terminal is nice, but
this is something VERY well suited to be a full-on interactive via mouse TUI,
rather than console-style TUI.

I don't understand what was unclear - what was the POINT of adding bubbletea and
lipgloss support if not to actually USE them to build a TUI tool that is
interactive and mouse-supporting?

A fully-featured solution MUST be designed and IMPLEMENTED, inclusive of UNIT
and INTEGRATION tests. USE bubbletea and lipgloss to build the TUI tool. The
REPL-style terminal is a nice-to-have, but secondary to the core requirement of
a full TUI tool. Now that the config exists, you're going to need to support
BOTH, which is achievable, but you need to PRIORITIZE the TUI tool.

This must be an IMPRESSIVELY well-designed application. It is meant to be
a core built-in command, alongside code-review etc. It is meant to be a
SHOWCASE of the deep integration with bubbletea and lipgloss, and the
capabilities of the overall system - it MUST be WELL-ARCHITECTED, PRODUCTION
READY, and EXHAUSTIVELY TESTED, _IMPRESSIVE_ and _VISUALLY AND FUNCTIONALLY
APPEALING_ addition - not some half-baked, lazy, incomplete, broken
implementation that doesn't even use the libraries it was meant to showcase.

Fuk you buddy.
