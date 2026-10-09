// This file handles choosing and reconciling subcommands: the SUBCOMMAND
// placeholder in the preview and the subcommand menu.
package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// countSubs returns the number of subcommand choices a fully-chosen path makes.
func countSubs(r Resolved) int {
	n := 0
	for _, c := range r.BranchChoices {
		if c.Kind == BranchSubcommand {
			n++
		}
	}
	return n
}

// prefixResolved truncates a fully-chosen path to the first subDepth subcommand
// levels. When truncated, the last segment's Sub is left unset so a SUBCOMMAND
// placeholder is shown and the subcommand menu can be opened.
func prefixResolved(leaf Resolved, subDepth int) Resolved {
	if countSubs(leaf) == 0 || subDepth >= countSubs(leaf) {
		return leaf
	}
	if subDepth < 0 {
		subDepth = 0
	}
	segs := append([]Segment(nil), leaf.Segments[:subDepth+1]...)
	segs[len(segs)-1].Sub = nil
	var choices []BranchChoice
	subs := 0
	for _, c := range leaf.BranchChoices {
		if c.Kind == BranchSubcommand {
			if subs >= subDepth {
				break
			}
			subs++
		}
		choices = append(choices, c)
	}
	name := ""
	for _, s := range segs {
		name = joinName(name, s.Node.Name)
	}
	return newResolved(name, segs, choices)
}

// pendingSub reports whether the active path still has a subcommand to choose.
func (m model) pendingSub() bool {
	res := m.res()
	if len(res.Segments) == 0 {
		return false
	}
	last := res.Segments[len(res.Segments)-1]
	return last.Sub == nil && len(last.Node.Subcommands) > 0
}

// currentSubcommands returns the subcommands available at the active dispatch
// node, or nil when none is pending.
func (m model) currentSubcommands() []*RNode {
	res := m.res()
	if len(res.Segments) == 0 {
		return nil
	}
	last := res.Segments[len(res.Segments)-1]
	if last.Sub != nil {
		return nil
	}
	return last.Node.Subcommands
}

// subcommandsAt returns the subcommands of the dispatch node at the given depth
// of the active path.
func (m model) subcommandsAt(depth int) []*RNode {
	res := m.res()
	if depth < 0 || depth >= len(res.Segments) {
		return nil
	}
	return res.Segments[depth].Node.Subcommands
}

// openSub opens the subcommand menu for the active dispatch node (the deepest
// unchosen level).
func (m *model) openSub() tea.Cmd {
	return m.openSubAt(m.subDepth)
}

// openSubAt opens the subcommand menu for the dispatch node at the given
// subcommand depth, pre-selecting the current choice when there is one.
func (m *model) openSubAt(depth int) tea.Cmd {
	res := m.res()
	if depth < 0 || depth >= len(res.Segments) {
		return nil
	}
	seg := res.Segments[depth]
	if len(seg.Node.Subcommands) == 0 {
		return nil
	}
	entries := make([]subEntry, 0, len(seg.Node.Subcommands))
	for i, s := range seg.Node.Subcommands {
		entries = append(entries, subEntry{name: s.Name, desc: s.Description, index: i})
	}
	m.screen = screenSub
	m.sub.entries = entries
	m.sub.filter = ""
	m.sub.sel = 0
	m.sub.depth = depth
	if seg.Sub != nil {
		for i, e := range entries {
			if e.name == seg.Sub.Name {
				m.sub.sel = i
				break
			}
		}
	}
	return nil
}

// chooseSub selects the subcommand at index si at the active depth.
func (m *model) chooseSub(si int) {
	m.chooseSubAt(m.subDepth, si)
}

// chooseSubAt selects the subcommand at index si at the given depth, replacing
// the choice there (and dropping any deeper choices).
func (m *model) chooseSubAt(depth, si int) {
	leaf := m.modes[m.mode]
	choices := subcommandChoices(leaf)
	if depth < 0 || depth >= len(choices) {
		return
	}
	prefix := make([]BranchChoice, 0, depth)
	subs := 0
	for _, c := range leaf.BranchChoices {
		if c.Kind == BranchSubcommand {
			if subs >= depth {
				break
			}
			subs++
			prefix = append(prefix, c)
		}
	}
	target := -1
	for i, l := range m.modes {
		ch := subcommandChoices(l)
		if len(ch) <= depth || !sameChoices(ch[:depth], prefix) {
			continue
		}
		if ch[depth].Index == si {
			target = i
			break
		}
	}
	if target < 0 {
		return
	}
	m.mode = target
	m.subDepth = depth + 1
	m.screen = screenMain
	m.afterModeChange()
}

// sameChoices reports whether two branch-choice prefixes are equal.
func sameChoices(a, b []BranchChoice) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// filteredSub returns the subcommand menu entries matching the current filter.
func (m model) filteredSub() []subEntry {
	var out []subEntry
	for _, e := range m.sub.entries {
		if fuzzyMatch(e.name, m.sub.filter) {
			out = append(out, e)
		}
	}
	return out
}

// handleSubKey drives the subcommand menu.
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
			m.chooseSubAt(m.sub.depth, f[m.sub.sel].index)
		} else {
			m.screen = screenMain
		}
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
	for i, e := range subs {
		label := e.name
		if d := strings.TrimSpace(e.desc); d != "" {
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
