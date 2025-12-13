# Comprehensive Guide to `github.com/rivo/tview`

> NOTICE: These are reference notes.

`tview` is a Go module for building rich, interactive terminal-based user interfaces (TUIs). It provides a suite of widgets and layout managers, enabling rapid development of data-driven and interactive applications in the terminal. This guide covers its core concepts, key APIs, design patterns, and practical usage, with comparisons to other UI paradigms like ReactJS.

---

## Table of Contents
1. [Overview](#overview)
2. [Core Concepts](#core-concepts)
3. [Widgets & Layouts](#widgets--layouts)
4. [Application Lifecycle](#application-lifecycle)
5. [Styling & Colors](#styling--colors)
6. [Concurrency & Event Handling](#concurrency--event-handling)
7. [Design Patterns & Comparison to ReactJS](#design-patterns--comparison-to-reactjs)
8. [Examples](#examples)
9. [Further Resources](#further-resources)

---

## Overview

`tview` builds on [`tcell`](https://github.com/gdamore/tcell) for terminal rendering and input. It is designed for simplicity, composability, and rapid prototyping, with a focus on data entry and exploration. Widgets are highly customizable and can be combined using layout managers.

## Core Concepts

- **Primitives**: All widgets implement the `Primitive` interface, allowing them to be composed and managed uniformly.
- **Box**: The base type for all widgets, providing border, title, and layout features.
- **Application**: The main event loop and rendering context. All UI code runs within an `Application`.
- **Method Chaining**: Many APIs support chaining, but note that methods from `Box` return `*Box`, breaking chains for subclasses.

## Widgets & Layouts

### Widgets
- **TextView**: Scrollable, multi-colored text display. Supports highlighting and regions.
- **TextArea**: Editable multi-line text input.
- **Table**: Tabular data display, with selectable cells, rows, columns.
- **TreeView**: Hierarchical data display, expandable/collapsible nodes.
- **List**: Navigable list with optional shortcuts.
- **InputField**: Single-line text input.
- **DropDown**: Select from a list of options.
- **Checkbox**: Boolean toggle.
- **Image**: Display images (limited by terminal capabilities).
- **Button**: Clickable action.
- **Form**: Composite widget for data entry (input fields, dropdowns, checkboxes, buttons).
- **Modal**: Centered message window with buttons.

### Layout Managers
- **Grid**: Grid-based layout.
- **Flex**: Flexbox-like layout (row/column, weighted sizing).
- **Pages**: Stack of named pages, for navigation and overlays.
- **Frame**: Decorate primitives with headers/footers.

## Application Lifecycle

```go
app := tview.NewApplication()
box := tview.NewBox().SetBorder(true).SetTitle("Hello, world!")
if err := app.SetRoot(box, true).Run(); err != nil {
    panic(err)
}
```
- Create widgets.
- Set up layout.
- Set the root primitive.
- Run the application (starts event loop).
- Stop the application with `app.Stop()` or Ctrl-C.

## Styling & Colors

- Uses `tcell.Style` and `tcell.Color`.
- Style tags in strings: `[red]Warning[white]!`, `[yellow:red:u]Underlined yellow on red`.
- Hyperlinks: `[:::https://example.com]Click here[:::-]` (if terminal supports).
- Global `Styles` variable for theme customization.

## Concurrency & Event Handling

- Most widget methods are **not thread-safe**. Use `Application.QueueUpdate` or `QueueUpdateDraw` for cross-goroutine updates.
- Key event callbacks run in the main goroutine.
- `TextView` implements `io.Writer` and is safe for concurrent writes.
- Mouse support: `app.EnableMouse(true)`.

## Design Patterns & Comparison to ReactJS

### tview
- **Imperative**: UI is built by instantiating and configuring widgets directly.
- **Event-driven**: Handlers are set via methods (e.g., `SetDoneFunc`, `SetSelectedFunc`).
- **Compositional**: Layout managers (Flex, Grid, Pages) allow nesting and arrangement of widgets.
- **Stateful**: Widgets maintain internal state; updates are performed via method calls.

### ReactJS
- **Declarative**: UI is described as a function of state; re-rendering is automatic on state change.
- **Component-based**: Components encapsulate logic and rendering.
- **Virtual DOM**: Efficient diffing and updates.
- **Unidirectional data flow**: State flows down, events flow up.

#### Comparison
- tview's imperative style is closer to classic desktop UI toolkits (e.g., Qt, WinForms) than React's declarative paradigm.
- State management and updates are manual in tview; in React, state changes trigger re-renders.
- tview's layout managers (Flex, Grid) are conceptually similar to CSS Flexbox/Grid, but configured via Go code.
- tview is optimized for terminal UIs, not graphical or web UIs.

## Examples

### Hello World
```go
package main
import "github.com/rivo/tview"
func main() {
    box := tview.NewBox().SetBorder(true).SetTitle("Hello, world!")
    tview.NewApplication().SetRoot(box, true).Run()
}
```

### Table
```go
package main
import (
    "github.com/gdamore/tcell/v2"
    "github.com/rivo/tview"
)
func main() {
    app := tview.NewApplication()
    table := tview.NewTable().SetBorders(true)
    table.SetCell(0, 0, tview.NewTableCell("Header").SetTextColor(tcell.ColorYellow).SetAlign(tview.AlignCenter))
    table.SetCell(1, 0, tview.NewTableCell("Row 1").SetTextColor(tcell.ColorWhite))
    if err := app.SetRoot(table, true).Run(); err != nil {
        panic(err)
    }
}
```

### Form
```go
package main
import "github.com/rivo/tview"
func main() {
    app := tview.NewApplication()
    form := tview.NewForm().
        AddInputField("Name", "", 20, nil, nil).
        AddCheckbox("Subscribe", false, nil).
        AddButton("Submit", func() { app.Stop() })
    form.SetBorder(true).SetTitle("Form Example")
    if err := app.SetRoot(form, true).Run(); err != nil {
        panic(err)
    }
}
```

### Flex Layout
```go
package main
import "github.com/rivo/tview"
func main() {
    app := tview.NewApplication()
    flex := tview.NewFlex().
        AddItem(tview.NewBox().SetBorder(true).SetTitle("Left"), 0, 1, false).
        AddItem(tview.NewBox().SetBorder(true).SetTitle("Right"), 0, 1, false)
    if err := app.SetRoot(flex, true).Run(); err != nil {
        panic(err)
    }
}
```

### TreeView
```go
package main
import "github.com/rivo/tview"
func main() {
    app := tview.NewApplication()
    root := tview.NewTreeNode("Root").
        AddChild(tview.NewTreeNode("Child 1")).
        AddChild(tview.NewTreeNode("Child 2"))
    tree := tview.NewTreeView().SetRoot(root).SetCurrentNode(root)
    if err := app.SetRoot(tree, true).Run(); err != nil {
        panic(err)
    }
}
```

## Further Resources
- [GitHub Wiki](https://github.com/rivo/tview/wiki)
- [Demos Directory](https://github.com/rivo/tview/tree/main/demos)
- [API Documentation](https://pkg.go.dev/github.com/rivo/tview)
- [Projects using tview](https://github.com/rivo/tview#projects-using-tview)

---

This guide provides a practical foundation for building terminal UIs with tview. For more advanced usage, consult the demos and API docs.