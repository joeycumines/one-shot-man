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
  batch-modifying nodes, linting, or any Figma/design operation.
  Also activates for: "design",
  "figma", "openpencil", "component", "design token", "canvas", "frame",
  "auto-layout", "JSX render", "export design", "design system",
  "UI mockup", "wireframe", "prototype", "design layout".
---

# OpenPencil / Figma Design

Manipulate Figma-compatible design documents through MCP tools (live app) and
the `openpencil` CLI (headless file operations).

## Prerequisites

Both `openpencil-mcp` and `openpencil` must already be installed and in PATH.
Do not attempt to install them — if either is missing, inform the user and stop.

- `openpencil-mcp` — MCP server providing ~90 design tools via stdio
- `openpencil` — CLI for headless file operations (export, lint, analyze,
  query, convert)

## Choosing Between MCP and CLI

The tools split into two families. Pick the right one based on context — don't
mix them unless you have a specific reason (e.g., exporting from a live session
to disk).

| Mode | When to use | Tools |
|------|-------------|-------|
| **Live (MCP)** | OpenPencil app is running with a document open. Use for interactive design work — creating, editing, inspecting nodes in real time. | All `mcp__openpencil-mcp__*` tools |
| **Headless (CLI)** | No app running, or operating on `.fig`/`.pen` files directly. Use for batch export, linting, CI/CD, or analysis without the app. | `openpencil` CLI via Bash |

Prefer MCP when available — it has a richer toolset and gives real-time
feedback. Fall back to CLI for file-based automation or when the app isn't
connected.

## Workflows (MCP)

> These workflows use MCP tools (live app connected). For headless/CLI
> equivalents, see [references/cli-reference.md](references/cli-reference.md).

### Design Creation Workflow

Building something new? Follow this sequence. Each step exists because skipping
it leads to predictable problems — mismatched colors, orphaned spacing values,
or designs that look wrong because nobody checked.

1. **Orient** — Understand the document before changing it.
   Understanding what already exists prevents you from duplicating work or
   clashing with established patterns.
   - `get_current_page` → `get_page_tree` to see structure
   - `describe(id)` on existing nodes to understand context
   - `analyze_colors` + `analyze_typography` to learn existing patterns
   - If variables exist: `list_variables` to find established tokens
2. **Plan** — Decide the approach before touching the canvas.
   Jumping straight to building wastes time on rework. Decide colors, spacing,
   and component strategy first.
   - Choose components to build (reference existing via `get_components`)
   - Select colors from existing palette or define new tokens
   - Pick a spacing scale consistent with the project's design context
   - Use `calc` for all sizing/positioning arithmetic
3. **Execute** — Build using `render` for multi-node structures.
   `render` with JSX is the most efficient way to create complex hierarchies —
   one call builds an entire subtree. See [references/jsx-patterns.md](references/jsx-patterns.md).
   - Use JSX via `render` for complex hierarchies
   - Use `batch_update` for bulk property changes
   - Bind colors/sizes to variables for consistency
   - Name layers descriptively (match code names when applicable)
4. **Validate** — Check quality before declaring done.
   Automated analysis catches things the eye misses — rogue colors, inconsistent
   spacing, font sprawl.
   - `export_image` → visually verify the result
   - `analyze_spacing` → confirm spacing consistency
   - `analyze_colors` → confirm palette discipline (no rogue colors)
   - `analyze_typography` → confirm font consistency
5. **Fix** — Address any issues found in validation.
   - Fix violations, then return to step 4
   - Repeat until validation passes cleanly

### Design Audit Workflow

Assessing an existing design for consistency and improvement opportunities.

1. **Inventory** — Catalog what exists.
   - `get_page_tree` → `describe` key frames
   - `design_to_component_map` to see component architecture
   - `list_variables` to see token coverage
2. **Analyze** — Run all analysis tools.
   - `analyze_colors(show_similar=true)` → find redundant colors
   - `analyze_spacing` → find inconsistent spacing
   - `analyze_typography` → find font sprawl
   - `analyze_clusters` → find repeated patterns that should be components
