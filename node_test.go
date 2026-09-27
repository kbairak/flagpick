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

func TestDispatcherTabDoesNotCycleSubcommands(t *testing.T) {
	cfg, err := parseConfigBytes(t, []byte(subcommandFixture))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	sess, err := Prefill(cfg, nil)
	if err != nil {
		t.Fatalf("empty prefill: %v", err)
	}
	m := newModel("mycmd", cfg, sess)
	if got := Assemble(m.res(), m.session); !reflect.DeepEqual(got, []string{"exec"}) {
		t.Fatalf("default path assemble = %v, want [exec]", got)
	}
	before := m.mode
	m.cycleMode(1)
	if m.mode != before {
		t.Fatalf("tab changed subcommand path to %q; use the subcommand menu", m.res().Name)
	}
}

func TestSubcommandMenuOption(t *testing.T) {
	cfg, err := parseConfigBytes(t, []byte(subcommandFixture))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	sess, err := Prefill(cfg, nil)
	if err != nil {
		t.Fatalf("prefill: %v", err)
	}
	m := newModel("mycmd", cfg, sess)

	// The active path exposes exactly one subcommand selector row.
	var sub *SubcommandOption
	for _, o := range m.listOptions() {
		if so, ok := o.(*SubcommandOption); ok {
			sub = so
		}
	}
	if sub == nil {
		t.Fatal("no subcommand selector in the option list")
	}
	if sub.Label != "exec" {
		t.Fatalf("selector label = %q, want exec", sub.Label)
	}

	// Open the menu and choose the next sibling (status Brief).
	m.openSub(sub.Level)
	if !m.subPicking || len(m.subPaths) != 3 {
		t.Fatalf("menu: picking=%v options=%d", m.subPicking, len(m.subPaths))
	}
	m.subSel = 1
	updated, _ := m.handleSubKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.res().Name != "status Brief" {
		t.Fatalf("after choose: %q, want status Brief", m.res().Name)
	}
	if got := Assemble(m.res(), m.session); !reflect.DeepEqual(got, []string{"status"}) {
		t.Fatalf("assemble = %v, want [status]", got)
	}
	// Once on the status path, the selector reflects that verb.
	for _, o := range m.listOptions() {
		if so, ok := o.(*SubcommandOption); ok && so.Label != "status" {
			t.Fatalf("selector label = %q, want status", so.Label)
		}
	}

	// A positional not present on the new path is dropped on switch.
	m.openSub(sub.Level)
	m.subSel = 0 // back to exec
	updated, _ = m.handleSubKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	m.session.PosVals["DIR"] = []string{"./x"}
	m.openSub(sub.Level)
	m.subSel = 1 // status Brief, which has no DIR
	updated, _ = m.handleSubKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if hasValue(m.session.PosVals["DIR"]) {
		t.Fatalf("DIR survived switching to status: %v", m.session.PosVals["DIR"])
	}
}

func TestSubcommandViewFits(t *testing.T) {
	cfg, err := parseConfigBytes(t, []byte(subcommandFixture))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	sess, err := Prefill(cfg, nil)
	if err != nil {
		t.Fatalf("prefill: %v", err)
	}
	for _, h := range []int{10, 14, 20, 30} {
		for _, w := range []int{40, 60, 100} {
			m := newModel("mycmd", cfg, sess)
			upd, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
			m = upd.(model)
			lines := strings.Split(m.View(), "\n")
			if len(lines) != h {
				t.Errorf("%dx%d: %d lines", w, h, len(lines))
			}
			for i, l := range lines {
				if n := len([]rune(l)); n > w-1 {
					t.Errorf("%dx%d: line %d is %d cols", w, h, i, n)
				}
			}
		}
	}
}

func TestSubcommandMenuFuzzy(t *testing.T) {
	cfg, err := parseConfigBytes(t, []byte(subcommandFixture))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	sess, err := Prefill(cfg, nil)
	if err != nil {
		t.Fatalf("prefill: %v", err)
	}
	m := newModel("mycmd", cfg, sess)

	var sub *SubcommandOption
	for _, o := range m.listOptions() {
		if so, ok := o.(*SubcommandOption); ok {
			sub = so
		}
	}
	m.openSub(sub.Level)
	typed, _ := m.handleSubKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ex")})
	m = typed.(model)
	if got := m.filteredSub(); len(got) != 1 || m.modes[got[0]].Name != "exec" {
		t.Fatalf("filtered = %v", got)
	}
	// ctrl-w deletes the last filter word.
	cleared, _ := m.handleSubKey(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = cleared.(model)
	if m.subFilter != "" {
		t.Fatalf("filter after ctrl-w = %q, want empty", m.subFilter)
	}
	if got := m.filteredSub(); len(got) != 3 {
		t.Fatalf("filtered after ctrl-w = %v, want all", got)
	}
	typed, _ = m.handleSubKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("ex")})
	m = typed.(model)
	updated, _ := m.handleSubKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = updated.(model)
	if m.res().Name != "exec" {
		t.Fatalf("selected %q, want exec", m.res().Name)
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
	m.setCursor(m.rowIndexOf("SHELL"))
	opened, _ := m.handleKey(tea.KeyMsg{Type: tea.KeySpace})
	m = opened.(model)
	if !m.enumPicking || len(m.enumValues) != 2 {
		t.Fatalf("enum menu: picking=%v values=%v", m.enumPicking, m.enumValues)
	}
	m.enumSel = 1
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
