package command

import (
	"bytes"
	"strings"
	"testing"
	"text/template"
)

func TestSuperDocumentTemplate_ConditionalDocuments(t *testing.T) {
	tmpl, err := template.New("super").Parse(superDocumentTemplate)
	if err != nil {
		t.Fatalf("parse template: %v", err)
	}

	t.Run("no documents", func(t *testing.T) {
		var buf bytes.Buffer
		data := map[string]interface{}{"contextTxtar": ""}
		if err := tmpl.Execute(&buf, data); err != nil {
			t.Fatalf("execute template: %v", err)
		}
		out := buf.String()
		// Normalize line endings to avoid Windows CRLF/LF differences in CI
		out = strings.ReplaceAll(out, "\r\n", "\n")
		if strings.Contains(out, "## DOCUMENTS") {
			t.Fatalf("expected no DOCUMENTS header, got output containing it:\n%s", out)
		}
		if strings.Contains(out, "\n---\n## DOCUMENTS\n---") {
			t.Fatalf("found documents header block when none expected:\n%s", out)
		}
	})

	t.Run("with documents", func(t *testing.T) {
		var buf bytes.Buffer
		data := map[string]interface{}{"documents": []map[string]string{{"id": "doc1", "content": "content1"}}}
		if err := tmpl.Execute(&buf, data); err != nil {
			t.Fatalf("execute template: %v", err)
		}
		out := buf.String()
		// Normalize line endings to avoid Windows CRLF/LF differences in CI
		out = strings.ReplaceAll(out, "\r\n", "\n")
		if !strings.Contains(out, "## DOCUMENTS") {
			t.Fatalf("expected DOCUMENTS header present, got:\n%s", out)
		}
		if !strings.Contains(out, "Document doc1:") {
			t.Fatalf("expected document id, got:\n%s", out)
		}
		if !strings.Contains(out, "```") || !strings.Contains(out, "content1") {
			t.Fatalf("expected fenced content with 'content1', got:\n%s", out)
		}
		// Check spacing: header should be preceded by exactly one blank line and the header separators should be present
		if !strings.Contains(out, "\n\n---\n## DOCUMENTS\n---\n") {
			t.Fatalf("expected header block to be preceded by one blank line and followed by separators, got:\n%s", out)
		}
	})
}
