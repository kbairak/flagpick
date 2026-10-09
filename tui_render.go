// This file renders the main view: the WYSIWYG preview box, the options and
// description panes, and the bottom status line.
package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	borderColor = lipgloss.Color("205")
	boxBorder   = lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(borderColor)
	// dimStyle is used for placeholders and secondary text. The color is
	// adaptive: a dark gray on light terminals, a light gray on dark terminals.
	dimStyle            = lipgloss.NewStyle().Foreground(lipgloss.AdaptiveColor{Light: "240", Dark: "245"})
	cursorStyle         = lipgloss.NewStyle().Reverse(true)
	activeStyle         = lipgloss.NewStyle().Background(lipgloss.Color("255")).Foreground(lipgloss.Color("0"))
	errStyle            = lipgloss.NewStyle().Foreground(lipgloss.Color("196"))
	statusStyle         = lipgloss.NewStyle().Foreground(lipgloss.Color("42"))
	titleStyle          = lipgloss.NewStyle().Bold(true)
	buttonStyle         = lipgloss.NewStyle().Padding(0, 2)
	buttonSelectedStyle = buttonStyle.Background(lipgloss.Color("255")).Foreground(lipgloss.Color("0"))
	// previewEmittedStyle leaves emitted tokens in the terminal's default
	// foreground ("white" on dark themes, black on light), so they stay legible
	// regardless of the background.
	previewEmittedStyle = lipgloss.NewStyle()
)

// popupFilterHint is the shared footer hint for the enum menu.
const popupFilterHint = "type to filter · ctrl-w word · enter choose · up/down · esc cancel"

// statusHint is shown in the bottom line when there is no transient message.
const statusHint = "tab focus · enter edit · type filter · up/down select · ctrl-n/p mode · ctrl-s run · ctrl-y copy · ctrl-o print · ctrl-shift-h/l reorder · ctrl-? help · ctrl-c quit"

// titledBox renders a rounded box with the title embedded in the top border,
// e.g. ╭─ Options ─────╮. w is the body content width (total width w+2), h the
// body height (total height h+2). lipgloss has no border-title support, so the
// top border line is drawn by hand; the body uses lipgloss borders (left,
// right, bottom) so ANSI widths are handled correctly.
func titledBox(title, content string, w, h int) string {
	if w < 1 {
		w = 1
	}
	if h < 1 {
		h = 1
	}
	lines := []string{}
	if content != "" {
		lines = strings.Split(content, "\n")
	}
	for len(lines) < h {
		lines = append(lines, "")
	}
	if len(lines) > h {
		lines = lines[:h]
	}
	body := boxBorder.BorderTop(false).Width(w).Height(h).Render(strings.Join(lines, "\n"))

	b := lipgloss.RoundedBorder()
	bc := lipgloss.NewStyle().Foreground(borderColor)
	// Top: TL + Top + " " + title + " " + fill*Top + TR, total width w+2.
	tw := lipgloss.Width(title)
	fill := w - tw - 3
	if fill < 0 {
		fill = 0
	}
	top := bc.Render(b.TopLeft) + bc.Render(b.Top) + " " + title + " " +
		bc.Render(strings.Repeat(b.Top, fill)) + bc.Render(b.TopRight)
	return top + "\n" + body
}

func (m model) View() string {
	switch m.screen {
	case screenHelp:
		return m.helpView()
	case screenConfirm:
		return m.confirmView()
	case screenEnum:
		return m.enumView()
	case screenSub:
		return m.subView()
	case screenPicker:
		return m.pickerView()
	}

	leftW, rightW := m.paneWidths()
	paneH := m.paneHeight()

	left := m.optionsBox(leftW, paneH)
	desc := m.descriptionBox(rightW, paneH)
	panes := lipgloss.JoinHorizontal(lipgloss.Top, left, " ", desc)

	lines := append([]string{}, m.previewFrame()...)
	lines = append(lines, panes, m.renderStatus())
	return strings.Join(lines, "\n")
}

