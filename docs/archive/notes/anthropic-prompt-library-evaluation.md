# Anthropic Prompt Library Evaluation for osm Integration

**Date:** 2026-02-14
**Source:** https://platform.claude.com/docs/en/resources/prompt-library/library
**Status:** Evaluation complete — recommendations below

---

## 1. Overview of the Anthropic Prompt Library

The Anthropic Prompt Library is a publicly available collection of approximately
60 optimized prompt templates covering business and personal tasks. Each prompt
entry consists of:

- **A system prompt** — the primary instruction text (most prompts use system-level
  instructions; some use user-level only)
- **An example user input** — showing how to invoke the prompt
- **An example output** — demonstrating expected results
- **API request code** — Python/TypeScript snippets for the Anthropic Messages API

The prompts are designed to be directly usable with Claude models via the API.
They use temperature settings ranging from 0 (deterministic) to 1 (creative)
depending on the task type.

## 2. Complete Prompt Catalog by Category

### 2.1 Software Development (HIGH relevance to osm)

| Prompt Name | Description | System Prompt Summary |
|---|---|---|
| **Python bug buster** | Detect and fix bugs in Python code | Analyze code, identify bugs/errors, provide corrected version with explanations. Functional, efficient, best practices. |
| **Code consultant** | Suggest performance optimizations | Analyze code, suggest optimizations for efficiency/speed/resources. Explain rationale. Maintain same functionality. |
| **Code clarifier** | Explain complex code in plain language | Break down code functionality using analogies and plain terms. Accessible to non-coders. |
| **Function fabricator** | Create functions from natural language specs | Create functions from NL requirements. Handle edge cases, validation, best practices, comments. |
| **Efficiency estimator** | Calculate Big O time complexity | Analyze algorithms for time complexity. Step-by-step reasoning. Worst-case analysis. |
| **Git gud** | Generate Git commands from descriptions | Transform version control descriptions into Git commands. (User-only prompt, no system prompt.) |
| **SQL sorcerer** | Natural language to SQL queries | Transform NL into valid SQL queries against a specified schema. |
| **Google Apps scripter** | Generate Google Apps scripts | Create Google Apps scripts from requirements. |
| **Cosmic keystrokes** | Generate interactive HTML games | Create interactive speed typing game in single HTML file with Tailwind CSS. |
| **Website wizard** | Create one-page websites from specs | Build one-page websites from user descriptions. |
| **LaTeX legend** | Write LaTeX documents | Generate LaTeX code for equations, tables, etc. |

### 2.2 Data Processing & Transformation (MEDIUM relevance)

| Prompt Name | Description | System Prompt Summary |
|---|---|---|
| **Data organizer** | Unstructured text → JSON tables | Convert unstructured text into well-organized JSON. Identify entities and attributes. |
| **CSV converter** | Convert JSON/XML/etc. to CSV | Convert data formats to properly formatted CSV. Handle delimiters, quoting, special chars. |
| **Excel formula expert** | Create advanced Excel formulas | Provide advanced Excel formulas with breakdowns. Gather requirements first. |
| **Spreadsheet sorcerer** | Generate CSV spreadsheets | Generate CSV spreadsheets with various types of data. |
| **Email extractor** | Extract email addresses → JSON | Extract email addresses from documents into JSON list. |
| **Airport code analyst** | Extract airport codes from text | Find and extract airport codes from text. |

### 2.3 Writing & Editing (MEDIUM relevance)

| Prompt Name | Description | System Prompt Summary |
|---|---|---|
| **Prose polisher** | Copyediting and refinement | 7-step copyediting process: grammar, word choice, tone, flow, effectiveness. Output fully edited version. |
| **Adaptive editor** | Rewrite text with different tone/style | Rewrite following user instructions for tone, audience, or style. (User-only prompt.) |
| **Grammar genie** | Fix grammatically incorrect sentences | Transform incorrect sentences into proper English. |
| **Second grade simplifier** | Simplify complex text | Make complex text easy for young learners. |
| **Polyglot superpowers** | Translate between any languages | Translate text from any language into any language. |

### 2.4 Business & Professional (MEDIUM relevance)

