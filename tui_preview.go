// This file builds the WYSIWYG preview: the command rendered as an ordered
// list of tokens (emitted args, one empty-option slot, positional slots) with a
// single focused token and a live validity signal.
package main

import (
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// pvKind classifies a preview token.
type pvKind int

const (
	pvEmitted     pvKind = iota // a real arg that will be sent (white)
	pvPlaceholder               // an unfilled positional slot (gray)
	pvEmpty                     // the synthetic empty-option slot (gray)
	pvSub                       // an unchosen subcommand placeholder (gray)
	pvVerb                      // a chosen subcommand verb (white, focusable)
)

// pvTok is one token of the preview line.
type pvTok struct {
	kind      pvKind
	text      string
	name      string // option/positional name; "" for the command token
	flag      Flag
	pos       Positional
	valIdx    int // index into the backing value list, or -1
	segIdx    int // path segment the token belongs to, or -1
	subLevel  int // subcommand depth for pvSub/pvVerb, else -1
	focusable bool
}

// previewTokens renders the active path as preview tokens. For every segment it
// emits filled flags (session order), an empty-option slot when the shape has
// flags, the positional slots, and then either the chosen subcommand verb or a
// SUBCOMMAND placeholder.
func (m model) previewTokens() []pvTok {
	toks := []pvTok{{kind: pvEmitted, text: m.cmd, segIdx: -1, subLevel: -1}}
	for i, seg := range m.res().Segments {
		byName := make(map[string]Flag, len(seg.Flags))
		for _, f := range seg.Flags {
			byName[f.OptName()] = f
		}
		for _, name := range m.session.Order {
			if !seg.flagSet[name] {
				continue
			}
			f, ok := byName[name]
			if !ok {
				continue
			}
			for vi, t := range f.Assemble(m.session.Flags[name]) {
				toks = append(toks, pvTok{kind: pvEmitted, text: t, name: name, flag: f, valIdx: vi, segIdx: i, subLevel: -1, focusable: true})
			}
		}
		if len(seg.Flags) > 0 {
			toks = append(toks, pvTok{kind: pvEmpty, text: "--option", valIdx: -1, segIdx: i, subLevel: -1, focusable: true})
		}
		for _, p := range seg.Positionals {
			vals := m.session.PosVals[p.OptName()]
			if hasValue(vals) {
				for vi, v := range vals {
					toks = append(toks, pvTok{kind: pvEmitted, text: v, name: p.OptName(), pos: p, valIdx: vi, segIdx: i, subLevel: -1, focusable: true})
				}
				continue
			}
			toks = append(toks, pvTok{kind: pvPlaceholder, text: placeholderText(p), name: p.OptName(), pos: p, valIdx: -1, segIdx: i, subLevel: -1, focusable: true})
		}
		switch {
		case seg.Sub != nil:
			toks = append(toks, pvTok{kind: pvVerb, text: seg.Sub.Name, valIdx: -1, segIdx: i, subLevel: i, focusable: true})
		case len(seg.Node.Subcommands) > 0:
			toks = append(toks, pvTok{kind: pvSub, text: "SUBCOMMAND", valIdx: -1, segIdx: i, subLevel: i, focusable: true})
		}
	}
	return toks
}

// placeholderText is the gray label for an unfilled positional slot.
func placeholderText(p Positional) string {
	if p.Required() {
		return p.OptName()
	}
	return "[" + p.OptName() + "]"
}

// focusedToken returns the preview token under the focus index.
func (m model) focusedToken() (pvTok, bool) {
	toks := m.previewTokens()
	if m.focus < 0 || m.focus >= len(toks) || !toks[m.focus].focusable {
		return pvTok{}, false
	}
	return toks[m.focus], true
}

// optionsSegment returns the path segment whose flags the Options pane should
// show: the focused token's segment, or the deepest segment when nothing is
// focused.
func (m model) optionsSegment() int {
	if t, ok := m.focusedToken(); ok && t.segIdx >= 0 {
		return t.segIdx
	}
	segs := m.res().Segments
	if len(segs) == 0 {
		return -1
	}
	return len(segs) - 1
}

// segmentFlags returns the flags of the segment the Options pane is scoped to.
func (m model) segmentFlags() []Flag {
	segs := m.res().Segments
	idx := m.optionsSegment()
	if idx < 0 || idx >= len(segs) {
		return nil
	}
	return segs[idx].Flags
}

// moveFocus moves the focus to the next/previous focusable token, wrapping.
// Moving focus clears the option-list selection.
func (m *model) moveFocus(delta int) {
	m.cursor = -1
	toks := m.previewTokens()
	var idx []int
	for i, t := range toks {
		if t.focusable {
			idx = append(idx, i)
		}
	}
	if len(idx) == 0 {
		m.focus = 0
		return
	}
	pos := 0
	for j, i := range idx {
		if i == m.focus {
			pos = j
			break
		}
	}
	pos = ((pos+delta)%len(idx) + len(idx)) % len(idx)
	m.focus = idx[pos]
}

// focusEmptySlot puts the focus on the natural default target: the deepest
// segment's empty-option slot, then the SUBCOMMAND placeholder, then the first
// placeholder slot, then any focusable token. It clears the option-list
// selection (nothing is highlighted until the user filters or navigates).
func (m *model) focusEmptySlot() {
	m.cursor = -1
	toks := m.previewTokens()
	for i := len(toks) - 1; i >= 0; i-- {
		if toks[i].kind == pvEmpty {
			m.focus = i
			return
		}
	}
	for i, t := range toks {
		if t.kind == pvSub {
			m.focus = i
			return
		}
	}
	for i, t := range toks {
		if t.kind == pvPlaceholder {
			m.focus = i
			return
		}
	}
	for i, t := range toks {
		if t.focusable {
			m.focus = i
			return
		}
	}
	m.focus = 0
}

// focusFirstPositional moves focus to the first positional slot, reporting
// whether one exists.
func (m *model) focusFirstPositional() bool {
	for i, t := range m.previewTokens() {
		if t.pos != nil {
			m.focus = i
			return true
		}
	}
	return false
}

// focusPendingSub moves focus to the deepest SUBCOMMAND placeholder, reporting
// whether one exists.
func (m *model) focusPendingSub() bool {
	toks := m.previewTokens()
	for i := len(toks) - 1; i >= 0; i-- {
		if toks[i].kind == pvSub {
			m.focus = i
			return true
		}
	}
	return false
}

// focusAfterActivate moves focus after an option has been applied: to the next
// positional when a positional was just filled, otherwise to the empty-option
// slot.
func (m *model) focusAfterActivate(name string) {
	if _, isPos := m.cfg.positional(name); !isPos {
		m.focusEmptySlot()
		return
	}
	m.focusNextPositionalAfter(name)
}

// focusNextPositionalAfter moves focus to the positional that follows the one
// named name, falling back to the empty-option slot when there is none.
func (m *model) focusNextPositionalAfter(name string) {
	toks := m.previewTokens()
	start := -1
	for i, t := range toks {
		if t.pos != nil && t.name == name {
			start = i
			break
		}
	}
	if start < 0 {
		m.focusEmptySlot()
		return
	}
	for i := start + 1; i < len(toks); i++ {
		if toks[i].pos != nil && toks[i].name != name {
			m.focus = i
			return
		}
	}
	m.focusEmptySlot()
}

// focusedIsPositional reports whether the focused token is a positional slot.
func (m model) focusedIsPositional() bool {
	t, ok := m.focusedToken()
	return ok && t.pos != nil
}

// focusedIsSub reports whether the focused token is a subcommand placeholder or
// verb, in which case the option list shows that level's subcommands.
func (m model) focusedIsSub() bool {
	t, ok := m.focusedToken()
	return ok && (t.kind == pvSub || t.kind == pvVerb)
}

// focusedSubLevel returns the subcommand depth of the focused subcommand token,
// reporting whether the focused token is a subcommand at all.
func (m model) focusedSubLevel() (int, bool) {
	t, ok := m.focusedToken()
	if !ok || (t.kind != pvSub && t.kind != pvVerb) {
		return 0, false
	}
	return t.subLevel, true
}

// descriptionText returns the description to show in the Description pane: the
// focused token's option, else the Options-list selection (a flag/positional or
// a subcommand).
func (m model) descriptionText() (string, bool) {
	if t, ok := m.focusedToken(); ok {
		if t.flag != nil {
			return t.flag.OptDescription(), true
		}
		if t.pos != nil {
			return t.pos.OptDescription(), true
		}
	}
	if r, ok := m.selectedRow(); ok {
		if r.kind == rowSub && r.sub != nil {
			return r.sub.Description, true
		}
		if r.opt != nil {
			return r.opt.OptDescription(), true
		}
	}
	return "", false
}

// focusNamed puts the focus on the first token backed by name, falling back to
// the empty slot.
func (m *model) focusNamed(name string) {
	for i, t := range m.previewTokens() {
		if t.name == name && t.focusable {
			m.focus = i
			return
		}
	}
	m.focusEmptySlot()
}

// validity reports whether the command is runnable, and the reason when not.
func (m model) validity() (bool, string) {
	res := m.res()
	if m.pendingSub() {
		return false, "choose a subcommand"
	}
	if !modeIncludesApplied(res, m.session) {
		return false, "no mode matches the current options"
	}
	if missing := MissingRequired(res, m.session); len(missing) > 0 {
		return false, "required: " + strings.Join(missing, ", ")
	}
	return true, ""
}

// pvSeg is a styled preview token plus its visible width.
type pvSeg struct {
	styled string
	width  int
}

// previewSegments renders every token and sums the line's visible width.
func (m model) previewSegments() ([]pvSeg, int) {
	toks := m.previewTokens()
	segs := make([]pvSeg, 0, len(toks))
	total := 0
	for i, t := range toks {
		focused := i == m.focus && t.focusable
		styled, w := m.renderToken(t, focused)
		segs = append(segs, pvSeg{styled, w})
		total += w
	}
	if len(segs) > 1 {
		total += len(segs) - 1
	}
	return segs, total
}

// renderToken renders one token, substituting the live input while editing it.
// Emitted arguments are shell-quoted when needed, except while being edited.
func (m model) renderToken(t pvTok, focused bool) (string, int) {
	plain := t.text
	switch {
	case m.editing && focused && (t.name == m.editingName || t.kind == pvEmpty):
		plain = m.editingText()
	case t.kind == pvEmitted:
		plain = shellQuotePart(plain)
	}
	var styled string
	switch {
	case focused:
		styled = activeStyle.Render(plain)
	case t.kind == pvPlaceholder || t.kind == pvEmpty || t.kind == pvSub:
		styled = dimStyle.Render(plain)
	default:
		styled = previewEmittedStyle.Render(plain)
	}
	return styled, lipgloss.Width(styled)
}

// editingText is the live text shown for the token currently being filled: a
// value flag renders as --long=value, a positional as its value.
func (m model) editingText() string {
	if f, ok := m.cfg.flag(m.editingName); ok {
		return f.Long() + "=" + m.input.Value() + "▏"
	}
	return m.input.Value() + "▏"
}

// wrapSegments greedily wraps styled segments to width pw on token boundaries.
func wrapSegments(segs []pvSeg, pw int) []string {
	if pw < 1 {
		pw = 1
	}
	var lines []string
	var cur []string
	curW := 0
	for _, s := range segs {
		if len(cur) == 0 {
			cur = append(cur, s.styled)
			curW = s.width
			continue
		}
		if curW+1+s.width > pw {
			lines = append(lines, strings.Join(cur, " "))
			cur = []string{s.styled}
			curW = s.width
			continue
		}
		cur = append(cur, s.styled)
		curW += 1 + s.width
	}
	if len(cur) > 0 {
		lines = append(lines, strings.Join(cur, " "))
	}
	if len(lines) == 0 {
		lines = []string{""}
	}
	return lines
}

// previewFrame renders the full-width Preview box as a slice of lines.
func (m model) previewFrame() []string {
	cw := m.contentWidth()
	if cw < 1 {
		cw = 1
	}
	segs, _ := m.previewSegments()
	valid, reason := m.validity()

	pw := cw - 2
	if pw < 1 {
		pw = 1
	}

	wrapped := wrapSegments(segs, pw)

	// Budget: leave the status line and at least a one-line panes box. The
	// title now lives in the top border, so the body is just the token lines
	// plus the optional reason.
	maxTokenLines := m.height - 7
	if maxTokenLines < 1 {
		maxTokenLines = 1
	}
	showReason := !valid && len(wrapped)+1 <= maxTokenLines
	limit := maxTokenLines
	if showReason {
		limit = maxTokenLines - 1
	}
	if limit < 1 {
		limit = 1
	}
	if len(wrapped) > limit {
		wrapped = wrapped[:limit]
	}

	var body []string
	body = append(body, wrapped...)
	if showReason {
		body = append(body, errStyle.Render(truncate(reason, pw)))
	}

	budget := pw - 3
	if budget < 1 {
		budget = 1
	}
	box := titledBox(m.previewTitle(budget, valid), strings.Join(body, "\n"), pw, len(body))
	return strings.Split(box, "\n")
}

// previewTitle renders the box title: the validity glyph plus, when the command
// has more than one mode, a segmented mode control with the active mode
// highlighted.
func (m model) previewTitle(pw int, valid bool) string {
	glyph := "✓"
	if !valid {
		glyph = "✗"
	}
	base := titleStyle.Render("Preview " + glyph)
	sibs := m.modeSiblings()
	if len(sibs) < 2 {
		return base
	}
	used := lipgloss.Width(base)
	var parts []string
	for _, i := range sibs {
		name := m.modes[i].Name
		if name == "" {
			name = "mode"
		}
		seg := dimStyle.Render(name)
		if i == m.mode {
			seg = activeStyle.Render(name)
		}
		w := lipgloss.Width(seg)
		if used+2+w > pw {
			break
		}
		parts = append(parts, seg)
		used += 2 + w
	}
	if len(parts) == 0 {
		return base
	}
	return base + "  " + strings.Join(parts, "  ")
}

// previewTotalHeight is the number of rendered preview-box lines.
func (m model) previewTotalHeight() int {
	return len(m.previewFrame())
}
