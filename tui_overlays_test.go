package main

import (
	"reflect"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestPathPickerOpenSelectEsc checks that a path positional opens the picker,
// that a selection is appended, and that esc cancels.
func TestPathPickerOpenSelectEsc(t *testing.T) {
	m := testPathModel(t)
	m.setCursor(m.rowIndexOf("PATH"))
	m.focusEmptySlot()

	opened, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = opened.(model)
	if m.screen != screenPicker {
		t.Fatal("path positional did not open the picker")
	}

	cancelled, _ := m.handlePickerKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = cancelled.(model)
	if m.screen == screenPicker {
		t.Fatal("esc did not close the picker")
	}

	// Reopen and commit a selection directly.
	opened, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = opened.(model)
	m.commitPath("/tmp/project")
	m.screen = screenMain
	m.focusEmptySlot()
	if got := m.session.PosVals["PATH"]; len(got) != 1 || got[0] != "/tmp/project" {
		t.Fatalf("PATH = %v, want [/tmp/project]", got)
	}
}

// TestPathPickerFilter checks that typing fuzzy-filters the picker and that
// enter commits the highlighted entry.
func TestPathPickerFilter(t *testing.T) {
	m := testPathModel(t)
	m.setCursor(m.rowIndexOf("PATH"))
	m.focusEmptySlot()
	opened, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
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
	if m.screen == screenPicker {
		t.Fatal("enter did not close the picker")
	}
	if got := m.session.PosVals["PATH"]; len(got) != 1 || got[0] != path {
		t.Fatalf("PATH = %v, want [%s]", got, path)
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
	if m.screen != screenConfirm || m.confirm.sel != 1 {
		t.Fatalf("after esc: confirm=%v sel=%d, want true/1", m.screen == screenConfirm, m.confirm.sel)
	}

	// ESC ESC closes it.
	closed, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = closed.(model)
	if m.screen == screenConfirm {
		t.Fatal("esc did not close the popup")
	}

	// ESC LEFT ENTER cancels.
	opened, _ = m.handleKey(tea.KeyMsg{Type: tea.KeyEsc})
	m = opened.(model)
	left, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyLeft})
	m = left.(model)
	if m.confirm.sel != 0 {
		t.Fatalf("after left: sel=%d, want 0", m.confirm.sel)
	}
	done, cmd := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = done.(model)
	if m.screen == screenConfirm || quits(cmd) {
		t.Fatalf("left+enter should cancel, not exit (confirm=%v)", m.screen == screenConfirm)
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

// TestEnumMenu checks enter opens a value menu for enum flags.
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
	m = addOption(m, "Color")

	if m.screen != screenEnum || len(m.enum.values) != 3 {
		t.Fatalf("enum menu not open: %v values=%v", m.screen == screenEnum, m.enum.values)
	}
	down, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyDown})
	m = down.(model)
	chosen, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
	m = chosen.(model)
	if m.screen == screenEnum {
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
	m = addOption(m, "Color")

	typed, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("al")})
	m = typed.(model)
	if m.enum.filter != "al" {
		t.Fatalf("filter = %q, want al", m.enum.filter)
	}
	if got := m.filteredEnum(); !reflect.DeepEqual(got, []string{"always"}) {
		t.Fatalf("filtered = %v, want [always]", got)
	}
	// ctrl-w deletes the last filter word.
	cleared, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyCtrlW})
	m = cleared.(model)
	if m.enum.filter != "" {
		t.Fatalf("filter after ctrl-w = %q, want empty", m.enum.filter)
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