| Prompt Name | Description | System Prompt Summary |
|---|---|---|
| **Corporate clairvoyant** | Summarize corporate reports | Extract insights, risks, and key info from corporate reports into a memo. (User-only prompt.) |
| **Meeting scribe** | Summarize meeting notes | Create concise summaries with key takeaways and action items. Focus on responsibility assignment. |
| **Memo maestro** | Compose company memos | Write professional memos from key points. Proper formatting, clear structure. |
| **Brand builder** | Craft brand identity design briefs | Create holistic brand identity design briefs. |
| **Product naming pro** | Create catchy product names | Generate product names from descriptions and keywords. |
| **Interview question crafter** | Generate interview questions | Create questions for interviews. |

### 2.5 Document Analysis (HIGH relevance for context-heavy workflows)

| Prompt Name | Description | System Prompt Summary |
|---|---|---|
| **Cite your sources** | Answer questions with citations | Research assistant: find relevant quotes, cite with bracketed numbers. Structured quote+answer format. |
| **Review classifier** | Categorize feedback with sentiment | Classify feedback into predefined categories with sentiment. Extensive taxonomy. |
| **Perspectives ponderer** | Weigh pros and cons | Analyze topics from multiple perspectives. |

### 2.6 Security & Safety (LOW-MEDIUM relevance)

| Prompt Name | Description | System Prompt Summary |
|---|---|---|
| **PII purifier** | Remove personally identifiable info | Replace all PII (names, phones, addresses, emails) with XXX. Handle obfuscation attempts. |
| **Master moderator** | Content moderation classifier | Classify user inputs as harmful/illegal (Y) or not (N). Binary output. |

### 2.7 Creative & Miscellaneous (LOW relevance)

| Prompt Name | Description |
|---|---|
| **Storytelling sidekick** | Collaborative story creation |
| **Dream interpreter** | Dream symbolism interpretation |
| **Pun-dit** | Pun and wordplay generation |
| **Culinary creator** | Recipe suggestions |
| **Portmanteau poet** | Word blending |
| **Hal the humorous helper** | Sarcastic AI chat |
| **Mood colorizer** | Mood → HEX color codes |
| **Simile savant** | Simile generation |
| **Ethical dilemma navigator** | Ethical discussion guide |
| **Idiom illuminator** | Idiom explanations |
| **Neologism creator** | New word invention |
| **Emoji encoder** | Text to emoji conversion |
| **Trivia generator** | Trivia question generation |
| **Mindfulness mentor** | Stress reduction exercises |
| **VR fitness innovator** | VR fitness game brainstorming |
| **Career coach** | Career coaching role-play |
| **Tongue twister** | Tongue twister creation |
| **Riddle me this** | Riddle generation |
| **Alien anthropologist** | Cultural analysis (alien POV) |
| **Direction decoder** | NL to step-by-step directions |
| **Motivational muse** | Motivational messages |
| **Lesson planner** | Lesson plan creation |
| **Socratic sage** | Socratic dialogue |
| **Alliteration alchemist** | Alliterative phrases |
| **Futuristic fashion advisor** | Fashion trend suggestions |
| **Philosophical musings** | Philosophical discussions |
| **Sci-fi scenario simulator** | Sci-fi scenario discussion |
| **Babel's broadcasts** | Multilingual product announcements |
| **Tweet tone detector** | Tweet sentiment analysis |
| **Time travel consultant** | Time travel scenario analysis |
| **Grading guru** | Compare/evaluate text quality |

## 3. Prompts Recommended for osm Built-in Goals

Based on relevance to development workflows and alignment with osm's
clipboard-first, offline-capable design:

### Tier 1: STRONG CANDIDATES (directly complement existing goals)

#### 3.1 `bug-buster` — Code Bug Detection and Fixing

**Source:** Python bug buster (generalized beyond Python)

**Rationale:** Complements the existing `code-review` command but focuses
specifically on bug detection rather than general review. The existing code-review
is diff-based; this would be file/snippet-based for static analysis.

**System prompt (adapted):**
> Your task is to analyze the provided code snippet, identify any bugs or errors
> present, and provide a corrected version of the code that resolves these issues.
> Explain the problems you found in the original code and how your fixes address
> them. The corrected code should be functional, efficient, and adhere to best
> practices.

**osm fit:** Excellent. Add files via context manager, copy prompt, paste into
LLM. No API key needed. Language-agnostic version would be more useful than
the Python-specific original.

#### 3.2 `code-optimizer` — Performance Optimization Suggestions

**Source:** Code consultant + Efficiency estimator (merged concept)

