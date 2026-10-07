// This file handles editing, clearing, and flag reordering.
package main

import (
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

func (m *model) activate() tea.Cmd {
	r, ok := m.selectedRow()
	if !ok {
		return nil
	}
	switch r.kind {
	case rowAdd:
		if m.isPath(r.opt) {
			return m.openPicker(r.opt.OptName(), -1)
		}
		if m.isEnum(r.opt) {
			m.openEnum(r.opt.OptName(), -1)
			return nil
		}
		m.beginEdit(r.opt.OptName(), -1, "", "+ "+r.opt.OptName()+": ")
	case rowValue:
		if m.isPath(r.opt) {
			return m.openPicker(r.opt.OptName(), r.valIdx)
		}
		if m.isEnum(r.opt) {
			m.openEnum(r.opt.OptName(), r.valIdx)
			return nil
		}
		m.beginEdit(r.opt.OptName(), r.valIdx, r.value, valueLabel(r.opt.OptName(), r.valIdx))
		return nil
	}
	switch v := r.opt.(type) {
	case *SubcommandOption:
		m.openSub(v.Level)
	case *BoolFlag:
		m.cycleBool(r.opt.OptName(), v.Negative() != "")
		if !v.Repeatable() {
			m.filter = ""
			m.descScroll = 0
			// The toggled flag may have moved into the checked group; keep
			// the cursor on it.
			m.cursor = m.rowIndexOf(r.opt.OptName())
		}
	case *StringPositional:
		m.beginEdit(r.opt.OptName(), -1, firstValue(m.session.PosVals[r.opt.OptName()]), r.opt.OptName()+": ")
	case *EnumFlag:
		m.openEnum(r.opt.OptName(), -1)
	case *EnumPositional:
		m.openEnum(r.opt.OptName(), -1)
	case *CountFlag:
		state := m.session.Flags[r.opt.OptName()]
		state.Count = (state.Count + 1) % 4
		m.session.Flags[r.opt.OptName()] = state
		m.setCursor(m.rowIndexOf(r.opt.OptName()))
	case *StringFlag, *IntFlag, *SizeFlag:
		m.beginEdit(r.opt.OptName(), -1, m.session.Flags[r.opt.OptName()].First(), r.opt.OptName()+": ")
	case *PathPositional:
		return m.openPicker(r.opt.OptName(), -1)
	}
	return nil
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
		state := m.session.Flags[name]
		if m.isRepeatableFlag(name) {
			state.Values = editValueList(state.Values, m.editingIndex, val)
		} else if val == "" {
			state.Values = nil
		} else {
			state.Values = []string{val}
		}
		m.session.Flags[name] = state
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

// deleteValue removes the variadic value under the cursor. It is a no-op on
// any other row.
func (m *model) deleteValue() {
	r, ok := m.selectedRow()
	if !ok || r.kind != rowValue {
		return
	}
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

// clearSelected empties the selected entry: a checked bool is unchecked (like
// space), a text/number value is cleared, and a variadic value is deleted. It
// is a no-op on an already-empty entry, the add row, and separators.
func (m *model) clearSelected() {
	r, ok := m.selectedRow()
	if !ok {
		return
	}
	switch r.kind {
	case rowValue:
		m.deleteValue()
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

func (m model) isVariadic(name string) bool {
	p, ok := m.cfg.positional(name)
	return ok && p.Variadic()
}

func (m *model) reorder(delta int) {
	r, ok := m.selectedRow()
	if !ok {
		return
	}
	if _, isFlag := r.opt.(Flag); !isFlag {
		return
	}
	name := r.opt.OptName()
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
	// Keep the cursor on the moved flag as it changes position.
	m.setCursor(m.rowIndexOf(name))
}

// flagEnabled reports whether a flag currently contributes to the command.
func (m model) flagEnabled(name string) bool {
	fs, ok := m.session.Flags[name]
	if !ok {
		return false
	}
	return fs.Checked || fs.Filled()
}

// isRepeatableFlag reports whether name is a repeatable value flag.
func (m model) isRepeatableFlag(name string) bool {
	f, ok := m.cfg.flag(name)
	return ok && f.Repeatable() && f.TakesValue()
}

// validate checks the active path. If exactly one *mode alternative* of the
// same subcommand accepts the current state and validates, it is adopted.
func (m *model) validate() bool {
	if modeIncludesApplied(m.res(), m.session) && len(MissingRequired(m.res(), m.session)) == 0 {
		return true
	}
	var valid []int
	for _, i := range m.modeSiblings() {
		if acceptsAt(m.modes, i, m.session) && len(MissingRequired(m.modes[i], m.session)) == 0 {
			valid = append(valid, i)
		}
	}
	if len(valid) == 1 {
		m.mode = valid[0]
		return true
	}
	if missing := MissingRequired(m.res(), m.session); len(missing) > 0 {
		m.errMsg = "required: " + strings.Join(missing, ", ")
	} else {
		m.errMsg = "no mode matches the current options"
	}
	m.statusMsg = ""
	return false
}

func (m model) preview() string {
	return ShellQuote(append([]string{m.cmd}, Assemble(m.res(), m.session)...))
}
