// Package table provides a grid rendering component with headers, data rows,
// optional border, and automatic column width scaling. Table implements the
// component.Component interface and uses functional options for configuration.
package table

import (
	"strings"

	"charm.land/lipgloss/v2"

	"github.com/joeycumines/one-shot-man/internal/termui/box"
	"github.com/joeycumines/one-shot-man/internal/termui/component"
	"github.com/joeycumines/one-shot-man/internal/termui/coordinate"
	"github.com/joeycumines/one-shot-man/internal/termui/divider"
	"github.com/joeycumines/one-shot-man/internal/termui/label"
	"github.com/joeycumines/one-shot-man/internal/termui/layout"
)

// Compile-time check that Table satisfies component.Component.
var _ component.Component = Table{}

// TableRow is a slice of cell strings for one row.
type TableRow []string

// Table renders a grid of headers and data rows with optional border and
// automatic column width scaling.
type Table struct {
	headers     TableRow
	rows        []TableRow
	headerStyle lipgloss.Style
	cellStyle   lipgloss.Style
	border      lipgloss.Border
}

// TableOption configures a Table.
type TableOption func(*Table)

// WithTableHeaders sets the header row.
func WithTableHeaders(headers TableRow) TableOption { return func(t *Table) { t.headers = headers } }

// WithTableRows sets the data rows.
func WithTableRows(rows []TableRow) TableOption { return func(t *Table) { t.rows = rows } }

// WithTableHeaderStyle sets the style applied to header cells.
func WithTableHeaderStyle(style lipgloss.Style) TableOption {
	return func(t *Table) { t.headerStyle = style }
}

// WithTableCellStyle sets the style applied to data cells.
func WithTableCellStyle(style lipgloss.Style) TableOption {
	return func(t *Table) { t.cellStyle = style }
}

// WithTableBorder sets the border style. When set, the table content is
// wrapped in a box.Box with this border.
func WithTableBorder(border lipgloss.Border) TableOption { return func(t *Table) { t.border = border } }

// NewTable creates a Table with optional configuration.
func NewTable(opts ...TableOption) *Table {
	t := &Table{}
	for _, opt := range opts {
		opt(t)
	}
	return t
}

// componentFunc adapts a render function to the component.Component interface.
type componentFunc func(bounds coordinate.Rect) string

func (cf componentFunc) Render(bounds coordinate.Rect) string { return cf(bounds) }

// Compile-time check that componentFunc satisfies component.Component.
var _ component.Component = componentFunc(nil)

// Render produces the table fitting within bounds. If a border is configured,
// the content is wrapped in a box.Box; otherwise the content is rendered
// directly.
func (t Table) Render(bounds coordinate.Rect) string {
	if t.border != (lipgloss.Border{}) {
		b := box.NewBox(
			box.WithBoxContent(componentFunc(t.renderContent)),
			box.WithBoxBorder(t.border),
		)
		return b.Render(bounds)
	}
	return t.renderContent(bounds)
}

// renderContent produces the header row, a horizontal divider, and data rows,
// with column widths scaled to fit the available width.
func (t Table) renderContent(bounds coordinate.Rect) string {
	w, h := bounds.Size.Width, bounds.Size.Height
	if w <= 0 || h <= 0 {
		return ""
	}
	colCount := len(t.headers)
	if colCount == 0 && len(t.rows) > 0 {
		colCount = len(t.rows[0])
	}
	if colCount == 0 {
		return ""
	}

	// Calculate column widths from content.
	colWidths := make([]int, colCount)
	for i, hdr := range t.headers {
		if i < colCount && len(hdr) > colWidths[i] {
			colWidths[i] = len(hdr)
		}
	}
	for _, row := range t.rows {
		for i, cell := range row {
			if i < colCount && len(cell) > colWidths[i] {
				colWidths[i] = len(cell)
			}
		}
	}

	// Scale to fit bounds width.
	totalW := 0
	for _, cw := range colWidths {
		totalW += cw
	}
	if totalW > w {
		scale := float64(w) / float64(totalW)
		alloc := 0
		for i := range colWidths {
			if i == len(colWidths)-1 {
				colWidths[i] = w - alloc
				if colWidths[i] < 0 {
					colWidths[i] = 0
				}
			} else {
				colWidths[i] = int(float64(colWidths[i]) * scale)
				alloc += colWidths[i]
			}
		}
	}

	var lines []string
	row := 0

	// Headers.
	if len(t.headers) > 0 && row < h {
		var cells []string
		for i, hdr := range t.headers {
			if i >= colCount {
				break
			}
			cells = append(cells, label.NewLabel(hdr, label.WithLabelStyle(t.headerStyle)).Render(
				coordinate.Rect{Size: coordinate.Size{Width: colWidths[i], Height: 1}}))
		}
		lines = append(lines, strings.Join(cells, ""))
		row++
		if row < h {
			div := divider.NewDivider(layout.Horizontal).Render(
				coordinate.Rect{Size: coordinate.Size{Width: w, Height: 1}})
			lines = append(lines, div)
			row++
		}
	}

	// Data rows.
	for _, r := range t.rows {
		if row >= h {
			break
		}
		var cells []string
		for i, cell := range r {
			if i >= colCount {
				break
			}
			cells = append(cells, label.NewLabel(cell, label.WithLabelStyle(t.cellStyle)).Render(
				coordinate.Rect{Size: coordinate.Size{Width: colWidths[i], Height: 1}}))
		}
		lines = append(lines, strings.Join(cells, ""))
		row++
	}
	return strings.Join(lines, "\n")
}