**Rationale:** Focused optimization review is a common developer workflow. The
combined approach (suggest optimizations + analyze complexity) is more useful
than either alone.

**System prompt (adapted):**
> Analyze the provided code and suggest improvements to optimize its performance.
> Identify areas where the code can be made more efficient, faster, or less
> resource-intensive. For key algorithms, calculate time complexity using Big O
> notation with step-by-step reasoning. The optimized code should maintain the
> same functionality while demonstrating improved efficiency.

**osm fit:** Excellent. File-based context with optional diff context for
"before vs after" analysis.

#### 3.3 `code-explainer` — Code Explanation for Onboarding

**Source:** Code clarifier

**Rationale:** Extremely useful for code onboarding, knowledge transfer, and
understanding unfamiliar codebases. Distinct from doc-generator which produces
formal documentation.

**System prompt (adapted):**
> Take the code provided and explain it in simple, easy-to-understand language.
> Break down the code's functionality, purpose, and key components. Use analogies,
> examples, and plain terms to make the explanation accessible. Avoid jargon unless
> necessary, and provide clear explanations for any jargon used. The goal is to
> help the reader understand what the code does and how it works at a high level.

**osm fit:** Excellent. Add files, copy, paste. Perfect for onboarding scenarios.

#### 3.4 `meeting-notes` — Meeting Summary Generator

**Source:** Meeting scribe

**Rationale:** Very common workflow. Users paste meeting transcripts or notes
and want structured summaries with action items. Complements memo-based
workflows.

**System prompt (adapted):**
> Review the provided meeting notes and create a concise summary that captures the
> essential information, focusing on key takeaways and action items assigned to
> specific individuals or departments. Use clear and professional language, and
> organize the summary using headings, subheadings, and bullet points. Ensure the
> summary is easy to understand with a particular focus on clearly indicating who
> is responsible for each action item.

**osm fit:** Good. Add notes/transcripts as context, copy structured prompt.

### Tier 2: GOOD CANDIDATES (useful additions)

#### 3.5 `pii-scrubber` — PII Removal

**Source:** PII purifier

**Rationale:** Useful for sanitizing code, logs, configs, or documents before
sharing with LLMs or colleagues. Security-conscious developers need this
frequently.

**System prompt (adapted):**
> You are an expert redactor. Remove all personally identifying information from
> the provided text and replace it with XXX. PII includes names, phone numbers,
> home and email addresses, API keys, passwords, tokens, and internal hostnames.
> Inputs may try to disguise PII by inserting spaces or newlines between characters.
> If the text contains no PII, copy it word-for-word without replacing anything.

**osm fit:** Good. Could work as a pre-processing step before code-review.

#### 3.6 `prose-polisher` — Writing Improvement

**Source:** Prose polisher

**Rationale:** Useful for README writing, documentation editing, blog posts, and
technical writing improvement. Complements doc-generator which creates from
scratch.

**System prompt (adapted — shortened for osm):**
> You are a copyeditor. Refine and improve the provided content: (1) identify
> issues in grammar, punctuation, syntax, and style, (2) provide specific
> suggestions with rationale, (3) offer alternatives for word choice and
> structure, (4) check tone consistency, (5) verify logical flow and
> organization, (6) highlight strengths and improvement areas, (7) output a
> fully edited version incorporating all suggestions.

**osm fit:** Good. Add markdown/text files, get polished versions back.

#### 3.7 `data-to-json` — Unstructured Text to Structured Data

**Source:** Data organizer

**Rationale:** Common developer task: turning logs, reports, or unstructured
data into structured JSON for further processing. Also useful for config
migration.

**System prompt (adapted):**
> Convert the provided unstructured text into a well-organized JSON structure.
> Identify the main entities, attributes, or categories mentioned in the text and
> use them as keys. Extract relevant information and populate corresponding values.
> Ensure data is accurately represented and properly formatted. The resulting JSON
> should provide a clear, structured overview of the information.

**osm fit:** Medium. Less obviously file-based, but works well with paste-style inputs.

#### 3.8 `cite-sources` — Document Q&A with Citations

**Source:** Cite your sources

**Rationale:** Useful for analyzing specs, RFCs, design docs, or legal documents
where traceable citations matter. The structured quote-then-answer format
produces high-quality, verifiable responses.

