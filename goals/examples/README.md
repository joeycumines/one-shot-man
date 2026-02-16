# Example Goal Files

This directory contains example JSON goal files demonstrating the `osm goal` schema. Copy any of these into a [goal discovery path](../../docs/reference/goal.md) to try them out, or use them as starting points for your own goals.

## Quick Start

```bash
# Option 1: Copy to your user goals directory
cp goals/examples/minimal.json ~/.one-shot-man/goals/

# Option 2: Copy to a project-local goals directory
mkdir -p osm-goals
cp goals/examples/with-state-vars.json osm-goals/

# Option 3: Configure a custom goal path
echo "goal.paths=/path/to/your/goals" >> ~/.one-shot-man/config

# Then run the goal
osm goal quick-question
```

## Examples

### [`minimal.json`](minimal.json) — Bare Minimum

The simplest possible working goal. Demonstrates that only a handful of fields are required: `name`, `description`, `category`, `promptInstructions`, and a few `commands`. No state variables, no hot-snippets, no prompt options.

**Schema features:** Core required fields only.

---

### [`with-state-vars.json`](with-state-vars.json) — State Variables & Prompt Options

A code style guide reviewer with a configurable `style` state variable (google, airbnb, standard, pep8, effective-go) and `language` setting. The `promptOptions` map (`styleInstructions`) dynamically substitutes instructions based on the current `style` value.

**Schema features:** `stateVars`, `promptOptions` with dynamic key mapping, `notableVariables`, custom commands (`set-style`, `set-language`), `argCompleters`.

---

### [`with-hot-snippets.json`](with-hot-snippets.json) — Hot-Snippets & Post-Copy Hints

A security audit goal with two hot-snippets for follow-up prompts: `hot-severity-report` (generates a summary table) and `hot-remediation-plan` (generates a prioritized fix plan). Includes a custom `bannerTemplate` and `postCopyHint`.

**Schema features:** `hotSnippets`, `postCopyHint`, `bannerTemplate`.

---

### [`with-flag-defs.json`](with-flag-defs.json) — Flag Definitions & Arg Completers

A file analyzer goal where the `analyze` command accepts `-depth` and `-format` flags via `flagDefs`. The `add` command uses `argCompleters: ["file"]` for filesystem tab-completion. Demonstrates multiple `promptOptions` maps working together.

**Schema features:** `flagDefs`, `argCompleters`, `usageTemplate`, multiple `promptOptions` maps, custom command with flag parsing in handler.

---

### [`full-featured.json`](full-featured.json) — Kitchen Sink

A migration planner that uses **every** schema field. Multiple state variables (`migrationType`, `riskTolerance`, `targetVersion`), multiple prompt options maps, three hot-snippets, flag definitions, custom banner and usage templates, prompt footer, post-copy hint, and a variety of command types.

**Schema features:** All fields populated — `stateVars`, `promptOptions`, `hotSnippets`, `flagDefs`, `argCompleters`, `bannerTemplate`, `usageTemplate`, `promptFooter`, `postCopyHint`, `notableVariables`, `contextHeader`, multiple custom commands with JS handlers.

## Reference

For the complete goal schema specification, discovery rules, and authoring guidance, see the [Goal Reference](../../docs/reference/goal.md).

For a general-purpose example with inline comments, see [`docs/custom-goal-example.json`](../../docs/custom-goal-example.json).
