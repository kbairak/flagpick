package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type CommandConfig struct {
	Options OptionsList         `yaml:"options"`
	Groups  map[string][]string `yaml:"groups"`
	Modes   []Mode              `yaml:"modes"`

	// Populated from Options after decode.
	Flags       Flags       `yaml:"-"`
	Positionals Positionals `yaml:"-"`

	resolved []Resolved
}

// OptionsList is the ordered catalog of flags and positionals. Order is the
// order they appear in the config and drives the UI.
type OptionsList []Option

func (os *OptionsList) UnmarshalYAML(node *yaml.Node) error {
	var out OptionsList
	for _, item := range node.Content {
		o, err := decodeOption(item)
		if err != nil {
			return err
		}
		out = append(out, o)
	}
	*os = out
	return nil
}

func decodeOption(node *yaml.Node) (Option, error) {
	var head struct {
		Kind string `yaml:"kind"`
	}
	if err := node.Decode(&head); err != nil {
		return nil, err
	}
	switch head.Kind {
	case "flag":
		return decodeFlag(node)
	case "positional":
		return decodePositional(node)
	case "":
		return nil, fmt.Errorf("option is missing 'kind' (flag or positional)")
	default:
		return nil, fmt.Errorf("unsupported option kind %q", head.Kind)
	}
}

// splitOptions partitions the ordered options into flags and positionals for
// the rest of the machinery.
func (cfg *CommandConfig) splitOptions() {
	cfg.Flags = nil
	cfg.Positionals = nil
	for _, o := range cfg.Options {
		switch v := o.(type) {
		case Flag:
			cfg.Flags = append(cfg.Flags, v)
		case Positional:
			cfg.Positionals = append(cfg.Positionals, v)
		}
	}
}

type Flags []Flag

func decodeFlag(node *yaml.Node) (Flag, error) {
	var head struct {
		Type OptionKind `yaml:"type"`
	}
	if err := node.Decode(&head); err != nil {
		return nil, err
	}
	var f Flag
	var allowed map[string]bool
	switch head.Type {
	case KindBool:
		f = &BoolFlag{}
		allowed = fieldSet("kind", "name", "type", "description", "long", "short", "negative", "repeatable", "conflicts", "requires")
	case KindString:
		f = &StringFlag{}
		allowed = fieldSet("kind", "name", "type", "description", "long", "short", "repeatable", "conflicts", "requires")
	case KindInt:
		f = &IntFlag{}
		allowed = fieldSet("kind", "name", "type", "description", "long", "short", "repeatable", "conflicts", "requires")
	case KindEnum:
		f = &EnumFlag{}
		allowed = fieldSet("kind", "name", "type", "description", "long", "short", "repeatable", "conflicts", "requires", "values")
	case KindSize:
		f = &SizeFlag{}
		allowed = fieldSet("kind", "name", "type", "description", "long", "short", "repeatable", "conflicts", "requires")
	case KindCount:
		f = &CountFlag{}
		allowed = fieldSet("kind", "name", "type", "description", "long", "short", "repeatable", "conflicts", "requires")
	default:
		return nil, fmt.Errorf("unsupported flag type %q", head.Type)
	}
	if err := decodeStrict(node, f, allowed); err != nil {
		return nil, err
	}
	return f, nil
}

type Positionals []Positional

func decodePositional(node *yaml.Node) (Positional, error) {
	var head struct {
		Type OptionKind `yaml:"type"`
	}
	if err := node.Decode(&head); err != nil {
		return nil, err
	}
	var p Positional
	var allowed map[string]bool
	switch head.Type {
	case KindString:
		p = &StringPositional{}
		allowed = fieldSet("kind", "name", "type", "description", "required", "variadic")
	case KindPath:
		p = &PathPositional{}
		allowed = fieldSet("kind", "name", "type", "description", "required", "variadic")
	default:
		return nil, fmt.Errorf("unsupported positional type %q", head.Type)
	}
	if err := decodeStrict(node, p, allowed); err != nil {
		return nil, err
	}
	return p, nil
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

// dirs returns the override directory (config) and the cache directory,
// both suffixed with "flagpick". Honors $XDG_CONFIG_HOME / $XDG_CACHE_HOME,
// falling back to ~/.config / ~/.cache (NOTES.md).
func dirs() (override, cache string, err error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", "", err
	}
	cfgBase := os.Getenv("XDG_CONFIG_HOME")
	if cfgBase == "" {
		cfgBase = filepath.Join(home, ".config")
	}
	cacheBase := os.Getenv("XDG_CACHE_HOME")
	if cacheBase == "" {
		cacheBase = filepath.Join(home, ".cache")
	}
	return filepath.Join(cfgBase, "flagpick"), filepath.Join(cacheBase, "flagpick"), nil
}

// ConfigNotFoundError signals that no config file exists for a command.
type ConfigNotFoundError struct{ Name string }

func (e *ConfigNotFoundError) Error() string {
	return fmt.Sprintf("no config for '%s'; add '%s.yaml' under $XDG_CACHE_HOME/flagpick/", e.Name, e.Name)
}

// LoadConfig resolves <override>/<name>.yaml then <cache>/<name>.yaml.
func LoadConfig(name string) (*CommandConfig, error) {
	override, cache, err := dirs()
	if err != nil {
		return nil, err
	}
	var path string
	for _, dir := range []string{override, cache} {
		p := filepath.Join(dir, name+".yaml")
		if _, statErr := os.Stat(p); statErr == nil {
			path = p
			break
		}
	}
	if path == "" {
		return nil, &ConfigNotFoundError{Name: name}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(b))
	dec.KnownFields(true)
	var cfg CommandConfig
	if err := dec.Decode(&cfg); err != nil {
		return nil, err
	}
	cfg.splitOptions()
	if err := validateConfig(&cfg); err != nil {
		return nil, err
	}
	if err := cfg.buildModes(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// validateConfig checks the split flag/positional catalog.
func validateConfig(cfg *CommandConfig) error {
	seen := map[string]bool{}
	seenNeg := map[string]bool{}
	for _, f := range cfg.Flags {
		if f.OptName() == "" {
			return fmt.Errorf("flag is missing a name")
		}
		if f.Long() == "" {
			return fmt.Errorf("flag %q is missing a long form", f.OptName())
		}
		if seen[f.OptName()] {
			return fmt.Errorf("duplicate flag name %q", f.OptName())
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
			if seenNeg[neg] {
				return fmt.Errorf("duplicate negative form %q", neg)
			}
			seenNeg[neg] = true
		}
		seen[f.OptName()] = true
	}
	seenVariadic := false
	for _, p := range cfg.Positionals {
		if p.OptName() == "" {
			return fmt.Errorf("positional is missing a name")
		}
		if seen[p.OptName()] {
			return fmt.Errorf("duplicate positional name %q", p.OptName())
		}
		seen[p.OptName()] = true
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

// ListCommands returns the sorted basenames of *.yaml configs, with
// override-dir entries winning over cache-dir entries.
func ListCommands() ([]string, error) {
	override, cache, err := dirs()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, dir := range []string{cache, override} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
				continue
			}
			seen[strings.TrimSuffix(e.Name(), ".yaml")] = true
		}
	}
	out := make([]string, 0, len(seen))
	for name := range seen {
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}
