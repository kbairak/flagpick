package main

import (
	"os"
	"strings"
	"testing"
)

func loadConfigFile(t *testing.T, name string) *CommandConfig {
	t.Helper()
	b, err := os.ReadFile("config/" + name + ".yaml")
	if err != nil {
		t.Fatalf("read %s: %v", name, err)
	}
	cfg, err := parseConfigBytes(t, b)
	if err != nil {
		t.Fatalf("parse %s: %v", name, err)
	}
	return cfg
}

// TestSubcommandFlow checks the direnv SUBCOMMAND placeholder, validity, and
// selection.
func TestSubcommandFlow(t *testing.T) {
	cfg := loadConfigFile(t, "direnv")
	sess, err := Prefill(cfg, nil)
	if err != nil {
		t.Fatalf("prefill: %v", err)
	}
	m := newModel("direnv", cfg, sess)

	if !m.pendingSub() {
		t.Fatal("expected a pending subcommand")
	}
	if ok, reason := m.validity(); ok || reason != "choose a subcommand" {
		t.Fatalf("validity = %v %q, want invalid 'choose a subcommand'", ok, reason)
	}
	if tk, ok := m.focusedToken(); !ok || tk.kind != pvSub {
		t.Fatalf("focus = %#v, want the SUBCOMMAND placeholder", tk)
	}

	// Choose "edit".
	editIdx := -1
	for i, s := range m.currentSubcommands() {
		if s.Name == "edit" {
			editIdx = i
		}
	}
	if editIdx < 0 {
		t.Fatal("edit subcommand not found")
	}
	m.chooseSub(editIdx)

	if m.pendingSub() {
		t.Fatal("subcommand should be chosen")
	}
	if m.res().Name != "edit" {
		t.Fatalf("path = %q, want edit", m.res().Name)
	}
	if got := m.preview(); !strings.Contains(got, "direnv edit") {
		t.Fatalf("preview = %q, want it to contain 'direnv edit'", got)
	}

	// edit requires PATH_TO_RC.
	if ok, reason := m.validity(); ok || !strings.Contains(reason, "PATH_TO_RC") {
		t.Fatalf("edit without PATH_TO_RC: valid=%v reason=%q", ok, reason)
	}
	m.session.PosVals["PATH_TO_RC"] = []string{".envrc"}
	if ok, reason := m.validity(); !ok {
		t.Fatalf("edit with PATH_TO_RC should validate: %q", reason)
	}
}

// TestNestedSubcommandFlow checks git remote add.
func TestNestedSubcommandFlow(t *testing.T) {
	cfg := loadConfigFile(t, "git")
	sess, err := Prefill(cfg, nil)
	if err != nil {
		t.Fatalf("prefill: %v", err)
	}
	m := newModel("git", cfg, sess)

	remoteIdx := subIndex(m.currentSubcommands(), "remote")
	if remoteIdx < 0 {
		t.Fatal("remote subcommand not found")
	}
	m.chooseSub(remoteIdx)
	if !m.pendingSub() {
		t.Fatal("remote should still need a subcommand")
	}

	addIdx := subIndex(m.currentSubcommands(), "add")
	if addIdx < 0 {
		t.Fatal("add subcommand not found")
	}
	m.chooseSub(addIdx)
	if m.pendingSub() {
		t.Fatal("add should be a complete path")
	}
	if got := m.preview(); !strings.Contains(got, "git remote add") {
		t.Fatalf("preview = %q, want it to contain 'git remote add'", got)
	}
}

// TestPrefillSubcommand checks that a prefilled subcommand opens on that shape.
func TestPrefillSubcommand(t *testing.T) {
	cfg := loadConfigFile(t, "direnv")
	sess, err := Prefill(cfg, []string{"edit"})
	if err != nil {
		t.Fatalf("prefill: %v", err)
	}
	m := newModel("direnv", cfg, sess)
	if m.pendingSub() {
		t.Fatal("a prefilled subcommand should not be pending")
	}
	if m.res().Name != "edit" {
		t.Fatalf("path = %q, want edit", m.res().Name)
	}
}

func subIndex(subs []*RNode, name string) int {
	for i, s := range subs {
		if s.Name == name {
			return i
		}
	}
	return -1
}

// TestChangeSubcommand checks that a chosen subcommand can be changed, at the
// top level and when nested (dropping deeper choices).
func TestChangeSubcommand(t *testing.T) {
	cfg := loadConfigFile(t, "git")
	sess, _ := Prefill(cfg, nil)
	m := newModel("git", cfg, sess)

	// remote -> add
	m.chooseSubAt(0, subIndex(m.subcommandsAt(0), "remote"))
	m.chooseSubAt(1, subIndex(m.subcommandsAt(1), "add"))
	if m.subDepth != 2 {
		t.Fatalf("subDepth = %d, want 2", m.subDepth)
	}

	// Change the top-level subcommand to stash; "add" must be dropped.
	m.chooseSubAt(0, subIndex(m.subcommandsAt(0), "stash"))
	if m.subDepth != 1 {
		t.Fatalf("after change subDepth = %d, want 1", m.subDepth)
	}
	if !m.pendingSub() {
		t.Fatal("stash should still need a subcommand")
	}
	if got := m.res().Segments[0].Sub; got == nil || got.Name != "stash" {
		t.Fatalf("top-level verb = %#v, want stash", got)
	}
	if got := m.preview(); !strings.Contains(got, "git") || strings.Contains(got, "add") {
		t.Fatalf("preview = %q, want no 'add'", got)
	}
}

// TestOptionsScopeBySegment checks the Options pane shows the flags of the
// segment the focused token belongs to (global flags at the root, subcommand
// flags deeper in).
func TestOptionsScopeBySegment(t *testing.T) {
	cfg := loadConfigFile(t, "git")
	sess, _ := Prefill(cfg, nil)
	m := newModel("git", cfg, sess)

	names := func() map[string]bool {
		out := map[string]bool{}
		for _, o := range m.listOptions() {
			out[o.OptName()] = true
		}
		return out
	}

	// Startup: focus is the root SUBCOMMAND -> root globals.
	if got := names(); !got["Git dir"] || got["Add interactive"] {
		t.Fatalf("root scope = %v, want globals and no add flags", got)
	}

	// remote -> add: focus is add's empty slot -> add flags, not globals.
	m.chooseSubAt(0, subIndex(m.subcommandsAt(0), "remote"))
	m.chooseSubAt(1, subIndex(m.subcommandsAt(1), "add"))
	if got := names(); !got["Remote add fetch"] || got["Git dir"] {
		t.Fatalf("remote add scope = %v, want remote-add flags and no globals", got)
	}
}
