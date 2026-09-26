package main

import "fmt"

// Mode is one invocation shape: a named subset of flags and positionals.
type Mode struct {
	Name    string   `yaml:"name"`
	Usage   string   `yaml:"usage"`
	Options []string `yaml:"options"`
}

// Resolved is a mode with its option references looked up.
type Resolved struct {
	Name        string
	Usage       string
	Flags       []Flag
	Positionals []Positional
	Options     []Option // ordered as written in the mode's options list
	flagSet     map[string]bool
	posSet      map[string]bool
}

func (r Resolved) HasFlag(name string) bool       { return r.flagSet[name] }
func (r Resolved) HasPositional(name string) bool { return r.posSet[name] }

func (r *Resolved) index() {
	r.flagSet = map[string]bool{}
	for _, f := range r.Flags {
		r.flagSet[f.OptName()] = true
	}
	r.posSet = map[string]bool{}
	for _, p := range r.Positionals {
		r.posSet[p.OptName()] = true
	}
}

// Resolved returns the mode views for the config. Configs without modes have a
// single implicit mode containing every flag and positional.
func (cfg *CommandConfig) Resolved() []Resolved { return cfg.resolved }

// buildModes validates and resolves groups/modes. It is called after decode.
func (cfg *CommandConfig) buildModes() error {
	flags := map[string]Flag{}
	for _, f := range cfg.Flags {
		flags[f.OptName()] = f
	}
	pos := map[string]Positional{}
	for _, p := range cfg.Positionals {
		pos[p.OptName()] = p
	}

	if len(cfg.Modes) == 0 {
		res := Resolved{Options: append([]Option(nil), cfg.Options...)}
		for _, o := range cfg.Options {
			switch v := o.(type) {
			case Flag:
				res.Flags = append(res.Flags, v)
			case Positional:
				res.Positionals = append(res.Positionals, v)
			}
		}
		res.index()
		cfg.resolved = []Resolved{res}
		return nil
	}

	expand := func(context string, names []string) ([]Option, error) {
		var out []Option
		seenOpt := map[string]bool{}
		seenGroup := map[string]bool{}
		var walk func(path []string, list []string) error
		walk = func(path []string, list []string) error {
			for _, n := range list {
				if members, ok := cfg.Groups[n]; ok {
					for _, g := range path {
						if g == n {
							return fmt.Errorf("%s: group %q: cycle detected", context, n)
						}
					}
					if seenGroup[n] {
						continue // already expanded via another group
					}
					seenGroup[n] = true
					next := append(append([]string(nil), path...), n)
					if err := walk(next, members); err != nil {
						return err
					}
					continue
				}
				if f, ok := flags[n]; ok {
					if !seenOpt[f.OptName()] {
						seenOpt[f.OptName()] = true
						out = append(out, f)
					}
					continue
				}
				if p, ok := pos[n]; ok {
					if !seenOpt[p.OptName()] {
						seenOpt[p.OptName()] = true
						out = append(out, p)
					}
					continue
				}
				return fmt.Errorf("%s: unknown option or group %q", context, n)
			}
			return nil
		}
		if err := walk(nil, names); err != nil {
			return nil, err
		}
		return out, nil
	}

	seen := map[string]bool{}
	for _, md := range cfg.Modes {
		if md.Name == "" {
			return fmt.Errorf("mode is missing a name")
		}
		if seen[md.Name] {
			return fmt.Errorf("duplicate mode name %q", md.Name)
		}
		seen[md.Name] = true
		opts, err := expand(fmt.Sprintf("mode %q", md.Name), md.Options)
		if err != nil {
			return err
		}
		res := Resolved{Name: md.Name, Usage: md.Usage, Options: opts}
		for _, o := range opts {
			switch v := o.(type) {
			case Flag:
				res.Flags = append(res.Flags, v)
			case Positional:
				res.Positionals = append(res.Positionals, v)
			}
		}
		res.index()
		cfg.resolved = append(cfg.resolved, res)
	}
	return nil
}
