package main

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestValueFlagViewFits checks the layout while inline-editing a string flag.
func TestValueFlagViewFits(t *testing.T) {
	for _, h := range []int{8, 12, 20, 30} {
		for _, w := range []int{40, 60, 100, 160} {
			m := testValueModel(t)
			upd, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
			m = upd.(model)
			m.setCursor(m.rowIndexOf("Replace"))
			m.focusEmptySlot()
			opened, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
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

// TestPreviewQuotes checks the assembled preview shell-quotes args with spaces,
// while the template shows the raw value.
func TestPreviewQuotes(t *testing.T) {
	m := testModel(t)
	m.width, m.height = 80, 24
	m.session.PosVals["PATTERN"] = []string{"foo bar"}
	if !strings.Contains(m.preview(), "'foo bar'") {
		t.Fatalf("preview = %q, want quoted arg", m.preview())
	}
	if !strings.Contains(m.View(), "'foo bar'") {
		t.Fatalf("view does not show the quoted arg")
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
// than the terminal has, for both invalid (missing required) and valid states.
func TestViewFitsTerminal(t *testing.T) {
	for _, h := range []int{8, 12, 20, 30, 50} {
		for _, w := range []int{40, 60, 100, 160} {
			for _, filled := range []bool{false, true} {
				m := testModel(t)
				upd, _ := m.Update(tea.WindowSizeMsg{Width: w, Height: h})
				m = upd.(model)
				if filled {
					m.session.PosVals["PATTERN"] = []string{"foo"}
				}

				lines := strings.Split(m.View(), "\n")
				if len(lines) != h {
					t.Errorf("%dx%d filled=%v: rendered %d lines", w, h, filled, len(lines))
				}
				for i, l := range lines {
					if n := len([]rune(l)); n > w-1 {
						t.Errorf("%dx%d filled=%v: line %d is %d columns: %q", w, h, filled, i, n, l)
					}
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
