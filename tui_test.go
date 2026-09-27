package main

import (
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

const tuiFixture = `pool:
  options:
    - name: Ignore case
      kind: flag
      type: bool
      long: --ignore-case
      short: -i
      description: >
        Toggle case-insensitive matching. This description is intentionally long so
        that the description pane has to wrap it across several lines and, at small
        heights, scroll it.
    - name: Smart case
      kind: flag
      type: bool
      long: --smart-case
      short: -S
      description: Smart case matching.
    - name: PATTERN
      kind: positional
      type: string
      required: true
      description: A regular expression used for searching.
    - name: PATH
      kind: positional
      type: string
      required: false
      variadic: true
      description: A file or directory to search.
options: [Ignore case, Smart case, PATTERN, PATH]
`

func testModel(t *testing.T) model {
	t.Helper()
	cfg, err := parseConfigBytes(t, []byte(tuiFixture))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	sess, err := Prefill(cfg, nil)
	if err != nil {
		t.Fatalf("prefill: %v", err)
	}
	return newModel("rg", cfg, sess)
}

const pathFixture = `pool:
  options:
    - name: PATH
      kind: positional
      type: path
      required: false
      variadic: true
      description: A file or directory to search.
options: [PATH]
`

func testPathModel(t *testing.T) model {
	t.Helper()
	cfg, err := parseConfigBytes(t, []byte(pathFixture))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	sess, err := Prefill(cfg, nil)
	if err != nil {
		t.Fatalf("prefill: %v", err)
	}
	return newModel("rg", cfg, sess)
}

const valueFixture = `pool:
  options:
    - name: Replace
      kind: flag
      type: string
      long: --replace
      short: -r
    - name: After context
      kind: flag
      type: int
      long: --after-context
      short: -A
    - name: PATTERN
      kind: positional
      type: string
      required: true
options: [Replace, After context, PATTERN]
`

func testValueModel(t *testing.T) model {
	t.Helper()
	cfg, err := parseConfigBytes(t, []byte(valueFixture))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	sess, err := Prefill(cfg, nil)
	if err != nil {
		t.Fatalf("prefill: %v", err)
	}
	return newModel("rg", cfg, sess)
}

// TestValueFlagViewFits checks the layout while editing a string flag.
func TestValueFlagViewFits(t *testing.T) {
	for _, h := range []int{8, 12, 20, 30} {
		for _, w := range []int{40, 60, 100, 160} {
			m := testValueModel(t)
			upd, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
			m = upd.(model)
			m.setCursor(m.rowIndexOf("Replace"))
			opened, _ := m.handleKey(tea.KeyMsg{Type: tea.KeySpace})
			m = opened.(model)
			typed, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("value")})
			m = typed.(model)

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

// TestIntFlagInput checks that an int flag field rejects non-digits.
func TestIntFlagInput(t *testing.T) {
	m := testValueModel(t)
	m.setCursor(m.rowIndexOf("After context"))

	opened, _ := m.handleKey(tea.KeyMsg{Type: tea.KeySpace})
	m = opened.(model)
	if !m.editing {
		t.Fatal("space did not begin editing the int flag")
	}

	typed, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("a")})
	m = typed.(model)
	if m.input.Value() != "" {
		t.Fatalf("non-digit accepted: %q", m.input.Value())
	}
	ok, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("5")})
	m = ok.(model)
	if m.input.Value() != "5" {
		t.Fatalf("digit rejected: %q", m.input.Value())
	}

	m.commitEdit()
	m.editing = false
	if got := m.session.Flags["After context"].First(); got != "5" {
		t.Fatalf("After context = %q, want 5", got)
	}
	if !strings.Contains(m.preview(), "--after-context=5") {
		t.Fatalf("preview = %q", m.preview())
	}
}

// TestPreviewQuotes checks the preview shell-quotes args with spaces.
func TestPreviewQuotes(t *testing.T) {
	m := testModel(t)
	m.width, m.height = 80, 24
	m.session.PosVals["PATTERN"] = []string{"foo bar"}
	if !strings.Contains(m.preview(), "'foo bar'") {
		t.Fatalf("preview = %q, want quoted arg", m.preview())
	}
	if !strings.Contains(m.View(), "'foo bar'") {
		t.Fatalf("view does not show quoted arg")
	}
}

