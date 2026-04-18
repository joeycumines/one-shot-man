---
name: openpencil-figma-design
description: >
  Creates, edits, inspects, and analyzes Figma/OpenPencil design documents
  via the openpencil-mcp server (live app) and the openpencil CLI (headless).
  Covers node creation, styling (fills, strokes, effects, typography),
  auto-layout, components, variables/design tokens, icons, vectors, boolean
  ops, image/stock-photo fills, export (PNG/JPG/WEBP/SVG/JSX/fig), design analysis,
  linting, and batch operations. Activates for working with .fig files,
  creating UI designs, building component systems, extracting design tokens,
  analyzing design systems, rendering JSX to canvas, exporting designs,
  linting, or any Figma/design operation. Also activates for: "design",
  "figma", "openpencil", "component", "design token", "canvas", "frame",
  "auto-layout", "JSX render", "export design", "design system",
  "UI mockup", "wireframe", "prototype", "layout".
---

# OpenPencil / Figma Design

Manipulate Figma-compatible design documents through MCP tools (live app) and
the `openpencil` CLI (headless file operations).

## Prerequisites

CRITICAL: Both `openpencil-mcp` and `openpencil` MUST already be installed and
in PATH. NEVER attempt to install them — no `npm install`, no `npx`, no
package manager commands. If either command is missing, inform the user and
stop.

- `openpencil-mcp` — MCP server providing ~90 design tools via stdio
- `openpencil` — CLI for headless file operations (export, lint, analyze,
  query, convert)

Two operating modes, selected automatically:

| Mode | When | Tools |
|------|------|-------|
| **Live (MCP)** | OpenPencil app running with document open | All `mcp__openpencil-mcp__*` tools |
| **Headless (CLI)** | No app, or operating on files directly | `openpencil` CLI via Bash |

Prefer MCP when available — richer toolset, real-time feedback. Fall back to
CLI for batch export, linting, CI/CD, or when the app isn't connected.

## Workflows

### Design Creation Workflow

Follow these steps for any new design work:

1. **Orient** — Understand the document before changing it.
   - `get_current_page` → `get_page_tree` to see structure
   - `describe(id)` on existing nodes to understand context
   - `analyze_colors` + `analyze_typography` to learn existing patterns
   - If variables exist: `list_variables` to find established tokens
2. **Plan** — Decide the approach before touching the canvas.
   - Choose components to build (reference existing via `get_components`)
   - Select colors from existing palette or define new tokens
   - Pick a spacing scale consistent with the project's design context
   - Use `calc` for all sizing/positioning arithmetic
3. **Execute** — Build using `render` for multi-node structures.
   - Use JSX via `render` for complex hierarchies (see [references/jsx-patterns.md](references/jsx-patterns.md))
   - Use `batch_update` for bulk property changes
   - Bind colors/sizes to variables for consistency
   - Name layers descriptively (match code names when applicable)
4. **Validate** — Check quality before declaring done.
   - `export_image` → visually verify the result
   - `analyze_spacing` → confirm spacing consistency
   - `analyze_colors` → confirm palette discipline (no rogue colors)
   - `analyze_typography` → confirm font consistency
5. **Fix** — Address any issues found in validation.
   - Fix violations, then return to step 4
   - Repeat until validation passes cleanly

### Design Audit Workflow

Follow these steps to assess and improve an existing design:

1. **Inventory** — Catalog what exists.
   - `get_page_tree` → `describe` key frames
   - `design_to_component_map` to see component architecture
   - `list_variables` to see token coverage
2. **Analyze** — Run all analysis tools.
   - `analyze_colors(show_similar=true)` → find redundant colors
   - `analyze_spacing` → find inconsistent spacing
   - `analyze_typography` → find font sprawl
   - `analyze_clusters` → find repeated patterns that should be components
3. **Report** — Summarize findings for the user.
   - List color violations, spacing violations, font violations
   - Identify candidate components from clusters
   - Note missing variable bindings
4. **Fix** — Apply corrections systematically.
   - Merge similar colors via `set_fill`
   - Align spacing to consistent scale via `set_layout`
   - Convert repeated clusters to components via `create_component`
