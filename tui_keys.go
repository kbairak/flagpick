// This file routes key events and implements the run/copy/print actions.
package main

import (
	"reflect"
	"strconv"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
)

// ctrlCSIKeys maps a ctrl+<letter> CSI-u codepoint to the legacy key type it
// stands for, so modified keys keep working when a terminal reports them as
// escape sequences.
var ctrlCSIKeys = map[rune]tea.KeyType{
	'c': tea.KeyCtrlC,
	'd': tea.KeyCtrlD,
	'h': tea.KeyCtrlH,
	'n': tea.KeyCtrlN,
	'p': tea.KeyCtrlP,
	'r': tea.KeyCtrlR,
	'u': tea.KeyCtrlU,
	'w': tea.KeyCtrlW,
	'x': tea.KeyCtrlX,
	'y': tea.KeyCtrlY,
}

// handleCSIKey decodes a CSI-u key event (kitty keyboard protocol), which is
// how modified keys like ctrl+shift+j/k arrive. It reports whether the event
// was recognized.
func (m *model) handleCSIKey(msg tea.Msg) (tea.Cmd, bool) {
	rv := reflect.ValueOf(msg)
	if rv.Kind() != reflect.Slice || rv.Type().Elem().Kind() != reflect.Uint8 {
		return nil, false
	}
	b := trimCSIPrefix(rv.Bytes())
	if len(b) == 0 || b[len(b)-1] != 'u' {
		return nil, false
	}
	parts := strings.Split(string(b[:len(b)-1]), ";")
	cp, err := strconv.Atoi(parts[0])
	if err != nil {
		return nil, false
	}
	mods := 0
	if len(parts) > 1 {
		mods, _ = strconv.Atoi(parts[1])
	}
	bits := mods - 1
	// Ambiguous special keys also come through as CSI-u.
	switch cp {
	case 27:
		return m.dispatchKey(tea.KeyEscape)
	case 13:
		return m.dispatchKey(tea.KeyEnter)
	case 9:
		return m.dispatchKey(tea.KeyTab)
	case 127:
		return m.dispatchKey(tea.KeyBackspace)
	}
	if bits&4 == 0 { // require ctrl
		return nil, false
	}
	shift := bits&1 != 0
	if bits&2 != 0 { // alt/modified beyond ctrl(+shift): ignore
		return nil, false
	}
	switch cp {
	case 'k', 'K':
		if shift {
			m.reorder(-1)
		} else {
			m.moveCursor(-1)
		}
		return nil, true
	case 'j', 'J':
		if shift {
			m.reorder(1)
		} else {
			m.moveCursor(1)
		}
		return nil, true
	case 'x', 'X':
		if shift {
			m.clearAll()
		} else {
			m.clearSelected()
		}
		return nil, true
	case 'y', 'Y':
		if shift {
			return m.copyStay(), true
		}
		return m.copyExit(), true
	}
	if kt, ok := ctrlCSIKeys[rune(cp)]; ok {
		return m.dispatchKey(kt)
	}
	return nil, false
}

// dispatchKey routes a synthesized key through the main handler.
func (m *model) dispatchKey(kt tea.KeyType) (tea.Cmd, bool) {
	updated, cmd := m.handleKey(tea.KeyMsg{Type: kt})
	if mm, ok := updated.(model); ok {
		*m = mm
	}
	return cmd, true
}

// trimCSIPrefix removes a leading ESC [ from an unknown CSI sequence.
func trimCSIPrefix(b []byte) []byte {
	if len(b) >= 2 && b[0] == 0x1b && b[1] == '[' {
		return b[2:]
	}
	return b
}

// handlePickerKey routes keys to the filesystem picker: printable runes fuzzy
// filter entries, enter selects, l opens a directory, h/backspace go up, esc
// cancels.
func (m model) handlePickerKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()
	switch key {
	case "esc":
		m.screen = screenMain
		return m, nil
	case "enter":
		if path, ok := m.picker.selectedPath(); ok {
			m.commitPath(path)
			m.screen = screenMain
		}
		return m, nil
	case "right", "l":
		m.picker.descend()
		return m, nil
	case "left", "h":
		m.picker.parent()
		return m, nil
	case "up", "ctrl+k", "ctrl+p":
		m.picker.move(-1)
		return m, nil
	case "down", "ctrl+j", "ctrl+n":
		m.picker.move(1)
		return m, nil
	case "backspace", "ctrl+h":
		if !m.picker.backspace() {
			m.picker.parent()
		}
		return m, nil
	}
	if msg.Type == tea.KeyRunes {
		m.picker.pushFilter(string(msg.Runes))
		return m, nil
	}
	if msg.Type == tea.KeySpace {
		m.picker.pushFilter(" ")
		return m, nil
	}
	return m, nil
}

