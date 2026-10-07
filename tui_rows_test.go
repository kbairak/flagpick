package main

import (
	"reflect"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestReorder checks ctrl-shift-j/k (via the ctrl-up/down fallback) reorders
// the selected flag.
func TestReorder(t *testing.T) {
	m := testModel(t)
	m.session.Flags["Ignore case"] = FlagState{Checked: true}
	m.session.Flags["Smart case"] = FlagState{Checked: true}
	m.setCursor(m.rowIndexOf("Ignore case"))

	if len(m.session.Order) < 2 || m.session.Order[0] != "Ignore case" {
		t.Fatalf("initial order = %v", m.session.Order)
	}
	down, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlDown})
	m = down.(model)
	if m.session.Order[0] != "Smart case" || m.session.Order[1] != "Ignore case" {
		t.Fatalf("after ctrl+down: %v", m.session.Order)
	}
	// The options list must reflect the reorder too.
	var firstFlag string
	for _, o := range m.listOptions() {
		if _, ok := o.(*BoolFlag); ok {
			firstFlag = o.OptName()
			break
		}
	}
	if firstFlag != "Smart case" {
		t.Fatalf("first flag in list = %q, want Smart case", firstFlag)
	}
	m.setCursor(m.rowIndexOf("Ignore case"))
	up, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlUp})
	m = up.(model)
	if m.session.Order[0] != "Ignore case" {
		t.Fatalf("after ctrl+up: %v", m.session.Order)
	}

	// Reorder is ignored for unapplied flags.
	m2 := testModel(t)
	before := append([]string(nil), m2.session.Order...)
	m2.setCursor(m2.rowIndexOf("Ignore case"))
	ignored, _ := m2.handleKey(tea.KeyMsg{Type: tea.KeyCtrlDown})
	m2 = ignored.(model)
	if !reflect.DeepEqual(m2.session.Order, before) {
		t.Fatalf("unapplied reorder changed order: %v", m2.session.Order)
	}
}

// TestFilledOptionsSortToTop checks that any filled option (positional or
// flag, bool or value) is listed before empty ones.
func TestFilledOptionsSortToTop(t *testing.T) {
	m := testValueModel(t)
	m.session.PosVals["PATTERN"] = []string{"foo"}
	r := m.session.Flags["Replace"]
	r.Values = []string{"x"}
	m.session.Flags["Replace"] = r
	a := m.session.Flags["After context"]
	a.Values = []string{"2"}
	m.session.Flags["After context"] = a

	var names []string
	for _, o := range m.listOptions() {
		names = append(names, o.OptName())
	}
	want := []string{"Replace", "After context", "PATTERN"}
	for i, n := range want {
		if i >= len(names) || names[i] != n {
			t.Fatalf("listOptions = %v, want prefix %v", names, want)
		}
	}

	// Clearing the value moves it back below the filled ones.
	r.Values = []string{""}
	m.session.Flags["Replace"] = r
	names = names[:0]
	for _, o := range m.listOptions() {
		names = append(names, o.OptName())
	}
	if names[0] != "After context" || names[1] != "PATTERN" {
		t.Fatalf("after clearing: %v, want After context, PATTERN first", names)
	}
}

// TestCheckedFlagsSortToTop checks that checked booleans are grouped above
// unchecked ones and that the cursor follows a flag as it moves.
func TestCheckedFlagsSortToTop(t *testing.T) {
	m := testModel(t)
	m.session.Flags["Smart case"] = FlagState{Checked: true}

	m.setCursor(m.rowIndexOf("Ignore case"))
	toggled, _ := m.handleKey(tea.KeyMsg{Type: tea.KeySpace})
	m = toggled.(model)

	if !m.session.Flags["Ignore case"].Checked {
		t.Fatal("Ignore case was not checked")
	}
	if m.rows()[m.cursor].opt.OptName() != "Ignore case" {
		t.Fatalf("cursor = %d, want Ignore case", m.cursor)
	}
	for _, o := range m.listOptions() {
		if _, ok := o.(*BoolFlag); ok {
			if o.OptName() != "Ignore case" {
				t.Fatalf("first flag = %q, want Ignore case", o.OptName())
			}
			break
		}
	}
}

// TestFuzzyMatchCaseInsensitive checks that filtering ignores case.
func TestFuzzyMatchCaseInsensitive(t *testing.T) {
	cases := []struct {
		label, filter string
		want          bool
	}{
		{"Ignore case", "IC", true},
		{"Ignore case", "ic", true},
		{"Ignore case", "IGNORE", true},
		{"Smart case", "sc", true},
		{"Case sensitive", "CS", true},
		{"Ignore case", "xc", false},
		{"", "", true},
	}
	for _, tc := range cases {
		if got := fuzzyMatch(tc.label, tc.filter); got != tc.want {
			t.Errorf("fuzzyMatch(%q, %q) = %v, want %v", tc.label, tc.filter, got, tc.want)
		}
	}
}

// TestConfigOrder checks the options list follows the config order.
func TestConfigOrder(t *testing.T) {
	m := testValueModel(t)
	var names []string
	for _, o := range m.listOptions() {
		names = append(names, o.OptName())
	}
	want := []string{"Replace", "After context", "PATTERN"}
	if !reflect.DeepEqual(names, want) {
		t.Fatalf("listOptions = %v, want %v", names, want)
	}
}

// TestClearAll checks ctrl-shift-x resets every option.
func TestClearAll(t *testing.T) {
	m := testValueModel(t)
	m.session.PosVals["PATTERN"] = []string{"foo"}
	m.session.Flags["Replace"] = FlagState{Values: []string{"bar"}}
	updated, _ := m.Update([]byte("\x1b[120;6u")) // ctrl+shift+x
	m = updated.(model)
	if hasValue(m.session.PosVals["PATTERN"]) || m.session.Flags["Replace"].Filled() {
		t.Fatalf("clearAll left state: PATTERN=%v Replace=%v", m.session.PosVals["PATTERN"], m.session.Flags["Replace"])
	}
}

// TestCursorCycle checks selection wraps around and half-page moves.
func TestCursorCycle(t *testing.T) {
	m := testValueModel(t) // Replace, After context, PATTERN
	n := len(m.rows())
	m.setCursor(0)
	m.moveCursor(-1)
	if m.cursor != n-1 {
		t.Fatalf("up wrap: cursor=%d, want %d", m.cursor, n-1)
	}
	m.moveCursor(1)
	if m.cursor != 0 {
		t.Fatalf("down wrap: cursor=%d, want 0", m.cursor)
	}
	m.moveCursor(halfPage(m.pageSize()))
	if m.cursor == 0 {
		t.Fatal("half-page down did not move")
	}
}

// TestPositionalFirstConfig checks list order follows the config's options
// order (positionals can lead).
func TestPositionalFirstConfig(t *testing.T) {
	cfg, err := parseConfigBytes(t, []byte(`
pool:
  options:
    - name: P
      kind: positional
      type: string
      required: true
    - name: F
      kind: flag
      type: bool
      long: --f
options: [P, F]
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	sess, _ := Prefill(cfg, nil)
	m := newModel("x", cfg, sess)
	var names []string
	for _, o := range m.listOptions() {
		names = append(names, o.OptName())
	}
	if !reflect.DeepEqual(names, []string{"P", "F"}) {
		t.Fatalf("order = %v, want [P F]", names)
	}
}