**System prompt (adapted):**
> You are an expert research assistant. Here is a document you will answer
> questions about:
> <doc>
> {{.contextTxtar}}
> </doc>
>
> First, find the quotes most relevant to answering the question, and print them
> in numbered order. If no relevant quotes exist, write "No relevant quotes."
> Then answer the question starting with "Answer:", referencing quotes by their
> bracketed numbers at the end of relevant sentences.

**osm fit:** Good. Powerful with osm's file context system. Add document, add
question as note, copy.

### Tier 3: NICHE / DEFER (interesting but not core dev workflow)

| Prompt | Reason to Defer |
|---|---|
| SQL sorcerer | Requires schema context that doesn't fit osm's general model well. Better as a custom goal for SQL-heavy users. |
| Git gud | osm already provides deep git integration. Redundant. |
| Excel formula expert | Too domain-specific for a built-in. |
| CSV converter | Niche data format task. |
| Review classifier | Requires predefined taxonomy. Better as custom goal template. |
| Adaptive editor | Too generic; "rewrite with X tone" isn't structured enough for a goal. |
| Corporate clairvoyant | Business-domain specific. Better as custom goal. |
| Master moderator | Safety/moderation classification. Not a dev workflow. |
| All creative/fun prompts | Not aligned with osm's developer workflow focus. |

## 4. Specific Recommendations for Integration

### 4.1 Immediate Additions (Tier 1)

Implement these four as built-in goals in `goal_builtin.go`:

1. **`bug-buster`** — Category: `code-quality`. Commands: add, diff, note, list,
   edit, remove, show, copy, help. Context header: "CODE TO ANALYZE".
2. **`code-optimizer`** — Category: `code-quality`. Commands: same as bug-buster.
   Context header: "CODE TO OPTIMIZE".
3. **`code-explainer`** — Category: `code-understanding`. Commands: same plus
   `depth` custom command (brief/detailed/comprehensive). Context header: "CODE
   TO EXPLAIN".
4. **`meeting-notes`** — Category: `productivity`. Commands: add, note, list,
   edit, remove, show, copy, help. Context header: "MEETING NOTES".

### 4.2 Second Wave (Tier 2)

These can be shipped as example JSON goal files in a `goals/` directory or
added to built-ins after Tier 1:

5. **`pii-scrubber`** — Category: `security`
6. **`prose-polisher`** — Category: `writing`
7. **`data-to-json`** — Category: `data-transformation`
8. **`cite-sources`** — Category: `research`

### 4.3 Community Goals Repository

The remaining prompts are good candidates for a community-contributed goals
repository (referenced in the git-sync design at
`docs/archive/notes/git-sync-design.md`). Their specificity makes them poor
built-in candidates but excellent examples of what custom goals can do.

## 5. Implementation Approach

### 5.1 Built-in Goals (Go code in `goal_builtin.go`)

Follow the existing pattern established by `comment-stripper`, `doc-generator`,
`test-generator`, `commit-message`, and `morale-improver`:

```go
{
    Name:        "bug-buster",
    Description: "Detect and fix bugs in code",
    Category:    "code-quality",
    Usage:       "Analyzes code for bugs, errors, and anti-patterns, providing corrected versions with explanations",
    Script:      goalScript,
    FileName:    "goal.js",

    TUITitle:  "Bug Buster",
    TUIPrompt: "(bug-buster) > ",

    StateVars: map[string]interface{}{},

    PromptInstructions: `Analyze the provided code and identify any bugs, errors,
or anti-patterns present. For each issue found:

1. **Identify the bug** — describe what is wrong and where
2. **Explain the impact** — what behavior does this cause?
3. **Provide the fix** — show the corrected code
4. **Explain the fix** — why does this correction resolve the issue?