// TestCtrlX checks ctrl-x clears the selected entry.
func TestCtrlX(t *testing.T) {
	// Checked bool -> unchecked.
	m := testModel(t)
	m.session.Flags["Ignore case"] = FlagState{Checked: true}
	m.setCursor(m.rowIndexOf("Ignore case"))
	x, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlX})
	m = x.(model)
	if m.session.Flags["Ignore case"].Checked {
		t.Fatal("ctrl-x did not uncheck the bool")
	}

	// Value flag -> emptied.
	m2 := testValueModel(t)
	m2.session.Flags["Replace"] = FlagState{Values: []string{"x"}}
	m2.setCursor(m2.rowIndexOf("Replace"))
	x, _ = m2.handleKey(tea.KeyMsg{Type: tea.KeyCtrlX})
	m2 = x.(model)
	if m2.session.Flags["Replace"].First() != "" {
		t.Fatalf("Replace = %q, want empty", m2.session.Flags["Replace"].First())
	}

	// Positional -> emptied.
	m3 := testModel(t)
	m3.session.PosVals["PATTERN"] = []string{"foo"}
	m3.setCursor(m3.rowIndexOf("PATTERN"))
	x, _ = m3.handleKey(tea.KeyMsg{Type: tea.KeyCtrlX})
	m3 = x.(model)
	if hasValue(m3.session.PosVals["PATTERN"]) {
		t.Fatalf("PATTERN = %v, want empty", m3.session.PosVals["PATTERN"])
	}
}

// TestCSIReorder checks ctrl+shift+j/k delivered as CSI-u sequences reorder.
func TestCSIReorder(t *testing.T) {
	m := testModel(t)
	m.session.Flags["Ignore case"] = FlagState{Checked: true}
	m.session.Flags["Smart case"] = FlagState{Checked: true}
	m.setCursor(m.rowIndexOf("Ignore case"))

	updated, _ := m.Update([]byte("\x1b[106;6u")) // ctrl+shift+j
	m = updated.(model)
	if m.session.Order[0] != "Smart case" || m.session.Order[1] != "Ignore case" {
		t.Fatalf("after ctrl+shift+j: %v", m.session.Order)
	}

	updated, _ = m.Update([]byte("\x1b[107;6u")) // ctrl+shift+k
	m = updated.(model)
	if m.session.Order[0] != "Ignore case" {
		t.Fatalf("after ctrl+shift+k: %v", m.session.Order)
	}

	// ctrl+j without shift (CSI-u) still moves the cursor.
	m2 := testModel(t)
	m2.setCursor(0)
	updated, _ = m2.Update([]byte("\x1b[106;5u")) // ctrl+j
	m2 = updated.(model)
	if m2.cursor != 1 {
		t.Fatalf("ctrl+j csi cursor = %d, want 1", m2.cursor)
	}

	// Other ctrl+letters delivered as CSI-u still work (ctrl+p prints).
	m3 := testModel(t)
	m3.session.PosVals["PATTERN"] = []string{"foo"}
	updated, cmd := m3.Update([]byte("\x1b[112;5u")) // ctrl+p
	m3 = updated.(model)
	if m3.action != actionPrint || cmd == nil {
		t.Fatalf("ctrl+p csi: action=%v cmd=%v", m3.action, cmd)
	}

	// Ambiguous special keys as CSI-u: ESC clears the filter, enter opens menu.
	m4 := testModel(t)
	m4.filter = "abc"
	updated, _ = m4.Update([]byte("\x1b[27u")) // escape
	m4 = updated.(model)
	if m4.filter != "" {
		t.Fatalf("escape csi did not clear filter: %q", m4.filter)
	}
	m5 := testModel(t)
	updated, _ = m5.Update([]byte("\x1b[13u")) // enter
	m5 = updated.(model)
	if !m5.actionMenu {
		t.Fatal("enter csi did not open the menu")
	}
}

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

