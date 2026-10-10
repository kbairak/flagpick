// This file handles choosing and reconciling subcommands: the SUBCOMMAND
// placeholder in the preview and the subcommand list.
package main

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
