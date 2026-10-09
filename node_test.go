package main

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

const subcommandFixture = `pool:
  options:
    - {name: VERBOSE, kind: flag, type: bool, long: --verbose, short: -v}
    - {name: DIR, kind: positional, type: string, required: true}
    - {name: ARGS, kind: positional, type: passthrough, required: true}
usage: mycmd COMMAND
options: [VERBOSE]
subcommands:
  - name: exec
    aliases: [run, x]
    usage: mycmd exec DIR COMMAND [...ARGS]
    options: [DIR, ARGS]
  - name: status
    usage: mycmd status
    modes:
      - {name: Brief, usage: mycmd status, options: [VERBOSE]}
      - name: Full
        usage: mycmd status --json
        options: [VERBOSE]
`

func TestSubcommandPaths(t *testing.T) {
	cfg, err := parseConfigBytes(t, []byte(subcommandFixture))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var names []string
	for _, r := range cfg.Resolved() {
		names = append(names, r.Name)
	}
	want := []string{"exec", "status Brief", "status Full"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("paths = %v, want %v", names, want)
	}
}

func TestSubcommandAssemblyGlobalsBeforeVerb(t *testing.T) {
	cfg, err := parseConfigBytes(t, []byte(subcommandFixture))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	s, err := Prefill(cfg, []string{"--verbose", "exec", "./proj", "rg", "--json", "-i", "foo"})
	if err != nil {
		t.Fatalf("prefill: %v", err)
	}
	got := Assemble(cfg.Resolved()[0], s)
	want := []string{"--verbose", "exec", "./proj", "rg", "--json", "-i", "foo"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("assemble = %v, want %v", got, want)
	}
}

func TestSubcommandAliasEmitsCanonical(t *testing.T) {
	cfg, err := parseConfigBytes(t, []byte(subcommandFixture))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	s, err := Prefill(cfg, []string{"run", "./proj"})
	if err != nil {
		t.Fatalf("prefill alias: %v", err)
	}
	got := Assemble(cfg.Resolved()[0], s)
	if len(got) == 0 || got[0] != "exec" {
		t.Fatalf("assemble = %v, want leading exec", got)
	}
	if !reflect.DeepEqual(s.PosVals["DIR"], []string{"./proj"}) {
		t.Fatalf("DIR = %v", s.PosVals["DIR"])
	}
}

func TestSubcommandUnknownVerb(t *testing.T) {
	cfg, err := parseConfigBytes(t, []byte(subcommandFixture))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, err := Prefill(cfg, []string{"bogus", "./proj"}); err == nil {
		t.Fatal("expected error for unknown verb")
	}
}

