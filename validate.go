// This file validates a decoded config tree: node shapes, options, and groups.
package main

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"
)

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

// decodeStrict rejects keys outside `allowed`, then decodes normally.
func decodeStrict(node *yaml.Node, dst any, allowed map[string]bool) error {
	var raw map[string]yaml.Node
	if err := node.Decode(&raw); err != nil {
		return err
	}
	for k := range raw {
		if !allowed[k] {
			return fmt.Errorf("unknown field %q", k)
		}
	}
	return node.Decode(dst)
}

func fieldSet(names ...string) map[string]bool {
	m := make(map[string]bool, len(names))
	for _, n := range names {
		m[n] = true
	}
	return m
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
