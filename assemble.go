package main

import (
	"fmt"
	"strings"
)

type Session struct {
	Flags   map[string]FlagState // flag name -> state
	Order   []string             // flag names, emit order (starts as schema order)
	PosVals map[string][]string  // positional name -> values (scalars hold <=1)
}

// NewSession builds a session with Order set to schema flag order.
func NewSession(cfg *CommandConfig) *Session {
	s := &Session{
		Flags:   map[string]FlagState{},
		Order:   make([]string, 0, len(cfg.Flags)),
		PosVals: map[string][]string{},
	}
	for _, f := range cfg.Flags {
		s.Flags[f.OptName()] = FlagState{}
		s.Order = append(s.Order, f.OptName())
	}
	for _, p := range cfg.Positionals {
		s.PosVals[p.OptName()] = nil
	}
	return s
}

// Assemble builds the argv tokens deterministically, depth-first: for each
// segment, its flags (in session order) then its positionals (in selection
// order), followed by the chosen child's verb token.
func Assemble(res Resolved, s *Session) []string {
	out := []string{}
	for _, seg := range res.Segments {
		byName := make(map[string]Flag, len(seg.Flags))
		for _, f := range seg.Flags {
			byName[f.OptName()] = f
		}
		for _, name := range s.Order {
			if !seg.flagSet[name] {
				continue
			}
			f, ok := byName[name]
			if !ok {
				continue
			}
			out = append(out, f.Assemble(s.Flags[name])...)
		}
		for _, p := range seg.Positionals {
			out = append(out, p.Assemble(PosState{Values: s.PosVals[p.OptName()]})...)
		}
		if seg.Sub != nil {
			out = append(out, seg.Sub.Name)
		}
	}
	return out
}

// ShellQuote joins parts with spaces, single-quoting any part that contains
// unsafe characters.
func ShellQuote(parts []string) string {
	quoted := make([]string, len(parts))
	for i, p := range parts {
		quoted[i] = shellQuotePart(p)
	}
	return strings.Join(quoted, " ")
}

