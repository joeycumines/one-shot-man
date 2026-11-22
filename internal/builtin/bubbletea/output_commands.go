package bubbletea

import (
	"reflect"

	tea "charm.land/bubbletea/v2"
)

// BubbleTea's renderer-independent terminal commands use private message
// types. Identify them through its public constructors, without private type
// names or reflection into their fields. These constructors only return
// messages (including the three string-carrying commands); none performs I/O.
// Keep this list in step with the execute/executeQuery cases in BubbleTea's
// event loop when updating the pinned dependency.
var terminalOutputTypes = map[reflect.Type]struct{}{
	reflect.TypeOf(tea.RequestBackgroundColor()):      {},
	reflect.TypeOf(tea.RequestForegroundColor()):      {},
	reflect.TypeOf(tea.RequestCursorColor()):          {},
	reflect.TypeOf(tea.RequestCursorPosition()):       {},
	reflect.TypeOf(tea.RequestTerminalVersion()):      {},
	reflect.TypeOf(tea.RequestProgramStatusSupport()): {},
	reflect.TypeOf(tea.RequestCapability("")()):       {},
	reflect.TypeOf(tea.ReadClipboard()):               {},
	reflect.TypeOf(tea.ReadPrimaryClipboard()):        {},
	reflect.TypeOf(tea.SetClipboard("")()):            {},
	reflect.TypeOf(tea.SetPrimaryClipboard("")()):     {},
}

// filterTerminalOutput prevents BubbleTea from generating terminal queries or
// clipboard controls for redirected output. RawMsg and Exec are explicit I/O
// operations and retain their payloads and output destination. Responses and
// application messages also pass through unchanged.
func filterTerminalOutput(_ tea.Model, msg tea.Msg) tea.Msg {
	if _, terminal := terminalOutputTypes[reflect.TypeOf(msg)]; terminal {
		return nil
	}
	return msg
}