// TestSeparatorRow checks a divider is inserted between filled and unused
// options and that the cursor skips it.
func TestSeparatorRow(t *testing.T) {
	m := testValueModel(t)
	r := m.session.Flags["Replace"]
	r.Values = []string{"x"}
	m.session.Flags["Replace"] = r

	rows := m.rows()
	sep := -1
	for i, row := range rows {
		if row.kind == rowSep {
			if sep >= 0 {
				t.Fatal("more than one separator")
			}
			sep = i
		}
	}
	if sep <= 0 || sep >= len(rows)-1 {
		t.Fatalf("separator at %d in %d rows", sep, len(rows))
	}
	if rows[sep-1].opt == nil || rows[sep+1].opt == nil {
		t.Fatalf("separator not between options: %#v", rows)
	}

	m.setCursor(sep)
	if r, _ := m.selectedRow(); r.kind == rowSep {
		t.Fatal("cursor landed on the separator")
	}

	// Crossing the separator works in both directions.
	m.setCursor(sep + 1)
	m.moveCursor(-1)
	if r, _ := m.selectedRow(); r.opt == nil || r.opt.OptName() != "Replace" {
		t.Fatalf("moving up crossed to %#v, want Replace", r)
	}
	m.setCursor(sep - 1)
	m.moveCursor(1)
	if r, _ := m.selectedRow(); r.opt == nil || r.opt.OptName() != "After context" {
		t.Fatalf("moving down crossed to %#v, want After context", r)
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

// TestEditValueFlag checks that space edits a string flag and the preview
// updates to --long=value.
func TestEditValueFlag(t *testing.T) {
	m := testValueModel(t)
	m.setCursor(m.rowIndexOf("Replace"))

	opened, _ := m.handleKey(tea.KeyMsg{Type: tea.KeySpace})
	m = opened.(model)
	if !m.editing {
		t.Fatal("space did not begin editing the value flag")
	}
	m.input.SetValue("bar")
	m.commitEdit()
	m.editing = false

	if got := m.session.Flags["Replace"].First(); got != "bar" {
		t.Fatalf("Replace = %q, want bar", got)
	}
	if !strings.Contains(m.preview(), "--replace=bar") {
		t.Fatalf("preview = %q, want --replace=bar", m.preview())
	}
}

// TestPathPickerOpenSelectEsc checks that a path positional opens the picker,
// that a selection is appended, and that esc cancels.
func TestPathPickerOpenSelectEsc(t *testing.T) {
	m := testPathModel(t)

	opened, _ := m.handleKey(tea.KeyMsg{Type: tea.KeySpace})
	m = opened.(model)
	if !m.picking {
		t.Fatal("path positional did not open the picker")
	}

	cancelled, _ := m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = cancelled.(model)
	if m.picking {
		t.Fatal("esc did not close the picker")
	}

	// Reopen and commit a selection directly.
	opened, _ = m.handleKey(tea.KeyMsg{Type: tea.KeySpace})
	m = opened.(model)
	m.commitPath("/tmp/project")
	m.picking = false
	if got := m.session.PosVals["PATH"]; len(got) != 1 || got[0] != "/tmp/project" {
		t.Fatalf("PATH = %v, want [/tmp/project]", got)
	}
}

// TestPathPickerFilter checks that typing fuzzy-filters the picker and that
// enter commits the highlighted entry.
func TestPathPickerFilter(t *testing.T) {
	m := testPathModel(t)
	opened, _ := m.handleKey(tea.KeyMsg{Type: tea.KeySpace})
	m = opened.(model)

	typed, _ := m.handlePickerKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("config")})
	m = typed.(model)
	if m.picker.filter != "config" {
		t.Fatalf("filter = %q, want config", m.picker.filter)
	}
	path, ok := m.picker.selectedPath()
	if !ok {
		t.Fatal("expected a selected entry matching config")
	}
	committed, _ := m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = committed.(model)
	if m.picking {
		t.Fatal("enter did not close the picker")
	}
	if got := m.session.PosVals["PATH"]; len(got) != 1 || got[0] != path {
		t.Fatalf("PATH = %v, want [%s]", got, path)
	}
}

// TestPickerViewFitsTerminal guards the picker overlay against cropping.
func TestPickerViewFitsTerminal(t *testing.T) {
	for _, h := range []int{10, 16, 24} {
		for _, w := range []int{50, 80, 120} {
			m := testPathModel(t)
			upd, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
			m = upd.(model)
			m.picking = true

			lines := strings.Split(m.View(), "\n")
			if len(lines) != h {
				t.Errorf("%dx%d: rendered %d lines", w, h, len(lines))
			}
			for i, l := range lines {
				if n := len([]rune(l)); n > w-1 {
					t.Errorf("%dx%d: line %d is %d columns", w, h, i, n)
				}
			}
		}
	}
}

