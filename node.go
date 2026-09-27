package main

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

// Node is one node of a config tree. A node is either a fork (it has `modes`)
// or a shape (it has its own arguments), never both. Every node may carry a
// `pool` and, when used as a subcommand, a `name` and `aliases`.
type Node struct {
	Pool        Pool     `yaml:"pool"`
	Options     []string `yaml:"options"`
	Usage       string   `yaml:"usage"`
	Description string   `yaml:"description"`
	Subcommands []*Node  `yaml:"subcommands"`
	Modes       []*Node  `yaml:"modes"`

	Name    string   `yaml:"name"`
	Aliases []string `yaml:"aliases"`

	hasModes bool
	hasShape bool
}

// Pool holds a node's reusable definitions and groups.
type Pool struct {
	Options OptionsList         `yaml:"options"`
	Groups  map[string][]string `yaml:"groups"`
}

func hasKey(n *yaml.Node, key string) bool {
	if n.Kind != yaml.MappingNode {
		return false
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return true
		}
	}
	return false
}

func checkKeys(n *yaml.Node, allowed map[string]bool) error {
	if n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i+1 < len(n.Content); i += 2 {
		if k := n.Content[i].Value; !allowed[k] {
			return fmt.Errorf("unknown field %q", k)
		}
	}
	return nil
}

func (n *Node) UnmarshalYAML(value *yaml.Node) error {
	if err := checkKeys(value, fieldSet(
		"pool", "options", "usage", "description", "subcommands",
		"modes", "name", "aliases",
	)); err != nil {
		return err
	}
	type raw Node
	var r raw
	if err := value.Decode(&r); err != nil {
		return err
	}
	*n = Node(r)
	n.hasModes = hasKey(value, "modes")
	// A fork owns only modes; usage/description are accepted as metadata (the
	// spec's recursion example annotates a mode-bearing subcommand with usage).
	n.hasShape = hasKey(value, "options") || hasKey(value, "subcommands")
	return nil
}

func (p *Pool) UnmarshalYAML(value *yaml.Node) error {
	if err := checkKeys(value, fieldSet("options", "groups")); err != nil {
		return err
	}
	type raw Pool
	var r raw
	if err := value.Decode(&r); err != nil {
		return err
	}
	*p = Pool(r)
	return nil
}

// scope is a node's visible pool: its own definitions/groups merged over those
// of all its ancestors (nearest definition wins).
type scope struct {
	defs   map[string]Option
	groups map[string][]string
}

func newScope() *scope {
	return &scope{defs: map[string]Option{}, groups: map[string][]string{}}
}

func (s *scope) clone() *scope {
	out := newScope()
	for k, v := range s.defs {
		out.defs[k] = v
	}
	for k, v := range s.groups {
		out.groups[k] = v
	}
	return out
}

// addPool overlays p onto the scope, validating p's own definitions. Names are
// unique within a node's own pool.options.
func (s *scope) addPool(p Pool) error {
	own := map[string]bool{}
	for _, o := range p.Options {
		name := o.OptName()
		if name == "" {
			return fmt.Errorf("pool option is missing a name")
		}
		if own[name] {
			return fmt.Errorf("duplicate option name %q", name)
		}
		own[name] = true
		if err := validateDef(o); err != nil {
			return err
		}
		s.defs[name] = o
	}
	for name, members := range p.Groups {
		if name == "" {
			return fmt.Errorf("group is missing a name")
		}
		s.groups[name] = members
	}
	return nil
}