3. **Report** — Present findings as candidates, not mandates.
   Some inconsistencies are intentional. List what you found and let the user
   decide what to fix.
   - List color, spacing, and font inconsistencies
   - Identify candidate components from clusters
   - Note missing variable bindings
   - **Ask the user** which findings to act on
4. **Fix** — Apply user-approved corrections.
   - Unify colors: `set_fill`/`set_stroke` for direct fixes, or `bind_variable` to link to tokens
   - Normalize spacing via `set_layout` / `batch_update`
   - Componentize repeated patterns: promote exemplar with `create_component`, then use `node_bounds` + `create_instance` + `delete_node` to swap duplicates
5. **Verify** — Re-run analysis to confirm fixes.
   - Repeat steps 2-4 for any remaining user-approved items

### Design-to-Code Workflow

When generating frontend code from a design, start by calling `get_codegen_prompt`
to get the project's code generation guidelines. These guidelines tell you how to
map design tokens to code constructs, which framework conventions to follow, and
what output format to use. Without them you'd be guessing at mappings.

1. **Get guidelines** — `get_codegen_prompt` → read and follow
2. **Extract structure** — `get_jsx(id)` for component hierarchies
3. **Extract tokens** — `design_to_tokens(format)` for colors, spacing, etc.
4. **Generate code** — following the guidelines from step 1

## Quick Reference by Task

### Orient in the document

```
get_current_page → get_page_tree → get_node(id, depth) → drill down
get_selection                    — user's current selection
describe(id)                     — semantic summary
describe(ids=[...])              — batch describe multiple nodes
list_pages / switch_page         — navigate pages
```

### Create nodes

- `create_shape` — frames, rectangles, ellipses, text, lines, sections
- `render` — JSX for multi-node hierarchies (most efficient for complex builds)
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
- `list_available_fonts` — check what's available on the system

### Components

- `create_component` → `create_instance` to place
- `node_to_component(ids=[...])` — batch convert frames/groups
- `get_components` — find existing ones
- `design_to_component_map` — decomposition analysis

### Variables (design tokens)

- `create_collection` → `create_variable` → `bind_variable`
- `design_to_tokens` — export as CSS/Tailwind/JSON

### Icons

- `search_icons(queries=[...])` → `fetch_icons(names=[...])` → `insert_icon`
- Batch insert: `insert_icon(names=[...], parent_id=...)`
- Popular sets: lucide (outline), mdi (filled), heroicons, tabler

### Analyze design

- `analyze_colors`, `analyze_spacing`, `analyze_typography`
- `analyze_clusters` — find repeated patterns
- `design_to_component_map` — component architecture

### Export & verify

- `export_image(ids=[...])` (PNG/JPG/WEBP) or `export_svg(ids=[...])`
- Omit `ids` to export all top-level nodes on the current page
- `diff_create`, `diff_jsx`, `diff_show` — compare nodes

### Design to code

- `get_codegen_prompt` — get project-specific code generation guidelines
- `get_jsx(id)` — get JSX representation of a node tree
- `design_to_tokens(format)` — extract tokens as CSS/Tailwind/JSON

### Canvas management

- `arrange(ids=[...])` — tidy overlapping nodes (grid/row/column)
- `viewport_set` / `viewport_zoom_to_fit(ids=[...])`

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

Use `calc` instead of mental math for positioning/sizing calculations. It
supports single expressions or arrays for parallel evaluation.

### Use `node_replace_with` for in-place edits

Replaces a node while preserving its position in the parent — ideal for
swapping skeleton placeholders with real content.

### Batch operations with arrays

Several tools accept `ids` arrays for batch operations. Use these to avoid
multiple sequential calls:
- `select_nodes(ids=[...])` — select multiple nodes
- `group_nodes(ids=[...])` — group specific nodes
- `flatten_nodes(ids=[...])` — flatten specific nodes
- `export_image(ids=[...])` / `export_svg(ids=[...])` — export specific nodes

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
