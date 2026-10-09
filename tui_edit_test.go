package main

import (
	"errors"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

// TestEditValueFlag checks that enter on the empty slot with a value flag
// selected edits it and the preview updates to --long=value.
func TestEditValueFlag(t *testing.T) {
	m := testValueModel(t)
	m = addOption(m, "Replace")
	if !m.editing {
		t.Fatal("enter did not begin editing the value flag")
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

func TestVariadicValueEditing(t *testing.T) {
	m := testModel(t)
	m.session.PosVals["PATH"] = []string{"a", "b"}

	// Edit the second value.
	m = focusPreviewToken(m, "PATH")
	// focusPreviewToken returns the first PATH token (valIdx 0); move to 1.
	for i, tk := range m.previewTokens() {
		if tk.name == "PATH" && tk.valIdx == 1 {
			m.focus = i
		}
	}
	m.activateToken(m.previewTokens()[m.focus])
	m.input.SetValue("B")
	m.commitEdit()
	if got := m.session.PosVals["PATH"]; len(got) != 2 || got[1] != "B" {
		t.Fatalf("after edit: %v, want [a B]", got)
	}

	// Append a new value from the empty-option slot.
	m.activateOption("PATH")
	m.input.SetValue("c")
	m.commitEdit()
	if got := m.session.PosVals["PATH"]; len(got) != 3 || got[2] != "c" {
		t.Fatalf("after add: %v, want [a B c]", got)
	}

	// Delete the first value by focusing its preview token.
	for i, tk := range m.previewTokens() {
		if tk.name == "PATH" && tk.valIdx == 0 {
			m.focus = i
		}
	}
	m.removeFocusedToken()
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

// TestCountFlagCycle checks enter on the empty slot activates a count flag and
// repeated activation cycles it 0..3.
func TestCountFlagCycle(t *testing.T) {
	cfg, err := parseConfigBytes(t, []byte(
		"pool:\n  options:\n    - name: U\n      kind: flag\n      type: count\n      long: --unrestricted\n      short: -u\noptions: [U]\n",
	))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	sess, _ := Prefill(cfg, nil)
	m := newModel("rg", cfg, sess)
	m = addOption(m, "U")
	if m.session.Flags["U"].Count != 1 {
		t.Fatalf("after add: count=%d, want 1", m.session.Flags["U"].Count)
	}
	for i := 2; i <= 4; i++ {
		updated, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
		m = updated.(model)
		want := i % 4
		if m.session.Flags["U"].Count != want {
			t.Fatalf("after %d activations: count=%d, want %d", i, m.session.Flags["U"].Count, want)
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
	m = addOption(m, "Heading")
	press := func() {
		updated, _ := m.handleKey(tea.KeyMsg{Type: tea.KeyEnter})
		m = updated.(model)
	}
	// unset -> on (done by addOption)
	if !m.session.Flags["Heading"].Checked {
		t.Fatal("not on after add")
	}
	if got := Assemble(m.res(), m.session); !strings.Contains(strings.Join(got, " "), "--heading") {
		t.Fatalf("on emits %v", got)
	}
	// on -> off
	press()
	if m.session.Flags["Heading"].Checked || !m.session.Flags["Heading"].Neg {
		t.Fatalf("not off after second enter: %+v", m.session.Flags["Heading"])
	}
	if got := Assemble(m.res(), m.session); !strings.Contains(strings.Join(got, " "), "--no-heading") {
		t.Fatalf("off emits %v", got)
	}
	// off -> unset
	press()
	if m.session.Flags["Heading"].Filled() {
		t.Fatal("not unset after third enter")
	}
	if got := Assemble(m.res(), m.session); len(got) != 0 {
		t.Fatalf("unset emits %v", got)
	}
}