func TestUnknownSubcommandTokenIsError(t *testing.T) {
	cfg, err := parseConfigBytes(t, []byte(`
pool:
  options:
    - {name: DIR, kind: positional, type: string}
subcommands:
  - name: exec
    options: [DIR]
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// A non-flag token in a subcommand position that is not a verb is an error.
	if _, err := Prefill(cfg, []string{"nope"}); err == nil {
		t.Fatal("expected error for unmatched verb")
	}
}

func TestPassthroughSwallowsFlags(t *testing.T) {
	cfg, err := parseConfigBytes(t, []byte(`
pool:
  options:
    - {name: DIR, kind: positional, type: path, required: true}
    - {name: ARGS, kind: positional, type: passthrough, required: true}
options: [DIR, ARGS]
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	s, err := Prefill(cfg, []string{"./proj", "rg", "--json", "-i", "foo"})
	if err != nil {
		t.Fatalf("prefill: %v", err)
	}
	if !reflect.DeepEqual(s.PosVals["DIR"], []string{"./proj"}) {
		t.Fatalf("DIR = %v", s.PosVals["DIR"])
	}
	want := []string{"rg", "--json", "-i", "foo"}
	if !reflect.DeepEqual(s.PosVals["ARGS"], want) {
		t.Fatalf("ARGS = %v, want %v", s.PosVals["ARGS"], want)
	}
	if got := Assemble(cfg.Resolved()[0], s); !reflect.DeepEqual(got, append([]string{"./proj"}, want...)) {
		t.Fatalf("assemble = %v", got)
	}
}

func TestNestedSubcommandRecursion(t *testing.T) {
	cfg, err := parseConfigBytes(t, []byte(`
pool:
  options:
    - {name: NAME, kind: positional, type: string, required: true}
    - {name: URL,  kind: positional, type: string, required: true}
usage: mycmd remote COMMAND
subcommands:
  - name: remote
    usage: mycmd remote COMMAND
    subcommands:
      - name: add
        usage: mycmd remote add <name> <url>
        options: [NAME, URL]
      - name: remove
        usage: mycmd remote remove <name>
        options: [NAME]
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	var names []string
	for _, r := range cfg.Resolved() {
		names = append(names, r.Name)
	}
	if want := []string{"remote add", "remote remove"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("paths = %v, want %v", names, want)
	}
	s, err := Prefill(cfg, []string{"remote", "add", "origin", "git@x"})
	if err != nil {
		t.Fatalf("prefill: %v", err)
	}
	got := Assemble(cfg.Resolved()[0], s)
	want := []string{"remote", "add", "origin", "git@x"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("assemble = %v, want %v", got, want)
	}
}

func TestEnumPositional(t *testing.T) {
	cfg, err := parseConfigBytes(t, []byte(`
pool:
  options:
    - {name: SHELL, kind: positional, type: enum, required: true, values: [bash, zsh]}
options: [SHELL]
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	s, err := Prefill(cfg, []string{"zsh"})
	if err != nil {
		t.Fatalf("prefill: %v", err)
	}
	if got := Assemble(cfg.Resolved()[0], s); !reflect.DeepEqual(got, []string{"zsh"}) {
		t.Fatalf("assemble = %v, want [zsh]", got)
	}
	if _, err := Prefill(cfg, []string{"tcsh"}); err == nil {
		t.Fatal("expected error for a value outside the enum")
	}
	// The TUI opens the value menu for an enum positional.
	sess, _ := Prefill(cfg, nil)
	m := newModel("mycmd", cfg, sess)
	m.focusEmptySlot()
	m.moveFocus(1) // onto the SHELL placeholder
	opened, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = opened.(model)
	if m.screen != screenEnum || len(m.enum.values) != 2 {
		t.Fatalf("enum menu: picking=%v values=%v", m.screen == screenEnum, m.enum.values)
	}
	m.enum.sel = 1
	chosen, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = chosen.(model)
	if got := m.session.PosVals["SHELL"]; len(got) != 1 || got[0] != "zsh" {
		t.Fatalf("SHELL = %v, want [zsh]", got)
	}
}

func TestConfigValidationRules(t *testing.T) {
	cases := []struct {
		name    string
		yaml    string
		wantSub string
	}{
		{
			name:    "modes and shape together",
			yaml:    "modes:\n  - {name: A, usage: x}\noptions: []\n",
			wantSub: "both modes and shape",
		},
		{
			name:    "mode missing name",
			yaml:    "modes:\n  - {usage: x}\n",
			wantSub: "mode is missing a name",
		},
		{
			name:    "duplicate mode names",
			yaml:    "modes:\n  - {name: A, usage: x}\n  - {name: A, usage: y}\n",
			wantSub: "duplicate mode name",
		},
		{
			name:    "subcommand missing name",
			yaml:    "subcommands:\n  - {usage: x}\n",
			wantSub: "subcommand is missing a name",
		},
		{
			name:    "subcommand alias duplicates sibling",
			yaml:    "subcommands:\n  - {name: a, aliases: [b]}\n  - {name: b}\n",
			wantSub: "duplicate subcommand",
		},
		{
			name:    "verb leading dash",
			yaml:    "subcommands:\n  - {name: --bad}\n",
			wantSub: "must not start with -",
		},
		{
			name:    "passthrough variadic field",
			yaml:    "pool:\n  options:\n    - {name: A, kind: positional, type: passthrough, variadic: true}\noptions: [A]\n",
			wantSub: "unknown field",
		},
		{
			name:    "enum positional without values",
			yaml:    "pool:\n  options:\n    - {name: A, kind: positional, type: enum}\noptions: [A]\n",
			wantSub: "enum requires at least one value",
		},
		{
			name:    "unknown selection ref",
			yaml:    "options: [NOPE]\n",
			wantSub: "unknown option or group",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := parseConfigBytes(t, []byte(tc.yaml))
			if err == nil {
				t.Fatal("expected error")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}