5. **Verify** — Re-run analysis to confirm fixes.
   - Repeat steps 2-4 until analysis is clean

## Quick Reference by Task

### Orient in the document

```
get_current_page → get_page_tree → get_node(id, depth) → drill down
get_selection                    — user's current selection
describe(id)                     — semantic summary
list_pages / switch_page         — navigate pages
```

### Create nodes

- `create_shape` — frames, rectangles, ellipses, text, lines, sections
- `render` — JSX for multi-node hierarchies (most efficient)
- `create_vector` — custom vector paths
- `create_slice` — export regions

### Style nodes

```
set_fill (solid or gradient)  set_stroke  set_effects  set_blend
set_opacity  set_radius  set_rotation  set_image_fill
```

### Layout (Auto-Layout / Flexbox)

- `set_layout` — direction, spacing, padding, alignment
- `set_layout_child` — sizing (FIXED/HUG/FILL), grow, alignment
- `batch_update` — multiple property changes in one call

### Text

- `set_text`, `set_font`, `set_font_range` (per-character styling)
- `set_text_properties` — alignment, auto-resize, decoration
- `list_available_fonts` — check what's available

### Components

- `create_component` → `create_instance` to place
- `get_components` — find existing ones
- `design_to_component_map` — decomposition analysis

### Variables (design tokens)

- `create_collection` → `create_variable` → `bind_variable`
- `design_to_tokens` — export as CSS/Tailwind/JSON

### Icons

- `search_icons` → `fetch_icons` → `insert_icon`
- Popular sets: lucide (outline), mdi (filled), heroicons, tabler

### Analyze design

- `analyze_colors`, `analyze_spacing`, `analyze_typography`
- `analyze_clusters` — find repeated patterns
- `design_to_component_map` — component architecture

### Export & verify

- `export_image` (PNG/JPG/WEBP) or `export_svg`
- `diff_create`, `diff_jsx`, `diff_show` — compare nodes

### Canvas management

- `arrange` — tidy overlapping nodes (grid/row/column)
- `viewport_set` / `viewport_zoom_to_fit`

## Key Patterns

### Use `render` for complex creation

Prefer `render` with JSX over multiple `create_shape` calls. One render builds
an entire subtree. See [references/jsx-patterns.md](references/jsx-patterns.md)
for a cookbook of reusable patterns.

```jsx
<Frame name="Card" w={320} h="hug" flex="col" gap={16} p={24} bg="#FFF" rounded={16}>
  <Text size={18} weight="bold">Title</Text>
  <Rectangle w="fill" h={1} fill="#E0E0E0" />
</Frame>
```

### Use `batch_update` for bulk changes

```json
[{"id":"0:5","props":{"spacing":16}},{"id":"0:6","props":{"sizing_horizontal":"FILL","grow":1}}]
```

### Use `calc` for arithmetic

Always use `calc` instead of mental math for positioning/sizing calculations.

### Use `node_replace_with` for in-place edits

Replaces a node while preserving its position in the parent — ideal for
swapping skeleton placeholders with real content.

## Reference Files

### [references/tool-reference.md](references/tool-reference.md)

Complete parameter tables for ALL MCP tools. Load when you need exact
parameter names, types, defaults, or enum values for any tool call. Organized
by category: Reading, CRUD, Styling, Layout, Text, Components, Variables,
Paths, Booleans, Icons, Export, Analysis, Viewport, Utilities.

### [references/jsx-patterns.md](references/jsx-patterns.md)

Cookbook of battle-tested JSX patterns for `render` and `node_replace_with`.
Covers: frames, cards, lists, forms, navigation, responsive layouts,
typography hierarchies, and component patterns. Load when building UI
structures.

### [references/cli-reference.md](references/cli-reference.md)

Full reference for the `openpencil` CLI tool. Covers: export (PNG/JPG/WEBP/
SVG/JSX/fig), analyze (colors/typography/spacing/clusters), lint, query
(XPath), convert, eval, and headless file operations. Load when operating on
files without the app, or for CI/batch workflows.
