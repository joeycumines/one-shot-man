# OpenPencil CLI Reference

Headless file operations via the `openpencil` CLI. Use when the app is not
running, for CI/CD pipelines, or for batch operations on `.fig` / `.pen` files.

## Table of Contents

- [Overview](#overview)
- [Export](#export)
- [Analyze](#analyze)
- [Lint](#lint)
- [Query](#query)
- [Convert](#convert)
- [Eval](#eval)
- [Inspection](#inspection)
- [Common Patterns](#common-patterns)

---

## Overview

```text
openpencil <command> [FILE] [OPTIONS]
```

Most commands accept an optional `FILE` argument — omit it to connect to the
running OpenPencil app (equivalent to MCP mode). Provide a file path for
headless operation.

---

## Export

```bash
openpencil export [FILE] [OPTIONS]
```

Export a document or specific node to various formats.

| Option | Default | Description |
|--------|---------|-------------|
| `-f, --format` | `png` | `png`, `jpg`, `webp`, `svg`, `jsx`, `fig` |
| `-o, --output` | `<name>.<format>` | Output file path |
| `-s, --scale` | `1` | Export scale multiplier |
| `-q, --quality` | `90` | Quality 0-100 (JPG/WEBP) |
| `--page` | first page | Export specific page by name |
| `--node` | — | Export specific node by ID (excludes `--page`) |
| `--style` | `openpencil` | JSX style: `openpencil` or `tailwind` |
| `--thumbnail` | off | Export page thumbnail |
| `--width` | `1920` | Thumbnail width |
| `--height` | `1080` | Thumbnail height |

### Examples

```bash
# Export current page to PNG at 2x
openpencil export design.fig -f png -s 2 -o output@2x.png

# Export specific node as SVG
openpencil export design.fig --node "0:5" -f svg -o icon.svg

# Export as JSX (Tailwind style)
openpencil export design.fig -f jsx --style tailwind -o component.jsx

# Page thumbnail
openpencil export design.fig --thumbnail -f jpg -q 80

# Convert .fig to .pen
openpencil export design.fig -f fig -o output.fig
```

---

## Analyze

```bash
openpencil analyze <colors|typography|spacing|clusters> [FILE] [OPTIONS]
```

Design token and pattern analysis. Subcommands:

| Subcommand | Output |
|------------|--------|
| `colors` | Color palette usage and frequency |
| `typography` | Font families, sizes, weights |
| `spacing` | Gap/padding values, grid compliance |
| `clusters` | Repeated patterns (potential components) |

---

## Lint

```bash
openpencil lint [FILE] [OPTIONS]
```

Lint design documents for consistency, structure, and accessibility.

| Option | Default | Description |
|--------|---------|-------------|
| `--preset` | `recommended` | `recommended`, `strict`, `accessibility` |
| `--rule` | all | Run specific rule(s) only (repeatable) |
| `--json` | off | Output as JSON |
| `--list-rules` | — | List rules and exit |

### Example

```bash
openpencil lint design.fig --preset accessibility --json
```

---

## Query

```bash
openpencil query [FILE] <SELECTOR> [OPTIONS]
```

Query nodes using XPath selectors.

| Option | Default | Description |
|--------|---------|-------------|
| `--page` | all pages | Page name |
| `--limit` | `1000` | Max results |
| `--json` | off | Output as JSON |

### XPath examples

```bash
openpencil query design.fig "//FRAME"
openpencil query design.fig "//FRAME[@width < 300]"
openpencil query design.fig "//COMPONENT[starts-with(@name, 'Button')]"
openpencil query design.fig "//SECTION/FRAME"
openpencil query design.fig "//TEXT[contains(@text, 'Hello')]"
openpencil query design.fig "//*[@cornerRadius > 0]"
```

### XPath attributes

Node types: `FRAME`, `TEXT`, `RECTANGLE`, `ELLIPSE`, `LINE`, `STAR`,
`POLYGON`, `SECTION`, `GROUP`, `COMPONENT`, `INSTANCE`, `VECTOR`.

Attributes: `name`, `width`, `height`, `x`, `y`, `visible`, `opacity`,
`cornerRadius`, `fontSize`, `fontFamily`, `fontWeight`, `layoutMode`,
`itemSpacing`, `paddingTop/Right/Bottom/Left`, `strokeWeight`, `rotation`,
`locked`, `blendMode`, `text`, `lineHeight`, `letterSpacing`.

---

## Convert

```bash
openpencil convert <FILE> [OPTIONS]
```

Convert a document to another writable format.

| Option | Default | Description |
|--------|---------|-------------|
| `-f, --format` | `fig` | Output format: `fig` |
| `-o, --output` | `<name>.<format>` | Output file path |

---

## Eval

```bash
openpencil eval [FILE] [OPTIONS]
```

Execute JavaScript with full Figma Plugin API access. The `figma` global is
available.

| Option | Description |
|--------|-------------|
| `-c, --code` | JavaScript code to execute |
| `--stdin` | Read code from stdin |
| `-w, --write` | Write changes back to input file |
| `-o, --output` | Write to a different file |
| `--json` | Output as JSON |
| `-q, --quiet` | Suppress output |

### Example

```bash
openpencil eval design.fig -c "
  const page = figma.currentPage;
  const frames = page.findAll(n => n.type === 'FRAME');
  console.log('Frames:', frames.length);
"
```

---

## Inspection

| Command | Description |
|---------|-------------|
| `openpencil info [FILE]` | Pages, node counts, fonts |
| `openpencil pages [FILE]` | List pages |
| `openpencil tree [FILE]` | Print node tree |
| `openpencil node [FILE]` | Show node properties by ID |
| `openpencil find [FILE]` | Find nodes by name or type |
| `openpencil selection` | Get current selection from running app |
| `openpencil variables [FILE]` | List design variables and collections |
| `openpencil formats` | List supported formats |

All support `--json` for machine-readable output.

---

## Common Patterns

### Export all pages as PNG

```bash
openpencil pages design.fig --json | jq -r '.[].name' | \
  while read page; do
    openpencil export design.fig --page "$page" -f png -o "${page}.png"
  done
```

### Audit design tokens

```bash
openpencil analyze colors design.fig
openpencil analyze typography design.fig
openpencil analyze spacing design.fig
```

### Find and export all components

```bash
openpencil query design.fig "//COMPONENT" --json | \
  jq -r '.[].id' | \
  while read id; do
    openpencil export design.fig --node "$id" -f svg -o "comp-${id}.svg"
  done
```

### Lint and fix accessibility issues

```bash
openpencil lint design.fig --preset accessibility --json > issues.json
# Review issues, then fix via eval or MCP
```
