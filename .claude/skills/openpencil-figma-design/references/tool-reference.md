# Figma MCP Tool Reference

Complete parameter reference for all available Figma MCP tools.
Verified against openpencil-mcp v0.11.6.

## Table of Contents

- [Reading & Navigation](#reading--navigation)
- [Node CRUD](#node-crud)
- [Properties & Styling](#properties--styling)
- [Layout (Auto-Layout / Flexbox)](#layout-auto-layout--flexbox)
- [Text](#text)
- [Components & Instances](#components--instances)
- [Variables & Collections](#variables--collections)
- [Paths & Vectors](#paths--vectors)
- [Boolean Operations](#boolean-operations)
- [Icons & Images](#icons--images)
- [Export](#export)
- [Analysis](#analysis)
- [Viewport](#viewport)
- [Utilities](#utilities)

---

## Reading & Navigation

| Tool | Description | Key Params |
|------|-------------|------------|
| `get-node` | Detailed properties by ID. `depth` controls child recursion (0=self, unlimited default). | `id`, `depth?` |
| `get-jsx` | JSX representation of node subtree. Compact round-trip format. | `id` |
| `get-page-tree` | Lightweight hierarchy of current page: id, type, name, size. | — |
| `get-selection` | Currently selected nodes. | — |
| `get-current-page` | Current page name and ID. | — |
| `node-children` | Direct children of a node. | `id` |
| `node-ancestors` | Ancestor chain to page root. | `id`, `depth?` |
| `node-bounds` | Absolute bounding box. | `id` |
| `node-tree` | Node tree with types and hierarchy. | `id`, `depth?` |
| `node-bindings` | Variable bindings for a node. | `id` |
| `find-nodes` | Search by name pattern and/or type. | `name?`, `type?` |
| `query-nodes` | XPath selector across nodes. | `selector`, `page?`, `limit?` (default 1000) |
| `describe` | Semantic description of node(s). Auto-depth adapts to subtree size. | `id` or `ids`, `depth?`, `grid?` (default 8) |
| `list-pages` | All pages in the document. | — |
| `page-bounds` | Bounding box of all objects on current page. | — |

---

## Node CRUD

| Tool | Description | Key Params |
|------|-------------|------------|
| `create-shape` | FRAME (containers/cards), RECTANGLE (solid), ELLIPSE (circles), TEXT (labels), SECTION (page sections), LINE, STAR, POLYGON. | `type`, `x`, `y`, `width`, `height`, `name`, `parent-id?` |
| `create-slice` | Export region on canvas. | `x`, `y`, `width`, `height`, `name`, `parent-id?` |
| `create-vector` | Vector node with path data. | `x`, `y`, `name`, `path`, `fill?`, `stroke?`, `stroke-weight?`, `parent-id?` |
| `clone-node` | Duplicate a node. | `id` |
| `delete-node` | Delete by ID. | `id` |
| `update-node` | Update position, size, opacity, corner radius, visibility, text, font, name. | `id`, plus optional props |
| `rename-node` | Rename in layers panel. | `id`, `name` |
| `node-move` | Move to coordinates. | `id`, `x`, `y` |
| `node-resize` | Resize node. | `id`, `width` (min 1), `height` (min 1) |
| `reparent-node` | Move into different parent. | `id`, `parent-id` |
| `group-nodes` | Group nodes. | `ids` (string array) |
| `ungroup-node` | Ungroup a group. | `id` |
| `flatten-nodes` | Flatten into single vector. | `ids` (string array) |

---

## Properties & Styling

| Tool | Description | Key Params |
|------|-------------|------------|
| `set-fill` | Solid (`color`) or gradient (`gradient` + `color` + `color-end`). | `id`, `color`, `color-end?`, `gradient?` |
| `set-stroke` | Border color, weight, alignment. | `id`, `color`, `weight?` (default 1), `align?` (INSIDE/CENTER/OUTSIDE) |
| `set-stroke-align` | Stroke alignment only. | `id`, `align` |
| `set-effects` | Drop shadow, inner shadow, blur. Single or array. | `id`, `type`, `color?`, `offset-x?`, `offset-y?` (default 4), `radius?` (default 4), `spread?` |
| `set-blend` | Blend mode. | `id`, `mode` (NORMAL, DARKEN, MULTIPLY, SCREEN, OVERLAY, etc.) |
| `set-opacity` | Opacity 0-1. | `id`, `value` |
| `set-radius` | Corner radius (uniform or per-corner). | `id`, `radius?` or `top-left?`, `top-right?`, `bottom-right?`, `bottom-left?` |
| `set-rotation` | Rotation in degrees. | `id`, `angle` |
| `set-locked` | Lock/unlock node. | `id`, `value` (bool) |
| `set-visible` | Show/hide node. | `id`, `value` (bool) |
| `set-constraints` | Resize constraints within parent. | `id`, `horizontal` (MIN/CENTER/MAX/STRETCH/SCALE), `vertical` |
| `set-minmax` | Min/max width and height. | `id`, `min-width?`, `max-width?`, `min-height?`, `max-height?` |
| `set-image-fill` | Image fill from base64 data. | `id`, `image-data`, `scale-mode?` (FILL/FIT/CROP/TILE) |

---

## Layout (Auto-Layout / Flexbox)

| Tool | Description | Key Params |
|------|-------------|------------|
| `set-layout` | Flexbox on a frame: direction, alignment, spacing, padding. | `id`, `direction?` (HORIZONTAL/VERTICAL), `spacing?`, `padding?`, `padding-horizontal?`, `padding-vertical?`, `align?` (MIN/CENTER/MAX/SPACE_BETWEEN), `counter-align?` (MIN/CENTER/MAX/STRETCH), `flow-direction?` (AUTO/LTR/RTL) |
| `set-layout-child` | Child sizing, grow, alignment, absolute positioning. | `id`, `sizing-horizontal?` (FIXED/HUG/FILL), `sizing-vertical?`, `grow?` (0=fixed, 1=grow), `align-self?`, `positioning?` (AUTO/ABSOLUTE) |
| `batch-update` | Multiple updates in one call. Each op is `{id, props}`. Props: spacing, padding, padding_horizontal, padding_vertical, counter_align, sizing_horizontal, sizing_vertical, grow, name, visible, corner_radius, auto_resize, direction. | `operations` (JSON array) |

---

## Text

| Tool | Description | Key Params |
|------|-------------|------------|
| `set-text` | Set text content. | `id`, `text` |
| `set-font` | Font family, size, style for a text node. | `id`, `family`, `size`, `style?` (Bold, Regular, Bold Italic) |
| `set-font-range` | Font props for a character range. | `id`, `start`, `end`, `family?`, `size?`, `style?`, `color?` |
| `set-text-properties` | Alignment, auto-resize, case, decoration, truncation. | `id`, `align-horizontal?`, `align-vertical?`, `auto-resize?`, `direction?`, `text-decoration?` |
| `set-text-resize` | Auto-resize mode. | `id`, `mode` (NONE/WIDTH_AND_HEIGHT/HEIGHT/TRUNCATE) |
| `list-fonts` | Fonts used in current page. | `family?` |
| `list-available-fonts` | All renderable fonts (system + bundled). | `family?` |

---

## Components & Instances

| Tool | Description | Key Params |
|------|-------------|------------|
| `create-component` | Convert frame/group to component. | `id` |
| `create-instance` | Create instance of a component. | `component-id`, `x`, `y` |
| `node-to-component` | Convert one or more frames/groups to components. | `ids` (string array) |
| `get-components` | List all components, optionally filtered. | `name?`, `limit?` (default 50) |

---

## Variables & Collections

| Tool | Description | Key Params |
|------|-------------|------------|
| `create-collection` | New variable collection. | `name` |
| `get-collection` | Collection by ID. | `id` |
| `list-collections` | All collections. | — |
| `delete-collection` | Delete collection and its variables. | `id` |
| `create-variable` | New variable. | `name`, `type` (COLOR/FLOAT/STRING/BOOLEAN), `collection-id`, `value` |
| `get-variable` | Variable by ID. | `id` |
| `set-variable` | Set value for a mode. | `id`, `mode`, `value` |
| `delete-variable` | Delete a variable. | `id` |
| `find-variables` | Search by name. | `query`, `type?` |
| `list-variables` | All variables. | `type?` |
| `bind-variable` | Bind variable to node property (fills, strokes, opacity, width, height). | `node-id`, `field`, `variable-id` |

---

## Paths & Vectors

| Tool | Description | Key Params |
|------|-------------|------------|
| `path-get` | Get vector path data. | `id` |
| `path-set` | Set vector path data (VectorNetwork JSON). | `id`, `path` |
| `path-move` | Move all points by offset. | `id`, `dx`, `dy` |
| `path-scale` | Scale from center. | `id`, `factor` |
| `path-flip` | Flip horizontally or vertically. | `id`, `axis` (horizontal/vertical) |

---

## Boolean Operations

| Tool | Description |
|------|-------------|
| `boolean-union` | Combine multiple nodes. |
| `boolean-subtract` | Subtract second from first. |
| `boolean-intersect` | Intersect multiple nodes. |
| `boolean-exclude` | XOR multiple nodes. |

---

## Icons & Images

| Tool | Description | Key Params |
|------|-------------|------------|
| `search-icons` | Search Iconify by keyword. Multiple queries searched in parallel. | `queries` (string array, e.g. `["heart", "arrow"]`), `limit?` (default 5), `prefix?` (e.g. lucide, mdi) |
| `fetch-icons` | Pre-fetch icons from Iconify into cache. Batch by prefix (one HTTP request per set). Call once with all needed icons, then use insert-icon to place them instantly. Popular sets: lucide, mdi, heroicons, tabler, solar, mingcute, ri. | `names` (string array, e.g. `["lucide:heart", "mdi:star"]`), `size?` (default 24) |
| `insert-icon` | Insert vector icons onto the canvas. Pass `name` for a single icon or `names` for batch insert into the same parent. If already cached by fetch-icons — instant. | `name?` (single icon), `names?` (string array for batch), `size?` (default 24), `color?` (hex, default #000000), `parent-id?` |
| `stock-photo` | Search stock photos and apply to nodes. JSON array for parallel fetch. Only works on leaf shapes (Rectangle/Ellipse). | `requests` (JSON: `[{id, query, index?, orientation?}]`) |

---

## Export

| Tool | Description | Key Params |
|------|-------------|------------|
| `export-image` | Export as PNG/JPG/WEBP. Returns base64. | `ids?` (string array; omit to export all top-level nodes), `format?` (PNG/JPG/WEBP), `scale?` (0.1-4, default 1) |
| `export-svg` | Export as SVG string. | `ids?` (string array; omit to export all top-level nodes) |
| `design-to-tokens` | Extract design tokens as CSS, Tailwind, or JSON. Resolves aliases, handles modes. | `format` (css/tailwind/json), `collection?`, `type?` |

---

## Analysis

| Tool | Description | Key Params |
|------|-------------|------------|
| `analyze-clusters` | Find repeated patterns (potential components). Groups by structure. | `min-count?` (default 2), `min-size?` (default 30), `limit?` (default 20) |
| `analyze-colors` | Color palette usage, frequency, variable bindings, similar clusters. | `limit?` (default 30), `show-similar?`, `threshold?` (default 15) |
| `analyze-spacing` | Spacing values (gap, padding) and grid compliance. | `grid?` (default 8) |
| `analyze-typography` | Font families, sizes, weights, frequencies. | `limit?` (default 30), `group-by?` (family/size/weight) |
| `design-to-component-map` | Component decomposition: variants, props, instance counts, dependencies. | `page?` |

---

## Viewport

| Tool | Description | Key Params |
|------|-------------|------------|
| `viewport-get` | Current viewport position and zoom. | — |
| `viewport-set` | Set viewport position and zoom. | `x`, `y`, `zoom` (min 0.01) |
| `viewport-zoom-to-fit` | Zoom to fit specified nodes. | `ids` (string array) |
| `arrange` | Arrange top-level nodes in grid/row/column. | `mode?` (grid/row/column), `gap?` (default 40), `cols?`, `ids?` (string array; defaults to all top-level children) |

---

## Utilities

| Tool | Description | Key Params |
|------|-------------|------------|
| `calc` | Arithmetic calculator. Use instead of mental math. | `expr` (single or JSON array) |
| `render` | Render JSX to design nodes. | `jsx`, `parent-id?`, `replace-id?`, `x?`, `y?`, `insert-index?` |
| `node-replace-with` | Replace node with JSX content. | `id`, `jsx` |
| `diff-create` | Structural diff between two node trees. | `from`, `to`, `depth?` (default 10) |
| `diff-jsx` | Diff in JSX format showing added/removed/changed. | `from`, `to` |
| `diff-show` | Preview property changes as unified diff. | `id`, `props` (JSON) |
| `switch-page` | Switch to page by name or ID. | `page` |
| `create-page` | Create new page. | `name` |
| `select-nodes` | Select nodes by ID. | `ids` (string array) |
| `save-file` | Save current document to disk. | — |
| `get-codegen-prompt` | Get design-to-code generation guidelines. Call before generating frontend code from a design. | — |

---

## JSX Syntax for `render` / `node-replace-with`

```jsx
<Frame name="Card" w={320} h="hug" flex="col" gap={16} p={24} bg="#FFF" rounded={16}>
  <Text size={18} weight="bold">Title</Text>
  <Rectangle w="fill" h={1} fill="#E0E0E0" />
</Frame>
```

Key attributes:
- `w` / `h`: pixel number, `"hug"`, or `"fill"`
- `flex`: `"col"` (VERTICAL) or `"row"` (HORIZONTAL)
- `gap`, `p` (padding), `bg` (background), `rounded` (corner radius)
- `fill` (fill color), `stroke` (stroke color + weight)
- Use `replace-id` to swap a placeholder node in-place
