// This file handles editing, clearing, and flag reordering.
package main

import (
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// activateToken activates a focused preview token.
func (m *model) activateToken(t pvTok) tea.Cmd {
	switch {
	case t.flag != nil:
		return m.activateFlag(t.flag, t.name, t.valIdx)
	case t.pos != nil:
		return m.activatePositional(t.pos, t.valIdx)
	}
	return nil
}

// activateOption activates the named flag or positional as if it were focused
// from the empty-option slot.
func (m *model) activateOption(name string) tea.Cmd {
	opt, ok := m.optionFor(name)
	if !ok {
		return nil
	}
	switch o := opt.(type) {
	case Flag:
		return m.activateFlag(o, name, -1)
	case Positional:
		return m.activatePositional(o, -1)
	}
	return nil
}

// activateFlag performs the per-type action for a flag.
func (m *model) activateFlag(f Flag, name string, valIdx int) tea.Cmd {
	switch v := f.(type) {
	case *BoolFlag:
		was := m.flagEnabled(name)
		m.cycleBool(name, v.Negative() != "")
		if !was && m.flagEnabled(name) {
			m.appendFlagOrder(name)
		}
		m.resetFilter()
		m.focusNamed(name)
	case *CountFlag:
		was := m.flagEnabled(name)
		state := m.session.Flags[name]
		state.Count = (state.Count + 1) % 4
		m.session.Flags[name] = state
		if !was && m.flagEnabled(name) {
			m.appendFlagOrder(name)
		}
		m.resetFilter()
		m.focusNamed(name)
	case *EnumFlag:
		m.openEnum(name, valIdx)
	case *StringFlag, *IntFlag, *SizeFlag:
		m.beginEdit(name, valIdx, m.session.Flags[name].First(), "")
	}
	return nil
}

// activatePositional performs the per-type action for a positional.
func (m *model) activatePositional(p Positional, valIdx int) tea.Cmd {
	switch v := p.(type) {
	case *PathPositional:
		return m.openPicker(v.OptName(), valIdx)
	case *EnumPositional:
		m.openEnum(v.OptName(), valIdx)
	case *StringPositional, *PassthroughPositional:
		m.beginEdit(v.OptName(), valIdx, firstValue(m.session.PosVals[v.OptName()]), "")
	}
	return nil
}

// removeFocusedToken clears the session state behind the focused emitted token
// and returns focus to the empty-option slot.
func (m *model) removeFocusedToken() {
	t, ok := m.focusedToken()
	if !ok || t.kind != pvEmitted {
		return
	}
	switch {
	case t.flag != nil:
		state := m.session.Flags[t.name]
		if _, isCount := t.flag.(*CountFlag); isCount {
			state.Count = 0
		} else if t.flag.Repeatable() && t.valIdx >= 0 && t.valIdx < len(state.Values) {
			state.Values = append(state.Values[:t.valIdx], state.Values[t.valIdx+1:]...)
		} else {
			state = FlagState{}
		}
		m.session.Flags[t.name] = state
	case t.pos != nil:
		vals := m.session.PosVals[t.name]
		if t.pos.Variadic() && t.valIdx >= 0 && t.valIdx < len(vals) {
			m.session.PosVals[t.name] = append(vals[:t.valIdx], vals[t.valIdx+1:]...)
		} else {
			m.session.PosVals[t.name] = nil
		}
	}
	m.cursor = 0
	m.descScroll = 0
	m.focusEmptySlot()
}

// isValueFlag reports whether name is a flag that takes a value.
func (m model) isValueFlag(name string) bool {
	f, ok := m.cfg.flag(name)
	return ok && f.TakesValue()
}

// isIntFlag reports whether name is an integer-valued flag.
func (m model) isIntFlag(name string) bool {
	f, ok := m.cfg.flag(name)
	return ok && f.OptType() == KindInt
}

func (m *model) beginEdit(name string, index int, value, label string) {
	leftW, _ := m.paneWidths()
	content := leftW - 2
	if content < 2 {
		content = 2
	}
	label = truncate(label, content-1)
	// Reserve one cell for the input's cursor, which is rendered in addition
	// to the width-limited value.
	w := content - lipgloss.Width(label) - 1
	if w < 1 {
		w = 1
	}
	m.input.Width = w
	m.editing = true
	m.editingName = name
	m.editingIndex = index
	m.editingLabel = label
	m.input.SetValue(value)
	m.input.CursorEnd()
	m.input.Focus()
}

func (m *model) commitEdit() {
	val := m.input.Value()
	name := m.editingName
	if m.isValueFlag(name) {
		if err := m.validateValue(name, val); err != nil {
			m.errMsg = err.Error()
			return
		}
		was := m.flagEnabled(name)
		state := m.session.Flags[name]
		if m.isRepeatableFlag(name) {
			state.Values = editValueList(state.Values, m.editingIndex, val)
		} else if val == "" {
			state.Values = nil
		} else {
			state.Values = []string{val}
		}
		m.session.Flags[name] = state
		if !was && m.flagEnabled(name) {
			m.appendFlagOrder(name)
		}
		return
	}
	vals := m.session.PosVals[name]
	m.session.PosVals[name] = editValueList(vals, m.editingIndex, val)
}

// editValueList applies an edit to a list of values: index < 0 appends a
// non-empty value; otherwise the indexed value is replaced (or removed when
// empty).
func editValueList(vals []string, index int, val string) []string {
	if index < 0 {
		if val == "" {
			return vals
		}
		return append(vals, val)
	}
	if index >= len(vals) {
		return vals
	}
	if val == "" {
		return append(vals[:index], vals[index+1:]...)
	}
	vals[index] = val
	return vals
}

// validateValue enforces kind-specific value rules.
func (m *model) validateValue(name, val string) error {
	if f, ok := m.cfg.flag(name); ok {
		return validateFlagValue(f, val)
	}
	return nil
}

// clearAll resets every option: unchecks bools, empties value flags, and
// clears positionals.
func (m *model) clearAll() {
	for name := range m.session.Flags {
		m.session.Flags[name] = FlagState{}
	}
	for name := range m.session.PosVals {
		m.session.PosVals[name] = nil
	}
	m.filter = ""
	m.cursor = 0
	m.descScroll = 0
	m.errMsg = ""
	m.statusMsg = ""
}

// cycleBool advances a bool flag. Non-nullable flags toggle; nullable flags
// cycle unset -> on -> off -> unset.
func (m *model) cycleBool(name string, nullable bool) {
	if !nullable {
		ToggleFlag(m.cfg, m.session, name)
		return
	}
	state := m.session.Flags[name]
	switch {
	case !state.Checked && !state.Neg:
		ToggleFlag(m.cfg, m.session, name)
	case state.Checked:
		state.Checked = false
		state.Neg = true
		m.session.Flags[name] = state
	default:
		state.Neg = false
		m.session.Flags[name] = state
	}
}

// clearSelected empties the focused preview token if it is filled, otherwise
// the Options-list selection: unchecks a bool, clears a value, or deletes a
// variadic value.
func (m *model) clearSelected() {
	if t, ok := m.focusedToken(); ok && t.kind == pvEmitted && t.name != "" {
		m.removeFocusedToken()
		return
	}
	r, ok := m.selectedRow()
	if !ok {
		return
	}
	switch r.kind {
	case rowValue:
		m.deleteListValue(r)
		return
	case rowAdd, rowSep:
		return
	}
	name := r.opt.OptName()
	switch r.opt.(type) {
	case *BoolFlag:
		if !m.session.Flags[name].Filled() {
			return
		}
		m.session.Flags[name] = FlagState{}
	case *StringFlag, *IntFlag, *EnumFlag, *SizeFlag:
		if !m.session.Flags[name].Filled() {
			return
		}
		state := m.session.Flags[name]
		state.Values = nil
		m.session.Flags[name] = state
	case *CountFlag:
		if m.session.Flags[name].Count == 0 {
			return
		}
		state := m.session.Flags[name]
		state.Count = 0
		m.session.Flags[name] = state
	case *StringPositional, *PathPositional, *EnumPositional:
		if !hasValue(m.session.PosVals[name]) {
			return
		}
		m.session.PosVals[name] = nil
	default:
		return
	}
	// The entry may have moved into the empty group; keep the cursor on it.
	m.setCursor(m.rowIndexOf(name))
}

// deleteListValue removes the variadic value under the Options-list cursor.
func (m *model) deleteListValue(r row) {
	name := r.opt.OptName()
	if f, ok := r.opt.(Flag); ok && f.Repeatable() {
		state := m.session.Flags[name]
		if r.valIdx < len(state.Values) {
			state.Values = append(state.Values[:r.valIdx], state.Values[r.valIdx+1:]...)
			m.session.Flags[name] = state
		}
	} else {
		vals := m.session.PosVals[name]
		if r.valIdx >= len(vals) {
			return
		}
		m.session.PosVals[name] = append(vals[:r.valIdx], vals[r.valIdx+1:]...)
	}
	list := m.rows()
	if m.cursor >= len(list) && m.cursor > 0 {
		m.cursor--
	}
}

func (m model) isVariadic(name string) bool {
	p, ok := m.cfg.positional(name)
	return ok && p.Variadic()
}

func (m *model) reorder(delta int) {
	t, ok := m.focusedToken()
	if !ok || t.flag == nil || t.kind != pvEmitted {
		return
	}
	name := t.name
	// Only applied (enabled) flags can be reordered; unapplied ones emit
	// nothing, so moving them has no visible effect.
	if !m.flagEnabled(name) {
		return
	}
	idx := -1
	for i, n := range m.session.Order {
		if n == name {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	// Swap with the nearest enabled flag in the given direction, so the
	// change is visible in the preview (disabled flags emit nothing).
	target := -1
	for j := idx + delta; j >= 0 && j < len(m.session.Order); j += delta {
		if m.flagEnabled(m.session.Order[j]) {
			target = j
			break
		}
	}
	if target < 0 {
		return
	}
	m.session.Order[idx], m.session.Order[target] = m.session.Order[target], m.session.Order[idx]
	// Keep the focus on the moved flag as it changes position.
	m.focusNamed(name)
}

// flagEnabled reports whether a flag currently contributes to the command.
func (m model) flagEnabled(name string) bool {
	fs, ok := m.session.Flags[name]
	if !ok {
		return false
	}
	return fs.Checked || fs.Filled()
}

// appendFlagOrder moves name to the end of the emit order, so a newly added
// flag lands after the flags already applied (and, since positionals always
// follow flags, before the positionals).
func (m *model) appendFlagOrder(name string) {
	idx := -1
	for i, n := range m.session.Order {
		if n == name {
			idx = i
			break
		}
	}
	if idx < 0 || idx == len(m.session.Order)-1 {
		return
	}
	m.session.Order = append(m.session.Order[:idx], m.session.Order[idx+1:]...)
	m.session.Order = append(m.session.Order, name)
}

// resetFilter clears the option filter and its selection/scroll.
func (m *model) resetFilter() {
	m.filter = ""
	m.cursor = -1
	m.descScroll = 0
}

// isRepeatableFlag reports whether name is a repeatable value flag.
func (m model) isRepeatableFlag(name string) bool {
	f, ok := m.cfg.flag(name)
	return ok && f.Repeatable() && f.TakesValue()
}

// validate checks the active path; it sets errMsg with the reason on failure.
func (m *model) validate() bool {
	ok, reason := m.validity()
	if ok {
		return true
	}
	m.errMsg = reason
	m.statusMsg = ""
	return false
}

func (m model) preview() string {
	return ShellQuote(append([]string{m.cmd}, Assemble(m.res(), m.session)...))
}
