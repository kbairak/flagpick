// This file renders the main two-pane view and computes layout.
package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	boxBorder           = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("205"))
	dimStyle            = lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	cursorStyle         = lipgloss.NewStyle().Reverse(true)
	activeStyle         = lipgloss.NewStyle().Background(lipgloss.Color("255")).Foreground(lipgloss.Color("0"))
	errStyle            = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	statusStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	titleStyle          = lipgloss.NewStyle().Bold(true)
	buttonStyle         = lipgloss.NewStyle().Padding(0, 2)
	buttonSelectedStyle = buttonStyle.Background(lipgloss.Color("255")).Foreground(lipgloss.Color("0"))
	previewStyle        = lipgloss.NewStyle().BorderTop(true).BorderStyle(lipgloss.NormalBorder()).BorderForeground(lipgloss.Color("240"))
)

// popupFilterHint is the shared footer hint for the enum and subcommand menus.
const popupFilterHint = "type to filter · ctrl-w word · enter choose · up/down · esc cancel"

func (m model) View() string {
	switch m.screen {
	case screenHelp:
		return m.helpView()
	case screenConfirm:
		return m.confirmView()
	case screenAction:
		return m.actionView()
	case screenEnum:
		return m.enumView()
	case screenSub:
		return m.subView()
	case screenPicker:
		return m.pickerView()
	}

	leftW, rightW := m.paneWidths()
	paneH := m.paneHeight()
	bodyH := m.pageSize()

	list := boxBorder.Width(leftW).Height(paneH).Render(titleStyle.Render("Options") + "\n" + m.renderList(bodyH, leftW))
	desc := boxBorder.Width(rightW).Height(paneH).Render(titleStyle.Render("Description") + "\n" + m.renderDescription(rightW, bodyH))

	panes := lipgloss.JoinHorizontal(lipgloss.Top, list, " ", desc)

	cw := m.contentWidth()

	filterLine := truncate("filter: "+m.filter+"▏", cw)

	preview := previewStyle.Width(cw).Render(truncate("$ "+m.preview(), cw))

	plain := m.statusMsg
	isErr := false
	if m.errMsg != "" {
		plain = m.errMsg
		isErr = true
	}
	if plain == "" {
		plain = "space toggle/edit · enter menu · ctrl-x clear · ctrl-j/k cycle · ctrl-d/u half-page · ctrl-shift-j/k reorder · pgup/pgdn scroll desc · ctrl-r run · ctrl-y copy+exit · ctrl-shift-y copy · ctrl-p print · ctrl-? help · esc confirm · ctrl-c quit"
	}
	plain = truncate(plain, cw)

	var status string
	switch {
	case isErr:
		status = errStyle.Render(plain)
	case m.statusMsg != "":
		status = statusStyle.Render(plain)
	default:
		status = dimStyle.Render(plain)
	}

	top := []string{preview, filterLine}
	if m.modeSelectorVisible() {
		top = append(top, m.modePicker(), m.modeUsage())
	}
	top = append(top, panes, status)
	return strings.Join(top, "\n")
}

// modeSelectorVisible reports whether the horizontal mode picker should render.
// Subcommands are chosen from the options pane instead, so a tree with no mode
// forks (a pure dispatcher like direnv) hides it.
func (m model) modeSelectorVisible() bool {
	return len(m.modes) > 1 && m.res().HasBranchKind(BranchMode)
}

// modePicker renders the horizontal mode selector.
func (m model) modePicker() string {
	var parts []string
	for i, r := range m.modes {
		name := r.Name
		if name == "" {
			name = "mode"
		}
		if i == m.mode {
			parts = append(parts, activeStyle.Render(name))
		} else {
			parts = append(parts, dimStyle.Render(name))
		}
	}
	return truncate("mode: "+strings.Join(parts, "  "), m.contentWidth())
}

// modeUsage renders the one-line usage for the active mode.
func (m model) modeUsage() string {
	return dimStyle.Render(truncate(m.res().Usage, m.contentWidth()))
}

// paneWidths returns the content widths of the left (options) and right
// (description) panes so that, with their borders and the one-column gap,
// they fill the usable width.
func (m model) paneWidths() (int, int) {
	const gap = 1
	cw := m.contentWidth()
	total := cw - 4 - gap
	if total < 20 {
		return 15, 15
	}
	left := cw / 3
	if left < 20 {
		left = 20
	}
	if left > 40 {
		left = 40
	}
	right := total - left
	if right < 15 {
		right = 15
		left = total - right
	}
	if left < 5 {
		left = 5
	}
	return left, right
}

// contentWidth is the usable width, one column short of the terminal so that
// no rendered line reaches the last cell (which makes terminals auto-wrap and
// scroll the whole UI).
func (m model) contentWidth() int {
	w := m.width - 1
	if w < 1 {
		w = 1
	}
	return w
}

// paneHeight returns the content height of the panes so the panes plus the
// three leading lines (filter, preview border, preview) and the trailing
// status line fit the terminal height. Each pane also has a top and bottom
// border.
func (m model) paneHeight() int {
	extra := 0
	if m.modeSelectorVisible() {
		extra = 2 // mode picker + usage line
	}
	h := m.height - 6 - extra
	if h < 1 {
		h = 1
	}
	return h
}

// truncate cuts s to at most w display cells, appending an ellipsis when it
// has to drop text. Intended for plain (unstyled) strings.
func truncate(s string, w int) string {
	if w < 1 {
		return ""
	}
	r := []rune(s)
	if len(r) <= w {
		return s
	}
	if w == 1 {
		return "…"
	}
	return string(r[:w-1]) + "…"
}

