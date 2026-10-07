package main

import (
	"errors"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

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