// TestViewFitsTerminal guards against the TUI rendering more rows or columns
// than the terminal has, which crops the layout. No line may reach the last
// terminal column either, since terminals auto-wrap there and scroll the UI.
func TestViewFitsTerminal(t *testing.T) {
	for _, h := range []int{8, 12, 20, 30, 50} {
		for _, w := range []int{40, 60, 100, 160} {
			for _, editing := range []bool{false, true} {
				m := testModel(t)
				upd, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
				m = upd.(model)
				if editing {
					m.setCursor(m.rowIndexOf("PATTERN"))
					opened, _ := m.handleKey(tea.KeyMsg{Type: tea.KeySpace})
					m = opened.(model)
					typed, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("value")})
					m = typed.(model)
				}

				lines := strings.Split(m.View(), "\n")
				if len(lines) != h {
					t.Errorf("%dx%d editing=%v: rendered %d lines", w, h, editing, len(lines))
				}
				for i, l := range lines {
					if n := len([]rune(l)); n > w-1 {
						t.Errorf("%dx%d editing=%v: line %d is %d columns: %q", w, h, editing, i, n, l)
					}
				}
			}
		}
	}
}

// TestArrowKeysMoveCursor checks that the plain arrow keys are aliases for
// ctrl-j / ctrl-k.
func TestArrowKeysMoveCursor(t *testing.T) {
	m := testModel(t)
	down, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	if m = down.(model); m.cursor != 1 {
		t.Fatalf("after down: cursor = %d, want 1", m.cursor)
	}
	up, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyUp})
	if m = up.(model); m.cursor != 0 {
		t.Fatalf("after up: cursor = %d, want 0", m.cursor)
	}
}

// TestEnterAdvancesToNextInput checks that committing a variadic value moves
// the cursor to the next element without activating it.
func TestEnterAdvancesToNextInput(t *testing.T) {
	m := testModel(t)
	m.filter = "PATH"
	m.setCursor(0)

	m.activate()
	if !m.editing {
		t.Fatal("activate did not begin editing")
	}
	typed, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("src")})
	m = typed.(model)
	committed, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = committed.(model)

	if got := m.session.PosVals["PATH"]; len(got) != 1 || got[0] != "src" {
		t.Fatalf("PATH = %v, want [src]", got)
	}
	if m.editing {
		t.Fatal("next element should not be activated")
	}
	r, ok := m.selectedRow()
	if !ok || r.kind != rowAdd {
		t.Fatalf("cursor row = %#v, want the add row", r)
	}
}

// TestEnterAdvancesFromScalar checks the same advance-after-commit for a
// non-variadic positional.
func TestEnterAdvancesFromScalar(t *testing.T) {
	m := testModel(t)
	m.setCursor(m.rowIndexOf("PATTERN"))

	m.activate()
	typed, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("foo")})
	m = typed.(model)
	committed, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = committed.(model)

	if got := m.session.PosVals["PATTERN"]; len(got) != 1 || got[0] != "foo" {
		t.Fatalf("PATTERN = %v, want [foo]", got)
	}
	if m.editing {
		t.Fatal("next element should not be activated")
	}
	r, ok := m.selectedRow()
	if !ok || r.kind == rowSep || (r.opt != nil && r.opt.OptName() == "PATTERN") {
		t.Fatalf("cursor did not advance to another element: %#v", r)
	}
}

// variadicRows returns the rows of the PATH positional (filtered to PATH).
func variadicRows(t *testing.T, m model) []row {
	t.Helper()
	m.filter = "PATH"
	var out []row
	for _, r := range m.rows() {
		if r.opt != nil && r.opt.OptName() == "PATH" {
			out = append(out, r)
		}
	}
	return out
}

