package main

type OptionKind string

const (
	KindBool        OptionKind = "bool"
	KindString      OptionKind = "string"
	KindInt         OptionKind = "int"
	KindEnum        OptionKind = "enum"
	KindSize        OptionKind = "size"
	KindCount       OptionKind = "count"
	KindPath        OptionKind = "path"
	KindPassthrough OptionKind = "passthrough"
)

// BaseOption holds fields common to every flag and positional.
type BaseOption struct {
	Name        string     `yaml:"name"`
	Type        OptionKind `yaml:"type"`
	Description string     `yaml:"description"`
}

func (b *BaseOption) OptName() string        { return b.Name }
func (b *BaseOption) OptType() OptionKind    { return b.Type }
func (b *BaseOption) OptDescription() string { return b.Description }

type Option interface {
	OptName() string
	OptType() OptionKind
	OptDescription() string
}

// SubcommandOption is a TUI-only pseudo option that selects a subcommand from
// a shape's alternatives. It is never assembled as an argument: the chosen
// verb is emitted separately by Assemble.
type SubcommandOption struct {
	BaseOption
	Level int
	Label string
}

// ---- flags ----

// FlagState is the runtime state of one flag. Bool flags use Checked; value
// flags use Values (a single element for non-repeatable flags); count flags use
// Count.
type FlagState struct {
	Checked bool
	Neg     bool
	Values  []string
	Count   int
}

// First returns the first value, or "" when there is none.
func (s FlagState) First() string {
	if len(s.Values) == 0 {
		return ""
	}
	return s.Values[0]
}

// Filled reports whether the flag has any value, count, or explicit state.
func (s FlagState) Filled() bool {
	if s.Checked || s.Neg || s.Count > 0 {
		return true
	}
	for _, v := range s.Values {
		if v != "" {
			return true
		}
	}
	return false
}

type Flag interface {
	Option
	Long() string
	Short() string
	Negative() string
	NegativeShort() string
	Repeatable() bool
	Conflicts() []string
	Requires() []string
	// TakesValue reports whether the flag consumes a value argument.
	TakesValue() bool
	// Assemble returns the argv tokens this flag contributes.
	Assemble(state FlagState) []string
	isFlag() // sealed: only kinds in this package implement Flag
}

type FlagBase struct {
	BaseOption        `yaml:",inline"`
	LongForm          string   `yaml:"long"`
	ShortForm         string   `yaml:"short"`
	NegativeForm      string   `yaml:"negative"`
	NegativeShortForm string   `yaml:"negative_short"`
	IsRepeatable      bool     `yaml:"repeatable"`
	ConflictList      []string `yaml:"conflicts"`
	RequireList       []string `yaml:"requires"`
}

func (f *FlagBase) Long() string          { return f.LongForm }
func (f *FlagBase) Short() string         { return f.ShortForm }
func (f *FlagBase) Negative() string      { return f.NegativeForm }
func (f *FlagBase) NegativeShort() string { return f.NegativeShortForm }
func (f *FlagBase) Repeatable() bool      { return f.IsRepeatable }
func (f *FlagBase) Conflicts() []string   { return f.ConflictList }
func (f *FlagBase) Requires() []string    { return f.RequireList }

type BoolFlag struct {
	FlagBase `yaml:",inline"`
}

func (f *BoolFlag) isFlag()          {} // concrete marker; FlagBase deliberately does NOT implement Flag
func (f *BoolFlag) TakesValue() bool { return false }
func (f *BoolFlag) Assemble(state FlagState) []string {
	if state.Checked {
		return []string{f.LongForm}
	}
	if state.Neg && f.NegativeForm != "" {
		return []string{f.NegativeForm}
	}
	return nil
}

// StringFlag is a non-repeatable flag that takes one string value and emits it
// as a single --long=value token. Empty values emit nothing.
type StringFlag struct {
	FlagBase `yaml:",inline"`
}

func (f *StringFlag) isFlag()          {}
func (f *StringFlag) TakesValue() bool { return true }
func (f *StringFlag) Assemble(state FlagState) []string {
	return valueTokens(f.LongForm, state)
}

// IntFlag is a non-repeatable flag whose value must be a non-negative integer.
type IntFlag struct {
	FlagBase `yaml:",inline"`
}

