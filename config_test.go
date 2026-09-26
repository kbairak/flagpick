package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

var (
	_ Flag       = (*BoolFlag)(nil)
	_ Positional = (*StringPositional)(nil)
)

func parseConfigBytes(t *testing.T, b []byte) (*CommandConfig, error) {
	t.Helper()
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

func TestParseRealConfig(t *testing.T) {
	b, err := os.ReadFile("config/rg.yaml")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}
	cfg, err := parseConfigBytes(t, b)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(cfg.Flags) != 102 {
		t.Fatalf("flags = %d, want 102", len(cfg.Flags))
	}
	modes := cfg.Resolved()
	if len(modes) != 4 {
		t.Fatalf("modes = %d, want 4", len(modes))
	}
	if modes[0].Name != "Pattern" || !modes[0].HasPositional("PATTERN") || !modes[0].HasFlag("Ignore case") {
		t.Fatalf("Pattern mode = %+v", modes[0])
	}
	if modes[1].Name != "Regexp" || modes[1].HasPositional("PATTERN") || !modes[1].HasFlag("Regular expression") {
		t.Fatalf("Regexp mode = %+v", modes[1])
	}
	if modes[2].Name != "Info" || !modes[2].HasFlag("Help") || modes[2].HasPositional("PATTERN") {
		t.Fatalf("Info mode = %+v", modes[2])
	}
	if modes[0].HasFlag("Help") {
		t.Fatal("Info flags should be exclusive to the Info mode")
	}
	if modes[3].Name != "Generate" || !modes[3].HasFlag("Generate") || modes[3].HasPositional("PATTERN") {
		t.Fatalf("Generate mode = %+v", modes[3])
	}
	if modes[0].HasFlag("Generate") {
		t.Fatal("Generate flag should be exclusive to the Generate mode")
	}
	// Nullable bool: Heading has a negative form.
	var heading Flag
	for _, fl := range cfg.Flags {
		if fl.OptName() == "Heading" {
			heading = fl
		}
	}
	if heading == nil || heading.Negative() != "--no-heading" {
		t.Fatalf("Heading negative = %q", heading.Negative())
	}
	if modes[0].HasFlag("Regular expression") {
		t.Fatal("Regexp flag should be exclusive to the Regexp mode")
	}
	if len(cfg.Positionals) != 2 {
		t.Fatalf("positionals = %d, want 2", len(cfg.Positionals))
	}
	var ignoreCase Flag
	for _, fl := range cfg.Flags {
		if fl.OptName() == "Ignore case" {
			ignoreCase = fl
		}
	}
	if _, ok := ignoreCase.(*BoolFlag); !ok {
		t.Fatalf("Ignore case type = %T, want *BoolFlag", ignoreCase)
	}
	if ignoreCase.Short() != "-i" {
		t.Errorf("short = %q, want %q", ignoreCase.Short(), "-i")
	}
	if got := ignoreCase.Conflicts(); len(got) != 2 || got[0] != "Case sensitive" || got[1] != "Smart case" {
		t.Errorf("conflicts = %v, want [Case sensitive Smart case]", got)
	}
	p, ok := cfg.Positionals[0].(*StringPositional)
	if !ok {
		t.Fatalf("Positionals[0] type = %T, want *StringPositional", cfg.Positionals[0])
	}
	if !p.Required() {
		t.Errorf("PATTERN.Required() = false, want true")
	}
	path, ok := cfg.Positionals[1].(*PathPositional)
	if !ok {
		t.Fatalf("Positionals[1] type = %T, want *PathPositional", cfg.Positionals[1])
	}
	if path.OptName() != "PATH" {
		t.Errorf("name = %q, want PATH", path.OptName())
	}
	if path.Required() {
		t.Errorf("PATH.Required() = true, want false")
	}
	if !path.Variadic() {
		t.Errorf("PATH.Variadic() = false, want true")
	}
	var replace Flag
	for _, f := range cfg.Flags {
		if f.OptName() == "Replace" {
			replace = f
		}
	}
	if replace == nil {
		t.Fatal("Replace flag not found")
	}
	if _, ok := replace.(*StringFlag); !ok {
		t.Fatalf("Replace type = %T, want *StringFlag", replace)
	}
	if !replace.TakesValue() {
		t.Errorf("Replace.TakesValue() = false, want true")
	}
	var after Flag
	for _, f := range cfg.Flags {
		if f.OptName() == "After context" {
			after = f
		}
	}
	if _, ok := after.(*IntFlag); !ok {
		t.Fatalf("After context type = %T, want *IntFlag", after)
	}
}

func TestInvalidConfigs(t *testing.T) {
	cases := []struct {
		name    string
		yaml    string
		wantSub string
	}{
		{
			name: "malformed yaml",
			yaml: "options:\n  - name: x\n    kind: flag\n    type: bool\n",
		},
		{
			name:    "unknown top-level field",
			yaml:    "options: []\nbogus: 1\n",
			wantSub: "bogus",
		},
		{
			name:    "unknown option field",
			yaml:    "options:\n  - name: x\n    kind: flag\n    type: bool\n    long: --x\n    nope: 1\n",
			wantSub: "unknown field",
		},
		{
			name:    "unsupported flag type",
			yaml:    "options:\n  - name: x\n    kind: flag\n    type: number\n    long: --x\n",
			wantSub: "unsupported flag type",
		},
		{
			name:    "unsupported positional type",
			yaml:    "options:\n  - name: X\n    kind: positional\n    type: integer\n",
			wantSub: "unsupported positional type",
		},
		{
			name:    "missing flag name",
			yaml:    "options:\n  - kind: flag\n    type: bool\n    long: --x\n",
			wantSub: "missing a name",
		},
		{
			name:    "missing long form",
			yaml:    "options:\n  - name: x\n    kind: flag\n    type: bool\n",
			wantSub: "missing a long form",
		},
		{
			name:    "duplicate flag names",
			yaml:    "options:\n  - name: x\n    kind: flag\n    type: bool\n    long: --x\n  - name: x\n    kind: flag\n    type: bool\n    long: --y\n",
			wantSub: "duplicate flag name",
		},
		{
			name:    "positional after variadic",
			yaml:    "options:\n  - name: X\n    kind: positional\n    type: string\n    variadic: true\n  - name: Y\n    kind: positional\n    type: string\n",
			wantSub: "must not follow a variadic",
		},
		{
			name:    "two variadics",
			yaml:    "options:\n  - name: X\n    kind: positional\n    type: string\n    variadic: true\n  - name: Y\n    kind: positional\n    type: string\n    variadic: true\n",
			wantSub: "only one variadic",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseConfigBytes(t, []byte(tc.yaml))
			if err == nil {
				t.Fatalf("expected error, got nil")
			}
			if tc.wantSub != "" && !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestConfigNotFound(t *testing.T) {
	_, err := LoadConfig("definitely-not-a-real-command-xyz")
	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := err.(*ConfigNotFoundError); !ok {
		t.Fatalf("error type = %T, want *ConfigNotFoundError", err)
	}
}
