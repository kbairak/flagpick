package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestPickerRecursiveFilter(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alpha.txt", "beta.log", filepath.Join("sub", "inner.txt")} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	p := newPicker(dir)
	if len(p.matches) != 4 { // sub/, alpha.txt, beta.log, sub/inner.txt
		t.Fatalf("matches = %d, want 4", len(p.matches))
	}

	// Filtering recurses into subfolders: "inner" matches sub/inner.txt.
	p, _ = p.update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("inner")})
	if len(p.matches) != 1 || p.matches[0].entry.path != "sub/inner.txt" {
		t.Fatalf("filter inner -> %#v, want [sub/inner.txt]", p.matches)
	}
	if path, ok := p.selectedPath(); !ok || path != filepath.Join(dir, "sub", "inner.txt") {
		t.Fatalf("selectedPath = %q, %v", path, ok)
	}

	// Backspace is forwarded to the query and widens the filter again.
	p, _ = p.update(tea.KeyMsg{Type: tea.KeyBackspace})
	if p.filter != "inne" {
		t.Fatalf("filter = %q, want inne", p.filter)
	}
}

func TestPickerMoveClamps(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a", "b", "c"} {
		_ = os.WriteFile(filepath.Join(dir, name), nil, 0o644)
	}
	p := newPicker(dir)
	p.move(-1)
	if p.cursor != 0 {
		t.Fatalf("cursor = %d, want 0", p.cursor)
	}
	p.move(10)
	if p.cursor != len(p.matches)-1 {
		t.Fatalf("cursor = %d, want %d", p.cursor, len(p.matches)-1)
	}
}

func TestPickerRenderFits(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a", "b", "c"} {
		_ = os.WriteFile(filepath.Join(dir, name), nil, 0o644)
	}
	p := newPicker(dir)
	for _, w := range []int{30, 50, 80} {
		for _, h := range []int{8, 12, 20} {
			out := p.render(w, h)
			lines := strings.Split(out, "\n")
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