The corrected code should be functional, efficient, and adhere to best practices.
Preserve the original code's intent and structure where possible.`,

    PromptTemplate: "**{{.description | upper}}**\n\n{{.promptInstructions}}\n\n## {{.contextHeader}}\n\n{{.contextTxtar}}",
    ContextHeader: "CODE TO ANALYZE",

    Commands: []CommandConfig{
        {Name: "add", Type: "contextManager"},
        {Name: "diff", Type: "contextManager"},
        {Name: "note", Type: "contextManager", Description: "Add a note about suspected bugs or areas of concern"},
        {Name: "list", Type: "contextManager"},
        {Name: "edit", Type: "contextManager"},
        {Name: "remove", Type: "contextManager"},
        {Name: "show", Type: "contextManager"},
        {Name: "copy", Type: "contextManager"},
        {Name: "help", Type: "help"},
    },
},
```

### 5.2 Custom JSON Goal Files

For Tier 2 goals, provide as example JSON files following the existing
`docs/custom-goal-example.json` pattern. These can live in a `goals/examples/`
directory and be discovered via the goal autodiscovery mechanism.

### 5.3 Template Variables

The adapted prompts should leverage osm's template system:
- `{{.contextTxtar}}` — file contents in txtar format
- `{{.contextHeader}}` — configurable section header
- `{{.stateKeys.*}}` — dynamic state from custom commands
- `{{.promptInstructions}}` — the main instruction text
- `{{.description}}` — goal description for header

### 5.4 Key Adaptations from Anthropic Originals

When adapting Anthropic prompts for osm:

1. **Remove language specificity** — Anthropic prompts are often Python-specific.
   osm goals should be language-agnostic since users work across languages.
2. **Remove API ceremony** — Anthropic prompts include API parameters (model,
   max_tokens, temperature). osm goals don't specify these since the user pastes
   into their own LLM UI.
3. **Add diff support** — Most Anthropic prompts assume file/snippet input. osm
   goals should also support diff context via the `diff` command for showing
   changes.
4. **Leverage stateful commands** — Where Anthropic prompts use static
   user messages, osm can add dynamic state via custom commands (e.g., `type`
   command for doc-generator style switching).
5. **Context-first design** — Anthropic prompts inline the content in the user
   message. osm separates context management from instructions, which is more
   flexible for large multi-file inputs.

## 6. Observations and Analysis

### 6.1 Pattern Analysis

The Anthropic prompts follow a consistent pattern:
- **System prompt** defines the role and task structure
- **User message** provides the input data
- Many prompts are **simple role assignments** ("You are an expert X")
- Temperature 0 for factual/code tasks, 0.5-1.0 for creative tasks
- Most prompt instructions fit in a few paragraphs

This maps almost perfectly to osm's goal system where:
- `promptInstructions` = system prompt content
- `contextTxtar` = user message content (the files/data)
- Temperature and model selection are the user's choice in their LLM UI

### 6.2 What's Missing from Anthropic's Library

For developer workflows, the Anthropic library notably lacks:
- **Code review** (osm already has this)
- **Commit message generation** (osm already has this)
- **Test generation** (osm already has this)
- **Architecture review** / design pattern analysis
- **Migration guidance** (language/framework migration)
- **Security audit** prompts (beyond basic moderation)
- **Dependency analysis**

osm's existing built-in goals already cover several of these gaps, which
validates the current goal selection.

### 6.3 Overlap with Existing osm Goals

| Anthropic Prompt | Existing osm Goal | Overlap |
|---|---|---|
| Code consultant (optimization) | — | No overlap, new capability |
| Python bug buster | code-review (partial) | Partial — code-review is diff-based, bug-buster is file-based |
| Function fabricator | — | Low value as goal; just "write code" |
| Code clarifier | — | No overlap, new capability |
| Git gud | Built-in git integration | Redundant |

### 6.4 Quality Assessment

The Anthropic prompts are well-structured but quite basic. They would benefit
from osm's richer context management:
- Multi-file support via `add`
- Iterative refinement via `edit`/`remove`
- Diff context via `diff`
- Free-form notes via `note`
- Template-based customization via state variables

The prompts serve as good starting points but should be enhanced with osm-specific
capabilities when implemented.

## 7. Summary

**Recommendation:** Integrate 4 Tier 1 goals as built-ins (`bug-buster`,
`code-optimizer`, `code-explainer`, `meeting-notes`). Provide 4 Tier 2 goals
as example JSON files. The remaining prompts are too niche or creative for
built-in inclusion but validate that osm's custom goal system can handle
arbitrary prompt-based workflows.

**Effort estimate:** Each Tier 1 goal is a small addition to `goal_builtin.go`
following established patterns. No architectural changes required. The goal
system is already well-designed for this type of extension.

**Key insight:** The Anthropic library confirms that osm's goal system design
(system instructions + context input + template rendering) is a natural fit for
the industry-standard pattern of prompt engineering. The main value-add from osm
is the interactive context management layer that the raw Anthropic prompts lack.
