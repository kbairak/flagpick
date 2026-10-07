package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

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
	if m5.screen != screenAction {
		t.Fatal("enter csi did not open the menu")
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
