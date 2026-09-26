package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPickerBrowseAndFilter(t *testing.T) {
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
	if len(p.visible()) != 4 { // "..", sub/, alpha.txt, beta.log
		t.Fatalf("visible = %d, want 4", len(p.visible()))
	}
	if e := p.visible()[1]; e.name != "sub" || !e.dir {
		t.Fatalf("entry[1] = %#v, want sub dir", e)
	}

	// Fuzzy filter.
	p.pushFilter("bt")
	got := p.visible()
	if len(got) != 1 || got[0].name != "beta.log" {
		t.Fatalf("filter bt -> %#v, want [beta.log]", got)
	}

	// Backspace empties the filter again.
	if !p.backspace() || !p.backspace() {
		t.Fatal("backspace did not consume both runes")
	}
	if p.filter != "" || p.backspace() {
		t.Fatal("filter should be empty")
	}

	// Descend into sub/.
	p.pushFilter("sub")
	p.descend()
	if p.dir != filepath.Join(dir, "sub") {
		t.Fatalf("dir = %q, want sub", p.dir)
	}
	if p.filter != "" {
		t.Fatal("filter should reset on descend")
	}
	path, ok := p.selectedPath()
	if !ok || path != filepath.Join(dir, "sub", "inner.txt") {
		t.Fatalf("selectedPath = %q, %v; want inner.txt", path, ok)
	}

	// Parent back out.
	if !p.parent() {
		t.Fatal("parent returned false")
	}
	if p.dir != dir {
		t.Fatalf("dir = %q, want %q", p.dir, dir)
	}
}

func TestPickerRenderFits(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a", "b", "c"} {
		os.WriteFile(filepath.Join(dir, name), nil, 0o644)
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
