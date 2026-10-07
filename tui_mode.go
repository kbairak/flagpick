// This file selects and reconciles the active mode/path through the config tree.
package main

import (
	"reflect"
)

// detectMode returns the first mode that accepts the current state.
func detectMode(modes []Resolved, session *Session) int {
	for i := range modes {
		if modeIncludesApplied(modes[i], session) {
			return i
		}
	}
	return 0
}

// acceptsAt reports whether mode i accepts the applied flags: all applied
// flags are in the mode, and a non-default mode additionally requires at least
// one flag exclusive to it (so e.g. Regexp needs an -e value).
func acceptsAt(modes []Resolved, i int, session *Session) bool {
	r := modes[i]
	exclusive := false
	for name, fs := range session.Flags {
		if !fs.Checked && !fs.Filled() {
			continue
		}
		if !r.HasFlag(name) {
			return false
		}
		if i > 0 && !modes[0].HasFlag(name) {
			exclusive = true
		}
	}
	return i == 0 || exclusive
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
}

// modeIncludesApplied reports whether r contains every applied flag and
// positional (ignoring the exclusivity rule, so a manually chosen mode or
// subcommand is respected).
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

// cycleMode switches to the next/previous mode whose parser accepts the
// current command, re-parsing the composed argv so the state round-trips
// (e.g. PATTERN becomes PATH). Only mode alternatives are considered:
// subcommands are chosen from the options pane. Modes that can't parse are
// skipped.
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
		// With nothing applied there is no composed argv to re-parse, so hop
		// directly to the next mode.
		if empty {
			m.mode = idx
			m.session = NewSession(m.cfg)
			m.filter = ""
			m.cursor = 0
			m.descScroll = 0
			return
		}
		session, err := prefillPath(m.cfg, m.modes[idx], tokens)
		if err != nil {
			continue
		}
		m.mode = idx
		m.session = session
		m.filter = ""
		m.cursor = 0
		m.descScroll = 0
		return
	}
}

// modeSiblings returns the path indices reachable from the current path by
// changing only mode choices (subcommands are held fixed).
func (m *model) modeSiblings() []int {
	curSubs := subcommandChoices(m.res())
	var out []int
	for i, p := range m.modes {
		if reflect.DeepEqual(subcommandChoices(p), curSubs) {
			out = append(out, i)
		}
	}
	return out
}

func subcommandChoices(r Resolved) []BranchChoice {
	var out []BranchChoice
	for _, c := range r.BranchChoices {
		if c.Kind == BranchSubcommand {
			out = append(out, c)
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