func (m model) handleKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	key := msg.String()

	switch m.screen {
	case screenHelp:
		m.screen = screenMain
		return m, nil
	case screenConfirm:
		return m.handleConfirmKey(msg)
	case screenAction:
		return m.handleActionKey(msg)
	case screenEnum:
		return m.handleEnumKey(msg)
	case screenSub:
		return m.handleSubKey(msg)
	}

	if m.editing {
		switch key {
		case "enter":
			m.commitEdit()
			m.editing = false
			m.input.Blur()
			if m.editingIndex < 0 && m.isVariadic(m.editingName) {
				m.setCursor(m.rowIndexOfKind(m.editingName, rowAdd))
			} else {
				m.setCursor(m.rowIndexOf(m.editingName) + 1)
			}
			return m, nil
		case "esc":
			m.editing = false
			m.input.Blur()
			return m, nil
		default:
			if m.isIntFlag(m.editingName) && !isDigitInput(msg) {
				return m, nil
			}
			var cmd tea.Cmd
			m.input, cmd = m.input.Update(msg)
			return m, cmd
		}
	}

	m.errMsg = ""
	m.statusMsg = ""

	switch key {
	case "ctrl+c":
		m.action = actionNone
		return m, tea.Quit
	case "esc":
		if m.filter != "" {
			m.filter = ""
			m.cursor = 0
			m.descScroll = 0
			return m, nil
		}
		m.screen = screenConfirm
		m.confirm.sel = 1
		return m, nil
	case "ctrl+j", "down":
		m.moveCursor(1)
		return m, nil
	case "ctrl+k", "up":
		m.moveCursor(-1)
		return m, nil
	case "ctrl+d":
		m.moveCursorClamped(halfPage(m.pageSize()))
		return m, nil
	case "ctrl+u":
		m.moveCursorClamped(-halfPage(m.pageSize()))
		return m, nil
	case "ctrl+x":
		m.clearSelected()
		return m, nil
	case "ctrl+shift+x":
		m.clearAll()
		return m, nil
	case "pgdown":
		m.descScroll += m.pageSize()
		return m, nil
	case "pgup":
		m.descScroll -= m.pageSize()
		return m, nil
	// Reorder. ctrl-shift-j/k is preferred; terminals that don't emit a
	// distinct sequence for it fall back to ctrl+up/down.
	case "ctrl+shift+k", "ctrl+up":
		m.reorder(-1)
		return m, nil
	case "ctrl+shift+j", "ctrl+down":
		m.reorder(1)
		return m, nil
	case "enter":
		m.screen = screenAction
		m.actions.sel = 0
		return m, nil
	case " ":
		return m, m.activate()
	case "ctrl+r":
		return m, m.runCommand()
	case "ctrl+y":
		return m, m.copyCommand(false)
	case "ctrl+shift+y":
		return m, m.copyCommand(true)
	case "ctrl+p":
		return m, m.printCommand()
	case "ctrl+?", "ctrl+/":
		m.screen = screenHelp
		return m, nil
	case "tab":
		m.cycleMode(1)
		return m, nil
	case "shift+tab":
		m.cycleMode(-1)
		return m, nil
	case "backspace", "ctrl+h":
		if m.filter != "" {
			m.filter = trimLastRune(m.filter)
			m.cursor = 0
			m.descScroll = 0
		}
		return m, nil
	case "ctrl+w":
		m.filter = trimLastWord(m.filter)
		m.cursor = 0
		m.descScroll = 0
		return m, nil
	}

	if msg.Type == tea.KeyRunes {
		m.filter += string(msg.Runes)
		m.cursor = 0
		m.descScroll = 0
		return m, nil
	}
	return m, nil
}

// runCommand validates and quits to exec the command.
func (m *model) runCommand() tea.Cmd {
	if !m.validate() {
		return nil
	}
	m.action = actionRun
	return tea.Quit
}

// copyCommand validates and copies the composed command. On failure it stays
// open with an error; on success it sets actionCopy and, unless stay is set,
// quits.
func (m *model) copyCommand(stay bool) tea.Cmd {
	if !m.validate() {
		return nil
	}
	if err := copyFn(ShellQuote(append([]string{m.cmd}, Assemble(m.res(), m.session)...))); err != nil {
		m.errMsg = "copy: " + err.Error()
		return nil
	}
	m.statusMsg = "copied"
	m.action = actionCopy
	if stay {
		return nil
	}
	return tea.Quit
}

// printCommand validates and quits to print the command.
func (m *model) printCommand() tea.Cmd {
	if !m.validate() {
		return nil
	}
	m.action = actionPrint
	return tea.Quit
}

