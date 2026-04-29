# Validation Discovery

Before any validation can run, you need to know what tools exist in the repo. This checklist covers the common patterns.

## Build Systems

Look for these files and infer the build command:

| File | Likely build command | Likely test command |
|------|---------------------|---------------------|
| `Makefile` | `make build` or `make` | `make test` |
| `package.json` | `npm run build` | `npm test` |
| `pyproject.toml` | `pip install -e .` or `python -m build` | `pytest` |
| `Cargo.toml` | `cargo build` | `cargo test` |
| `go.mod` | `go build ./...` | `go test ./...` |
| `BUILD.bazel` | `bazel build //...` | `bazel test //...` |
| `build.gradle` / `pom.xml` | `./gradlew build` / `mvn compile` | `./gradlew test` / `mvn test` |
| `Gemfile` | `bundle install` | `bundle exec rake test` |

## Linting and Formatting

| File | Command |
|------|---------|
| `.eslintrc*` / `eslint.config.*` | `npx eslint .` |
| `golangci.yml` / `.golangci.yml` | `golangci-lint run` |
| `pyproject.toml` (ruff section) | `ruff check .` |
| `pyproject.toml` (mypy section) | `mypy .` |
| `.pre-commit-config.yaml` | `pre-commit run --all-files` |
| `.rubocop.yml` | `bundle exec rubocop` |
| `clippy` (Rust) | `cargo clippy` |

## Type Checking

| Language | Likely command |
|----------|---------------|
| TypeScript | `npx tsc --noEmit` |
| Go | `go vet ./...` |
| Python (mypy) | `mypy .` |
| Rust | `cargo check` |

## Pre-commit Hooks

Check `.pre-commit-config.yaml`, `.husky/`, and `.git/hooks/pre-commit`. These define project-mandatory checks.

## CI Configuration

Read `.github/workflows/*.yml`, `.gitlab-ci.yml`, `Jenkinsfile`, or equivalent. These define what the project considers mandatory — your commits should pass the same checks CI would run.

## Available Skills and Agents

Beyond build/lint/test, the environment may contain skills and agents that enforce quality:

1. Scan `.claude/skills/` for installed skills — look for ones related to review, quality, or validation.
2. Scan `.claude/agents/` for installed agents — these may provide domain-specific enforcement.
3. Check for project-specific review configs (e.g., `.opencode/agents/`).

For each discovered skill/agent, record:
- Its name and file path
- Its applicability to commit verification (e.g., "use before each COMMIT")
- Any hints about how to use it (from its description field)

### Integration Pattern

When a skill like `strict-review-gate` is available, the VERIFY step becomes more rigorous. Instead of just running build/lint/test, also:

1. Re-read the skill's full instructions.
2. Follow its protocol (e.g., spawn subagent reviews).
3. Record the results in the cycle log.

This is why the discovery section of the ledger matters — it tells the VERIFY step what arsenal is available, and the reread-point discipline ensures the skill's instructions are fresh when used.

## Ways of Working

Beyond build/lint/test tools, every project has behavioral standards that code must conform to. These are not enforced by automated tooling — they are enforced by review and convention. Discover them during CAPTURE and record in `discovery.ways_of_working`.

| File | What to look for |
|------|-----------------|
| `CLAUDE.md`, `CLAUDE.local.md` | Project-level Claude instructions, coding conventions, preferred patterns |
| `AGENTS.md` | Agent behavior standards, interaction rules |
| `.editorconfig` | Editor conventions (indent style, charset, line endings) |
| `CONTRIBUTING.md` | Contribution guidelines, PR requirements, code style expectations |
| `DEVELOPMENT.md` | Development setup, testing expectations, coding patterns |
| `.github/PULL_REQUEST_TEMPLATE.md` | What the project expects in PR descriptions |
| `.github/CODEOWNERS` | Who owns what code — relevant for understanding review expectations |
| `.gitattributes` | File handling conventions (line endings, binary vs text) |
| `README.md` | Often contains coding conventions or architecture notes |

For each discovered standard, record:
- Its file path
- The key rules or conventions it defines (summarized, not verbatim)
- How it applies to the commit being made

## Review Pattern Selection

Not all reviews are equal. The review pattern used for each commit must be explicitly chosen and recorded.

### Recommended: Subagent Rule of Two (strict-review-gate pattern)

This is the gold standard for commit-loom review. It provides the strongest defense against LLM nondeterminism and hallucination through probability stacking.

**Protocol:**
1. Spawn at least two subagents with **identical prompts** (only trivial logistics like output file path may differ)
2. Both review the **exact same diff** — no code changes between runs
3. Both must produce PASS verdicts
4. If either fails → fix → diff changes → reset counter → restart at Run 1 against new diff

**Why identical prompts matter:** Two reviews of the same material reduce miss probability to P(miss)². If the prompts differ or the diff changes, they become independent reviews with independent failure modes — the probability stacking is lost.

### When Rule of Two is not available

If the strict-review-gate skill is not installed, or if the environment does not support spawning subagents:
- Fall back to a single thorough review using whatever review tools are available
- Record the limitation in the ledger
- Do NOT pretend the review was as rigorous as a Rule of Two gate

## Distinction: Automated vs. Manual vs. Review

These three categories are NOT interchangeable. Record each separately.

**Automated checks** (Tier 1): Commands that return pass/fail without judgment.
- `make build`, `make lint`, `make test`, `go vet ./...`, `npx tsc --noEmit`
- These are objective. Either the command exits 0 or it doesn't.
- Record: command, exit code, relevant output

**Manual checks** (not subagent review): Human judgment applied to specific concerns.
- "Does this conform to CLAUDE.md conventions?"
- "Is this API change backwards compatible?"
- "Does the commit message match the repo's style?"
- Record: what was checked, verdict, rationale

**Review checks** (Tier 2, subagent-based): Independent assessment of correctness by a separate agent.
- Subagent Rule of Two review
- Domain-specific quality checks from project agents
- These must be performed by a subagent, not self-assessed
- Record: reviewer identity, prompt used, verdict, findings

## Fallback

If no build/lint/test commands are discovered, validation degrades gracefully:
- At minimum, verify that the staged changes compile or are syntactically valid.
- Use language-appropriate syntax checkers if available.
- If truly nothing is available, note this in the ledger and proceed with reduced validation.

Even in degraded mode, ways-of-working discovery must still be performed and recorded. Behavioral standards from `CLAUDE.md`, `AGENTS.md`, and similar files apply regardless of tooling availability. The review pattern used (or the reason one was not used) must also be recorded in the ledger.