// optionsBox renders the Options pane: filter line and flag list, with the
// title in the border. When a positional is focused the pane is empty
// (positionals are edited from the preview).
func (m model) optionsBox(w, paneH int) string {
	var body []string
	if !m.focusedIsPositional() && !m.focusedIsSub() && len(m.segmentFlags()) > 0 {
		listMax := paneH
		if paneH >= 2 {
			body = append(body, dimStyle.Render(truncate("Filter: "+m.filter+"▏", w)))
			listMax = paneH - 1
		}
		if listMax >= 1 {
			body = append(body, m.renderList(listMax, w))
		}
	}
	return titledBox("Options", strings.Join(body, "\n"), w, paneH)
}

// descriptionBox renders the Description pane for the Options-list selection.
func (m model) descriptionBox(w, paneH int) string {
	var body []string
	if paneH >= 1 {
		body = append(body, m.renderDescription(w, paneH))
	}
	return titledBox("Description", strings.Join(body, "\n"), w, paneH)
}

// renderStatus renders the bottom line: a transient message or the key hint.
func (m model) renderStatus() string {
	cw := m.contentWidth()
	plain := m.statusMsg
	isErr := false
	if m.errMsg != "" {
		plain = m.errMsg
		isErr = true
	}
	if plain == "" {
		plain = statusHint
	}
	plain = truncate(plain, cw)
	switch {
	case isErr:
		return errStyle.Render(plain)
	case m.statusMsg != "":
		return statusStyle.Render(plain)
	default:
		return dimStyle.Render(plain)
	}
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

// paneHeight is the content height of the Options/Description boxes: the
// terminal height minus the preview box, the bottom status line, and each
// box's two borders.
func (m model) paneHeight() int {
	h := m.height - m.previewTotalHeight() - 3
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

// pageSize is the number of option rows shown and the step used to scroll the
// description.
func (m model) pageSize() int {
	h := m.paneHeight() - 2
	if h < 1 {
		h = 1
	}
	return h
}

func (m model) renderDescription(width, maxLines int) string {
	if maxLines < 1 {
		maxLines = 1
	}
	opt, ok := m.descriptionOption()
	if !ok {
		return dimStyle.Render(truncate("(none)", width))
	}
	desc := strings.TrimSpace(opt.OptDescription())
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

// activeRow reports whether r is the row currently being edited. Editing now
// happens in the preview, so this is always false; kept for the list renderer.
func (m model) activeRow(row) bool { return false }

func renderRow(m model, r row, content int) string {
	if r.kind == rowSep {
		return dimStyle.Render(strings.Repeat("─", content))
	}
	name := r.opt.OptName()
	switch r.kind {
	case rowAdd:
		verb := "+ add "
		if m.isPath(r.opt) {
			verb = "+ pick "
		}
		return truncate(verb+name, content)
	case rowValue:
		return truncate(valueLabel(name, r.valIdx)+r.value, content)
	}

	switch v := r.opt.(type) {
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
		return truncate(name+": "+firstValue(m.session.PosVals[name]), content)
	case *EnumPositional:
		return truncate(name+": "+firstValue(m.session.PosVals[name]), content)
	case *StringFlag, *IntFlag, *EnumFlag, *SizeFlag:
		return truncate(name+": "+m.session.Flags[name].First(), content)
	case *CountFlag:
		n := m.session.Flags[name].Count
		line := name
		if n > 0 {
			line += ": " + strings.Repeat("×", n)
		}
		if v.Short() != "" && len([]rune(line))+1+len([]rune(v.Short())) <= content {
			line += " " + dimStyle.Render(v.Short())
		}
		return truncate(line, content)
	case *PathPositional:
		return truncate(name+": "+firstValue(m.session.PosVals[name]), content)
	default:
		return "? " + name
	}
}
