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
	// Populated from the decoded tree.
	Flags       Flags
	Positionals Positionals

	root  *RNode
	paths []Resolved
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
		allowed = fieldSet("kind", "name", "type", "description", "long", "short", "negative", "negative_short", "repeatable", "conflicts", "requires")
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
	case KindEnum:
		p = &EnumPositional{}
		allowed = fieldSet("kind", "name", "type", "description", "required", "variadic", "values")
	case KindPassthrough:
		p = &PassthroughPositional{}
		allowed = fieldSet("kind", "name", "type", "description", "required")
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

// xdgDir resolves a per-app directory from an XDG env var. When the variable
// is unset it falls back to <home>/fallback. The result is <base>/flagpick.
func xdgDir(env, fallback string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	base := os.Getenv(env)
	if base == "" {
		base = filepath.Join(home, filepath.FromSlash(fallback))
	}
	return filepath.Join(base, "flagpick"), nil
}

// configDir holds user override configs.
func configDir() (string, error) { return xdgDir("XDG_CONFIG_HOME", ".config") }

// dataDir holds downloaded copies of upstream configs.
func dataDir() (string, error) { return xdgDir("XDG_DATA_HOME", ".local/share") }

// stateDir holds run state such as the last composition for --resume.
func stateDir() (string, error) { return xdgDir("XDG_STATE_HOME", ".local/state") }

// findConfig returns <name>.yaml, scanning the config directory (user
// overrides) first and then the data directory (downloads). The bool reports
// whether a file was found.
func findConfig(configDir, dataDir, name string) (string, bool) {
	for _, dir := range []string{configDir, dataDir} {
		p := filepath.Join(dir, name+".yaml")
		if _, err := os.Stat(p); err == nil {
			return p, true
		}
	}
	return "", false
}

// ConfigNotFoundError signals that no config file exists for a command.
type ConfigNotFoundError struct{ Name string }

func (e *ConfigNotFoundError) Error() string {
	return fmt.Sprintf("no config for '%s'; run 'fp --update'", e.Name)
}

// LoadConfig resolves <config>/<name>.yaml (user override) then
// <data>/<name>.yaml (downloaded).
func LoadConfig(name string) (*CommandConfig, error) {
	config, err := configDir()
	if err != nil {
		return nil, err
	}
	data, err := dataDir()
	if err != nil {
		return nil, err
	}
	path, ok := findConfig(config, data, name)
	if !ok {
		return nil, &ConfigNotFoundError{Name: name}
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return parseConfig(b)
}

// parseConfig decodes a config tree and resolves it into selectable paths.
func parseConfig(b []byte) (*CommandConfig, error) {
	dec := yaml.NewDecoder(bytes.NewReader(b))
	var root Node
	if err := dec.Decode(&root); err != nil {
		return nil, err
	}
	return buildConfig(&root)
}

// buildConfig validates and resolves a decoded root node.
func buildConfig(root *Node) (*CommandConfig, error) {
	rn, err := resolve(root, newScope())
	if err != nil {
		return nil, err
	}
	cfg := &CommandConfig{root: rn}
	if err := cfg.buildPaths(); err != nil {
		return nil, err
	}
	cfg.flatten()
	return cfg, nil
}

// ListCommands returns the sorted basenames of *.yaml configs, scanning the
// config directory (user overrides) first and then the data directory.
func ListCommands() ([]string, error) {
	config, err := configDir()
	if err != nil {
		return nil, err
	}
	data, err := dataDir()
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	for _, dir := range []string{config, data} {
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
