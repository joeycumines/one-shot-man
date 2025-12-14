- [x] Explore repository structure and understand existing patterns
- [x] Add bubbletea and lipgloss dependencies to go.mod
- [x] Create `internal/builtin/bubbletea` package with JavaScript bindings
  - [x] Define `Manager` struct for bubbletea program management
  - [x] Implement `Require` function exposing bubbletea API to JavaScript
  - [x] Export Program, Model, Cmd, Msg, and tea.* primitives
  - [x] Implement comprehensive unit tests
- [x] Create `internal/builtin/lipgloss` package with JavaScript bindings
  - [x] Define `Manager` struct for lipgloss styling
  - [x] Implement `Require` function exposing lipgloss API to JavaScript
  - [x] Export Style, Color, Border, Position, and lipgloss.* primitives
  - [x] Implement comprehensive unit tests
- [x] Register bubbletea and lipgloss modules in `internal/builtin/register.go`
- [x] Create `super-document` command
  - [x] Create `internal/command/super_document.go` command file
  - [x] Create `internal/command/super_document_script.js` TUI script
  - [x] Create `internal/command/super_document_template.md`
  - [x] Implement document management (add/remove/update)
  - [x] Implement keyboard navigation commands
  - [x] Implement one-click copy output
  - [x] Implement file-based document loading
  - [x] Implement comprehensive unit tests
- [x] Register `super-document` command in `cmd/osm/main.go`
- [x] Document the pathway to "full support" for bubbletea/lipgloss
- [x] Address critical issues from code review:
  - [x] Fix runProgram deadlock (mutex held during p.Run())
  - [x] Fix goroutine leak (added wg.Wait() after p.Run())
  - [x] Fix lipgloss immutability violation (all style methods now return new wrappers)
  - [x] Add ctrl modifier to KeyMsg (using strings.HasPrefix for safe detection)
  - [x] Fix padding/margin default cases to maintain immutability
  - [x] Remove unused quitOnce field
  - [x] Remove unused BubbleteaManagerProvider/LipglossManagerProvider interfaces
  - [x] Add immutability test for lipgloss
- [x] Run CodeQL security check

## Security Summary
- No vulnerabilities found in dependency additions (bubbletea v1.3.10, lipgloss v1.1.0)
- CodeQL analysis found 0 security alerts

<!-- START COPILOT CODING AGENT SUFFIX -->



<!-- START COPILOT ORIGINAL PROMPT -->



<details>

<summary>Original prompt</summary>

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


</details>
