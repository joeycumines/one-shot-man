# PR Split Analysis

## Branch: {{baseBranch}} → {{currentBranch}}

### Changed Files ({{fileCount}})

{{#each files}}
- `{{this.path}}` ({{this.status}}, +{{this.additions}}/-{{this.deletions}})
{{/each}}

### Split Strategy: {{strategy}}

**Heuristic Grouping** ({{strategy}}):
Files grouped by {{strategy}} into {{groupCount}} splits.

{{#each groups}}
#### Split {{@index}}: {{this.label}} ({{this.files.length}} files)
{{#each this.files}}
- `{{this}}`
{{/each}}
{{/each}}

### Execution Plan

| # | Branch | Files | Description |
|---|--------|-------|-------------|
{{#each plan}}
| {{this.index}} | `{{this.branch}}` | {{this.fileCount}} | {{this.description}} |
{{/each}}

### Verification

{{#if verified}}
✅ All splits verified. Tree hash equivalence confirmed.
{{else}}
⏳ Verification pending. Run `execute` to create branches and verify.
{{/if}}