func TestVariadicRowsAndEditing(t *testing.T) {
	m := testModel(t)
	m.session.PosVals["PATH"] = []string{"a", "b"}

	rows := variadicRows(t, m)
	if len(rows) != 3 {
		t.Fatalf("rows = %d, want 2 values + 1 add", len(rows))
	}
	if rows[0].kind != rowValue || rows[0].value != "a" || rows[0].valIdx != 0 {
		t.Errorf("row 0 = %#v", rows[0])
	}
	if rows[1].kind != rowValue || rows[1].value != "b" || rows[1].valIdx != 1 {
		t.Errorf("row 1 = %#v", rows[1])
	}
	if rows[2].kind != rowAdd {
		t.Errorf("row 2 kind = %v, want rowAdd", rows[2].kind)
	}

	// Edit the second value.
	m.beginEdit("PATH", 1, "b", valueLabel("PATH", 1))
	m.input.SetValue("B")
	m.commitEdit()
	if got := m.session.PosVals["PATH"]; len(got) != 2 || got[1] != "B" {
		t.Fatalf("after edit: %v, want [a B]", got)
	}

	// Append a new value (index -1) via the add row.
	m.beginEdit("PATH", -1, "", "+ PATH: ")
	m.input.SetValue("c")
	m.commitEdit()
	if got := m.session.PosVals["PATH"]; len(got) != 3 || got[2] != "c" {
		t.Fatalf("after add: %v, want [a B c]", got)
	}

	// Delete the first value by selecting it.
	m.filter = "PATH"
	m.cursor = 0
	m.deleteValue()
	if got := m.session.PosVals["PATH"]; len(got) != 2 || got[0] != "B" {
		t.Fatalf("after delete: %v, want [B c]", got)
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

// TestActionMenu covers the enter action popup (run/copy/print/cancel).
func TestActionMenu(t *testing.T) {
	quits := func(cmd tea.Cmd) bool {
		if cmd == nil {
			return false
		}
		_, ok := cmd().(tea.QuitMsg)
		return ok
	}

	m := testModel(t)
	m.session.PosVals["PATTERN"] = []string{"foo"}

	// Enter opens the menu with Run focused.
	opened, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = opened.(model)
	if !m.actionMenu || m.actionSel != 0 {
		t.Fatalf("after enter: menu=%v sel=%d, want true/0", m.actionMenu, m.actionSel)
	}
	if !strings.Contains(m.View(), m.preview()) {
		t.Errorf("action menu does not show the command preview")
	}

	// j/k navigate like up/down.
	j, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("j")})
	m = j.(model)
	if m.actionSel != 1 {
		t.Fatalf("after j: sel=%d, want 1", m.actionSel)
	}
	k, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("k")})
	m = k.(model)
	if m.actionSel != 0 {
		t.Fatalf("after k: sel=%d, want 0", m.actionSel)
	}

	// Enter ESC opens then closes.
	closed, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = closed.(model)
	if m.actionMenu {
		t.Fatal("esc did not close the action menu")
	}

	// Enter ENTER runs.
	opened, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = opened.(model)
	done, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = done.(model)
	if !quits(cmd) || m.action != actionRun {
		t.Fatalf("enter enter: quit=%v action=%v, want run", quits(cmd), m.action)
	}

	// Navigate to Cancel and activate.
	m = testModel(t)
	m.session.PosVals["PATTERN"] = []string{"foo"}
	opened, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = opened.(model)
	for i := 0; i < 4; i++ {
		down, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
		m = down.(model)
	}
	if m.actionSel != 4 {
		t.Fatalf("actionSel = %d, want 4", m.actionSel)
	}
	cancelled, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = cancelled.(model)
	if m.actionMenu || quits(cmd) {
		t.Fatal("cancel should close without quitting")
	}

	// ctrl-p bypasses the popup.
	printed, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlP})
	m = printed.(model)
	if !quits(cmd) || m.action != actionPrint {
		t.Fatalf("ctrl+p: quit=%v action=%v, want print", quits(cmd), m.action)
	}
}

// TestActionViewFits checks the action popup matches the terminal size.
func TestActionViewFits(t *testing.T) {
	for _, h := range []int{12, 20, 30} {
		for _, w := range []int{40, 80, 120} {
			m := testModel(t)
			upd, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
			m = upd.(model)
			m.actionMenu = true

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

// TestQuitConfirm covers the ESC confirmation popup state machine.
func TestQuitConfirm(t *testing.T) {
	m := testModel(t)
	quits := func(cmd tea.Cmd) bool {
		if cmd == nil {
			return false
		}
		_, ok := cmd().(tea.QuitMsg)
		return ok
	}

	// ESC opens the popup, defaulting to Exit.
	opened, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = opened.(model)
	if !m.confirm || m.confirmSel != 1 {
		t.Fatalf("after esc: confirm=%v sel=%d, want true/1", m.confirm, m.confirmSel)
	}

	// ESC ESC closes it.
	closed, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = closed.(model)
	if m.confirm {
		t.Fatal("esc did not close the popup")
	}

	// ESC LEFT ENTER cancels.
	opened, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = opened.(model)
	left, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyLeft})
	m = left.(model)
	if m.confirmSel != 0 {
		t.Fatalf("after left: sel=%d, want 0", m.confirmSel)
	}
	done, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = done.(model)
	if m.confirm || quits(cmd) {
		t.Fatalf("left+enter should cancel, not exit (confirm=%v)", m.confirm)
	}

	// ESC ENTER exits.
	opened, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = opened.(model)
	_, cmd = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	if !quits(cmd) {
		t.Fatal("esc+enter should quit")
	}

	// ctrl+c exits immediately without opening the popup.
	_, cmd = m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlC})
	if !quits(cmd) {
		t.Fatal("ctrl+c should quit")
	}
}

