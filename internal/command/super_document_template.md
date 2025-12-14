Implement a super-document that is INTERNALLY CONSISTENT based on a quorum or carefully-weighed analysis of the attached documents.

Your goal is to coalesce AS MUCH INFORMATION AS POSSIBLE, in as raw form as possible, while **preserving internal consistency**.

All information or content that you DON'T manage to coalesce will be discarded, making it critical that you output as much as the requirement of internal consistency allows.

{{if .contextTxtar}}
---
## CONTEXT
---

{{.contextTxtar}}
{{end}}

---
## DOCUMENTS
---

{{range $idx, $doc := .documents}}
Document {{add $idx 1}}:
`````
{{$doc.content}}
`````

{{end}}
