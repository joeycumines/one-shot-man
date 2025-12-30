#!/usr/bin/env osm script

const tea = require('osm:bubbletea');
const lipgloss = require('osm:lipgloss');
const zone = require('osm:bubblezone');
const textareaLib = require('osm:bubbles/textarea');
const viewportLib = require('osm:bubbles/viewport');

// --- Styles ---
const styles = {
    title: lipgloss.newStyle().foreground("#7D56F4").bold(true).marginBottom(1),
    item: lipgloss.newStyle().paddingLeft(2).paddingRight(1),
    selectedItem: lipgloss.newStyle().paddingLeft(1).paddingRight(1).border(lipgloss.normalBorder(), false, false, false, true).borderForeground("#7D56F4").foreground("#FFF"),
    doneItem: lipgloss.newStyle().foreground("#555").strikethrough(true),
    help: lipgloss.newStyle().foreground("#666").marginTop(1).border(lipgloss.normalBorder(), true, false, false, false).borderForeground("#333"),
    button: lipgloss.newStyle().background("#7D56F4").foreground("#FFF").padding(0, 1).bold(true),
};

// --- Logic Helpers ---

function renderTodoList(model) {
    const lines = model.todos.map((todo, idx) => {
        const isSelected = idx === model.selectedIdx;
        const baseStyle = isSelected ? styles.selectedItem : styles.item;

        let prefix = todo.done ? "[x] " : "[ ] ";
        let text = todo.text;

        if (todo.done) {
            text = styles.doneItem.render(text);
        }

        return baseStyle.render(`${prefix}${text}`);
    });

    if (lines.length === 0) {
        return lipgloss.newStyle().foreground("#555").paddingLeft(2).render("No tasks. Press 'a' to add one.");
    }

    return lipgloss.joinVertical(lipgloss.Left, ...lines);
}

function ensureSelectionVisible(model) {
    const top = model.selectedIdx;
    const bottom = top + 1;
    const vpHeight = model.viewport.height();
    const currentOffset = model.viewport.yOffset();

    if (top < currentOffset) {
        model.viewport.setYOffset(top);
    } else if (bottom > currentOffset + vpHeight) {
        model.viewport.setYOffset(bottom - vpHeight);
    }
}

// --- Main Program ---

const program = tea.newModel({
    init: function () {
        const vp = viewportLib.new(0, 0);

        // Initialize Textarea
        const ta = textareaLib.new();
        ta.setPlaceholder("What needs to be done?");
        ta.focus(); // FIXED: Used focus() instead of setFocus(true)
        ta.setHeight(1);
        ta.setShowLineNumbers(false);

        return {
            mode: 'list', // 'list' | 'add'
            todos: [
                {id: 1, text: "Buy milk", done: false},
                {id: 2, text: "Walk the dog", done: true},
                {id: 3, text: "Learn Bubble Tea", done: false}
            ],
            nextId: 4,
            selectedIdx: 0,
            viewport: vp,
            textarea: ta,
            width: 0,
            height: 0,
            headerHeight: 2,
        };
    },

    update: function (msg, model) {
        // --- 1. Global Event Handling ---
        if (msg.type === 'WindowSize') {
            model.width = msg.width;
            model.height = msg.height;

            const headerHeight = lipgloss.height(styles.title.render("Header"));
            const footerHeight = 4;
            const vpHeight = Math.max(0, model.height - headerHeight - footerHeight);

            model.headerHeight = headerHeight;
            model.viewport.setWidth(model.width);
            model.viewport.setHeight(vpHeight);
            model.textarea.setWidth(model.width - 4);

            return [model, null];
        }

        // --- 2. Input Mode Handling ---
        if (model.mode === 'add') {
            if (msg.type === 'Key') {
                if (msg.key === 'esc') {
                    model.mode = 'list';
                    model.textarea.setValue(""); // FIXED: Used setValue("") for safe reset
                    return [model, null];
                }
                if (msg.key === 'enter') {
                    const val = model.textarea.value().trim();
                    if (val) {
                        model.todos.push({id: model.nextId++, text: val, done: false});
                        model.selectedIdx = model.todos.length - 1;
                        ensureSelectionVisible(model);
                    }
                    model.mode = 'list';
                    model.textarea.setValue(""); // FIXED: Used setValue("") for safe reset
                    return [model, null];
                }

                const [newTa, cmd] = model.textarea.update(msg);
                model.textarea = newTa;
                return [model, cmd];
            }
            return [model, null];
        }

        // --- 3. List Mode Handling ---
        if (model.mode === 'list') {
            if (msg.type === 'Key') {
                switch (msg.key) {
                    case 'q':
                    case 'ctrl+c':
                        return [model, tea.quit()];
                    case 'a':
                        model.mode = 'add';
                        model.textarea.focus();
                        return [model, null]; // Removed ambiguous blink cmd
                    case 'up':
                    case 'k':
                        model.selectedIdx = Math.max(0, model.selectedIdx - 1);
                        ensureSelectionVisible(model);
                        break;
                    case 'down':
                    case 'j':
                        model.selectedIdx = Math.min(model.todos.length - 1, model.selectedIdx + 1);
                        ensureSelectionVisible(model);
                        break;
                    case ' ':
                    case 'enter':
                        if (model.todos[model.selectedIdx]) {
                            model.todos[model.selectedIdx].done = !model.todos[model.selectedIdx].done;
                        }
                        break;
                }
            }

            if (msg.type === 'Mouse') {
                if (msg.action === 'press' && msg.button === 'left') {
                    if (zone.inBounds("add-btn", msg)) {
                        model.mode = 'add';
                        model.textarea.focus();
                        return [model, null];
                    }

                    const yRelative = msg.y - model.headerHeight;
                    if (yRelative >= 0 && yRelative < model.viewport.height()) {
                        const clickedRow = yRelative + model.viewport.yOffset();
                        if (clickedRow >= 0 && clickedRow < model.todos.length) {
                            model.selectedIdx = clickedRow;
                            model.todos[model.selectedIdx].done = !model.todos[model.selectedIdx].done;
                        }
                    }
                }

                if (msg.button === 'wheel up') {
                    model.viewport.lineUp(1);
                } else if (msg.button === 'wheel down') {
                    model.viewport.lineDown(1);
                }
            }
        }

        return [model, null];
    },

    view: function (model) {
        const title = styles.title.render("📝 Minimal TODO");

        if (model.mode === 'add') {
            const inputView = lipgloss.joinVertical(lipgloss.Left,
                "Enter new task:",
                "",
                model.textarea.view(),
                "",
                lipgloss.newStyle().foreground("#666").render("Enter: Submit • Esc: Cancel")
            );
            return lipgloss.joinVertical(lipgloss.Left, title, inputView);
        }

        const listContent = renderTodoList(model);
        model.viewport.setContent(listContent);

        const addBtn = zone.mark("add-btn", styles.button.render(" [A]dd New "));
        const helpBar = styles.help.width(model.width).render(
            lipgloss.joinHorizontal(lipgloss.Top,
                addBtn,
                "  ↑/↓: Nav • Space: Toggle • Q: Quit"
            )
        );

        return zone.scan(lipgloss.joinVertical(lipgloss.Left,
            title,
            model.viewport.view(),
            helpBar
        ));
    }
});

tea.run(program, {
    altScreen: true,
    mouse: true
});
