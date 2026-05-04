# PR Split Analysis

## Branch: {{.baseBranch}} → {{.currentBranch}}

### Changed Files ({{.fileCount}})

### Split Strategy: {{.strategy}}

**Heuristic Grouping** ({{.strategy}}):
Files grouped by {{.strategy}} into {{.groupCount}} splits.

{{range .groups}}
#### Split: {{.label}} ({{len .files}} files)
{{range .files}}
- `{{.}}`
{{end}}
{{end}}

### Execution Plan

| # | Branch | Files | Description |
|---|--------|-------|-------------|
{{range .plan}}| {{.index}} | `{{.branch}}` | {{.fileCount}} | {{.description}} |
{{end}}

### Verification

{{if .verified}}
✅ All splits verified. Tree hash equivalence confirmed.
{{else}}
⏳ Verification pending. Run `execute` to create branches and verify.
{{end}}