func validateDef(o Option) error {
	if p, ok := o.(*EnumPositional); ok {
		if len(p.Allowed()) == 0 {
			return fmt.Errorf("positional %q: enum requires at least one value", p.OptName())
		}
	}
	f, ok := o.(Flag)
	if !ok {
		return nil
	}
	if f.Long() == "" {
		return fmt.Errorf("flag %q is missing a long form", f.OptName())
	}
	if e, ok := f.(*EnumFlag); ok && len(e.Allowed()) == 0 {
		return fmt.Errorf("flag %q: enum requires at least one value", f.OptName())
	}
	if neg := f.Negative(); neg != "" {
		if !strings.HasPrefix(neg, "--") {
			return fmt.Errorf("flag %q: negative must start with --", f.OptName())
		}
		if neg == f.Long() {
			return fmt.Errorf("flag %q: negative must differ from long", f.OptName())
		}
	}
	if negShort := f.NegativeShort(); negShort != "" {
		if !strings.HasPrefix(negShort, "-") || strings.HasPrefix(negShort, "--") {
			return fmt.Errorf("flag %q: negative short must start with a single -", f.OptName())
		}
		if negShort == f.Short() {
			return fmt.Errorf("flag %q: negative short must differ from short", f.OptName())
		}
		if f.Negative() == "" {
			return fmt.Errorf("flag %q: negative short requires a negative form", f.OptName())
		}
	}
	return nil
}

// expand resolves an ordered list of refs (definitions or groups) against the
// visible scope, deduplicating by name (first occurrence wins).
func expand(sc *scope, refs []string, context string) ([]Option, error) {
	var out []Option
	seen := map[string]bool{}
	var walk func(path []string, list []string) error
	walk = func(path []string, list []string) error {
		for _, n := range list {
			if members, ok := sc.groups[n]; ok {
				for _, g := range path {
					if g == n {
						return fmt.Errorf("%s: group %q: cycle detected", context, n)
					}
				}
				next := append(append([]string(nil), path...), n)
				if err := walk(next, members); err != nil {
					return err
				}
				continue
			}
			o, ok := sc.defs[n]
			if !ok {
				return fmt.Errorf("%s: unknown option or group %q", context, n)
			}
			if !seen[o.OptName()] {
				seen[o.OptName()] = true
				out = append(out, o)
			}
		}
		return nil
	}
	if err := walk(nil, refs); err != nil {
		return nil, err
	}
	return out, nil
}

// RNode is a resolved node: a fork owns only modes; a shape owns its resolved
// option selection and any subcommands.
type RNode struct {
	Name        string
	Aliases     []string
	Usage       string
	Description string

	IsFork bool
	Modes  []*RNode

	Flags       []Flag
	Positionals []Positional
	Options     []Option
	Subcommands []*RNode
}

func resolve(n *Node, parent *scope) (*RNode, error) {
	sc := parent.clone()
	if err := sc.addPool(n.Pool); err != nil {
		return nil, err
	}
	if n.hasModes && n.hasShape {
		return nil, fmt.Errorf("node %s: cannot define both modes and shape fields", label(n))
	}
	r := &RNode{Name: n.Name, Aliases: n.Aliases, Usage: n.Usage, Description: n.Description}

	if n.hasModes {
		r.IsFork = true
		seen := map[string]bool{}
		for _, m := range n.Modes {
			if m.Name == "" {
				return nil, fmt.Errorf("mode is missing a name")
			}
			if seen[m.Name] {
				return nil, fmt.Errorf("duplicate mode name %q", m.Name)
			}
			seen[m.Name] = true
			child, err := resolve(m, sc)
			if err != nil {
				return nil, err
			}
			r.Modes = append(r.Modes, child)
		}
		return r, nil
	}

	opts, err := expand(sc, n.Options, label(n))
	if err != nil {
		return nil, err
	}
	r.Options = opts
	for _, o := range opts {
		switch v := o.(type) {
		case Flag:
			r.Flags = append(r.Flags, v)
		case Positional:
			r.Positionals = append(r.Positionals, v)
		}
	}
	if err := validateShape(r); err != nil {
		return nil, err
	}

	seen := map[string]bool{}
	for _, sub := range n.Subcommands {
		if sub.Name == "" {
			return nil, fmt.Errorf("subcommand is missing a name")
		}
		if strings.HasPrefix(sub.Name, "-") {
			return nil, fmt.Errorf("subcommand name %q must not start with -", sub.Name)
		}
		if seen[sub.Name] {
			return nil, fmt.Errorf("duplicate subcommand name %q", sub.Name)
		}
		seen[sub.Name] = true
		for _, a := range sub.Aliases {
			if a == "" || strings.HasPrefix(a, "-") {
				return nil, fmt.Errorf("subcommand %q: invalid alias %q", sub.Name, a)
			}
			if seen[a] {
				return nil, fmt.Errorf("duplicate subcommand name or alias %q", a)
			}
			seen[a] = true
		}
		child, err := resolve(sub, sc)
		if err != nil {
			return nil, err
		}
		r.Subcommands = append(r.Subcommands, child)
	}
	return r, nil
}