// handleEnumKey drives the enum value menu (up/down choose, enter selects,
// esc cancels). Printable runes fuzzy-filter the values.
func (m model) handleEnumKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = screenMain
		return m, nil
	case "up", "ctrl+k":
		if m.enum.sel > 0 {
			m.enum.sel--
		}
		return m, nil
	case "down", "ctrl+j":
		if m.enum.sel < len(m.filteredEnum())-1 {
			m.enum.sel++
		}
		return m, nil
	case "backspace", "ctrl+h":
		m.enum.filter = trimLastRune(m.enum.filter)
		m.enum.sel = 0
		return m, nil
	case "ctrl+w":
		m.enum.filter = trimLastWord(m.enum.filter)
		m.enum.sel = 0
		return m, nil
	case "enter":
		if f := m.filteredEnum(); m.enum.sel < len(f) {
			m.commitEnum(f[m.enum.sel])
		}
		m.screen = screenMain
		m.setCursor(m.rowIndexOf(m.enum.name))
		return m, nil
	case "ctrl+c":
		m.action = actionNone
		return m, tea.Quit
	}
	if msg.Type == tea.KeyRunes {
		m.enum.filter += string(msg.Runes)
		m.enum.sel = 0
	}
	if msg.Type == tea.KeySpace {
		m.enum.filter += " "
		m.enum.sel = 0
	}
	return m, nil
}

// handleSubKey drives the subcommand menu (up/down choose, enter selects, esc
// cancels). Printable runes fuzzy-filter the subcommands. Choosing one
// switches the active path and keeps the state the new path still accepts.
func (m model) handleSubKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = screenMain
		return m, nil
	case "up", "ctrl+k":
		if m.sub.sel > 0 {
			m.sub.sel--
		}
		return m, nil
	case "down", "ctrl+j":
		if m.sub.sel < len(m.filteredSub())-1 {
			m.sub.sel++
		}
		return m, nil
	case "backspace", "ctrl+h":
		m.sub.filter = trimLastRune(m.sub.filter)
		m.sub.sel = 0
		return m, nil
	case "ctrl+w":
		m.sub.filter = trimLastWord(m.sub.filter)
		m.sub.sel = 0
		return m, nil
	case "enter":
		if f := m.filteredSub(); m.sub.sel < len(f) {
			m.mode = f[m.sub.sel]
		}
		m.screen = screenMain
		m.reconcileMode()
		m.filter = ""
		m.cursor = 0
		m.descScroll = 0
		return m, nil
	case "ctrl+c":
		m.action = actionNone
		return m, tea.Quit
	}
	if msg.Type == tea.KeyRunes {
		m.sub.filter += string(msg.Runes)
		m.sub.sel = 0
	}
	if msg.Type == tea.KeySpace {
		m.sub.filter += " "
		m.sub.sel = 0
	}
	return m, nil
}

// handleActionKey drives the run/copy/print/cancel popup.
func (m model) handleActionKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = screenMain
		return m, nil
	case "up", "ctrl+k", "k":
		if m.actions.sel > 0 {
			m.actions.sel--
		}
		return m, nil
	case "down", "ctrl+j", "ctrl+n", "j":
		if m.actions.sel < 4 {
			m.actions.sel++
		}
		return m, nil
	case "enter":
		switch m.actions.sel {
		case 0:
			cmd := m.runCommand()
			if cmd == nil {
				m.screen = screenMain
			}
			return m, cmd
		case 1:
			return m, m.copyExit()
		case 2:
			return m, m.copyStay()
		case 3:
			cmd := m.printCommand()
			if cmd == nil {
				m.screen = screenMain
			}
			return m, cmd
		default:
			m.screen = screenMain
			return m, nil
		}
	case "ctrl+r":
		m.screen = screenMain
		return m, m.runCommand()
	case "ctrl+y":
		return m, m.copyExit()
	case "ctrl+shift+y":
		return m, m.copyStay()
	case "ctrl+p":
		m.screen = screenMain
		return m, m.printCommand()
	case "ctrl+c":
		m.action = actionNone
		return m, tea.Quit
	}
	return m, nil
}

// copyExit copies and, on success, quits.
func (m *model) copyExit() tea.Cmd {
	cmd := m.copyCommand(false)
	if cmd == nil {
		// Copy failed (stay with error) or validation failed.
		if m.errMsg == "" {
			m.screen = screenMain
		}
		return nil
	}
	m.screen = screenMain
	return cmd
}

// copyStay copies and stays open.
func (m *model) copyStay() tea.Cmd {
	cmd := m.copyCommand(true)
	m.screen = screenMain
	return cmd
}

// handleConfirmKey drives the quit-confirmation popup. esc cancels, enter
// activates the focused button (Cancel or Exit), left/right/tab move focus,
// and ctrl+c always exits.
func (m model) handleConfirmKey(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.screen = screenMain
		return m, nil
	case "left", "h":
		m.confirm.sel = 0
		return m, nil
	case "right", "l":
		m.confirm.sel = 1
		return m, nil
	case "tab", "shift+tab":
		m.confirm.sel = 1 - m.confirm.sel
		return m, nil
	case "enter":
		if m.confirm.sel == 0 {
			m.screen = screenMain
			return m, nil
		}
		m.action = actionNone
		return m, tea.Quit
	case "ctrl+c":
		m.action = actionNone
		return m, tea.Quit
	}
	return m, nil
}

// isDigitInput reports whether a key may be applied to an integer field:
// control keys pass through, rune input must be ASCII digits.
func isDigitInput(msg tea.KeyMsg) bool {
	if msg.Type != tea.KeyRunes {
		return true
	}
	for _, r := range msg.Runes {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
