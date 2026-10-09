package main

import (
	"reflect"
	"testing"
)

// TestReorder checks ctrl-shift-h/l (via CSI-u) reorders the focused flag.
func TestReorder(t *testing.T) {
	m := testModel(t)
	m.session.Flags["Ignore case"] = FlagState{Checked: true}
	m.session.Flags["Smart case"] = FlagState{Checked: true}
	m = focusPreviewToken(m, "Ignore case")

	if len(m.session.Order) < 2 || m.session.Order[0] != "Ignore case" {
		t.Fatalf("initial order = %v", m.session.Order)
	}
	down, _ := m.Update([]byte("\x1b[108;6u")) // ctrl+shift+l moves right
	m = down.(model)
	if m.session.Order[0] != "Smart case" || m.session.Order[1] != "Ignore case" {
		t.Fatalf("after ctrl+shift+l: %v", m.session.Order)
	}
	m = focusPreviewToken(m, "Ignore case")
	up, _ := m.Update([]byte("\x1b[104;6u")) // ctrl+shift+h moves left
	m = up.(model)
	if m.session.Order[0] != "Ignore case" {
		t.Fatalf("after ctrl+shift+h: %v", m.session.Order)
	}

	// Reorder is ignored for unapplied flags.
	m2 := testModel(t)
	before := append([]string(nil), m2.session.Order...)
	m2.focusEmptySlot()
	m2.reorder(1)
	if !reflect.DeepEqual(m2.session.Order, before) {
		t.Fatalf("unapplied reorder changed order: %v", m2.session.Order)
	}
}

// TestNoHoisting checks that the option list stays in config order regardless
// of which options are filled (no filled-first section).
func TestNoHoisting(t *testing.T) {
	m := testValueModel(t)
	r := m.session.Flags["After context"]
	r.Values = []string{"2"}
	m.session.Flags["After context"] = r

	var names []string
	for _, o := range m.listOptions() {
		names = append(names, o.OptName())
	}
	if want := []string{"Replace", "After context"}; !reflect.DeepEqual(names, want) {
		t.Fatalf("listOptions = %v, want %v", names, want)
	}
}

// TestCheckedFlagStaysInPlace checks toggling a bool does not reorder the list
// and the cursor is not left on any row.
func TestCheckedFlagStaysInPlace(t *testing.T) {
	m := testModel(t)
	m.session.Flags["Smart case"] = FlagState{Checked: true}

	m = addOption(m, "Ignore case")
	if !m.session.Flags["Ignore case"].Checked {
		t.Fatal("Ignore case was not checked")
	}
	var flags []string
	for _, o := range m.listOptions() {
		if _, ok := o.(*BoolFlag); ok {
			flags = append(flags, o.OptName())
		}
	}
	if len(flags) < 2 || flags[0] != "Ignore case" || flags[1] != "Smart case" {
		t.Fatalf("list order = %v, want config order [Ignore case Smart case]", flags)
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
	want := []string{"Replace", "After context"}
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

// TestPositionalExcludedFromList checks positionals are not listed (they are
// edited from the preview); only flags appear, in config order.
func TestPositionalExcludedFromList(t *testing.T) {
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
	if !reflect.DeepEqual(names, []string{"F"}) {
		t.Fatalf("order = %v, want [F]", names)
	}
}