// TestConfirmViewFits checks the popup overlay matches the terminal size.
func TestConfirmViewFits(t *testing.T) {
	for _, h := range []int{10, 16, 24} {
		for _, w := range []int{40, 80, 120} {
			m := testModel(t)
			upd, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
			m = upd.(model)
			m.confirm = true
			m.confirmSel = 1

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

// TestFilterCtrlW checks ctrl-w deletes one word from the filter.
func TestFilterCtrlW(t *testing.T) {
	m := testModel(t)
	m.filter = "foo bar"
	deleted, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = deleted.(model)
	if m.filter != "foo " {
		t.Fatalf("filter = %q, want %q", m.filter, "foo ")
	}
	deleted, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = deleted.(model)
	if m.filter != "" {
		t.Fatalf("filter = %q, want empty", m.filter)
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

const modesFixture = `pool:
  options:
    - name: E
      kind: flag
      type: string
      long: --regexp
      short: -e
      repeatable: true
    - name: PATTERN
      kind: positional
      type: string
      required: true
    - name: PATH
      kind: positional
      type: path
      variadic: true
modes:
  - name: Pattern
    usage: rg [OPTIONS] PATTERN [PATH ...]
    options: [PATTERN, PATH]
  - name: Regexp
    usage: rg [OPTIONS] -e PATTERN ... [PATH ...]
    options: [E, PATH]
`

func testModesModel(t *testing.T) model {
	t.Helper()
	cfg, err := parseConfigBytes(t, []byte(modesFixture))
	if err != nil {
		t.Fatalf("parse fixture: %v", err)
	}
	sess, err := Prefill(cfg, nil)
	if err != nil {
		t.Fatalf("prefill: %v", err)
	}
	return newModel("rg", cfg, sess)
}

// TestModes covers mode selection, auto-switch, and validation.
func TestModes(t *testing.T) {
	m := testModesModel(t)
	if len(m.modes) != 2 || m.mode != 0 {
		t.Fatalf("modes=%d mode=%d, want 2/0", len(m.modes), m.mode)
	}
	if m.validate() {
		t.Fatal("empty Pattern mode should not validate")
	}

	// Round-trip: PATTERN becomes PATH and back.
	m.session.PosVals["PATTERN"] = []string{"func"}
	m.cycleMode(1)
	if m.mode != 1 {
		t.Fatalf("cycle to Regexp: mode=%d", m.mode)
	}
	if !reflect.DeepEqual(m.session.PosVals["PATH"], []string{"func"}) || hasValue(m.session.PosVals["PATTERN"]) {
		t.Fatalf("reparse: PATH=%v PATTERN=%v", m.session.PosVals["PATH"], m.session.PosVals["PATTERN"])
	}
	m.cycleMode(1)
	if m.mode != 0 || !reflect.DeepEqual(m.session.PosVals["PATTERN"], []string{"func"}) {
		t.Fatalf("round-trip back: mode=%d PATTERN=%v", m.mode, m.session.PosVals["PATTERN"])
	}

	// A mode that can't parse the command is skipped.
	m.mode = 1
	m.session.Flags["E"] = FlagState{Values: []string{"bar"}}
	m.cycleMode(1) // Pattern can't parse --regexp -> stay
	if m.mode != 1 {
		t.Fatalf("unparseable mode not skipped: mode=%d", m.mode)
	}
	if !m.validate() {
		t.Fatalf("Regexp mode should validate: %s", m.errMsg)
	}
}

// TestModesViewFits checks the mode picker/usage rows fit the terminal.
func TestModesViewFits(t *testing.T) {
	for _, h := range []int{10, 14, 20, 30} {
		for _, w := range []int{40, 60, 100} {
			m := testModesModel(t)
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

// TestCopy checks copy-and-exit, copy-and-stay, and copy failure.
func TestCopy(t *testing.T) {
	orig := copyFn
	defer func() { copyFn = orig }()

	var got string
	copyFn = func(s string) error { got = s; return nil }

	quits := func(cmd tea.Cmd) bool {
		if cmd == nil {
			return false
		}
		_, ok := cmd().(tea.QuitMsg)
		return ok
	}

	// copy and exit
	m := testModel(t)
	m.session.PosVals["PATTERN"] = []string{"foo"}
	cmd := m.copyCommand(false)
	if !quits(cmd) || m.action != actionCopy {
		t.Fatalf("copy exit: quit=%v action=%v", quits(cmd), m.action)
	}
	if !strings.Contains(got, "foo") {
		t.Fatalf("copied %q, want it to contain foo", got)
	}

	// copy and stay
	m2 := testModel(t)
	m2.session.PosVals["PATTERN"] = []string{"foo"}
	if cmd := m2.copyCommand(true); cmd != nil || m2.action != actionCopy || m2.statusMsg != "copied" {
		t.Fatalf("copy stay: cmd=%v action=%v status=%q", cmd, m2.action, m2.statusMsg)
	}

	// failure stays open with an error
	copyFn = func(string) error { return errors.New("nope") }
	m3 := testModel(t)
	m3.session.PosVals["PATTERN"] = []string{"foo"}
	if cmd := m3.copyCommand(false); cmd != nil || m3.errMsg == "" {
		t.Fatalf("copy failure: cmd=%v err=%q", cmd, m3.errMsg)
	}
}

// TestCopyKeys checks the ctrl-y / ctrl-shift-y bindings.
func TestCopyKeys(t *testing.T) {
	orig := copyFn
	defer func() { copyFn = orig }()
	copyFn = func(string) error { return nil }

	m := testModel(t)
	m.session.PosVals["PATTERN"] = []string{"foo"}
	exited, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlY})
	m = exited.(model)
	if cmd == nil || m.action != actionCopy {
		t.Fatalf("ctrl-y: cmd=%v action=%v", cmd, m.action)
	}

	m2 := testModel(t)
	m2.session.PosVals["PATTERN"] = []string{"foo"}
	updated, cmd := m2.Update([]byte("\x1b[121;6u")) // ctrl+shift+y
	m2 = updated.(model)
	if cmd != nil || m2.action != actionCopy || m2.statusMsg != "copied" {
		t.Fatalf("ctrl-shift-y: cmd=%v action=%v status=%q", cmd, m2.action, m2.statusMsg)
	}
}

// TestModeSkipNonEmpty checks a mode that can't parse the command is skipped.
func TestModeSkipNonEmpty(t *testing.T) {
	b, err := os.ReadFile("config/rg.yaml")
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := parseConfigBytes(t, b)
	if err != nil {
		t.Fatal(err)
	}
	s := NewSession(cfg)
	s.PosVals["PATTERN"] = []string{"foo"}
	m := newModel("rg", cfg, s)
	m.cycleMode(1) // Pattern -> Regexp (parses "foo" as PATH)
	if m.mode != 1 {
		t.Fatalf("mode=%d, want 1", m.mode)
	}
	m.cycleMode(1) // Regexp -> Info can't parse, must skip to Pattern
	if m.mode != 0 {
		t.Fatalf("Info not skipped: mode=%d, want 0", m.mode)
	}
}

// TestEnumMenu checks space opens a value menu for enum flags.
func TestEnumMenu(t *testing.T) {
	cfg, err := parseConfigBytes(t, []byte(`
pool:
  options:
    - name: Color
      kind: flag
      type: enum
      long: --color
      values: [never, auto, always]
options: [Color]
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	sess, _ := Prefill(cfg, nil)
	m := newModel("rg", cfg, sess)
	m.setCursor(m.rowIndexOf("Color"))

	opened, _ := m.handleKey(tea.KeyMsg{Type: tea.KeySpace})
	m = opened.(model)
	if !m.enumPicking || len(m.enumValues) != 3 {
		t.Fatalf("enum menu not open: %v values=%v", m.enumPicking, m.enumValues)
	}
	down, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	m = down.(model)
	chosen, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = chosen.(model)
	if m.enumPicking {
		t.Fatal("enter did not close the enum menu")
	}
	if got := m.session.Flags["Color"].First(); got != "auto" {
		t.Fatalf("chosen = %q, want auto", got)
	}
	if !strings.Contains(m.preview(), "--color=auto") {
		t.Fatalf("preview = %q", m.preview())
	}
}

// TestEnumMenuFuzzy checks typing in the enum menu filters the values.
func TestEnumMenuFuzzy(t *testing.T) {
	cfg, err := parseConfigBytes(t, []byte(`
pool:
  options:
    - name: Color
      kind: flag
      type: enum
      long: --color
      values: [never, auto, always]
options: [Color]
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	sess, _ := Prefill(cfg, nil)
	m := newModel("rg", cfg, sess)
	m.setCursor(m.rowIndexOf("Color"))

	opened, _ := m.handleKey(tea.KeyMsg{Type: tea.KeySpace})
	m = opened.(model)
	typed, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("al")})
	m = typed.(model)
	if m.enumFilter != "al" {
		t.Fatalf("filter = %q, want al", m.enumFilter)
	}
	if got := m.filteredEnum(); !reflect.DeepEqual(got, []string{"always"}) {
		t.Fatalf("filtered = %v, want [always]", got)
	}
	// ctrl-w deletes the last filter word.
	cleared, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = cleared.(model)
	if m.enumFilter != "" {
		t.Fatalf("filter after ctrl-w = %q, want empty", m.enumFilter)
	}
	if got := m.filteredEnum(); len(got) != 3 {
		t.Fatalf("filtered after ctrl-w = %v, want all", got)
	}
	typed, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("al")})
	m = typed.(model)
	chosen, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = chosen.(model)
	if got := m.session.Flags["Color"].First(); got != "always" {
		t.Fatalf("chosen = %q, want always", got)
	}
}

// TestCountFlagCycle checks space cycles a count flag 0..3.
func TestCountFlagCycle(t *testing.T) {
	cfg, err := parseConfigBytes(t, []byte(
		"pool:\n  options:\n    - name: U\n      kind: flag\n      type: count\n      long: --unrestricted\n      short: -u\noptions: [U]\n",
	))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	sess, _ := Prefill(cfg, nil)
	m := newModel("rg", cfg, sess)
	m.setCursor(m.rowIndexOf("U"))
	for i := 1; i <= 4; i++ {
		updated, _ := m.handleKey(tea.KeyMsg{Type: tea.KeySpace})
		m = updated.(model)
		want := i % 4
		if m.session.Flags["U"].Count != want {
			t.Fatalf("after %d toggles: count=%d, want %d", i, m.session.Flags["U"].Count, want)
		}
	}
}

// TestNullableBoolCycle checks the three-state cycle and glyphs.
func TestNullableBoolCycle(t *testing.T) {
	cfg, err := parseConfigBytes(t, []byte(`
pool:
  options:
    - name: Heading
      kind: flag
      type: bool
      long: --heading
      negative: --no-heading
options: [Heading]
`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	sess, _ := Prefill(cfg, nil)
	m := newModel("rg", cfg, sess)
	m.setCursor(m.rowIndexOf("Heading"))
	press := func() {
		updated, _ := m.handleKey(tea.KeyMsg{Type: tea.KeySpace})
		m = updated.(model)
	}
	// unset -> on
	press()
	if !m.session.Flags["Heading"].Checked {
		t.Fatal("not on after first space")
	}
	if got := Assemble(m.res(), m.session); !strings.Contains(strings.Join(got, " "), "--heading") {
		t.Fatalf("on emits %v", got)
	}
	// on -> off
	press()
	if m.session.Flags["Heading"].Checked || !m.session.Flags["Heading"].Neg {
		t.Fatalf("not off after second space: %+v", m.session.Flags["Heading"])
	}
	if got := Assemble(m.res(), m.session); !strings.Contains(strings.Join(got, " "), "--no-heading") {
		t.Fatalf("off emits %v", got)
	}
	// off -> unset
	press()
	if m.session.Flags["Heading"].Filled() {
		t.Fatal("not unset after third space")
	}
	if got := Assemble(m.res(), m.session); len(got) != 0 {
		t.Fatalf("unset emits %v", got)
	}
}