func label(n *Node) string {
	if n.Name != "" {
		return fmt.Sprintf("%q", n.Name)
	}
	return "root"
}

// validateShape enforces positional ordering: at most one variadic positional,
// and it must be last; a passthrough is always variadic and must be last.
func validateShape(r *RNode) error {
	seenVariadic := false
	for _, p := range r.Positionals {
		if p.Variadic() {
			if seenVariadic {
				return fmt.Errorf("only one variadic positional is allowed")
			}
			seenVariadic = true
			continue
		}
		if seenVariadic {
			return fmt.Errorf("positional %q must not follow a variadic positional", p.OptName())
		}
	}
	return nil
}

// Segment is one shape in a resolved path, plus the subcommand chosen after it.
type Segment struct {
	Node        *RNode
	Flags       []Flag
	Positionals []Positional
	Options     []Option
	Sub         *RNode

	flagSet map[string]bool
	posSet  map[string]bool
}

func (s *Segment) index() {
	s.flagSet = map[string]bool{}
	for _, f := range s.Flags {
		s.flagSet[f.OptName()] = true
	}
	s.posSet = map[string]bool{}
	for _, p := range s.Positionals {
		s.posSet[p.OptName()] = true
	}
}

// BranchKind distinguishes the two kinds of choice along a path.
type BranchKind int

const (
	BranchMode BranchKind = iota
	BranchSubcommand
)

// BranchChoice records one choice made along a path: which alternative was
// picked and whether it was a mode or a subcommand.
type BranchChoice struct {
	Kind        BranchKind
	Label       string
	Description string
	Index       int
}

// Resolved is one complete selection through the tree: a chain of segments
// (root shape -> subcommand shape -> ...). Fork modes contribute no segment of
// their own; the chosen mode does.
type Resolved struct {
	Name          string
	Usage         string
	Description   string
	Segments      []Segment
	BranchChoices []BranchChoice

	Flags       []Flag
	Positionals []Positional
	Options     []Option

	flagSet map[string]bool
	posSet  map[string]bool
}

func (r Resolved) HasFlag(name string) bool       { return r.flagSet[name] }
func (r Resolved) HasPositional(name string) bool { return r.posSet[name] }

func newResolved(name string, segs []Segment, choices []BranchChoice) Resolved {
	r := Resolved{
		Name:          name,
		Segments:      segs,
		BranchChoices: choices,
		flagSet:       map[string]bool{},
		posSet:        map[string]bool{},
	}
	seenOpt := map[string]bool{}
	for _, seg := range segs {
		for n := range seg.flagSet {
			r.flagSet[n] = true
		}
		for n := range seg.posSet {
			r.posSet[n] = true
		}
		for _, o := range seg.Options {
			if seenOpt[o.OptName()] {
				continue
			}
			seenOpt[o.OptName()] = true
			r.Options = append(r.Options, o)
			switch v := o.(type) {
			case Flag:
				r.Flags = append(r.Flags, v)
			case Positional:
				r.Positionals = append(r.Positionals, v)
			}
		}
	}
	// Expose each subcommand branch as a selectable pseudo option, outermost
	// first. Modes are cycled with tab and are not listed here.
	subCount := 0
	for _, c := range choices {
		if c.Kind == BranchSubcommand {
			subCount++
		}
	}
	subLevel := 0
	var pseudo []Option
	for i, c := range choices {
		if c.Kind != BranchSubcommand {
			continue
		}
		subLevel++
		name := "Command"
		if subCount > 1 {
			name = fmt.Sprintf("Command %d", subLevel)
		}
		pseudo = append(pseudo, &SubcommandOption{
			BaseOption: BaseOption{Name: name, Description: "Choose the subcommand to run."},
			Level:      i,
			Label:      c.Label,
		})
	}
	r.Options = append(pseudo, r.Options...)
	for i := len(segs) - 1; i >= 0; i-- {
		if r.Usage == "" {
			r.Usage = segs[i].Node.Usage
		}
		if r.Description == "" {
			r.Description = segs[i].Node.Description
		}
	}
	return r
}

