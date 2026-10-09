// This file selects, cycles, and reconciles the active mode/path through the
// config tree.
package main

import "reflect"

// detectMode returns the first mode that accepts the current state.
func detectMode(modes []Resolved, session *Session) int {
	for i := range modes {
		if modeIncludesApplied(modes[i], session) {
			return i
		}
	}
	return 0
}

// modeIncludesApplied reports whether r contains every applied flag and
// positional.
func modeIncludesApplied(r Resolved, session *Session) bool {
	for name, fs := range session.Flags {
		if (fs.Checked || fs.Filled()) && !r.HasFlag(name) {
			return false
		}
	}
	for name, vals := range session.PosVals {
		if hasValue(vals) && !r.HasPositional(name) {
			return false
		}
	}
	return true
}

// subcommandChoices returns the subcommand branch choices of a path.
func subcommandChoices(r Resolved) []BranchChoice {
	var out []BranchChoice
	for _, c := range r.BranchChoices {
		if c.Kind == BranchSubcommand {
			out = append(out, c)
		}
	}
	return out
}

// modeSiblings returns the path indices reachable from the current path by
// changing only mode choices (subcommands are held fixed).
func (m *model) modeSiblings() []int {
	cur := subcommandChoices(m.res())
	var out []int
	for i, p := range m.modes {
		if reflect.DeepEqual(subcommandChoices(p), cur) {
			out = append(out, i)
		}
	}
	return out
}

// sessionEmpty reports whether no flag or positional carries a value.
func sessionEmpty(s *Session) bool {
	for _, fs := range s.Flags {
		if fs.Checked || fs.Filled() {
			return false
		}
	}
	for _, vals := range s.PosVals {
		if hasValue(vals) {
			return false
		}
	}
	return true
}

// cycleMode switches to the next/previous sibling mode, re-parsing the composed
// argv into the target shape so the state round-trips (e.g. PATTERN becomes
// PATH). Shapes that cannot parse the command are skipped.
func (m *model) cycleMode(delta int) {
	cand := m.modeSiblings()
	if len(cand) < 2 {
		return
	}
	pos := 0
	for i, c := range cand {
		if c == m.mode {
			pos = i
			break
		}
	}
	tokens := Assemble(m.res(), m.session)
	empty := sessionEmpty(m.session)
	n := len(cand)
	for step := 1; step <= n; step++ {
		idx := cand[((pos+delta*step)%n+n)%n]
		if empty {
			m.mode = idx
			m.session = NewSession(m.cfg)
			m.afterModeChange()
			return
		}
		session, err := prefillPath(m.cfg, m.modes[idx], tokens)
		if err != nil {
			continue
		}
		m.mode = idx
		m.session = session
		m.afterModeChange()
		return
	}
}

// switchMode re-parses the current command into mode idx, reporting success.
func (m *model) switchMode(idx int) bool {
	tokens := Assemble(m.res(), m.session)
	session, err := prefillPath(m.cfg, m.modes[idx], tokens)
	if err != nil {
		return false
	}
	m.mode = idx
	m.session = session
	return true
}

// syncMode keeps the current mode while it still includes every applied flag,
// and otherwise switches to the first mode that accepts the state.
func (m *model) syncMode() {
	if len(m.modes) < 2 {
		return
	}
	if modeIncludesApplied(m.res(), m.session) {
		return
	}
	next := detectMode(m.modes, m.session)
	if next == m.mode {
		return
	}
	if !m.switchMode(next) {
		m.mode = next
		m.reconcileMode()
	}
	m.afterModeChange()
}

// afterModeChange resets the option filter and preview focus after the active
// shape changes.
func (m *model) afterModeChange() {
	m.resetFilter()
	m.focusEmptySlot()
}

// reconcileMode clears flag and positional state not present in the active
// mode.
func (m *model) reconcileMode() {
	res := m.res()
	for name, fs := range m.session.Flags {
		if (fs.Checked || fs.Filled()) && !res.HasFlag(name) {
			m.session.Flags[name] = FlagState{}
		}
	}
	for name, vals := range m.session.PosVals {
		if len(vals) > 0 && !res.HasPositional(name) {
			m.session.PosVals[name] = nil
		}
	}
}