func (f *IntFlag) isFlag()          {}
func (f *IntFlag) TakesValue() bool { return true }
func (f *IntFlag) Assemble(state FlagState) []string {
	return valueTokens(f.LongForm, state)
}

// valueTokens emits one --long=value token per non-empty value.
func valueTokens(long string, state FlagState) []string {
	var out []string
	for _, v := range state.Values {
		if v != "" {
			out = append(out, long+"="+v)
		}
	}
	return out
}

// EnumFlag is a value flag restricted to a fixed set of values.
type EnumFlag struct {
	FlagBase      `yaml:",inline"`
	AllowedValues []string `yaml:"values"`
}

func (f *EnumFlag) isFlag()           {}
func (f *EnumFlag) TakesValue() bool  { return true }
func (f *EnumFlag) Allowed() []string { return f.AllowedValues }
func (f *EnumFlag) Assemble(state FlagState) []string {
	return valueTokens(f.LongForm, state)
}

// SizeFlag is a value flag of the form NUM with an optional K/M/G suffix.
type SizeFlag struct {
	FlagBase `yaml:",inline"`
}

func (f *SizeFlag) isFlag()          {}
func (f *SizeFlag) TakesValue() bool { return true }
func (f *SizeFlag) Assemble(state FlagState) []string {
	return valueTokens(f.LongForm, state)
}

// CountFlag is a valueless flag that can be repeated (e.g. -u/-uu/-uuu). It
// emits its long form once per count. Zero emits nothing.
type CountFlag struct {
	FlagBase `yaml:",inline"`
}

func (f *CountFlag) isFlag()          {}
func (f *CountFlag) TakesValue() bool { return false }
func (f *CountFlag) Assemble(state FlagState) []string {
	var out []string
	for i := 0; i < state.Count; i++ {
		out = append(out, f.LongForm)
	}
	return out
}

// isSizeValue reports whether s is NUM optionally followed by K/M/G.
func isSizeValue(s string) bool {
	if s == "" {
		return false
	}
	last := s[len(s)-1]
	digits := s
	switch last {
	case 'k', 'K', 'm', 'M', 'g', 'G':
		digits = s[:len(s)-1]
	}
	return isNumeric(digits)
}

// ---- positionals ----

// PosState is the runtime state of one positional. Scalars hold at most one
// value; variadic positionals hold zero or more, emitted in order.
type PosState struct {
	Values []string
}

type Positional interface {
	Option
	Required() bool
	Variadic() bool
	Assemble(state PosState) []string
	isPositional() // sealed
}

type PositionalBase struct {
	BaseOption `yaml:",inline"`
	IsRequired bool `yaml:"required"`
	IsVariadic bool `yaml:"variadic"`
}

func (p *PositionalBase) Required() bool { return p.IsRequired }
func (p *PositionalBase) Variadic() bool { return p.IsVariadic }

type StringPositional struct {
	PositionalBase `yaml:",inline"`
}

func (p *StringPositional) isPositional()                    {}
func (p *StringPositional) Assemble(state PosState) []string { return assembleValues(state) }

// PathPositional is a string positional whose values are chosen from the
// filesystem via a picker rather than typed. It emits its values verbatim,
// like a StringPositional.
type PathPositional struct {
	PositionalBase `yaml:",inline"`
}

func (p *PathPositional) isPositional()                    {}
func (p *PathPositional) Assemble(state PosState) []string { return assembleValues(state) }

// EnumPositional is a positional restricted to a fixed set of values.
type EnumPositional struct {
	PositionalBase `yaml:",inline"`
	AllowedValues  []string `yaml:"values"`
}

func (p *EnumPositional) isPositional()                    {}
func (p *EnumPositional) Allowed() []string                { return p.AllowedValues }
func (p *EnumPositional) Assemble(state PosState) []string { return assembleValues(state) }

// PassthroughPositional swallows every remaining token verbatim. It is always
// variadic, so `variadic` is not a valid field on it.
type PassthroughPositional struct {
	PositionalBase `yaml:",inline"`
}

func (p *PassthroughPositional) isPositional()                    {}
func (p *PassthroughPositional) Variadic() bool                   { return true }
func (p *PassthroughPositional) Assemble(state PosState) []string { return assembleValues(state) }

func assembleValues(state PosState) []string {
	var out []string
	for _, v := range state.Values {
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

// isNumeric reports whether s is a non-empty run of ASCII digits.
func isNumeric(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
