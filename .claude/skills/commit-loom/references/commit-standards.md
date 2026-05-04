# Commit Standards

## Commit Message Format

Match the repo's existing convention first. Check recent `git log --oneline -20` for patterns.

### Conventional Commits (default if no clear convention)

```
<type>(<scope>): <subject>

<body explaining why>
```

Types:
- `feat`: New functionality
- `fix`: Bug fix
- `refactor`: Code restructuring without behavior change
- `test`: Adding or updating tests
- `docs`: Documentation changes
- `chore`: Build, tooling, or maintenance changes
- `perf`: Performance improvement
- `style`: Formatting, whitespace (no logic change)

### Subject Line Rules

- Imperative mood ("add feature" not "added feature" or "adds feature")
- Under 72 characters
- States the *intent*, not the *mechanism*
- No period at the end

### Body Rules

- Explain *why* the change was made, not what was changed (the diff shows what)
- Wrap at 72 characters
- May reference issues, PRs, or prior commits when relevant

## Quality Criteria

A commit is "maintainer-grade" when:

1. **Self-sufficient**: The tree at this commit builds, passes tests, and behaves sensibly on its own. A reviewer checking out this commit can work with it.

2. **Justified**: The commit message makes the motivation clear. A reviewer reading the message first should think "yes, this makes sense" before looking at the diff.

3. **Scoped**: The change is coherent — all the changes in this commit relate to a single purpose. If a reviewer has to ask "why is this unrelated thing here?", the scope is wrong. However, "scoped" does not mean "atomic". A commit covering a coherent area of work that includes the primary change plus all necessary supporting changes (tests, docs, imports, cleanup) is acceptable and preferred over splitting into artificial sub-commits.

4. **Minimal but complete**: The commit contains everything needed for its purpose and nothing extraneous. No commented-out code, no TODO markers that don't add value, no leftover debug logging.

5. **Not a dump**: If you're staging more than ~5 files with a message like "various changes", stop and re-plan. Large commits are acceptable for mechanical changes (renames, mass migrations) but suspect for behavioral changes.

## Self-Contained vs. Atomic

The goal is **self-contained** commits, not "atomic" commits. These are different:

- **Atomic** = one concern per commit. Sounds clean but is often impractical for large change sets where concerns are deeply tangled.
- **Self-contained** = the commit is a coherent, complete unit of work that leaves the tree in a valid state. It may contain multiple concerns if they are interdependent.

**Why self-contained is preferred:**
- Fewer commits means fewer verification rounds
- Fewer opportunities for intermediate breakage
- Less risk of needing artificial temporary states to pass quality checks between commits
- Less total work for reviewers

**When rolling up is correct:**
- A refactor + its test updates + import cleanup — one commit
- A feature + its documentation + config changes — one commit
- A rename across multiple files — one commit (splitting would leave the tree inconsistent)

**When splitting is correct:**
- Two completely independent features — separate commits
- A behavioral change and an unrelated cleanup — separate commits
- A security fix and a cosmetic change — separate commits

**The test**: Can a reviewer check out this commit alone and work with it? If yes, it's self-contained. If the tree is broken without the next commit, the split was wrong.

## Reviewer Communication

When subagent reviewers are used during the commit-loom process, include this context in the review prompt:

> "This commit is part of a structured multi-commit sequence produced by commit-loom. Commits are intentionally self-contained rather than atomic — multiple related changes may be rolled up into a single commit to avoid leaving the tree in an inconsistent intermediate state. Review the changes as a coherent unit. Do not flag scope as a concern unless the changes are genuinely unrelated or the commit is too large to reason about effectively."

This prevents reviewers from pushing for unnecessary splits that would conflict with the self-contained approach.

## Examples

### Good

```
refactor(auth): extract JWT validation into standalone middleware

The auth logic was embedded in the HTTP handler, making it hard to
test and reuse. Extracting it into auth/middleware.go allows other
handlers to share the same validation path and makes the handler
tests simpler (they no longer need to set up JWT infrastructure).

The error propagation is slightly different: expired tokens now
return 401 instead of 403, which matches the RFC 6750 expectation.
```

### Good (small commit)

```
fix: handle nil profile in user greeting

When a user has no profile (new signup path), the greeting template
panicked on nil dereference. Return a default greeting instead.
```

### Bad (too vague)

```
fix stuff

various fixes
```

### Bad (too broad)

```
update auth and add tests and fix routing

Changes to auth middleware, new test file, routing fix in server.go,
and some import cleanup.
```

### Acceptable (large but justified)

```
refactor: rename User → Account across all API surfaces

Mechanical rename of the User entity to Account to align with the
new domain model (RFC #42). All API endpoints, database queries,
types, and tests updated. No behavioral changes.

This is a large commit because splitting it would leave the tree in
an inconsistent state (some files referencing User, others Account).
```
