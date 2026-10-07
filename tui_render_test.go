package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

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

// TestPickerViewFitsTerminal guards the picker overlay against cropping.
func TestPickerViewFitsTerminal(t *testing.T) {
	for _, h := range []int{10, 16, 24} {
		for _, w := range []int{50, 80, 120} {
			m := testPathModel(t)
			upd, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
			m = upd.(model)
			m.screen = screenPicker

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

// TestActionViewFits checks the action popup matches the terminal size.
func TestActionViewFits(t *testing.T) {
	for _, h := range []int{12, 20, 30} {
		for _, w := range []int{40, 80, 120} {
			m := testModel(t)
			upd, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
			m = upd.(model)
			m.screen = screenAction

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

// TestConfirmViewFits checks the popup overlay matches the terminal size.
func TestConfirmViewFits(t *testing.T) {
	for _, h := range []int{10, 16, 24} {
		for _, w := range []int{40, 80, 120} {
			m := testModel(t)
			upd, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
			m = upd.(model)
			m.screen = screenConfirm
			m.confirm.sel = 1

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