func shellQuotePart(s string) string {
	if s == "" {
		return "''"
	}
	if isShellSafe(s) {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

func isShellSafe(s string) bool {
	for _, r := range s {
		switch {
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r >= '0' && r <= '9':
		case strings.ContainsRune("_@%+=:,./-", r):
		default:
			return false
		}
	}
	return true
}

// ToggleFlag flips a flag's checked state and, when it became checked,
// auto-unchecks every currently-checked flag in its own Conflicts() list.
func ToggleFlag(cfg *CommandConfig, s *Session, name string) {
	var f Flag
	for _, cand := range cfg.Flags {
		if cand.OptName() == name {
			f = cand
			break
		}
	}
	if f == nil {
		return
	}
	switch f.(type) {
	case *BoolFlag:
		state := s.Flags[name]
		state.Checked = !state.Checked
		s.Flags[name] = state
		if state.Checked {
			for _, c := range f.Conflicts() {
				if cs, ok := s.Flags[c]; ok && cs.Checked {
					cs.Checked = false
					s.Flags[c] = cs
				}
			}
		}
	}
}

// Prefill applies tool tokens to a fresh session. It tries each selectable
// path in order and returns the first that parses; no match is an error.
func Prefill(cfg *CommandConfig, tokens []string) (*Session, error) {
	// No tokens cannot fail: open on the first selectable path so the TUI can
	// present the tree (e.g. a dispatcher like `direnv` with nothing chosen).
	if len(tokens) == 0 {
		return NewSession(cfg), nil
	}
	var firstErr error
	for _, res := range cfg.Resolved() {
		s, err := prefillPath(cfg, res, tokens)
		if err == nil {
			return s, nil
		}
		if firstErr == nil {
			firstErr = err
		}
	}
	if firstErr == nil {
		firstErr = fmt.Errorf("no mode accepts the given arguments")
	}
	return nil, firstErr
}

// flagIndex maps the accepted spellings of one segment's flags to the flags.
type flagIndex struct {
	byLong     map[string]Flag
	byShort    map[string]Flag
	byNeg      map[string]Flag
	byNegShort map[string]Flag
}

func newFlagIndex(flags []Flag) flagIndex {
	idx := flagIndex{
		byLong:     map[string]Flag{},
		byShort:    map[string]Flag{},
		byNeg:      map[string]Flag{},
		byNegShort: map[string]Flag{},
	}
	for _, f := range flags {
		idx.byLong[f.Long()] = f
		if f.Short() != "" {
			idx.byShort[f.Short()] = f
		}
		if neg := f.Negative(); neg != "" {
			idx.byNeg[neg] = f
		}
		if negShort := f.NegativeShort(); negShort != "" {
			idx.byNegShort[negShort] = f
		}
	}
	return idx
}

// matchFlag tries to consume tok (and the following token for value flags) as
// one of idx's flags. It returns the number of tokens consumed (0 = no match).
func matchFlag(cfg *CommandConfig, idx flagIndex, tok, next string, hasNext bool, s *Session) (int, error) {
	if f, ok := idx.byLong[tok]; ok {
		if f.TakesValue() {
			if !hasNext {
				return 0, fmt.Errorf("missing value for %s", f.Long())
			}
			if err := setFlagValue(s, f, next); err != nil {
				return 0, err
			}
			return 2, nil
		}
		applyPrefillFlag(cfg, s, f)
		return 1, nil
	}
	if f, ok := idx.byNeg[tok]; ok {
		state := s.Flags[f.OptName()]
		state.Checked = false
		state.Neg = true
		s.Flags[f.OptName()] = state
		return 1, nil
	}
	if f, ok := idx.byNegShort[tok]; ok {
		state := s.Flags[f.OptName()]
		state.Checked = false
		state.Neg = true
		s.Flags[f.OptName()] = state
		return 1, nil
	}
	if f, ok := idx.byShort[tok]; ok {
		if f.TakesValue() {
			if !hasNext {
				return 0, fmt.Errorf("missing value for %s", f.Short())
			}
			if err := setFlagValue(s, f, next); err != nil {
				return 0, err
			}
			return 2, nil
		}
		applyPrefillFlag(cfg, s, f)
		return 1, nil
	}
	// -uu / -uuu: repeated count short form.
	if f, n, ok := matchCountShort(tok, idx.byShort); ok {
		state := s.Flags[f.OptName()]
		state.Count += n
		s.Flags[f.OptName()] = state
		return 1, nil
	}
	// --long=value
	if name, val, ok := strings.Cut(tok, "="); ok {
		if f, ok := idx.byLong[name]; ok && f.TakesValue() {
			if err := setFlagValue(s, f, val); err != nil {
				return 0, err
			}
			return 1, nil
		}
	}
	// -rVALUE or -r=VALUE
	if f, ok := matchShortPrefix(tok, idx.byShort); ok {
		val := strings.TrimPrefix(tok, f.Short())
		val = strings.TrimPrefix(val, "=")
		if err := setFlagValue(s, f, val); err != nil {
			return 0, err
		}
		return 1, nil
	}
	return 0, nil
}

func prefillPath(cfg *CommandConfig, res Resolved, tokens []string) (*Session, error) {
	if len(res.Segments) == 0 {
		return nil, fmt.Errorf("empty path")
	}
	s := NewSession(cfg)
	si := 0
	posIdx := 0
	passName := ""
	idx := newFlagIndex(res.Segments[si].Flags)
	for i := 0; i < len(tokens); i++ {
		seg := res.Segments[si]
		tok := tokens[i]
		if passName != "" {
			s.PosVals[passName] = append(s.PosVals[passName], tok)
			continue
		}
		next := ""
		if i+1 < len(tokens) {
			next = tokens[i+1]
		}
		n, err := matchFlag(cfg, idx, tok, next, i+1 < len(tokens), s)
		if err != nil {
			return nil, err
		}
		if n > 0 {
			i += n - 1
			continue
		}
		if strings.HasPrefix(tok, "-") {
			return nil, fmt.Errorf("unrecognized argument: %s", tok)
		}
		if seg.Sub != nil {
			if !verbMatch(seg.Sub, tok) {
				return nil, fmt.Errorf("unrecognized argument: %s", tok)
			}
			si++
			posIdx = 0
			idx = newFlagIndex(res.Segments[si].Flags)
			continue
		}
		if posIdx >= len(seg.Positionals) {
			return nil, fmt.Errorf("too many positional arguments")
		}
		p := seg.Positionals[posIdx]
		if err := validatePositionalValue(p, tok); err != nil {
			return nil, err
		}
		if _, ok := p.(*PassthroughPositional); ok {
			passName = p.OptName()
		}
		if p.Variadic() {
			s.PosVals[p.OptName()] = append(s.PosVals[p.OptName()], tok)
			continue
		}
		s.PosVals[p.OptName()] = []string{tok}
		posIdx++
	}
	if si != len(res.Segments)-1 {
		return nil, fmt.Errorf("missing subcommand")
	}
	return s, nil
}

// validatePositionalValue enforces kind-specific positional value rules.
func validatePositionalValue(p Positional, value string) error {
	e, ok := p.(*EnumPositional)
	if !ok {
		return nil
	}
	for _, a := range e.Allowed() {
		if a == value {
			return nil
		}
	}
	return fmt.Errorf("invalid value for %s: %q (want one of %s)", p.OptName(), value, strings.Join(e.Allowed(), ", "))
}

func verbMatch(sub *RNode, tok string) bool {
	if tok == sub.Name {
		return true
	}
	for _, a := range sub.Aliases {
		if a == tok {
			return true
		}
	}
	return false
}

// matchCountShort recognizes repeated count short forms like -uu for short
// "-u", returning the flag and the number of repeats.
func matchCountShort(tok string, byShort map[string]Flag) (Flag, int, bool) {
	for short, f := range byShort {
		if _, ok := f.(*CountFlag); !ok {
			continue
		}
		if short == "" || !strings.HasPrefix(tok, short) || len(tok) == len(short) {
			continue
		}
		last := short[len(short)-1]
		rest := tok[len(short):]
		same := true
		for i := 0; i < len(rest); i++ {
			if rest[i] != last {
				same = false
				break
			}
		}
		if same {
			return f, 1 + len(rest), true
		}
	}
	return nil, 0, false
}

// matchShortPrefix finds a value-taking short flag that tok starts with, e.g.
// "-rfoo" for short "-r". Returns false when tok has no such prefix.
func matchShortPrefix(tok string, byShort map[string]Flag) (Flag, bool) {
	for short, f := range byShort {
		if f.TakesValue() && len(tok) > len(short) && strings.HasPrefix(tok, short) {
			return f, true
		}
	}
	return nil, false
}

func setFlagValue(s *Session, f Flag, value string) error {
	if err := validateFlagValue(f, value); err != nil {
		return err
	}
	state := s.Flags[f.OptName()]
	if f.Repeatable() {
		state.Values = append(state.Values, value)
	} else {
		state.Values = []string{value}
	}
	s.Flags[f.OptName()] = state
	return nil
}

// validateFlagValue enforces kind-specific value rules.
func validateFlagValue(f Flag, value string) error {
	if value == "" {
		return nil
	}
	switch v := f.(type) {
	case *IntFlag:
		if !isNumeric(value) {
			return fmt.Errorf("invalid number for %s: %q", f.Long(), value)
		}
	case *EnumFlag:
		for _, a := range v.Allowed() {
			if a == value {
				return nil
			}
		}
		return fmt.Errorf("invalid value for %s: %q (want one of %s)", f.Long(), value, strings.Join(v.Allowed(), ", "))
	case *SizeFlag:
		if !isSizeValue(value) {
			return fmt.Errorf("invalid size for %s: %q", f.Long(), value)
		}
	}
	return nil
}

func applyPrefillFlag(cfg *CommandConfig, s *Session, f Flag) {
	switch f.(type) {
	case *BoolFlag:
		ToggleFlag(cfg, s, f.OptName())
	case *CountFlag:
		state := s.Flags[f.OptName()]
		state.Count++
		s.Flags[f.OptName()] = state
	}
}

// MissingRequired returns names of required positionals that have no value.
func MissingRequired(res Resolved, s *Session) []string {
	var out []string
	for _, p := range res.Positionals {
		if !p.Required() {
			continue
		}
		if hasValue(s.PosVals[p.OptName()]) {
			continue
		}
		out = append(out, p.OptName())
	}
	return out
}

func hasValue(vals []string) bool {
	for _, v := range vals {
		if v != "" {
			return true
		}
	}
	return false
}
