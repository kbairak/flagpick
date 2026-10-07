// This file builds the option row list and moves the cursor over it.
package main

import (
	"strconv"
	"strings"
)

// allowedValues returns opt's allowed values when it is an enum flag or enum
// positional, and nil otherwise.
func allowedValues(opt Option) []string {
	switch o := opt.(type) {
	case *EnumFlag:
		return o.Allowed()
	case *EnumPositional:
		return o.Allowed()
	}
	return nil
}

// optionFor returns the flag or positional with the given name.
func (m model) optionFor(name string) (Option, bool) {
	if f, ok := m.cfg.flag(name); ok {
		return f, true
	}
	if p, ok := m.cfg.positional(name); ok {
		return p, true
	}
	return nil, false
}

// listOptions returns the active mode's options in the order written in the
// mode's `options:` list, with flag slots filled in session emission order (so
// reorder is visible) and filled options hoisted first.
func (m model) listOptions() []Option {
	res := m.res()
	byName := make(map[string]Flag, len(res.Flags))
	for _, f := range res.Flags {
		byName[f.OptName()] = f
	}

	seq := append([]Option(nil), res.Options...)
	// Fill flag slots in session order.
	var ordered []Flag
	for _, name := range m.session.Order {
		if f, ok := byName[name]; ok {
			ordered = append(ordered, f)
		}
	}
	k := 0
	for i, opt := range seq {
		if _, isFlag := opt.(Flag); isFlag && k < len(ordered) {
			seq[i] = ordered[k]
			k++
		}
	}

	var filled, empty []Option
	for _, opt := range seq {
		if m.isFilled(opt) {
			filled = append(filled, opt)
		} else {
			empty = append(empty, opt)
		}
	}
	return append(filled, empty...)
}

func filterOptions(opts []Option, filter string) []Option {
	if filter == "" {
		return opts
	}
	var out []Option
	for _, o := range opts {
		if fuzzyMatch(o.OptName(), filter) {
			out = append(out, o)
		}
	}
	return out
}

func fuzzyMatch(label, filter string) bool {
	fr := []rune(strings.ToLower(filter))
	i := 0
	for _, r := range strings.ToLower(label) {
		if i < len(fr) && r == fr[i] {
			i++
		}
	}
	return i == len(fr)
}

// trimLastWord removes the trailing word from a filter (ctrl-w), including any
// whitespace before it but keeping the whitespace separator.
func trimLastWord(s string) string {
	s = strings.TrimRight(s, " \t")
	if i := strings.LastIndexAny(s, " \t"); i >= 0 {
		return s[:i+1]
	}
	return ""
}

func (m model) filtered() []Option {
	return filterOptions(m.listOptions(), m.filter)
}

// rows expands the filtered options into selectable lines. A variadic
// positional becomes one rowValue per value plus a trailing rowAdd.
func (m model) rows() []row {
	var rows []row
	seenFilled := false
	sepInserted := false
	for _, opt := range m.filtered() {
		filled := m.isFilled(opt)
		if filled {
			seenFilled = true
		} else if seenFilled && !sepInserted {
			rows = append(rows, row{kind: rowSep})
			sepInserted = true
		}
		if m.isRepeatableOption(opt) {
			for i, v := range m.optionValues(opt) {
				rows = append(rows, row{opt: opt, kind: rowValue, valIdx: i, value: v})
			}
			rows = append(rows, row{opt: opt, kind: rowAdd})
			continue
		}
		rows = append(rows, row{opt: opt, kind: rowOption})
	}
	return rows
}

// isRepeatableOption reports whether opt expands into a list of values (a
// variadic positional or a repeatable value flag).
func (m model) isRepeatableOption(opt Option) bool {
	switch o := opt.(type) {
	case Positional:
		return o.Variadic()
	case Flag:
		return o.Repeatable() && o.TakesValue()
	}
	return false
}

// optionValues returns the current values backing a repeatable option.
func (m model) optionValues(opt Option) []string {
	switch o := opt.(type) {
	case Positional:
		return m.session.PosVals[o.OptName()]
	case Flag:
		return m.session.Flags[o.OptName()].Values
	}
	return nil
}