func joinName(prefix, name string) string {
	if name == "" {
		return prefix
	}
	if prefix == "" {
		return name
	}
	return prefix + " " + name
}

// buildPaths enumerates every complete selection (leaf) through the tree.
func (cfg *CommandConfig) buildPaths() error {
	var out []Resolved
	var walk func(n *RNode, name string, segs []Segment, choices []BranchChoice)
	walk = func(n *RNode, name string, segs []Segment, choices []BranchChoice) {
		if n.IsFork {
			prefix := joinName(name, n.Name)
			for i, m := range n.Modes {
				c := append(append([]BranchChoice(nil), choices...), BranchChoice{BranchMode, m.Name, m.Description, i})
				walk(m, prefix, segs, c)
			}
			return
		}
		seg := Segment{Node: n, Flags: n.Flags, Positionals: n.Positionals, Options: n.Options}
		seg.index()
		base := append([]Segment(nil), segs...)
		base = append(base, seg)
		childName := joinName(name, n.Name)
		if len(n.Subcommands) == 0 {
			out = append(out, newResolved(childName, base, choices))
			return
		}
		for i, sub := range n.Subcommands {
			path := append([]Segment(nil), base...)
			path[len(path)-1].Sub = sub
			c := append(append([]BranchChoice(nil), choices...), BranchChoice{BranchSubcommand, sub.Name, sub.Description, i})
			walk(sub, childName, path, c)
		}
	}
	walk(cfg.root, "", nil, nil)
	if len(out) == 0 {
		return fmt.Errorf("config has no selectable shape")
	}
	cfg.paths = out
	return nil
}

// HasBranchKind reports whether the path branches on the given kind.
func (r Resolved) HasBranchKind(kind BranchKind) bool {
	for _, c := range r.BranchChoices {
		if c.Kind == kind {
			return true
		}
	}
	return false
}

// SiblingIndices returns the path indices that share every branch with cur
// except at level, i.e. the alternatives offered at that branch.
func (cfg *CommandConfig) SiblingIndices(cur Resolved, level int) []int {
	var out []int
	for i, p := range cfg.paths {
		if sameBranchPrefix(p, cur, level) {
			out = append(out, i)
		}
	}
	return out
}

func sameBranchPrefix(a, b Resolved, level int) bool {
	if level >= len(a.BranchChoices) || level >= len(b.BranchChoices) {
		return false
	}
	if a.BranchChoices[level].Kind != b.BranchChoices[level].Kind {
		return false
	}
	for k := 0; k < level; k++ {
		if a.BranchChoices[k] != b.BranchChoices[k] {
			return false
		}
	}
	return true
}

// flatten collects every flag and positional reachable in the tree, first
// occurrence winning, for session bookkeeping and name lookups.
func (cfg *CommandConfig) flatten() {
	seenF := map[string]bool{}
	seenP := map[string]bool{}
	var walk func(n *RNode)
	walk = func(n *RNode) {
		if n.IsFork {
			for _, m := range n.Modes {
				walk(m)
			}
			return
		}
		for _, f := range n.Flags {
			if !seenF[f.OptName()] {
				seenF[f.OptName()] = true
				cfg.Flags = append(cfg.Flags, f)
			}
		}
		for _, p := range n.Positionals {
			if !seenP[p.OptName()] {
				seenP[p.OptName()] = true
				cfg.Positionals = append(cfg.Positionals, p)
			}
		}
		for _, sub := range n.Subcommands {
			walk(sub)
		}
	}
	walk(cfg.root)
}

// Resolved returns the config's selectable paths.
func (cfg *CommandConfig) Resolved() []Resolved { return cfg.paths }
