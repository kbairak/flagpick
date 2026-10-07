// This file renders the full-screen overlays (picker, enum, subcommand, action,
// confirm, help).
package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// subView renders the subcommand menu.
func (m model) subView() string {
	cw := m.contentWidth()
	inner := cw - 6
	if inner < 10 {
		inner = 10
	}
	normal := buttonStyle
	selected := buttonSelectedStyle

	lines := []string{titleStyle.Render(truncate("Select subcommand", inner)), ""}
	subs := m.filteredSub()
	if len(subs) == 0 {
		lines = append(lines, dimStyle.Render("(no match)"))
	}
	for i, pi := range subs {
		pc := m.modes[pi].BranchChoices[m.sub.level]
		label := pc.Label
		if d := strings.TrimSpace(pc.Description); d != "" {
			label += " — " + d
		}
		label = truncate(label, inner-2)
		if i == m.sub.sel {
			lines = append(lines, selected.Render(label))
		} else {
			lines = append(lines, normal.Render(label))
		}
	}
	filterLine := truncate("filter: "+m.sub.filter+"▏", inner)
	lines = append(lines, "", filterLine, dimStyle.Render(truncate(popupFilterHint, inner)))
	if maxBody := m.height - 4; maxBody > 0 && len(lines) > maxBody {
		lines = lines[:maxBody]
	}
	box := boxBorder.Padding(1, 2).Render(strings.Join(lines, "\n"))
	return lipgloss.Place(cw, m.height, lipgloss.Center, lipgloss.Center, box)
}

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

// actionView renders the vertical action popup: run, copy, print, cancel.
func (m model) actionView() string {
	cw := m.contentWidth()
	inner := cw - 6
	if inner < 10 {
		inner = 10
	}

	normal := buttonStyle
	selected := buttonSelectedStyle

	labels := []string{"Run", "Copy and exit", "Copy and stay", "Print composed command", "Cancel"}
	buttons := make([]string, len(labels))
	for i, l := range labels {
		plain := truncate(l, inner-2)
		if i == m.actions.sel {
			buttons[i] = selected.Render(plain)
		} else {
			buttons[i] = normal.Render(plain)
		}
	}

	lines := []string{
		titleStyle.Render(truncate("Command", inner)),
		"",
		truncate("$ "+m.preview(), inner),
		"",
	}
	lines = append(lines, buttons...)
	lines = append(lines, "", dimStyle.Render(truncate("enter confirm · up/down or j/k choose · esc cancel", inner)))
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
		"any printable   fuzzy-filter options",
		"backspace       delete last filter char",
		"ctrl-w          delete last filter word",
		"ctrl-j / ctrl-k cycle selection",
		"up / down       cycle selection (alt)",
		"ctrl-d / ctrl-u half-page down / up",
		"pgup / pgdown   scroll description",
		"space           toggle flag / edit value / add value",
		"space on path   open the file/dir picker",
		"space on enum   choose value from a menu",
		"space on command choose the subcommand",
		"space on count  cycle repeats (0-3)",
		"enter           open command menu (run/copy/print/cancel)",
		"ctrl-x          clear selected entry (uncheck/empty)",
		"ctrl-shift-x    clear all options",
		"ctrl-shift-j/k  reorder selected flag (or ctrl-up/down)",
		"tab / shift-tab switch mode",
		"ctrl-r          run (exec)",
		"ctrl-y          copy to clipboard and exit",
		"ctrl-shift-y    copy to clipboard and stay",
		"ctrl-p          print after exit",
		"esc             open quit confirm · esc cancel · esc clear filter",
		"ctrl-c          quit immediately",
		"ctrl-?          toggle this help",
		"",
		dimStyle.Render("press any key to close"),
	}, "\n")
	box := boxBorder.Padding(1, 2).Render(body)
	return lipgloss.Place(m.contentWidth(), m.height, lipgloss.Center, lipgloss.Center, box)
}
