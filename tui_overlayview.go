// This file renders the full-screen overlays (picker, enum, confirm, help).
package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// pickerView renders the filesystem picker as a full-screen overlay.
func (m model) pickerView() string {
	return m.picker.render(m.contentWidth(), m.height)
}

// enumView renders the enum value menu.
func (m model) enumView() string {
	cw := m.contentWidth()
	inner := cw - 6
	if inner < 10 {
		inner = 10
	}
	normal := buttonStyle
	selected := buttonSelectedStyle

	lines := []string{titleStyle.Render(truncate("Select "+m.enum.name, inner)), ""}
	values := m.filteredEnum()
	if len(values) == 0 {
		lines = append(lines, dimStyle.Render("(no match)"))
	}
	for i, v := range values {
		label := truncate(v, inner-2)
		if i == m.enum.sel {
			lines = append(lines, selected.Render(label))
		} else {
			lines = append(lines, normal.Render(label))
		}
	}
	filterLine := truncate("filter: "+m.enum.filter+"▏", inner)
	lines = append(lines, "", filterLine, dimStyle.Render(truncate(popupFilterHint, inner)))
	if maxBody := m.height - 4; maxBody > 0 && len(lines) > maxBody {
		lines = lines[:maxBody]
	}
	box := boxBorder.Padding(1, 2).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(cw, m.height, lipgloss.Center, lipgloss.Center, box)
}

// confirmView renders the quit-confirmation popup with Cancel on the left and
// Exit on the right.
func (m model) confirmView() string {
	cw := m.contentWidth()
	inner := cw - 6 // border (2) + horizontal padding (4)
	if inner < 10 {
		inner = 10
	}

	normal := buttonStyle
	selected := buttonSelectedStyle

	cancel := normal.Render("[ Cancel ]")
	exit := normal.Render("[ Exit ]")
	if m.confirm.sel == 0 {
		cancel = selected.Render("[ Cancel ]")
	} else {
		exit = selected.Render("[ Exit ]")
	}

	lines := []string{
		titleStyle.Render(truncate("Quit flagpick?", inner)),
		"",
		truncate("$ "+m.preview(), inner),
		"",
		cancel + "  " + exit,
		"",
		dimStyle.Render(truncate("enter confirm · left/right choose · esc cancel", inner)),
	}
	if maxBody := m.height - 4; maxBody > 0 && len(lines) > maxBody {
		lines = lines[:maxBody]
	}
	box := boxBorder.Padding(1, 2).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(cw, m.height, lipgloss.Center, lipgloss.Center, box)
}

func (m model) helpView() string {
	body := strings.Join([]string{
		titleStyle.Render("flagpick keymap"),
		"",
		"tab / shift-tab move focus between preview tokens",
		"enter           edit / activate the focused token",
		"any printable   fuzzy-filter options or subcommands",
		"backspace       delete focused token / filter char",
		"ctrl-w          delete last filter word",
		"ctrl-j / ctrl-k move the option list selection",
		"up / down       move the option list selection (alt)",
		"ctrl-d / ctrl-u half-page down / up",
		"pgup / pgdown   scroll description",
		"ctrl-x          clear focused token / selected entry",
		"ctrl-shift-x    clear all options",
		"ctrl-shift-h/l  reorder the focused flag",
		"ctrl-n / ctrl-p  next / previous mode",
		"ctrl-s          run (exec)",
		"ctrl-y          copy to clipboard and exit",
		"ctrl-shift-y    copy to clipboard and stay",
		"ctrl-o          print after exit",
		"esc             abort edit / clear filter / quit confirm",
		"ctrl-c          quit immediately",
		"ctrl-?          toggle this help",
		"",
		dimStyle.Render("press any key to close"),
	}, "\n")
	box := boxBorder.Padding(1, 2).Render(body)
	return lipgloss.Place(m.contentWidth(), m.height, lipgloss.Center, lipgloss.Center, box)
}
