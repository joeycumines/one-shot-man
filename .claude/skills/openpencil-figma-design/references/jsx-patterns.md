# JSX Patterns for `render` / `node_replace_with`

Cookbook of reusable JSX patterns for building design nodes efficiently.

## Table of Contents

- [JSX Attribute Reference](#jsx-attribute-reference)
- [Layout Primitives](#layout-primitives)
- [Common Components](#common-components)
- [Responsive Patterns](#responsive-patterns)
- [Typography](#typography)
- [Icons](#icons)
- [Tips](#tips)

---

## JSX Attribute Reference

| Attribute | Values | Notes |
|-----------|--------|-------|
| `w` / `h` | number, `"hug"`, `"fill"` | `"hug"` = fit content, `"fill"` = stretch to parent |
| `flex` | `"col"`, `"row"` | `"col"` = VERTICAL, `"row"` = HORIZONTAL |
| `gap` | number | Space between children |
| `p` | number | Padding (all sides) |
| `px` / `py` | number | Horizontal/vertical padding |
| `bg` | hex color | Background fill |
| `fill` | hex color | Fill color for shapes/text |
| `stroke` | hex color | Stroke color |
| `rounded` | number | Corner radius |
| `size` | number | Font size (Text only) |
| `weight` | string/number | Font weight: `"bold"`, `"regular"`, or 100-900 |
| `name` | string | Layer name in panel |
| `grow` | number | Flex grow (0 or 1) |

---

## Layout Primitives

### Column layout

```jsx
<Frame name="Col" w={320} h="hug" flex="col" gap={12} p={16} bg="#FFFFFF">
  {/* children */}
</Frame>
```

### Row layout

```jsx
<Frame name="Row" w="fill" h="hug" flex="row" gap={8} p={0}>
  {/* children */}
</Frame>
```

### Centered content

```jsx
<Frame name="Centered" w={400} h={300} flex="col" gap={0} bg="#F5F5F5">
  {/* set counter_align via set_layout after creation if needed */}
</Frame>
```

---

## Common Components

### Card with title + body + action

```jsx
<Frame name="Card" w={320} h="hug" flex="col" gap={12} p={20} bg="#FFFFFF" rounded={12}>
  <Text size={16} weight="bold" fill="#1A1A1A">Card Title</Text>
  <Text size={14} fill="#666666">Body text goes here. Keep it concise.</Text>
  <Frame name="Actions" w="fill" h="hug" flex="row" gap={8}>
    <Frame name="BtnPrimary" w="hug" h="hug" flex="row" p={8} px={16} bg="#0066FF" rounded={6}>
      <Text size={14} weight="bold" fill="#FFFFFF">Action</Text>
    </Frame>
    <Frame name="BtnGhost" w="hug" h="hug" flex="row" p={8} px={16} bg="#00000000" rounded={6}>
      <Text size={14} fill="#0066FF">Cancel</Text>
    </Frame>
  </Frame>
</Frame>
```

### List item

```jsx
<Frame name="ListItem" w="fill" h="hug" flex="row" gap={12} p={12} bg="#FFFFFF">
  <Ellipse name="Avatar" w={40} h={40} fill="#E0E0E0" />
  <Frame name="Content" w="fill" h="hug" flex="col" gap={4}>
    <Text size={14} weight="bold" fill="#1A1A1A">Item Title</Text>
    <Text size={12} fill="#999999">Description text</Text>
  </Frame>
  <Text size={12} fill="#CCCCCC">3m</Text>
</Frame>
```

### Input field

```jsx
<Frame name="InputField" w="fill" h="hug" flex="col" gap={6}>
  <Text size={12} weight="bold" fill="#666666">Label</Text>
  <Frame name="Input" w="fill" h={40} flex="row" p={8} px={12} bg="#FFFFFF" rounded={6}>
    <Text size={14} fill="#999999">Placeholder text...</Text>
  </Frame>
</Frame>
```

### Navigation bar

```jsx
<Frame name="NavBar" w="fill" h={56} flex="row" gap={0} p={0} px={16} bg="#FFFFFF">
  <Text size={18} weight="bold" fill="#1A1A1A">App Name</Text>
  <Frame name="Spacer" w="fill" h={1} />
  <Frame name="NavItems" w="hug" h="hug" flex="row" gap={24}>
    <Text size={14} fill="#666666">Home</Text>
    <Text size={14} fill="#0066FF" weight="bold">Dashboard</Text>
    <Text size={14} fill="#666666">Settings</Text>
  </Frame>
</Frame>
```

### Section with header

```jsx
<Frame name="Section" w="fill" h="hug" flex="col" gap={16} p={24} bg="#F8F8F8" rounded={16}>
  <Text size={20} weight="bold" fill="#1A1A1A">Section Title</Text>
  <Text size={14} fill="#666666">Section description or subtitle.</Text>
  {/* content */}
</Frame>
```

### Status badge

```jsx
<Frame name="Badge" w="hug" h="hug" flex="row" p={4} px={10} bg="#E8F5E9" rounded={12}>
  <Text size={11} weight="bold" fill="#2E7D32">Active</Text>
</Frame>
```

### Divider

```jsx
<Rectangle name="Divider" w="fill" h={1} fill="#E0E0E0" />
```

### Empty spacer

```jsx
<Frame name="Spacer" w="fill" h={1} />
```

---

## Responsive Patterns

### Two-column grid (manual)

```jsx
<Frame name="Grid2Col" w="fill" h="hug" flex="row" gap={16}>
  <Frame name="Col1" w="fill" h="hug" flex="col" gap={16} grow={1}>
    {/* left content */}
  </Frame>
  <Frame name="Col2" w="fill" h="hug" flex="col" gap={16} grow={1}>
    {/* right content */}
  </Frame>
</Frame>
```

### Sidebar + main content

```jsx
<Frame name="Layout" w="fill" h="hug" flex="row" gap={0}>
  <Frame name="Sidebar" w={240} h="hug" flex="col" gap={8} p={16} bg="#1A1A2E">
    {/* sidebar items */}
  </Frame>
  <Frame name="Main" w="fill" h="hug" flex="col" gap={16} p={24} bg="#FFFFFF" grow={1}>
    {/* main content */}
  </Frame>
</Frame>
```

---

## Typography

### Monospace (for TUI / terminal UI designs)

All text nodes same size, same font. Set font family via `set_font` after
creation since JSX attributes don't control font family directly.

```jsx
<Frame name="TUIRow" w={800} h="hug" flex="row" gap={0} p={8} bg="#1E1E1E">
  <Text size={14} fill="#569CD6">const</Text>
  <Text size={14} fill="#9CDCFE"> x </Text>
  <Text size={14} fill="#D4D4D4">= </Text>
  <Text size={14} fill="#CE9178">"hello"</Text>
</Frame>
```

After creation, batch-set all text nodes to monospace:
```
list_available_fonts(family="mono")  → pick one
set_font(id, family="JetBrains Mono", size=14, style="Regular")
```

### Heading hierarchy

```
H1: size=32, weight="bold"
H2: size=24, weight="bold"
H3: size=20, weight="bold"
Body: size=14, weight="regular"
Caption: size=12, fill="#999999"
```

---

## Icons

Icons use `insert_icon` after creating a parent frame — not inline JSX.

```text
Workflow:
1. search_icons(["home", "settings", "user"])  → get icon names
2. fetch_icons(["lucide:home", "lucide:settings", "lucide:user"])  → cache
3. insert_icon(name="lucide:home", parent_id="<frame_id>", size=20, color="#666666")
```

---

## Tips

- Use `render` with `replace_id` to swap skeleton placeholders with real content
  while preserving position in the parent
- Use `batch_update` to modify layout properties of multiple nodes in one call
- Use `calc` for all arithmetic — never mental math
- Set `auto_resize: "HEIGHT"` on text frames for auto-expanding text
- Use `set_effects` for drop shadows on cards (type: DROP_SHADOW, radius: 8,
  offset_y: 2, color: "#00000015")
