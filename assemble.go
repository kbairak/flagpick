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

// Assemble builds the argv tokens deterministically: the mode's flags in
// session order, then its positionals in schema order.
func Assemble(res Resolved, s *Session) []string {
	byName := make(map[string]Flag, len(res.Flags))
	for _, f := range res.Flags {
		byName[f.OptName()] = f
	}
	var out []string = []string{}
	for _, name := range s.Order {
		if !res.HasFlag(name) {
			continue
		}
		f, ok := byName[name]
		if !ok {
			continue
		}
		out = append(out, f.Assemble(s.Flags[name])...)
	}
	for _, p := range res.Positionals {
		out = append(out, p.Assemble(PosState{Values: s.PosVals[p.OptName()]})...)
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

// Prefill applies tool tokens to a fresh session. It tries each mode in order
// and returns the first that parses; a token matching no mode is an error.
func Prefill(cfg *CommandConfig, tokens []string) (*Session, error) {
	var firstErr error
	for _, res := range cfg.Resolved() {
		s, err := prefillWith(cfg, res, tokens)
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

func prefillWith(cfg *CommandConfig, res Resolved, tokens []string) (*Session, error) {
	s := NewSession(cfg)
	byLong := map[string]Flag{}
	byShort := map[string]Flag{}
	byNeg := map[string]Flag{}
	for _, f := range res.Flags {
		byLong[f.Long()] = f
		if f.Short() != "" {
			byShort[f.Short()] = f
		}
		if neg := f.Negative(); neg != "" {
			byNeg[neg] = f
		}
	}
	posIdx := 0
	for i := 0; i < len(tokens); i++ {
		tok := tokens[i]
		if f, ok := byLong[tok]; ok {
			if f.TakesValue() {
				i++
				if i >= len(tokens) {
					return nil, fmt.Errorf("missing value for %s", f.Long())
				}
				if err := setFlagValue(s, f, tokens[i]); err != nil {
					return nil, err
				}
				continue
			}
			applyPrefillFlag(cfg, s, f)
			continue
		}
		if f, ok := byNeg[tok]; ok {
			state := s.Flags[f.OptName()]
			state.Checked = false
			state.Neg = true
			s.Flags[f.OptName()] = state
			continue
		}
		if f, ok := byShort[tok]; ok {
			if f.TakesValue() {
				i++
				if i >= len(tokens) {
					return nil, fmt.Errorf("missing value for %s", f.Short())
				}
				if err := setFlagValue(s, f, tokens[i]); err != nil {
					return nil, err
				}
				continue
			}
			applyPrefillFlag(cfg, s, f)
			continue
		}
		// -uu / -uuu: repeated count short form.
		if f, n, ok := matchCountShort(tok, byShort); ok {
			state := s.Flags[f.OptName()]
			state.Count += n
			s.Flags[f.OptName()] = state
			continue
		}
		// --long=value
		if name, val, ok := strings.Cut(tok, "="); ok {
			if f, ok := byLong[name]; ok && f.TakesValue() {
				if err := setFlagValue(s, f, val); err != nil {
					return nil, err
				}
				continue
			}
		}
		// -rVALUE or -r=VALUE
		if f, ok := matchShortPrefix(tok, byShort); ok {
			val := strings.TrimPrefix(tok, f.Short())
			val = strings.TrimPrefix(val, "=")
			if err := setFlagValue(s, f, val); err != nil {
				return nil, err
			}
			continue
		}
		if strings.HasPrefix(tok, "-") {
			return nil, fmt.Errorf("unrecognized argument: %s", tok)
		}
		if posIdx >= len(res.Positionals) {
			return nil, fmt.Errorf("too many positional arguments")
		}
		p := res.Positionals[posIdx]
		if p.Variadic() {
			s.PosVals[p.OptName()] = append(s.PosVals[p.OptName()], tok)
			continue
		}
		s.PosVals[p.OptName()] = []string{tok}
		posIdx++
	}
	return s, nil
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