// pageSize is the number of body lines shown in a pane (pane height minus
// the title line) and the step used to scroll the description.
func (m model) pageSize() int {
	h := m.paneHeight() - 1
	if h < 1 {
		h = 1
	}
	return h
}

func (m model) renderDescription(width, maxLines int) string {
	r, ok := m.selectedRow()
	if !ok {
		return dimStyle.Render(truncate("(none)", width))
	}
	desc := strings.TrimSpace(r.opt.OptDescription())
	if desc == "" {
		return dimStyle.Render(truncate("(no description)", width))
	}
	lines := wrapText(desc, width)
	if len(lines) <= maxLines {
		return strings.Join(lines, "\n")
	}
	off := m.descScroll
	maxOff := len(lines) - maxLines
	if off > maxOff {
		off = maxOff
	}
	if off < 0 {
		off = 0
	}
	window := append([]string(nil), lines[off:off+maxLines]...)
	if off > 0 {
		window[0] = dimStyle.Render("↑ more")
	}
	if off+maxLines < len(lines) {
		window[len(window)-1] = dimStyle.Render("↓ more")
	}
	return strings.Join(window, "\n")
}

// wrapText word-wraps s to the given width, preserving paragraph breaks.
func wrapText(s string, width int) []string {
	if width < 1 {
		return strings.Split(s, "\n")
	}
	return strings.Split(lipgloss.NewStyle().Width(width).Render(s), "\n")
}

func (m model) renderList(maxLines, width int) string {
	rows := m.rows()
	if len(rows) == 0 {
		return dimStyle.Render("  (none)")
	}
	if maxLines < 1 {
		maxLines = 1
	}
	content := width - 2
	if content < 1 {
		content = 1
	}
	start := 0
	if m.cursor >= maxLines {
		start = m.cursor - maxLines + 1
	}
	if start > len(rows)-maxLines {
		start = len(rows) - maxLines
	}
	if start < 0 {
		start = 0
	}
	end := start + maxLines
	if end > len(rows) {
		end = len(rows)
	}
	var b strings.Builder
	for i := start; i < end; i++ {
		line := renderRow(m, rows[i], content)
		prefix := "  "
		if i == m.cursor {
			prefix = "> "
		}
		if m.activeRow(rows[i]) {
			line = prefix + activeStyle.Render(line)
		} else {
			line = prefix + line
		}
		b.WriteString(line)
		if i < end-1 {
			b.WriteString("\n")
		}
	}
	return b.String()
}

// activeRow reports whether r is the row currently open for editing.
func (m model) activeRow(r row) bool {
	if !m.editing || r.opt == nil || r.opt.OptName() != m.editingName {
		return false
	}
	switch r.kind {
	case rowAdd:
		return m.editingIndex < 0
	case rowValue:
		return m.editingIndex == r.valIdx
	}
	return editableKind(r.opt) && m.editingIndex < 0
}

func renderRow(m model, r row, content int) string {
	if r.kind == rowSep {
		return dimStyle.Render(strings.Repeat("─", content))
	}
	name := r.opt.OptName()
	switch r.kind {
	case rowAdd:
		if m.editing && m.editingName == name && m.editingIndex < 0 {
			return m.editingLabel + m.input.View()
		}
		verb := "+ add "
		if m.isPath(r.opt) {
			verb = "+ pick "
		}
		return truncate(verb+name, content)
	case rowValue:
		if m.editing && m.editingName == name && m.editingIndex == r.valIdx {
			return m.editingLabel + m.input.View()
		}
		return truncate(valueLabel(name, r.valIdx)+r.value, content)
	}

	switch v := r.opt.(type) {
	case *SubcommandOption:
		return truncate("Command: "+v.Label, content)
	case *BoolFlag:
		fs := m.session.Flags[name]
		glyph := "[ ]"
		switch {
		case fs.Checked:
			glyph = "[✔]"
		case fs.Neg && v.Negative() != "":
			glyph = "[x]"
		}
		line := glyph + " " + truncate(name, content-4)
		if v.Short() != "" && len([]rune(line))+1+len([]rune(v.Short())) <= content {
			line += " " + dimStyle.Render(v.Short())
		}
		return line
	case *StringPositional:
		if m.editing && m.editingName == name && m.editingIndex < 0 {
			return m.editingLabel + m.input.View()
		}
		return truncate(name+": "+firstValue(m.session.PosVals[name]), content)
	case *EnumPositional:
		return truncate(name+": "+firstValue(m.session.PosVals[name]), content)
	case *StringFlag, *IntFlag, *EnumFlag, *SizeFlag:
		if m.editing && m.editingName == name && m.editingIndex < 0 {
			return m.editingLabel + m.input.View()
		}
		return truncate(name+": "+m.session.Flags[name].First(), content)
	case *CountFlag:
		n := m.session.Flags[name].Count
		line := name
		if n > 0 {
			line += ": " + strings.Repeat("×", n)
		}
		if v, ok := r.opt.(*CountFlag); ok && v.Short() != "" && len([]rune(line))+1+len([]rune(v.Short())) <= content {
			line += " " + dimStyle.Render(v.Short())
		}
		return truncate(line, content)
	case *PathPositional:
		return truncate(name+": "+firstValue(m.session.PosVals[name]), content)
	default:
		return "? " + name
	}
}
