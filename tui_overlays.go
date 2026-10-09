// This file holds the state of the picker and enum overlays.
package main

import (
	tea "github.com/charmbracelet/bubbletea"
)

func (m *model) openPicker(name string, index int) tea.Cmd {
	m.screen = screenPicker
	m.pick.name = name
	m.pick.index = index
	m.picker = newPicker(m.picker.dir)
	return nil
}

func (m *model) commitPath(path string) {
	name := m.pick.name
	vals := m.session.PosVals[name]
	switch {
	case m.pick.index < 0:
		if m.isVariadic(name) {
			m.session.PosVals[name] = append(vals, path)
		} else {
			m.session.PosVals[name] = []string{path}
		}
	case m.pick.index < len(vals):
		vals[m.pick.index] = path
	}
}

// filteredEnum returns the enum values matching the current filter.
func (m model) filteredEnum() []string {
	var out []string
	for _, v := range m.enum.values {
		if fuzzyMatch(v, m.enum.filter) {
			out = append(out, v)
		}
	}
	return out
}

func trimLastRune(s string) string {
	r := []rune(s)
	if len(r) == 0 {
		return s
	}
	return string(r[:len(r)-1])
}

// openEnum opens the value menu for an enum flag or positional.
func (m *model) openEnum(name string, index int) {
	opt, ok := m.optionFor(name)
	if !ok {
		return
	}
	vals := allowedValues(opt)
	if len(vals) == 0 {
		return
	}
	_, pos := m.cfg.positional(name)
	cur := ""
	if index >= 0 {
		vs := m.enumState(name, pos)
		if index < len(vs) {
			cur = vs[index]
		}
	} else {
		if vs := m.enumState(name, pos); len(vs) > 0 {
			cur = vs[0]
		}
	}
	m.screen = screenEnum
	m.enum.name = name
	m.enum.index = index
	m.enum.values = vals
	m.enum.filter = ""
	m.enum.sel = 0
	for i, v := range vals {
		if v == cur {
			m.enum.sel = i
			break
		}
	}
}

// enumState returns the current value list backing an enum option: flag
// values or positional values.
func (m *model) enumState(name string, pos bool) []string {
	if pos {
		return m.session.PosVals[name]
	}
	return m.session.Flags[name].Values
}

// commitEnum stores the chosen enum value.
func (m *model) commitEnum(val string) {
	if _, pos := m.cfg.positional(m.enum.name); pos {
		vals := m.session.PosVals[m.enum.name]
		if m.enum.index < 0 {
			if m.isVariadic(m.enum.name) {
				m.session.PosVals[m.enum.name] = append(vals, val)
			} else {
				m.session.PosVals[m.enum.name] = []string{val}
			}
		} else if m.enum.index < len(vals) {
			vals[m.enum.index] = val
		}
		return
	}
	state := m.session.Flags[m.enum.name]
	was := m.flagEnabled(m.enum.name)
	if m.enum.index < 0 {
		if m.isRepeatableFlag(m.enum.name) {
			state.Values = append(state.Values, val)
		} else {
			state.Values = []string{val}
		}
	} else if m.enum.index < len(state.Values) {
		state.Values[m.enum.index] = val
	}
	m.session.Flags[m.enum.name] = state
	if !was && m.flagEnabled(m.enum.name) {
		m.appendFlagOrder(m.enum.name)
	}
}