// isFilled reports whether an option currently carries a value (checked bool,
// non-empty value flag, or a positional with a value).
func (m model) isFilled(opt Option) bool {
	switch o := opt.(type) {
	case Positional:
		return hasValue(m.session.PosVals[o.OptName()])
	case Flag:
		fs := m.session.Flags[o.OptName()]
		return fs.Checked || fs.Filled()
	}
	return false
}

func (m *model) moveCursor(delta int) {
	m.descScroll = 0
	list := m.rows()
	// Collect the selectable rows so movement cycles and skips separators.
	var idx []int
	for i, r := range list {
		if r.kind != rowSep {
			idx = append(idx, i)
		}
	}
	if len(idx) == 0 {
		m.cursor = 0
		return
	}
	pos := 0
	for j, i := range idx {
		if i == m.cursor {
			pos = j
			break
		}
	}
	pos = ((pos+delta)%len(idx) + len(idx)) % len(idx)
	m.cursor = idx[pos]
}

// halfPage is half the visible page, at least one row.
func halfPage(pageSize int) int {
	n := pageSize / 2
	if n < 1 {
		n = 1
	}
	return n
}

// moveCursorClamped moves by delta without wrapping (stops at the ends).
func (m *model) moveCursorClamped(delta int) {
	m.descScroll = 0
	var idx []int
	for i, r := range m.rows() {
		if r.kind != rowSep {
			idx = append(idx, i)
		}
	}
	if len(idx) == 0 {
		m.cursor = 0
		return
	}
	pos := 0
	for j, i := range idx {
		if i == m.cursor {
			pos = j
			break
		}
	}
	pos += delta
	if pos < 0 {
		pos = 0
	}
	if pos >= len(idx) {
		pos = len(idx) - 1
	}
	m.cursor = idx[pos]
}

// setCursor clamps the cursor to the current row list and resets the
// description scroll.
func (m *model) setCursor(i int) {
	m.descScroll = 0
	list := m.rows()
	n := len(list)
	if n == 0 {
		m.cursor = 0
		return
	}
	if i < 0 {
		i = 0
	}
	if i >= n {
		i = n - 1
	}
	// Separators are not selectable; step in the direction of travel, then
	// fall back the other way.
	if list[i].kind == rowSep {
		if j := nextSelectable(list, i, 1); j >= 0 {
			i = j
		} else if j := nextSelectable(list, i, -1); j >= 0 {
			i = j
		}
	}
	m.cursor = i
}

// nextSelectable returns the nearest selectable row index from start moving in
// direction dir, or -1 if none exists.
func nextSelectable(rows []row, start, dir int) int {
	for i := start + dir; i >= 0 && i < len(rows); i += dir {
		if rows[i].kind != rowSep {
			return i
		}
	}
	return -1
}

func (m model) selectedRow() (row, bool) {
	list := m.rows()
	if m.cursor < 0 || m.cursor >= len(list) {
		return row{}, false
	}
	return list[m.cursor], true
}

// isEnum reports whether opt is an enum flag or enum positional.
func (m model) isEnum(opt Option) bool {
	return opt.OptType() == KindEnum
}

func (m model) isPath(opt Option) bool {
	return opt.OptType() == KindPath
}

// editableKind reports whether opt opens an inline text editor when activated,
// as opposed to a menu or picker.
func editableKind(opt Option) bool {
	switch opt.OptType() {
	case KindString, KindInt, KindSize:
		return true
	case KindEnum:
		_, isFlag := opt.(Flag)
		return isFlag
	}
	return false
}

// rowIndexOf returns the row index of the plain option named name, or 0 if it
// is not present.
func (m model) rowIndexOf(name string) int {
	return m.rowIndexOfKind(name, rowOption)
}

// rowIndexOfKind returns the row index of the option named name with the given
// kind, falling back to the first row with that name, or 0.
func (m model) rowIndexOfKind(name string, kind rowKind) int {
	fallback := -1
	for i, r := range m.rows() {
		if r.opt == nil || r.opt.OptName() != name {
			continue
		}
		if r.kind == kind {
			return i
		}
		if fallback < 0 {
			fallback = i
		}
	}
	if fallback >= 0 {
		return fallback
	}
	return 0
}

func valueLabel(name string, idx int) string {
	return name + "[" + strconv.Itoa(idx) + "]: "
}
